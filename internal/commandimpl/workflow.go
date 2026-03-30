package commandimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/workflow"
)

// HandleWorkflowList executes the canonical `workflow_list` command.
func HandleWorkflowList(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	items, err := workflow.List(req.VaultPath, vaultCfg)
	if err != nil {
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
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
	return commandexec.Success(map[string]interface{}{"workflows": workflows}, &commandexec.Meta{Count: len(workflows)})
}

// HandleWorkflowAdd executes the canonical `workflow_add` command.
func HandleWorkflowAdd(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	name := strings.TrimSpace(stringArg(req.Args, "name"))
	svc := workflow.NewAuthoringService(req.VaultPath, vaultCfg)
	result, err := svc.AddWorkflow(workflow.AddWorkflowRequest{
		Name: name,
		File: stringArg(req.Args, "file"),
	})
	if err != nil {
		if de, ok := workflow.AsDomainError(err); ok && de.Code == workflow.CodeDuplicateName {
			return mapWorkflowFailure(err, fmt.Sprintf("Use 'rvn workflow remove %s' first to replace it", name))
		}
		return mapWorkflowFailure(err, fmt.Sprintf("Use a file path like %s<name>.yaml", vaultCfg.GetWorkflowDirectory()))
	}

	data := map[string]interface{}{
		"name":        result.Workflow.Name,
		"description": result.Workflow.Description,
		"source":      result.Source,
		"file":        result.FileRef,
	}
	if len(result.Workflow.Inputs) > 0 {
		data["inputs"] = result.Workflow.Inputs
	}
	if len(result.Workflow.Steps) > 0 {
		data["steps"] = result.Workflow.Steps
	}
	return commandexec.Success(data, nil)
}

// HandleWorkflowScaffold executes the canonical `workflow_scaffold` command.
func HandleWorkflowScaffold(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	name := strings.TrimSpace(stringArg(req.Args, "name"))
	svc := workflow.NewAuthoringService(req.VaultPath, vaultCfg)
	result, err := svc.ScaffoldWorkflow(workflow.ScaffoldWorkflowRequest{
		Name:        name,
		File:        stringArg(req.Args, "file"),
		Description: stringArg(req.Args, "description"),
		Force:       boolArg(req.Args, "force"),
	})
	if err != nil {
		if de, ok := workflow.AsDomainError(err); ok {
			if de.Code == workflow.CodeDuplicateName {
				return mapWorkflowFailure(
					err,
					fmt.Sprintf("A scaffold file was written to %s. Remove the existing workflow first or use a different name.", workflow.ScaffoldErrorFileRef(vaultCfg, name, stringArg(req.Args, "file"))),
				)
			}
			if de.Code == workflow.CodeFileExists {
				return mapWorkflowFailure(err, "Use --force to overwrite, or choose a different --file path")
			}
		}
		return mapWorkflowFailure(err, fmt.Sprintf("Use a file path like %s<name>.yaml", vaultCfg.GetWorkflowDirectory()))
	}

	return commandexec.Success(map[string]interface{}{
		"name":        result.Workflow.Name,
		"description": result.Workflow.Description,
		"file":        result.FileRef,
		"source":      result.Source,
		"scaffolded":  result.Scaffolded,
	}, nil)
}

// HandleWorkflowRemove executes the canonical `workflow_remove` command.
func HandleWorkflowRemove(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	svc := workflow.NewAuthoringService(req.VaultPath, vaultCfg)
	result, err := svc.RemoveWorkflow(workflow.RemoveWorkflowRequest{Name: stringArg(req.Args, "name")})
	if err != nil {
		return mapWorkflowFailure(err, "Run 'rvn workflow list' to see available workflows")
	}

	return commandexec.Success(map[string]interface{}{
		"name":    result.Name,
		"removed": result.Removed,
	}, nil)
}

// HandleWorkflowValidate executes the canonical `workflow_validate` command.
func HandleWorkflowValidate(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	svc := workflow.NewAuthoringService(req.VaultPath, vaultCfg)
	result, err := svc.ValidateWorkflows(workflow.ValidateWorkflowsRequest{Name: stringArg(req.Args, "name")})
	if err != nil {
		return mapWorkflowFailure(err, "Run 'rvn workflow list' to see available workflows")
	}

	payload := map[string]interface{}{
		"valid":   result.Valid,
		"checked": result.Checked,
		"invalid": result.Invalid,
		"results": result.Results,
	}
	if result.Invalid > 0 {
		return commandexec.Failure("WORKFLOW_INVALID", fmt.Sprintf("%d workflow(s) invalid", result.Invalid), payload, "Use 'rvn workflow show <name>' to inspect a workflow definition")
	}

	return commandexec.Success(payload, &commandexec.Meta{Count: len(result.Results)})
}

// HandleWorkflowShow executes the canonical `workflow_show` command.
func HandleWorkflowShow(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	wf, err := workflow.Get(req.VaultPath, stringArg(req.Args, "name"), vaultCfg)
	if err != nil {
		return commandexec.Failure("QUERY_NOT_FOUND", err.Error(), nil, "Use 'rvn workflow list' to see available workflows")
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
	return commandexec.Success(data, nil)
}

// HandleWorkflowStepAdd executes the canonical `workflow_step_add` command.
func HandleWorkflowStepAdd(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	step, err := workflow.ParseStepObject(req.Args["step-json"], true)
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "")
	}

	before := strings.TrimSpace(stringArg(req.Args, "before"))
	after := strings.TrimSpace(stringArg(req.Args, "after"))
	if before != "" && after != "" {
		return commandexec.Failure("INVALID_INPUT", "use either --before or --after, not both", nil, "")
	}

	workflowName := strings.TrimSpace(stringArg(req.Args, "workflow-name"))
	svc := workflow.NewAuthoringService(req.VaultPath, vaultCfg)
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
		return mapWorkflowFailure(err, "Run 'rvn workflow list' to see available workflows")
	}

	return commandexec.Success(workflowStepMutationPayload(workflowName, "add", result, ""), nil)
}

