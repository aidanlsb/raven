package workflow

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/config"
)

// FindStepIndexInSteps returns the index of stepID in steps, or -1 when not found.
func FindStepIndexInSteps(steps []*config.WorkflowStep, stepID string) int {
	if stepID == "" {
		return -1
	}
	for i, step := range steps {
		if step == nil {
			continue
		}
		if strings.TrimSpace(step.ID) == stepID {
			return i
		}
	}
	return -1
}

// MergeDetails combines two detail maps with primary values taking precedence.
func MergeDetails(primary, secondary map[string]interface{}) map[string]interface{} {
	if len(primary) == 0 && len(secondary) == 0 {
		return nil
	}
	out := map[string]interface{}{}
	for k, v := range secondary {
		out[k] = v
	}
	for k, v := range primary {
		out[k] = v
	}
	return out
}

// RunStateErrorDetails builds consistent run-state diagnostics for workflow errors.
func RunStateErrorDetails(wf *Workflow, state *WorkflowRunState, failedStepID string) map[string]interface{} {
	if state == nil {
		return nil
	}

	available := make([]string, 0, len(state.Steps))
	for id := range state.Steps {
		available = append(available, id)
	}
	sort.Strings(available)

	details := map[string]interface{}{
		"run_id":          state.RunID,
		"workflow_name":   state.WorkflowName,
		"status":          state.Status,
		"revision":        state.Revision,
		"cursor":          state.Cursor,
		"available_steps": available,
		"updated_at":      state.UpdatedAt.Format(time.RFC3339),
	}
	if wf != nil {
		details["step_summaries"] = BuildStepSummaries(wf, state)
	}
	if failedStepID != "" {
		details["failed_step_id"] = failedStepID
	}
	if state.Failure != nil {
		details["failure"] = state.Failure
	}
	return details
}

// OutcomeErrorDetails builds run-state diagnostics directly from a run outcome.
func OutcomeErrorDetails(outcome *RunExecutionOutcome, failedStepID string) map[string]interface{} {
	if outcome == nil {
		return nil
	}
	return RunStateErrorDetails(outcome.Workflow, outcome.State, failedStepID)
}

// ScaffoldErrorFileRef resolves the scaffold target file path for actionable errors.
func ScaffoldErrorFileRef(vaultCfg *config.VaultConfig, name, rawFileRef string) string {
	if vaultCfg == nil {
		return strings.TrimSpace(rawFileRef)
	}
	fileRef := strings.TrimSpace(rawFileRef)
	if fileRef == "" {
		fileRef = fmt.Sprintf("%s%s.yaml", vaultCfg.GetWorkflowDirectory(), name)
	}
	resolved, err := ResolveWorkflowFileRef(fileRef, vaultCfg.GetWorkflowDirectory())
	if err != nil {
		return fileRef
	}
	return resolved
}
