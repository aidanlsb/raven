package mcp

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/toolexec"
	"github.com/aidanlsb/raven/internal/workflow"
)

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

func mapDirectWorkflowDomainError(err error, fallbackSuggestion string) (string, bool) {
	de, ok := workflow.AsDomainError(err)
	if !ok {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), fallbackSuggestion, nil), true
	}

	suggestion := workflow.DomainCodeHint(de.Code)
	if suggestion == "" {
		suggestion = fallbackSuggestion
	}
	return errorEnvelope(workflow.DomainCodeToErrorCode(de.Code), de.Error(), suggestion, de.Details), true
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

func (s *Server) makeDirectWorkflowToolFunc(vaultPath string) func(tool string, args map[string]interface{}) (interface{}, error) {
	return func(tool string, args map[string]interface{}) (interface{}, error) {
		envelope, err := toolexec.Execute(vaultPath, s.executable, tool, args)
		if err != nil {
			return nil, err
		}
		return envelope, nil
	}
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

func (s *Server) callDirectWorkflowAdd(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	name := strings.TrimSpace(toString(normalized["name"]))
	if name == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name cannot be empty", "", nil), true
	}
	rawFileRef := strings.TrimSpace(toString(normalized["file"]))
	if rawFileRef == "" {
		return errorEnvelope("MISSING_ARGUMENT", "--file is required", "Use --file <workflow YAML path>", nil), true
	}

	svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
	result, err := svc.AddWorkflow(workflow.AddWorkflowRequest{
		Name: name,
		File: rawFileRef,
	})
	if err != nil {
		if de, ok := workflow.AsDomainError(err); ok && de.Code == workflow.CodeDuplicateName {
			return mapDirectWorkflowDomainError(err, fmt.Sprintf("Use 'rvn workflow remove %s' first to replace it", name))
		}
		return mapDirectWorkflowDomainError(err, fmt.Sprintf("Use a file path like %s<name>.yaml", vaultCfg.GetWorkflowDirectory()))
	}

	out := map[string]interface{}{
		"name":        result.Workflow.Name,
		"description": result.Workflow.Description,
		"source":      result.Source,
		"file":        result.FileRef,
	}
	if len(result.Workflow.Inputs) > 0 {
		out["inputs"] = result.Workflow.Inputs
	}
	if len(result.Workflow.Steps) > 0 {
		out["steps"] = result.Workflow.Steps
	}
	return successEnvelope(out, nil), false
}

func (s *Server) callDirectWorkflowScaffold(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	name := strings.TrimSpace(toString(normalized["name"]))
	if name == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name cannot be empty", "", nil), true
	}
	fileRef := strings.TrimSpace(toString(normalized["file"]))
	description := toString(normalized["description"])

	svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
	result, err := svc.ScaffoldWorkflow(workflow.ScaffoldWorkflowRequest{
		Name:        name,
		File:        fileRef,
		Description: description,
		Force:       boolValue(normalized["force"]),
	})
	if err != nil {
		if de, ok := workflow.AsDomainError(err); ok {
			if de.Code == workflow.CodeDuplicateName {
				resolved := workflow.ScaffoldErrorFileRef(vaultCfg, name, fileRef)
				return mapDirectWorkflowDomainError(
					err,
					fmt.Sprintf("A scaffold file was written to %s. Remove the existing workflow first or use a different name.", resolved),
				)
			}
			if de.Code == workflow.CodeFileExists {
				return mapDirectWorkflowDomainError(err, "Use --force to overwrite, or choose a different --file path")
			}
		}
		return mapDirectWorkflowDomainError(err, fmt.Sprintf("Use a file path like %s<name>.yaml", vaultCfg.GetWorkflowDirectory()))
	}

	return successEnvelope(map[string]interface{}{
		"name":        result.Workflow.Name,
		"description": result.Workflow.Description,
		"file":        result.FileRef,
		"source":      result.Source,
		"scaffolded":  result.Scaffolded,
	}, nil), false
}

