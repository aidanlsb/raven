package mcp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/workflow"
)

type workflowValidationItem struct {
	Name  string `json:"name"`
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

func (s *Server) resolveDirectWorkflowArgs(args map[string]interface{}) (string, *config.VaultConfig, map[string]interface{}, string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return "", nil, nil, errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return "", nil, nil, errorEnvelope("CONFIG_INVALID", "failed to load vault config", "Fix raven.yaml and try again", nil), true
	}
	return vaultPath, vaultCfg, normalizeArgs(args), "", false
}

func mapWorkflowDomainCodeToCLI(code workflow.Code) string {
	switch code {
	case workflow.CodeInvalidInput:
		return "INVALID_INPUT"
	case workflow.CodeDuplicateName:
		return "DUPLICATE_NAME"
	case workflow.CodeRefNotFound:
		return "REF_NOT_FOUND"
	case workflow.CodeFileNotFound:
		return "FILE_NOT_FOUND"
	case workflow.CodeFileReadError:
		return "FILE_READ_ERROR"
	case workflow.CodeFileWriteError:
		return "FILE_WRITE_ERROR"
	case workflow.CodeFileOutsideVault:
		return "FILE_OUTSIDE_VAULT"
	case workflow.CodeWorkflowNotFound:
		return "WORKFLOW_NOT_FOUND"
	case workflow.CodeWorkflowInvalid:
		return "WORKFLOW_INVALID"
	case workflow.CodeWorkflowChanged:
		return "WORKFLOW_CHANGED"
	case workflow.CodeWorkflowRunNotFound:
		return "WORKFLOW_RUN_NOT_FOUND"
	case workflow.CodeWorkflowNotAwaitingAgent:
		return "WORKFLOW_NOT_AWAITING_AGENT"
	case workflow.CodeWorkflowTerminalState:
		return "WORKFLOW_TERMINAL_STATE"
	case workflow.CodeWorkflowConflict:
		return "WORKFLOW_CONFLICT"
	case workflow.CodeWorkflowStateCorrupt:
		return "WORKFLOW_STATE_CORRUPT"
	case workflow.CodeWorkflowInputInvalid:
		return "WORKFLOW_INPUT_INVALID"
	case workflow.CodeWorkflowAgentOutputInvalid:
		return "WORKFLOW_AGENT_OUTPUT_INVALID"
	case workflow.CodeWorkflowInterpolationError:
		return "WORKFLOW_INTERPOLATION_ERROR"
	case workflow.CodeWorkflowToolExecutionFailed:
		return "WORKFLOW_TOOL_EXECUTION_FAILED"
	default:
		return "INTERNAL_ERROR"
	}
}

func workflowHintForDomainCode(code workflow.Code) string {
	switch code {
	case workflow.CodeWorkflowNotFound:
		return "Use 'rvn workflow list' to see available workflows"
	case workflow.CodeRefNotFound:
		return "Use 'rvn workflow show <name>' to inspect step ids"
	case workflow.CodeWorkflowChanged:
		return "Start a new run to use the latest workflow definition"
	case workflow.CodeWorkflowConflict:
		return "Fetch latest run state and retry"
	case workflow.CodeWorkflowAgentOutputInvalid:
		return "Provide valid agent output JSON with top-level 'outputs'"
	default:
		return ""
	}
}

func mapDirectWorkflowDomainError(err error, fallbackSuggestion string) (string, bool) {
	de, ok := workflow.AsDomainError(err)
	if !ok {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), fallbackSuggestion, nil), true
	}

	suggestion := workflowHintForDomainCode(de.Code)
	if suggestion == "" {
		suggestion = fallbackSuggestion
	}
	return errorEnvelope(mapWorkflowDomainCodeToCLI(de.Code), de.Error(), suggestion, de.Details), true
}

func parseWorkflowRunStatusFilter(raw string) (map[workflow.RunStatus]bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	statuses := map[workflow.RunStatus]bool{}
	for _, part := range strings.Split(raw, ",") {
		status := workflow.RunStatus(strings.TrimSpace(part))
		switch status {
		case workflow.RunStatusRunning, workflow.RunStatusAwaitingAgent, workflow.RunStatusCompleted, workflow.RunStatusFailed, workflow.RunStatusCancelled:
			statuses[status] = true
		default:
			return nil, fmt.Errorf("unknown status: %s", part)
		}
	}
	return statuses, nil
}