// HandleWorkflowStepUpdate executes the canonical `workflow_step_update` command.
func HandleWorkflowStepUpdate(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	workflowName := strings.TrimSpace(stringArg(req.Args, "workflow-name"))
	stepID := strings.TrimSpace(stringArg(req.Args, "step-id"))

	svc := workflow.NewAuthoringService(req.VaultPath, vaultCfg)
	result, err := svc.MutateStep(workflow.StepMutationRequest{
		WorkflowName: workflowName,
		Action:       workflow.StepMutationUpdate,
		TargetStepID: stepID,
		StepPatch:    req.Args["step-json"],
	})
	if err != nil {
		return mapWorkflowFailure(err, "Run 'rvn workflow list' to see available workflows")
	}

	return commandexec.Success(workflowStepMutationPayload(workflowName, "update", result, stepID), nil)
}

// HandleWorkflowStepRemove executes the canonical `workflow_step_remove` command.
func HandleWorkflowStepRemove(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	workflowName := strings.TrimSpace(stringArg(req.Args, "workflow-name"))
	svc := workflow.NewAuthoringService(req.VaultPath, vaultCfg)
	result, err := svc.MutateStep(workflow.StepMutationRequest{
		WorkflowName: workflowName,
		Action:       workflow.StepMutationRemove,
		TargetStepID: stringArg(req.Args, "step-id"),
	})
	if err != nil {
		return mapWorkflowFailure(err, "Run 'rvn workflow list' to see available workflows")
	}

	return commandexec.Success(workflowStepMutationPayload(workflowName, "remove", result, ""), nil)
}