func (s *Server) callDirectWorkflowRemove(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	name := strings.TrimSpace(toString(normalized["name"]))
	if name == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name cannot be empty", "", nil), true
	}

	svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
	result, err := svc.RemoveWorkflow(workflow.RemoveWorkflowRequest{Name: name})
	if err != nil {
		return mapDirectWorkflowDomainError(err, "Run 'rvn workflow list' to see available workflows")
	}

	return successEnvelope(map[string]interface{}{
		"name":    result.Name,
		"removed": result.Removed,
	}, nil), false
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

	name := strings.TrimSpace(toString(normalized["name"]))
	svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
	result, err := svc.ValidateWorkflows(workflow.ValidateWorkflowsRequest{Name: name})
	if err != nil {
		return mapDirectWorkflowDomainError(err, "Run 'rvn workflow list' to see available workflows")
	}

	payload := map[string]interface{}{
		"valid":   result.Valid,
		"checked": result.Checked,
		"invalid": result.Invalid,
		"results": result.Results,
	}
	if result.Invalid > 0 {
		return errorEnvelope(
			"WORKFLOW_INVALID",
			fmt.Sprintf("%d workflow(s) invalid", result.Invalid),
			"Use 'rvn workflow show <name>' to inspect a workflow definition",
			payload,
		), true
	}
	return successEnvelope(payload, nil), false
}

func (s *Server) callDirectWorkflowStepAdd(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	workflowName := strings.TrimSpace(toString(normalized["workflow-name"]))
	if workflowName == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name cannot be empty", "", nil), true
	}
	stepRaw, ok := normalized["step-json"]
	if !ok {
		return errorEnvelope("MISSING_ARGUMENT", "step-json is required", "", nil), true
	}
	step, err := workflow.ParseStepObject(stepRaw, true)
	if err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), "", nil), true
	}

	before := strings.TrimSpace(toString(normalized["before"]))
	after := strings.TrimSpace(toString(normalized["after"]))
	if before != "" && after != "" {
		return errorEnvelope("INVALID_INPUT", "use either --before or --after, not both", "", nil), true
	}

	svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
	result, err := svc.MutateStep(workflow.StepMutationRequest{
		WorkflowName: workflowName,
		Action:       workflow.StepMutationAdd,
		Step:         step,
		Position: workflow.PositionHint{
			BeforeStepID: before,
			AfterStepID:  after,
		},
	})
	if err != nil {
		return mapDirectWorkflowDomainError(err, "Run 'rvn workflow list' to see available workflows")
	}

	payload := map[string]interface{}{
		"workflow_name": workflowName,
		"file":          result.FileRef,
		"action":        "add",
		"step_id":       result.StepID,
		"step":          result.Step,
		"index":         result.Index,
	}
	return successEnvelope(payload, nil), false
}

func (s *Server) callDirectWorkflowStepUpdate(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	workflowName := strings.TrimSpace(toString(normalized["workflow-name"]))
	stepID := strings.TrimSpace(toString(normalized["step-id"]))
	if workflowName == "" || stepID == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name and step id are required", "", nil), true
	}

	stepPatchRaw, ok := normalized["step-json"]
	if !ok {
		return errorEnvelope("MISSING_ARGUMENT", "step-json is required", "", nil), true
	}

	wf, err := workflow.Get(vaultPath, workflowName, vaultCfg)
	if err != nil {
		return errorEnvelope("WORKFLOW_NOT_FOUND", err.Error(), "Run 'rvn workflow list' to see available workflows", nil), true
	}

	targetIdx := workflow.FindStepIndexInSteps(wf.Steps, stepID)
	if targetIdx < 0 {
		return errorEnvelope(
			"REF_NOT_FOUND",
			fmt.Sprintf("step '%s' not found", stepID),
			"Use 'rvn workflow show <name>' to inspect step ids",
			map[string]interface{}{"workflow_name": workflowName, "step_id": stepID},
		), true
	}

	updatedStep, err := workflow.ApplyStepPatch(wf.Steps[targetIdx], stepPatchRaw)
	if err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), "", nil), true
	}
	if updatedStep.ID == "" {
		updatedStep.ID = stepID
	}

	if updatedStep.ID != stepID {
		if idx := workflow.FindStepIndexInSteps(wf.Steps, updatedStep.ID); idx >= 0 {
			return errorEnvelope(
				"DUPLICATE_NAME",
				fmt.Sprintf("step id '%s' already exists", updatedStep.ID),
				"Use a unique step id",
				nil,
			), true
		}
	}

	svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
	result, err := svc.MutateStep(workflow.StepMutationRequest{
		WorkflowName: workflowName,
		Action:       workflow.StepMutationUpdate,
		TargetStepID: stepID,
		Step:         updatedStep,
	})
	if err != nil {
		return mapDirectWorkflowDomainError(err, "Run 'rvn workflow list' to see available workflows")
	}

	payload := map[string]interface{}{
		"workflow_name": workflowName,
		"file":          result.FileRef,
		"action":        "update",
		"step_id":       result.StepID,
		"previous_id":   stepID,
		"step":          result.Step,
		"index":         result.Index,
	}
	return successEnvelope(payload, nil), false
}

