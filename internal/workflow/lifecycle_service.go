package workflow

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/config"
)

type RunService struct {
	vaultPath string
	vaultCfg  *config.VaultConfig
	toolFunc  func(tool string, args map[string]interface{}) (interface{}, error)
	now       func() time.Time
}

func NewRunService(
	vaultPath string,
	vaultCfg *config.VaultConfig,
	toolFunc func(tool string, args map[string]interface{}) (interface{}, error),
) *RunService {
	return &RunService{
		vaultPath: vaultPath,
		vaultCfg:  vaultCfg,
		toolFunc:  toolFunc,
		now:       func() time.Time { return time.Now().UTC() },
	}
}

type StartRunRequest struct {
	WorkflowName string
	Inputs       map[string]interface{}
}

type ContinueRunRequest struct {
	RunID            string
	ExpectedRevision int
	AgentOutput      AgentOutputEnvelope
}

type RunExecutionOutcome struct {
	Workflow *Workflow
	State    *WorkflowRunState
	Result   *RunResult
}

type StepOutputRequest struct {
	RunID      string
	StepID     string
	Paginated  bool
	Path       string
	Offset     int
	Limit      int
	IncludeSum bool
}

type StepOutputResult struct {
	State            *WorkflowRunState
	StepOutput       interface{}
	StepOutputPage   *StepOutputPage
	Summaries        []RunStepSummary
	AvailableStepIDs []string
}

func (s *RunService) Start(req StartRunRequest) (*RunExecutionOutcome, error) {
	if s == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "run service is nil", nil)
	}
	if strings.TrimSpace(req.WorkflowName) == "" {
		return nil, newDomainError(CodeInvalidInput, "workflow name is required", nil)
	}
	if s.vaultCfg == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "vault config is nil", nil)
	}

	wf, err := Get(s.vaultPath, req.WorkflowName, s.vaultCfg)
	if err != nil {
		return nil, newDomainError(CodeWorkflowNotFound, err.Error(), err)
	}

	runCfg := s.vaultCfg.GetWorkflowRunsConfig()
	_, _ = AutoPruneRunStates(s.vaultPath, runCfg)

	inputs := req.Inputs
	if inputs == nil {
		inputs = map[string]interface{}{}
	}

	state, err := NewRunState(wf, inputs)
	if err != nil {
		return nil, newDomainError(CodeWorkflowInvalid, err.Error(), err)
	}

	runner := NewRunner(s.vaultPath, s.vaultCfg)
	runner.ToolFunc = s.toolFunc

	result, err := runner.RunWithState(wf, state)
	if err != nil {
		code, stepID := classifyRunnerFailure(err)
		markRunFailedState(state, string(code), stepID, err, s.now())
		ApplyRetentionExpiry(state, runCfg, state.UpdatedAt)
		_ = SaveRunState(s.vaultPath, runCfg, state)

		de := newDomainError(code, err.Error(), err)
		de.StepID = stepID
		return &RunExecutionOutcome{Workflow: wf, State: state}, de
	}

	ApplyRetentionExpiry(state, runCfg, state.UpdatedAt)
	syncRunResultState(result, state)
	if err := SaveRunState(s.vaultPath, runCfg, state); err != nil {
		return nil, newDomainError(CodeFileWriteError, "save workflow run state", err)
	}

	return &RunExecutionOutcome{
		Workflow: wf,
		State:    state,
		Result:   result,
	}, nil
}