// HandleWorkflowStepBatch executes the canonical `workflow_step_batch` command.
func HandleWorkflowStepBatch(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	mutations, err := workflow.ParseStepBatchMutations(req.Args["mutations-json"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "")
	}

	workflowName := strings.TrimSpace(stringArg(req.Args, "workflow-name"))
	svc := workflow.NewAuthoringService(req.VaultPath, vaultCfg)
	result, err := svc.MutateSteps(workflow.StepBatchMutationRequest{
		WorkflowName: workflowName,
		Mutations:    mutations,
	})
	if err != nil {
		return mapWorkflowFailure(err, "Run 'rvn workflow list' to see available workflows")
	}

	return commandexec.Success(workflowStepBatchPayload(result), nil)
}

// HandleWorkflowRun executes the canonical `workflow_run` command.
func HandleWorkflowRun(ctx context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	inputs, err := parseCanonicalWorkflowInputs(req.Args)
	if err != nil {
		return commandexec.Failure("WORKFLOW_INPUT_INVALID", err.Error(), nil, "")
	}

	svc := workflow.NewRunService(req.VaultPath, vaultCfg, canonicalWorkflowToolFunc(ctx, req))
	outcome, err := svc.Start(workflow.StartRunRequest{
		WorkflowName: stringArg(req.Args, "name"),
		Inputs:       inputs,
	})
	if err != nil {
		if de, ok := workflow.AsDomainError(err); ok {
			details := workflow.MergeDetails(de.Details, workflow.OutcomeErrorDetails(outcome, de.StepID))
			return commandexec.Failure(workflow.DomainCodeToErrorCode(de.Code), de.Error(), details, workflow.DomainCodeHint(de.Code))
		}
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
	}

	return commandexec.Success(outcome.Result, nil)
}

// HandleWorkflowContinue executes the canonical `workflow_continue` command.
func HandleWorkflowContinue(ctx context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	outputEnv, err := workflow.ParseAgentOutputEnvelope(
		stringArg(req.Args, "agent-output-file"),
		req.Args["agent-output-json"],
		stringArg(req.Args, "agent-output"),
	)
	if err != nil {
		return commandexec.Failure("WORKFLOW_AGENT_OUTPUT_INVALID", err.Error(), nil, "")
	}

	expectedRevision, _ := intArg(req.Args, "expected-revision")
	svc := workflow.NewRunService(req.VaultPath, vaultCfg, canonicalWorkflowToolFunc(ctx, req))
	outcome, err := svc.Continue(workflow.ContinueRunRequest{
		RunID:            stringArg(req.Args, "run-id"),
		ExpectedRevision: expectedRevision,
		AgentOutput:      outputEnv,
	})
	if err != nil {
		if de, ok := workflow.AsDomainError(err); ok {
			details := workflow.MergeDetails(de.Details, workflow.OutcomeErrorDetails(outcome, de.StepID))
			return commandexec.Failure(workflow.DomainCodeToErrorCode(de.Code), de.Error(), details, workflow.DomainCodeHint(de.Code))
		}
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
	}

	return commandexec.Success(outcome.Result, nil)
}

// HandleWorkflowRunsList executes the canonical `workflow_runs_list` command.
func HandleWorkflowRunsList(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	statuses, err := workflow.ParseRunStatusFilter(stringArg(req.Args, "status"))
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "")
	}

	svc := workflow.NewRunService(req.VaultPath, vaultCfg, nil)
	runs, runWarnings, err := svc.ListRuns(workflow.RunListFilter{
		Workflow: stringArg(req.Args, "workflow"),
		Statuses: statuses,
	})
	if err != nil {
		return mapWorkflowFailure(err, "")
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
				loaded, loadErr := workflow.Get(req.VaultPath, run.WorkflowName, vaultCfg)
				if loadErr != nil {
					workflowMissing[run.WorkflowName] = true
				} else {
					wf = loaded
					workflowCache[run.WorkflowName] = wf
				}
			}
			if wf != nil {
				item["step_summaries"] = workflow.BuildStepSummaries(wf, run)
			}
		}

		outRuns = append(outRuns, item)
	}

	warnings := canonicalWorkflowWarnings(runWarnings)
	if len(warnings) > 0 {
		return commandexec.SuccessWithWarnings(map[string]interface{}{"runs": outRuns}, warnings, &commandexec.Meta{Count: len(outRuns)})
	}
	return commandexec.Success(map[string]interface{}{"runs": outRuns}, &commandexec.Meta{Count: len(outRuns)})
}

