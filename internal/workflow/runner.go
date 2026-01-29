package workflow

import (
	"encoding/json"
	"fmt"

	"github.com/aidanlsb/raven/internal/config"
)

// Runner executes workflows step-by-step.
//
// It is intentionally generic: deterministic steps are executed via function
// hooks provided by the caller (CLI/MCP), and prompt steps only render prompts
// (they do not call an LLM).
type Runner struct {
	vaultPath string
	vaultCfg  *config.VaultConfig

	QueryFunc     func(query string) (interface{}, error)
	ReadFunc      func(ref string) (interface{}, error)
	BacklinksFunc func(target string) (interface{}, error)
	SearchFunc    func(term string, limit int) (interface{}, error)
}

func NewRunner(vaultPath string, vaultCfg *config.VaultConfig) *Runner {
	return &Runner{
		vaultPath: vaultPath,
		vaultCfg:  vaultCfg,
	}
}

// Run executes wf until it reaches a prompt step (returning Next) or completes.
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
		case "query":
			q, err := interpolate(step.RQL, resolvedInputs, steps)
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}
			if r.QueryFunc == nil {
				return nil, fmt.Errorf("step '%s': query function not configured", step.ID)
			}
			result, err := r.QueryFunc(q)
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}
			steps[step.ID] = map[string]interface{}{"results": result}

		case "read":
			ref, err := interpolate(step.Ref, resolvedInputs, steps)
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}
			if r.ReadFunc == nil {
				return nil, fmt.Errorf("step '%s': read function not configured", step.ID)
			}
			result, err := r.ReadFunc(ref)
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}
			steps[step.ID] = result

		case "search":
			term, err := interpolate(step.Term, resolvedInputs, steps)
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}
			if r.SearchFunc == nil {
				return nil, fmt.Errorf("step '%s': search function not configured", step.ID)
			}
			limit := step.Limit
			if limit == 0 {
				limit = 20
			}
			result, err := r.SearchFunc(term, limit)
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}
			steps[step.ID] = map[string]interface{}{"results": result}

		case "backlinks":
			target, err := interpolate(step.Target, resolvedInputs, steps)
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}
			if r.BacklinksFunc == nil {
				return nil, fmt.Errorf("step '%s': backlinks function not configured", step.ID)
			}
			result, err := r.BacklinksFunc(target)
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}
			steps[step.ID] = map[string]interface{}{"results": result}

		case "prompt":
			prompt, err := interpolate(step.Template, resolvedInputs, steps)
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}
			example := buildPromptOutputExample(step.Outputs)
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
				Next: &PromptRequest{
					StepID:   step.ID,
					Prompt:   promptWithContract,
					Outputs:  step.Outputs,
					Example:  example,
					Template: step.Template,
				},
			}, nil

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

func buildPromptOutputExample(outputs map[string]*config.WorkflowPromptOutput) map[string]interface{} {
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
