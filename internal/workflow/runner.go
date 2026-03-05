package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
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
	state.Inputs = materializeOptionalInputs(wf, resolvedInputs)
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

			return buildRunResult(wf, state, &AgentRequest{
				StepID:  step.ID,
				Prompt:  promptWithContract,
				Outputs: step.Outputs,
				Example: example,
			}, nil), nil

		case "tool":
			result, err := r.executeToolStep(step, state.Inputs, state.Steps, nil)
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

		case "foreach":
			result, status, err := r.runForEachStep(step, state.Inputs, state.Steps)
			if result != nil {
				state.Steps[step.ID] = result
			}
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}

			state.Cursor = i + 1
			state.UpdatedAt = time.Now().UTC()
			state.History = append(state.History, RunHistoryEvent{
				StepID:   step.ID,
				StepType: "foreach",
				Status:   status,
				At:       state.UpdatedAt,
			})

		case "switch":
			result, status, err := r.runSwitchStep(step, state.Inputs, state.Steps)
			if result != nil {
				state.Steps[step.ID] = result
			}
			if err != nil {
				return nil, fmt.Errorf("step '%s': %w", step.ID, err)
			}

			state.Cursor = i + 1
			state.UpdatedAt = time.Now().UTC()
			state.History = append(state.History, RunHistoryEvent{
				StepID:   step.ID,
				StepType: "switch",
				Status:   status,
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

	return buildRunResult(wf, state, nil, &RunSummary{
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
		spec, err := parseOutputType(def.Type)
		if err != nil {
			out[name] = nil
			continue
		}
		if spec.IsArray {
			out[name] = []interface{}{}
			continue
		}
		switch spec.Base {
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

func (r *Runner) runForEachStep(
	step *config.WorkflowStep,
	inputs map[string]interface{},
	steps map[string]interface{},
) (map[string]interface{}, string, error) {
	if step == nil || step.ForEach == nil {
		return nil, "", fmt.Errorf("foreach step config missing")
	}
	if r.ToolFunc == nil {
		return nil, "", fmt.Errorf("tool function not configured")
	}

	itemVar := strings.TrimSpace(step.ForEach.As)
	if itemVar == "" {
		itemVar = "item"
	}
	indexVar := strings.TrimSpace(step.ForEach.IndexAs)
	if indexVar == "" {
		indexVar = "index"
	}
	onError := strings.TrimSpace(step.ForEach.OnError)
	if onError == "" {
		onError = "fail_fast"
	}

	rawItems, err := resolveForEachItems(step.ForEach.Items, inputs, steps)
	if err != nil {
		return nil, "", err
	}

	results := make([]interface{}, 0, len(rawItems))
	successCount := 0
	errorCount := 0

	for idx, item := range rawItems {
		iteration := map[string]interface{}{
			"index": idx,
			"item":  item,
			"steps": map[string]interface{}{},
			"ok":    true,
		}
		iterationSteps := iteration["steps"].(map[string]interface{})
		scope := map[string]interface{}{
			itemVar:  item,
			indexVar: idx,
		}

		for _, nested := range step.ForEach.Steps {
			out, callErr := r.executeToolStep(nested, inputs, steps, scope)
			if callErr != nil {
				errMsg := fmt.Sprintf("foreach item %d nested step '%s': %v", idx, nested.ID, callErr)
				iteration["ok"] = false
				iteration["error"] = errMsg
				errorCount++
				results = append(results, iteration)
				if onError == "continue" {
					goto nextIteration
				}
				return buildForEachStepResult(onError, itemVar, indexVar, successCount, errorCount, results), "error", fmt.Errorf("%s", errMsg)
			}

			iterationSteps[nested.ID] = out
		}

		successCount++
		results = append(results, iteration)

	nextIteration:
	}

	status := "ok"
	if errorCount > 0 {
		status = "partial_failed"
	}
	return buildForEachStepResult(onError, itemVar, indexVar, successCount, errorCount, results), status, nil
}

func (r *Runner) runSwitchStep(
	step *config.WorkflowStep,
	inputs map[string]interface{},
	steps map[string]interface{},
) (map[string]interface{}, string, error) {
	if step == nil || step.Switch == nil {
		return nil, "", fmt.Errorf("switch step config missing")
	}

	value, err := resolveSwitchValue(step.Switch.Value, inputs, steps)
	if err != nil {
		return nil, "", err
	}

	selectedCase := value
	branch, ok := step.Switch.Cases[value]
	if !ok {
		branch = step.Switch.Default
		selectedCase = "default"
	}
	if branch == nil {
		return nil, "", fmt.Errorf("switch selected branch is nil")
	}

	branchResults := make(map[string]interface{})
	for _, nested := range branch.Steps {
		effectiveSteps := overlayStepState(steps, branchResults)
		switch nested.Type {
		case "tool":
			out, callErr := r.executeToolStep(nested, inputs, effectiveSteps, nil)
			if callErr != nil {
				return nil, "error", fmt.Errorf("switch branch '%s' nested step '%s': %w", selectedCase, nested.ID, callErr)
			}
			branchResults[nested.ID] = out
		case "foreach":
			out, _, runErr := r.runForEachStep(nested, inputs, effectiveSteps)
			if runErr != nil {
				return nil, "error", fmt.Errorf("switch branch '%s' nested step '%s': %w", selectedCase, nested.ID, runErr)
			}
			branchResults[nested.ID] = out
		default:
			return nil, "error", fmt.Errorf(
				"switch branch '%s' nested step '%s' has unsupported type '%s'",
				selectedCase,
				nested.ID,
				nested.Type,
			)
		}
	}

	var emit map[string]interface{}
	if len(branch.Emit) > 0 {
		effectiveSteps := overlayStepState(steps, branchResults)
		emit, err = interpolateObjectWithTypedInputs(branch.Emit, inputs, effectiveSteps)
		if err != nil {
			return nil, "error", fmt.Errorf("switch branch '%s' emit: %w", selectedCase, err)
		}
	} else {
		emit = map[string]interface{}{}
	}

	if len(step.Switch.Outputs) > 0 {
		if err := ValidateAgentOutputs(step.Switch.Outputs, emit); err != nil {
			return nil, "error", fmt.Errorf("switch branch '%s' emit invalid: %w", selectedCase, err)
		}
	}

	return map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"value":          value,
			"selected_case":  selectedCase,
			"output":         emit,
			"branch_results": branchResults,
		},
	}, "ok", nil
}

func resolveSwitchValue(
	raw string,
	inputs map[string]interface{},
	steps map[string]interface{},
) (string, error) {
	expr := strings.TrimSpace(raw)
	if expr == "" {
		return "", fmt.Errorf("switch.value is required")
	}
	if exact, ok := extractExactInterpolationExpr(expr); ok {
		val, exists, err := resolveExprRaw(exact, inputs, steps)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", fmt.Errorf("unknown variable: %s", exact)
		}
		strVal, ok := val.(string)
		if !ok {
			return "", fmt.Errorf("switch.value must resolve to string, got %T", val)
		}
		return strVal, nil
	}
	out, err := interpolateWithTypedInputs(expr, inputs, steps)
	if err != nil {
		return "", err
	}
	return out, nil
}

func overlayStepState(base map[string]interface{}, overlay map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func (r *Runner) executeToolStep(
	step *config.WorkflowStep,
	inputs map[string]interface{},
	steps map[string]interface{},
	scope map[string]interface{},
) (interface{}, error) {
	if step == nil {
		return nil, fmt.Errorf("tool step is nil")
	}
	if r.ToolFunc == nil {
		return nil, fmt.Errorf("tool function not configured")
	}
	args, err := interpolateObjectWithScope(step.Arguments, inputs, steps, scope)
	if err != nil {
		return nil, err
	}
	return r.ToolFunc(step.Tool, args)
}

func resolveForEachItems(
	raw string,
	inputs map[string]interface{},
	steps map[string]interface{},
) ([]interface{}, error) {
	expr := strings.TrimSpace(raw)
	if expr == "" {
		return nil, fmt.Errorf("foreach.items is required")
	}
	if exact, ok := extractExactInterpolationExpr(expr); ok {
		expr = exact
	}

	value, ok, err := resolveExprRaw(expr, inputs, steps)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("unknown variable: %s", expr)
	}

	items, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("foreach.items must resolve to array, got %T", value)
	}
	return items, nil
}

func buildForEachStepResult(
	onError string,
	itemVar string,
	indexVar string,
	successCount int,
	errorCount int,
	results []interface{},
) map[string]interface{} {
	total := successCount + errorCount
	return map[string]interface{}{
		"ok": errorCount == 0,
		"data": map[string]interface{}{
			"on_error":      onError,
			"items_total":   total,
			"success_count": successCount,
			"error_count":   errorCount,
			"results":       results,
			"item_var":      itemVar,
			"index_var":     indexVar,
		},
	}
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

func materializeOptionalInputs(wf *Workflow, inputs map[string]interface{}) map[string]interface{} {
	if wf == nil || wf.Inputs == nil {
		return inputs
	}
	for name := range wf.Inputs {
		if _, ok := inputs[name]; !ok {
			// Keep optional declared inputs addressable in interpolation paths.
			inputs[name] = nil
		}
	}
	return inputs
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

func buildRunResult(wf *Workflow, state *WorkflowRunState, next *AgentRequest, summary *RunSummary) *RunResult {
	result := &RunResult{
		RunID:         state.RunID,
		WorkflowName:  state.WorkflowName,
		Status:        state.Status,
		Revision:      state.Revision,
		Cursor:        state.Cursor,
		Inputs:        cloneInterfaceMap(state.Inputs),
		StepSummaries: BuildStepSummaries(wf, state),
		Next:          next,
		Result:        summary,
		Failure:       state.Failure,
		CreatedAt:     state.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     state.UpdatedAt.Format(time.RFC3339),
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

// BuildStepSummaries returns per-step status without embedding full step outputs.
func BuildStepSummaries(wf *Workflow, state *WorkflowRunState) []RunStepSummary {
	if wf == nil || state == nil {
		return nil
	}

	lastStatusByStep := map[string]string{}
	for _, ev := range state.History {
		if ev.StepID == "" {
			continue
		}
		lastStatusByStep[ev.StepID] = ev.Status
	}

	summaries := make([]RunStepSummary, 0, len(wf.Steps))
	for i, step := range wf.Steps {
		if step == nil {
			continue
		}

		_, hasOutput := state.Steps[step.ID]
		status := "pending"

		if last, ok := lastStatusByStep[step.ID]; ok && last != "" {
			status = last
		} else if state.Status == RunStatusAwaitingAgent && state.AwaitingStep == step.ID {
			status = string(RunStatusAwaitingAgent)
		} else if hasOutput || i < state.Cursor {
			if step.Type == "agent" {
				status = "accepted"
			} else {
				status = "ok"
			}
		}

		summaries = append(summaries, RunStepSummary{
			StepID:    step.ID,
			StepType:  step.Type,
			Status:    status,
			HasOutput: hasOutput,
		})
	}

	return summaries
}