// HandleWorkflowRunsStep executes the canonical `workflow_runs_step` command.
func HandleWorkflowRunsStep(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	offset, _ := intArg(req.Args, "offset")
	limit, ok := intArg(req.Args, "limit")
	if !ok {
		limit = 100
	}
	paginationRequested := hasAnyArgs(req.Args, "path", "offset", "limit")

	svc := workflow.NewRunService(req.VaultPath, vaultCfg, nil)
	stepResult, err := svc.StepOutput(workflow.StepOutputRequest{
		RunID:      stringArg(req.Args, "run-id"),
		StepID:     stringArg(req.Args, "step-id"),
		Paginated:  paginationRequested,
		Path:       stringArg(req.Args, "path"),
		Offset:     offset,
		Limit:      limit,
		IncludeSum: true,
	})
	if err != nil {
		if de, ok := workflow.AsDomainError(err); ok {
			hint := workflow.DomainCodeHint(de.Code)
			if de.Code == workflow.CodeInvalidInput {
				hint = "Use --path for nested fields and provide valid --offset/--limit values"
			}
			return commandexec.Failure(workflow.DomainCodeToErrorCode(de.Code), de.Error(), de.Details, hint)
		}
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
	}

	state := stepResult.State
	payload := map[string]interface{}{
		"run_id":        state.RunID,
		"workflow_name": state.WorkflowName,
		"status":        state.Status,
		"revision":      state.Revision,
		"step_id":       stringArg(req.Args, "step-id"),
	}
	if paginationRequested {
		payload["step_output_page"] = stepResult.StepOutputPage
	} else {
		payload["step_output"] = stepResult.StepOutput
	}
	if len(stepResult.Summaries) > 0 {
		payload["step_summaries"] = stepResult.Summaries
	}
	return commandexec.Success(payload, nil)
}

// HandleWorkflowRunsPrune executes the canonical `workflow_runs_prune` command.
func HandleWorkflowRunsPrune(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultCfg, failure := loadWorkflowConfig(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	statuses, err := workflow.ParseRunStatusFilter(stringArg(req.Args, "status"))
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "")
	}
	olderThan, err := workflow.ParseOlderThan(stringArg(req.Args, "older-than"))
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "")
	}

	confirm := boolArg(req.Args, "confirm") || req.Confirm
	svc := workflow.NewRunService(req.VaultPath, vaultCfg, nil)
	result, err := svc.PruneRuns(workflow.RunPruneOptions{
		Statuses:  statuses,
		OlderThan: olderThan,
		Apply:     confirm,
	})
	if err != nil {
		return mapWorkflowFailure(err, "")
	}

	data := map[string]interface{}{
		"dry_run": !confirm,
		"prune":   result,
	}
	warnings := canonicalWorkflowWarnings(result.Warnings)
	if len(warnings) > 0 {
		return commandexec.SuccessWithWarnings(data, warnings, nil)
	}
	return commandexec.Success(data, nil)
}

func loadWorkflowConfig(vaultPath string) (*config.VaultConfig, commandexec.Result) {
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return nil, commandexec.Failure("CONFIG_INVALID", "failed to load vault config", nil, "Fix raven.yaml and try again")
	}
	return vaultCfg, commandexec.Result{}
}