func parseWorkflowOlderThan(raw string) (*time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasSuffix(raw, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		if err != nil {
			return nil, fmt.Errorf("invalid days duration: %s", raw)
		}
		duration := time.Duration(days) * 24 * time.Hour
		return &duration, nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return nil, err
	}
	return &duration, nil
}

func workflowWarningsToDirect(warnings []workflow.RunStoreWarning) []directWarning {
	if len(warnings) == 0 {
		return nil
	}
	out := make([]directWarning, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, directWarning{
			Code:    warning.Code,
			Message: warning.Message,
			Ref:     warning.File,
		})
	}
	return out
}

func (s *Server) callDirectWorkflowList(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, _, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	items, err := workflow.List(vaultPath, vaultCfg)
	if err != nil {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	workflows := make([]map[string]interface{}, len(items))
	for i, item := range items {
		entry := map[string]interface{}{
			"name":        item.Name,
			"description": item.Description,
		}
		if len(item.Inputs) > 0 {
			entry["inputs"] = item.Inputs
		}
		workflows[i] = entry
	}

	return successEnvelope(map[string]interface{}{"workflows": workflows}, nil), false
}

func (s *Server) callDirectWorkflowShow(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	name := strings.TrimSpace(toString(normalized["name"]))
	if name == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name is required", "Usage: rvn workflow show <name>", nil), true
	}

	wf, err := workflow.Get(vaultPath, name, vaultCfg)
	if err != nil {
		return errorEnvelope("QUERY_NOT_FOUND", err.Error(), "Use 'rvn workflow list' to see available workflows", nil), true
	}

	data := map[string]interface{}{
		"name":        wf.Name,
		"description": wf.Description,
	}
	if len(wf.Inputs) > 0 {
		data["inputs"] = wf.Inputs
	}
	if len(wf.Steps) > 0 {
		data["steps"] = wf.Steps
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectWorkflowValidate(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	if len(vaultCfg.Workflows) == 0 {
		return successEnvelope(map[string]interface{}{
			"valid":   true,
			"checked": 0,
			"results": []workflowValidationItem{},
		}, nil), false
	}

	var names []string
	name := strings.TrimSpace(toString(normalized["name"]))
	if name != "" {
		if _, ok := vaultCfg.Workflows[name]; !ok {
			return errorEnvelope("WORKFLOW_NOT_FOUND", fmt.Sprintf("workflow '%s' not found", name), "Run 'rvn workflow list' to see available workflows", nil), true
		}
		names = []string{name}
	} else {
		names = make([]string, 0, len(vaultCfg.Workflows))
		for workflowName := range vaultCfg.Workflows {
			names = append(names, workflowName)
		}
		sort.Strings(names)
	}

	results := make([]workflowValidationItem, 0, len(names))
	invalidCount := 0
	for _, workflowName := range names {
		_, loadErr := workflow.LoadWithConfig(vaultPath, workflowName, vaultCfg.Workflows[workflowName], vaultCfg)
		item := workflowValidationItem{
			Name:  workflowName,
			Valid: loadErr == nil,
		}
		if loadErr != nil {
			item.Error = loadErr.Error()
			invalidCount++
		}
		results = append(results, item)
	}

	payload := map[string]interface{}{
		"valid":   invalidCount == 0,
		"checked": len(results),
		"invalid": invalidCount,
		"results": results,
	}
	if invalidCount > 0 {
		return errorEnvelope(
			"WORKFLOW_INVALID",
			fmt.Sprintf("%d workflow(s) invalid", invalidCount),
			"Use 'rvn workflow show <name>' to inspect a workflow definition",
			payload,
		), true
	}
	return successEnvelope(payload, nil), false
}

func (s *Server) callDirectWorkflowRunsList(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	statuses, err := parseWorkflowRunStatusFilter(toString(normalized["status"]))
	if err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), "", nil), true
	}

	svc := workflow.NewRunService(vaultPath, vaultCfg, nil)
	runs, runWarnings, err := svc.ListRuns(workflow.RunListFilter{
		Workflow: toString(normalized["workflow"]),
		Statuses: statuses,
	})
	if err != nil {
		return mapDirectWorkflowDomainError(err, "")
	}

	workflowCache := map[string]*workflow.Workflow{}
	workflowMissing := map[string]bool{}
	outRuns := make([]map[string]interface{}, 0, len(runs))
	for _, run := range runs {
		item := map[string]interface{}{
			"version":       run.Version,
			"run_id":        run.RunID,
			"workflow_name": run.WorkflowName,
			"workflow_hash": run.WorkflowHash,
			"status":        run.Status,
			"cursor":        run.Cursor,
			"revision":      run.Revision,
			"created_at":    run.CreatedAt.Format(time.RFC3339),
			"updated_at":    run.UpdatedAt.Format(time.RFC3339),
			"history":       run.History,
			"failure":       run.Failure,
		}

		availableSteps := make([]string, 0, len(run.Steps))
		for stepID := range run.Steps {
			availableSteps = append(availableSteps, stepID)
		}
		sort.Strings(availableSteps)
		item["available_steps"] = availableSteps

		if run.AwaitingStep != "" {
			item["awaiting_step_id"] = run.AwaitingStep
		}
		if run.CompletedAt != nil {
			item["completed_at"] = run.CompletedAt.Format(time.RFC3339)
		}
		if run.ExpiresAt != nil {
			item["expires_at"] = run.ExpiresAt.Format(time.RFC3339)
		}

		if !workflowMissing[run.WorkflowName] {
			wf := workflowCache[run.WorkflowName]
			if wf == nil {
				loadedWorkflow, loadErr := workflow.Get(vaultPath, run.WorkflowName, vaultCfg)
				if loadErr != nil {
					workflowMissing[run.WorkflowName] = true
				} else {
					wf = loadedWorkflow
					workflowCache[run.WorkflowName] = wf
				}
			}
			if wf != nil {
				item["step_summaries"] = workflow.BuildStepSummaries(wf, run)
			}
		}

		outRuns = append(outRuns, item)
	}

	return successEnvelope(map[string]interface{}{"runs": outRuns}, workflowWarningsToDirect(runWarnings)), false
}

