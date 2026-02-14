package workflow

import (
	"encoding/json"
	"fmt"

	"github.com/aidanlsb/raven/internal/config"
)

// Runner executes workflows step-by-step.
//
// It is intentionally generic: deterministic tool steps are executed via a
// caller-provided function hook, and agent steps only render prompts (they do
// not call an LLM).
type Runner struct {
	vaultPath string
	vaultCfg  *config.VaultConfig

	ToolFunc func(tool string, args map[string]interface{}) (interface{}, error)
}

func NewRunner(vaultPath string, vaultCfg *config.VaultConfig) *Runner {
	return &Runner{
		vaultPath: vaultPath,
		vaultCfg:  vaultCfg,
	}
}

// Run executes wf until it reaches an agent step (returning Next) or completes.
func (r *Runner) Run(wf *Workflow, inputs map[string]string) (*RunResult, error) {
	if wf == nil {
		return nil, fmt.Errorf("workflow is nil")
	}

	if err := validateInputs(wf, inputs); err != nil {
		return nil, err
	}
	resolvedInputs := applyDefaults(wf, inputs)

	steps := make(map[string]interface{}, len(wf.Steps))

	for _, step := range wf.Steps {
		if step == nil {
			return nil, fmt.Errorf("nil step")
		}

		switch step.Type {
		case "agent":
			prompt, err := interpolate(step.Prompt, resolvedInputs, steps)
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}
			example := buildOutputExample(step.Outputs)
			promptWithContract := prompt
			if len(step.Outputs) > 0 {
				if contract := renderOutputContract(example); contract != "" {
					promptWithContract = contract + "\n\n" + prompt
				}
			}
			// Store prompt metadata in step state (useful for callers).
			steps[step.ID] = map[string]interface{}{
				"prompt":  promptWithContract,
				"outputs": step.Outputs,
				"example": example,
				"raw":     "",
			}

			return &RunResult{
				Name:   wf.Name,
				Inputs: resolvedInputs,
				Steps:  steps,
				Next: &AgentRequest{
					StepID:  step.ID,
					Prompt:  promptWithContract,
					Outputs: step.Outputs,
					Example: example,
				},
			}, nil

		case "tool":
			if r.ToolFunc == nil {
				return nil, fmt.Errorf("step '%s': tool function not configured", step.ID)
			}
			args, err := interpolateObject(step.Arguments, resolvedInputs, steps)
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}
			result, err := r.ToolFunc(step.Tool, args)
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}
			steps[step.ID] = result

		default:
			return nil, fmt.Errorf("step '%s': unknown step type '%s'", step.ID, step.Type)
		}
	}

	return &RunResult{
		Name:   wf.Name,
		Inputs: resolvedInputs,
		Steps:  steps,
	}, nil
}

func buildOutputExample(outputs map[string]*config.WorkflowPromptOutput) map[string]interface{} {
	if len(outputs) == 0 {
		return nil
	}

	out := make(map[string]interface{}, len(outputs))
	for name, def := range outputs {
		if def == nil {
			continue
		}
		switch def.Type {
		case "markdown":
			out[name] = "..."
		case "string":
			out[name] = ""
		case "number":
			out[name] = 0
		case "bool":
			out[name] = false
		case "object":
			out[name] = map[string]interface{}{}
		case "array":
			out[name] = []interface{}{}
		default:
			out[name] = nil
		}
	}

	return map[string]interface{}{
		"outputs": out,
	}
}

func renderOutputContract(example map[string]interface{}) string {
	if example == nil {
		return ""
	}
	b, err := json.MarshalIndent(example, "", "  ")
	if err != nil {
		return ""
	}
	return fmt.Sprintf("Return ONLY valid JSON with this shape:\n\n```json\n%s\n```", string(b))
}

func validateInputs(wf *Workflow, inputs map[string]string) error {
	if wf.Inputs == nil {
		return nil
	}
	var missing []string
	for name, def := range wf.Inputs {
		if def != nil && def.Required {
			if _, ok := inputs[name]; !ok && def.Default == "" {
				missing = append(missing, name)
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required inputs: %v", missing)
	}
	return nil
}

func applyDefaults(wf *Workflow, inputs map[string]string) map[string]string {
	result := make(map[string]string, len(inputs))
	for k, v := range inputs {
		result[k] = v
	}
	if wf.Inputs != nil {
		for name, def := range wf.Inputs {
			if def != nil && def.Default != "" {
				if _, ok := result[name]; !ok {
					result[name] = def.Default
				}
			}
		}
	}
	return result
}