func (s *RunService) Continue(req ContinueRunRequest) (*RunExecutionOutcome, error) {
	if s == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "run service is nil", nil)
	}
	if strings.TrimSpace(req.RunID) == "" {
		return nil, newDomainError(CodeInvalidInput, "run id is required", nil)
	}
	if s.vaultCfg == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "vault config is nil", nil)
	}

	runCfg := s.vaultCfg.GetWorkflowRunsConfig()
	_, _ = AutoPruneRunStates(s.vaultPath, runCfg)

	state, err := LoadRunState(s.vaultPath, runCfg, req.RunID)
	if err != nil {
		code := CodeWorkflowRunNotFound
		if strings.Contains(err.Error(), "parse run state") {
			code = CodeWorkflowStateCorrupt
		}
		return nil, newDomainError(code, err.Error(), err)
	}

	if req.ExpectedRevision <= 0 {
		de := newDomainError(
			CodeWorkflowConflict,
			fmt.Sprintf("expected revision is required; retry with revision %d", state.Revision),
			nil,
		)
		de.Details = map[string]interface{}{
			"run_id":                     state.RunID,
			"workflow_name":              state.WorkflowName,
			"revision":                   state.Revision,
			"expected_revision_required": true,
		}
		return &RunExecutionOutcome{State: state}, de
	}

	if req.ExpectedRevision > 0 && state.Revision != req.ExpectedRevision {
		de := newDomainError(
			CodeWorkflowConflict,
			fmt.Sprintf("revision mismatch: expected %d, got %d", req.ExpectedRevision, state.Revision),
			nil,
		)
		de.Details = map[string]interface{}{
			"run_id":            state.RunID,
			"workflow_name":     state.WorkflowName,
			"expected_revision": req.ExpectedRevision,
			"revision":          state.Revision,
		}
		return &RunExecutionOutcome{State: state}, de
	}

	wf, err := Get(s.vaultPath, state.WorkflowName, s.vaultCfg)
	if err != nil {
		return &RunExecutionOutcome{State: state}, newDomainError(CodeWorkflowNotFound, err.Error(), err)
	}

	currentHash, err := WorkflowHash(wf)
	if err != nil {
		return &RunExecutionOutcome{Workflow: wf, State: state}, newDomainError(CodeWorkflowInvalid, "compute workflow hash", err)
	}
	if state.WorkflowHash != "" && currentHash != state.WorkflowHash {
		return &RunExecutionOutcome{Workflow: wf, State: state}, newDomainError(
			CodeWorkflowChanged,
			"workflow definition changed since run started",
			nil,
		)
	}

	if err := ApplyAgentOutputs(wf, state, req.AgentOutput); err != nil {
		code := classifyContinueValidationFailure(state, err)
		return &RunExecutionOutcome{Workflow: wf, State: state}, newDomainError(code, err.Error(), err)
	}

	state.Revision++
	runner := NewRunner(s.vaultPath, s.vaultCfg)
	runner.ToolFunc = s.toolFunc

	result, err := runner.RunWithState(wf, state)
	if err != nil {
		code, stepID := classifyRunnerFailure(err)
		markRunFailedState(state, string(code), stepID, err, s.now())
		state.Revision++
		ApplyRetentionExpiry(state, runCfg, state.UpdatedAt)
		_ = SaveRunState(s.vaultPath, runCfg, state)

		de := newDomainError(code, err.Error(), err)
		de.StepID = stepID
		return &RunExecutionOutcome{Workflow: wf, State: state}, de
	}

	ApplyRetentionExpiry(state, runCfg, state.UpdatedAt)
	syncRunResultState(result, state)
	if err := SaveRunState(s.vaultPath, runCfg, state); err != nil {
		return nil, newDomainError(CodeFileWriteError, "save workflow run state", err)
	}

	return &RunExecutionOutcome{
		Workflow: wf,
		State:    state,
		Result:   result,
	}, nil
}

func (s *RunService) ListRuns(filter RunListFilter) ([]*WorkflowRunState, []RunStoreWarning, error) {
	if s == nil || s.vaultCfg == nil {
		return nil, nil, newDomainError(CodeWorkflowInvalid, "run service is not configured", nil)
	}
	return ListRunStates(s.vaultPath, s.vaultCfg.GetWorkflowRunsConfig(), filter)
}

func (s *RunService) PruneRuns(opts RunPruneOptions) (*RunPruneResult, error) {
	if s == nil || s.vaultCfg == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "run service is not configured", nil)
	}
	return PruneRunStates(s.vaultPath, s.vaultCfg.GetWorkflowRunsConfig(), opts)
}

