package workflow

import (
	"encoding/json"
	"fmt"
	"time"

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
	typedInputs := make(map[string]interface{}, len(inputs))
	for k, v := range inputs {
		typedInputs[k] = v
	}
	state, err := NewRunState(wf, typedInputs)
	if err != nil {
		return nil, err
	}
	return r.RunWithState(wf, state)
}

// RunWithState executes or resumes a workflow from the provided persisted state.
func (r *Runner) RunWithState(wf *Workflow, state *WorkflowRunState) (*RunResult, error) {
	if wf == nil {
		return nil, fmt.Errorf("workflow is nil")
	}
	if state == nil {
		return nil, fmt.Errorf("run state is nil")
	}

	if state.Inputs == nil {
		state.Inputs = map[string]interface{}{}
	}
	if state.Steps == nil {
		state.Steps = map[string]interface{}{}
	}

	resolvedInputs, err := applyDefaultsTyped(wf, state.Inputs)
	if err != nil {
		return nil, err
	}
	if err := validateInputsTyped(wf, resolvedInputs); err != nil {
		return nil, err
	}
	state.Inputs = resolvedInputs
	state.Status = RunStatusRunning

	for i := state.Cursor; i < len(wf.Steps); i++ {
		step := wf.Steps[i]
		if step == nil {
			return nil, fmt.Errorf("nil step")
		}

		switch step.Type {
		case "agent":
			prompt, err := interpolateWithTypedInputs(step.Prompt, state.Inputs, state.Steps)
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

			stepState := map[string]interface{}{
				"prompt":            promptWithContract,
				"outputs":           step.Outputs,
				"example":           example,
				"raw":               "",
				"validated_outputs": nil,
			}
			state.Steps[step.ID] = stepState

			state.Status = RunStatusAwaitingAgent
			state.AwaitingStep = step.ID
			state.Cursor = i
			state.UpdatedAt = time.Now().UTC()
			state.History = append(state.History, RunHistoryEvent{
				StepID:   step.ID,
				StepType: "agent",
				Status:   string(RunStatusAwaitingAgent),
				At:       state.UpdatedAt,
			})

			return buildRunResult(state, &AgentRequest{
				StepID:  step.ID,
				Prompt:  promptWithContract,
				Outputs: step.Outputs,
				Example: example,
			}, nil), nil

		case "tool":
			if r.ToolFunc == nil {
				return nil, fmt.Errorf("step '%s': tool function not configured", step.ID)
			}
			args, err := interpolateObjectWithTypedInputs(step.Arguments, state.Inputs, state.Steps)
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}
			result, err := r.ToolFunc(step.Tool, args)
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}
			state.Steps[step.ID] = result
			state.Cursor = i + 1
			state.UpdatedAt = time.Now().UTC()
			state.History = append(state.History, RunHistoryEvent{
				StepID:   step.ID,
				StepType: "tool",
				Status:   "ok",
				At:       state.UpdatedAt,
			})

		default:
			return nil, fmt.Errorf("step '%s': unknown step type '%s'", step.ID, step.Type)
		}
	}

	now := time.Now().UTC()
	state.Status = RunStatusCompleted
	state.AwaitingStep = ""
	state.Cursor = len(wf.Steps)
	state.UpdatedAt = now
	state.CompletedAt = &now
	var terminal string
	if len(wf.Steps) > 0 && wf.Steps[len(wf.Steps)-1] != nil {
		terminal = wf.Steps[len(wf.Steps)-1].ID
	}

	return buildRunResult(state, nil, &RunSummary{
		TerminalStepID: terminal,
		Summary:        computeRunExecutionSummary(state),
	}), nil
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

func validateInputsTyped(wf *Workflow, inputs map[string]interface{}) error {
	if wf.Inputs == nil {
		return nil
	}

	for key := range inputs {
		if _, ok := wf.Inputs[key]; !ok {
			return fmt.Errorf("unknown workflow input: %s", key)
		}
	}

	var missing []string
	for name, def := range wf.Inputs {
		if def == nil {
			continue
		}
		val, ok := inputs[name]
		if !ok {
			if def.Required && def.Default == nil {
				missing = append(missing, name)
			}
			continue
		}
		if err := validateInputType(name, def.Type, val); err != nil {
			return err
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required inputs: %v", missing)
	}
	return nil
}

func applyDefaultsTyped(wf *Workflow, inputs map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(inputs))
	for k, v := range inputs {
		result[k] = v
	}
	if wf.Inputs != nil {
		for name, def := range wf.Inputs {
			if def == nil {
				continue
			}
			if _, ok := result[name]; ok {
				continue
			}
			if def.Default != nil {
				result[name] = def.Default
			}
		}
	}
	return result, nil
}

func validateInputType(name, typ string, value interface{}) error {
	switch typ {
	case "", "string", "markdown", "ref", "date", "datetime":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("workflow input '%s' must be string", name)
		}
	case "number":
		switch value.(type) {
		case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		default:
			return fmt.Errorf("workflow input '%s' must be number", name)
		}
	case "bool", "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("workflow input '%s' must be bool", name)
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("workflow input '%s' must be object", name)
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("workflow input '%s' must be array", name)
		}
	default:
		return fmt.Errorf("workflow input '%s' has unsupported type '%s'", name, typ)
	}
	return nil
}

func buildRunResult(state *WorkflowRunState, next *AgentRequest, summary *RunSummary) *RunResult {
	result := &RunResult{
		RunID:        state.RunID,
		WorkflowName: state.WorkflowName,
		Status:       state.Status,
		Revision:     state.Revision,
		Cursor:       state.Cursor,
		Inputs:       cloneInterfaceMap(state.Inputs),
		Steps:        state.Steps,
		Next:         next,
		Result:       summary,
		Failure:      state.Failure,
		CreatedAt:    state.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    state.UpdatedAt.Format(time.RFC3339),
	}
	if state.AwaitingStep != "" {
		result.AwaitingStepID = state.AwaitingStep
	}
	if state.CompletedAt != nil {
		result.CompletedAt = state.CompletedAt.Format(time.RFC3339)
	}
	if state.ExpiresAt != nil {
		result.ExpiresAt = state.ExpiresAt.Format(time.RFC3339)
	}
	return result
}

func computeRunExecutionSummary(state *WorkflowRunState) *RunExecutionSummary {
	if state == nil {
		return nil
	}
	summary := &RunExecutionSummary{}
	for _, ev := range state.History {
		if ev.StepType == "tool" && ev.Status == "ok" {
			summary.ToolsExecuted++
		}
		if ev.StepType == "agent" && ev.Status == "accepted" {
			summary.AgentBoundariesCrossed++
		}
	}
	return summary
}