func (s *Server) callDirectWorkflowRunsStep(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	runID := strings.TrimSpace(toString(normalized["run-id"]))
	if runID == "" {
		runID = strings.TrimSpace(toString(normalized["run_id"]))
	}
	stepID := strings.TrimSpace(toString(normalized["step-id"]))
	if stepID == "" {
		stepID = strings.TrimSpace(toString(normalized["step_id"]))
	}
	if runID == "" || stepID == "" {
		return errorEnvelope("MISSING_ARGUMENT", "run_id and step_id are required", "Usage: rvn workflow runs step <run-id> <step-id>", nil), true
	}

	paginationRequested := hasAnyArg(args, "path", "offset", "limit")

	svc := workflow.NewRunService(vaultPath, vaultCfg, nil)
	stepResult, err := svc.StepOutput(workflow.StepOutputRequest{
		RunID:      runID,
		StepID:     stepID,
		Paginated:  paginationRequested,
		Path:       toString(normalized["path"]),
		Offset:     intValueDefault(normalized["offset"], 0),
		Limit:      intValueDefault(normalized["limit"], 100),
		IncludeSum: true,
	})
	if err != nil {
		if de, ok := workflow.AsDomainError(err); ok {
			hint := workflowHintForDomainCode(de.Code)
			if de.Code == workflow.CodeInvalidInput {
				hint = "Use --path for nested fields and provide valid --offset/--limit values"
			}
			return errorEnvelope(mapWorkflowDomainCodeToCLI(de.Code), de.Error(), hint, de.Details), true
		}
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	state := stepResult.State
	payload := map[string]interface{}{
		"run_id":        state.RunID,
		"workflow_name": state.WorkflowName,
		"status":        state.Status,
		"revision":      state.Revision,
		"step_id":       stepID,
	}
	if paginationRequested {
		payload["step_output_page"] = stepResult.StepOutputPage
	} else {
		payload["step_output"] = stepResult.StepOutput
	}
	if len(stepResult.Summaries) > 0 {
		payload["step_summaries"] = stepResult.Summaries
	}

	return successEnvelope(payload, nil), false
}

func (s *Server) callDirectWorkflowRunsPrune(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	statuses, err := parseWorkflowRunStatusFilter(toString(normalized["status"]))
	if err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), "", nil), true
	}
	olderThan, err := parseWorkflowOlderThan(toString(normalized["older-than"]))
	if err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), "", nil), true
	}

	confirm := boolValue(normalized["confirm"])

	svc := workflow.NewRunService(vaultPath, vaultCfg, nil)
	result, err := svc.PruneRuns(workflow.RunPruneOptions{
		Statuses:  statuses,
		OlderThan: olderThan,
		Apply:     confirm,
	})
	if err != nil {
		return mapDirectWorkflowDomainError(err, "")
	}

	data := map[string]interface{}{
		"dry_run": !confirm,
		"prune":   result,
	}
	return successEnvelope(data, workflowWarningsToDirect(result.Warnings)), false
}