func (s *RunService) StepOutput(req StepOutputRequest) (*StepOutputResult, error) {
	if s == nil || s.vaultCfg == nil {
		return nil, newDomainError(CodeWorkflowInvalid, "run service is not configured", nil)
	}
	if strings.TrimSpace(req.RunID) == "" || strings.TrimSpace(req.StepID) == "" {
		return nil, newDomainError(CodeInvalidInput, "run id and step id are required", nil)
	}

	state, err := LoadRunState(s.vaultPath, s.vaultCfg.GetWorkflowRunsConfig(), req.RunID)
	if err != nil {
		code := CodeWorkflowRunNotFound
		if strings.Contains(err.Error(), "parse run state") {
			code = CodeWorkflowStateCorrupt
		}
		return nil, newDomainError(code, err.Error(), err)
	}

	stepOutput, ok := state.Steps[req.StepID]
	if !ok {
		available := make([]string, 0, len(state.Steps))
		for id := range state.Steps {
			available = append(available, id)
		}
		sort.Strings(available)
		de := newDomainError(CodeRefNotFound, fmt.Sprintf("step '%s' not found in run '%s'", req.StepID, req.RunID), nil)
		de.Details = map[string]interface{}{
			"run_id":          state.RunID,
			"workflow_name":   state.WorkflowName,
			"available_steps": available,
		}
		return nil, de
	}

	result := &StepOutputResult{
		State:      state,
		StepOutput: stepOutput,
	}
	if req.Paginated {
		page, err := PaginateStepOutput(stepOutput, req.Path, req.Offset, req.Limit)
		if err != nil {
			return nil, newDomainError(CodeInvalidInput, err.Error(), err)
		}
		result.StepOutputPage = page
	}

	if req.IncludeSum {
		if wf, wfErr := Get(s.vaultPath, state.WorkflowName, s.vaultCfg); wfErr == nil {
			result.Summaries = BuildStepSummaries(wf, state)
		}
	}

	result.AvailableStepIDs = make([]string, 0, len(state.Steps))
	for id := range state.Steps {
		result.AvailableStepIDs = append(result.AvailableStepIDs, id)
	}
	sort.Strings(result.AvailableStepIDs)
	return result, nil
}

func classifyRunnerFailure(err error) (Code, string) {
	if err == nil {
		return CodeWorkflowInvalid, ""
	}
	if de, ok := AsDomainError(err); ok {
		return de.Code, de.StepID
	}
	msg := err.Error()
	stepID := extractStepIDFromError(msg)

	switch {
	case strings.Contains(msg, "unknown variable:"),
		strings.Contains(msg, "invalid inputs reference"):
		return CodeWorkflowInterpolationError, stepID
	case strings.Contains(msg, "tool '"),
		strings.Contains(msg, "tool function not configured"):
		return CodeWorkflowToolExecutionFailed, stepID
	case strings.Contains(msg, "missing required inputs"),
		strings.Contains(msg, "unknown workflow input"),
		strings.Contains(msg, "workflow input '"):
		return CodeWorkflowInputInvalid, stepID
	default:
		return CodeWorkflowInvalid, stepID
	}
}

func classifyContinueValidationFailure(state *WorkflowRunState, err error) Code {
	if state != nil {
		switch state.Status {
		case RunStatusCompleted, RunStatusFailed:
			return CodeWorkflowTerminalState
		}
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if strings.Contains(msg, "not awaiting agent output") {
		return CodeWorkflowNotAwaitingAgent
	}
	return CodeWorkflowAgentOutputInvalid
}

func markRunFailedState(state *WorkflowRunState, code, stepID string, runErr error, now time.Time) {
	if state == nil {
		return
	}
	state.Status = RunStatusFailed
	state.Failure = &RunFailure{
		Code:    code,
		Message: runErr.Error(),
		StepID:  stepID,
		At:      now,
	}
	state.CompletedAt = &now
	state.UpdatedAt = now
	state.AwaitingStep = ""
}

func extractStepIDFromError(msg string) string {
	const marker = "step '"
	start := strings.Index(msg, marker)
	if start < 0 {
		return ""
	}
	rest := msg[start+len(marker):]
	end := strings.Index(rest, "'")
	if end <= 0 {
		return ""
	}
	return rest[:end]
}

func syncRunResultState(result *RunResult, state *WorkflowRunState) {
	if result == nil || state == nil {
		return
	}
	result.UpdatedAt = state.UpdatedAt.Format(time.RFC3339)
	if state.CompletedAt != nil {
		result.CompletedAt = state.CompletedAt.Format(time.RFC3339)
	}
	if state.ExpiresAt != nil {
		result.ExpiresAt = state.ExpiresAt.Format(time.RFC3339)
	} else {
		result.ExpiresAt = ""
	}
}