func mapWorkflowFailure(err error, fallbackSuggestion string) commandexec.Result {
	de, ok := workflow.AsDomainError(err)
	if !ok {
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, fallbackSuggestion)
	}

	suggestion := workflow.DomainCodeHint(de.Code)
	if suggestion == "" {
		suggestion = fallbackSuggestion
	}
	return commandexec.Failure(workflow.DomainCodeToErrorCode(de.Code), de.Error(), de.Details, suggestion)
}

func canonicalWorkflowWarnings(runWarnings []workflow.RunStoreWarning) []commandexec.Warning {
	if len(runWarnings) == 0 {
		return nil
	}
	warnings := make([]commandexec.Warning, 0, len(runWarnings))
	for _, w := range runWarnings {
		warnings = append(warnings, commandexec.Warning{
			Code:    w.Code,
			Message: w.Message,
			Ref:     w.File,
		})
	}
	return warnings
}

func workflowStepMutationPayload(workflowName, action string, result *workflow.StepMutationResult, previousID string) map[string]interface{} {
	payload := map[string]interface{}{
		"workflow_name": workflowName,
		"file":          result.FileRef,
		"action":        action,
		"step_id":       result.StepID,
		"index":         result.Index,
	}
	if result.Step != nil {
		payload["step"] = result.Step
	}
	if previousID != "" {
		payload["previous_id"] = previousID
	}
	return payload
}

func workflowStepBatchPayload(result *workflow.StepBatchMutationResult) map[string]interface{} {
	applied := make([]map[string]interface{}, 0, len(result.Applied))
	for _, mutation := range result.Applied {
		applied = append(applied, workflowStepMutationPayload(result.WorkflowName, string(mutation.Action), &mutation, mutation.PreviousID))
	}
	return map[string]interface{}{
		"workflow_name": result.WorkflowName,
		"file":          result.FileRef,
		"applied":       applied,
		"count":         len(applied),
	}
}

func parseCanonicalWorkflowInputs(args map[string]interface{}) (map[string]interface{}, error) {
	inputFile := stringArg(args, "input-file")
	inputJSON := args["input-json"]
	inputKV := workflowInputObjectToPairs(args["input"])
	return workflow.ParseInputs(inputFile, inputJSON, inputKV)
}

func workflowInputObjectToPairs(raw interface{}) []string {
	values, ok := raw.(map[string]interface{})
	if !ok || len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%v", key, values[key]))
	}
	return pairs
}

func canonicalWorkflowToolFunc(ctx context.Context, req commandexec.Request) func(tool string, args map[string]interface{}) (interface{}, error) {
	return func(tool string, args map[string]interface{}) (interface{}, error) {
		commandID, ok := commands.ResolveToolCommandID(strings.TrimSpace(tool))
		if !ok {
			return nil, fmt.Errorf("unknown tool: %s", tool)
		}
		if !commands.IsWorkflowAllowedCommandID(commandID) {
			return nil, fmt.Errorf("tool '%s' is not allowed in workflow steps", tool)
		}

		invoker, ok := commandexec.InvokerFromContext(ctx)
		if !ok {
			return nil, fmt.Errorf("workflow command runtime is unavailable")
		}

		result := invoker.Execute(ctx, commandexec.Request{
			CommandID:      commandID,
			VaultPath:      req.VaultPath,
			ConfigPath:     req.ConfigPath,
			StatePath:      req.StatePath,
			ExecutablePath: req.ExecutablePath,
			Caller:         commandexec.CallerWorkflow,
			Args:           args,
		})

		envelope, err := commandResultEnvelope(result)
		if err != nil {
			return nil, err
		}
		if !result.OK {
			b, _ := json.Marshal(envelope)
			return nil, fmt.Errorf("tool '%s' returned error: %s", tool, string(b))
		}
		return envelope, nil
	}
}

func commandResultEnvelope(result commandexec.Result) (map[string]interface{}, error) {
	b, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(b, &envelope); err != nil {
		return nil, err
	}
	return envelope, nil
}

func hasAnyArgs(args map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		value, ok := args[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return true
			}
		default:
			return true
		}
	}
	return false
}