func (s *Server) callDirectWorkflowStepRemove(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	workflowName := strings.TrimSpace(toString(normalized["workflow-name"]))
	stepID := strings.TrimSpace(toString(normalized["step-id"]))
	if workflowName == "" || stepID == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name and step id are required", "", nil), true
	}

	svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
	result, err := svc.MutateStep(workflow.StepMutationRequest{
		WorkflowName: workflowName,
		Action:       workflow.StepMutationRemove,
		TargetStepID: stepID,
	})
	if err != nil {
		return mapDirectWorkflowDomainError(err, "Run 'rvn workflow list' to see available workflows")
	}

	payload := map[string]interface{}{
		"workflow_name": workflowName,
		"file":          result.FileRef,
		"action":        "remove",
		"step_id":       result.StepID,
		"index":         result.Index,
	}
	return successEnvelope(payload, nil), false
}

func (s *Server) callDirectWorkflowRun(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	workflowName := strings.TrimSpace(toString(normalized["name"]))
	if workflowName == "" {
		return errorEnvelope("MISSING_ARGUMENT", "workflow name is required", "Usage: rvn workflow run <name>", nil), true
	}

	inputs, err := workflow.ParseInputs(toString(normalized["input-file"]), normalized["input-json"], keyValuePairs(normalized["input"]))
	if err != nil {
		return errorEnvelope("WORKFLOW_INPUT_INVALID", err.Error(), "", nil), true
	}

	svc := workflow.NewRunService(vaultPath, vaultCfg, s.makeDirectWorkflowToolFunc(vaultPath))
	outcome, err := svc.Start(workflow.StartRunRequest{
		WorkflowName: workflowName,
		Inputs:       inputs,
	})
	if err != nil {
		if de, ok := workflow.AsDomainError(err); ok {
			details := workflow.MergeDetails(de.Details, workflow.OutcomeErrorDetails(outcome, de.StepID))
			return errorEnvelope(workflow.DomainCodeToErrorCode(de.Code), de.Error(), workflow.DomainCodeHint(de.Code), details), true
		}
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	resultData, err := workflow.ParseJSONObject(outcome.Result)
	if err != nil {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	return successEnvelope(resultData, nil), false
}

func (s *Server) callDirectWorkflowContinue(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	runID := strings.TrimSpace(toString(normalized["run-id"]))
	if runID == "" {
		runID = strings.TrimSpace(toString(normalized["run_id"]))
	}
	if runID == "" {
		return errorEnvelope("MISSING_ARGUMENT", "run id is required", "Usage: rvn workflow continue <run-id>", nil), true
	}

	outputEnv, err := workflow.ParseAgentOutputEnvelope(
		toString(normalized["agent-output-file"]),
		normalized["agent-output-json"],
		toString(normalized["agent-output"]),
	)
	if err != nil {
		return errorEnvelope("WORKFLOW_AGENT_OUTPUT_INVALID", err.Error(), "", nil), true
	}

	svc := workflow.NewRunService(vaultPath, vaultCfg, s.makeDirectWorkflowToolFunc(vaultPath))
	outcome, err := svc.Continue(workflow.ContinueRunRequest{
		RunID:            runID,
		ExpectedRevision: intValueDefault(normalized["expected-revision"], 0),
		AgentOutput:      outputEnv,
	})
	if err != nil {
		if de, ok := workflow.AsDomainError(err); ok {
			details := workflow.MergeDetails(de.Details, workflow.OutcomeErrorDetails(outcome, de.StepID))
			return errorEnvelope(workflow.DomainCodeToErrorCode(de.Code), de.Error(), workflow.DomainCodeHint(de.Code), details), true
		}
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	resultData, err := workflow.ParseJSONObject(outcome.Result)
	if err != nil {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	return successEnvelope(resultData, nil), false
}

func (s *Server) callDirectWorkflowRunsList(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, normalized, errOut, isErr := s.resolveDirectWorkflowArgs(args)
	if isErr {
		return errOut, true
	}

	statuses, err := workflow.ParseRunStatusFilter(toString(normalized["status"]))
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
			hint := workflow.DomainCodeHint(de.Code)
			if de.Code == workflow.CodeInvalidInput {
				hint = "Use --path for nested fields and provide valid --offset/--limit values"
			}
			return errorEnvelope(workflow.DomainCodeToErrorCode(de.Code), de.Error(), hint, de.Details), true
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

	statuses, err := workflow.ParseRunStatusFilter(toString(normalized["status"]))
	if err != nil {
		return errorEnvelope("INVALID_INPUT", err.Error(), "", nil), true
	}
	olderThan, err := workflow.ParseOlderThan(toString(normalized["older-than"]))
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
