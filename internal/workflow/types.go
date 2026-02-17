// Package workflow provides workflow definition, loading, and running.
package workflow

import (
	"time"

	"github.com/aidanlsb/raven/internal/config"
)

// Workflow represents a fully loaded and validated workflow.
type Workflow struct {
	// Name is the workflow identifier (from the key in raven.yaml).
	Name string

	// Description is a brief summary of what the workflow does.
	Description string

	// Inputs defines the parameters the workflow accepts.
	Inputs map[string]*config.WorkflowInput

	// Steps are executed in order.
	Steps []*config.WorkflowStep
}

// AgentRequest is emitted when a workflow reaches an agent step.
type AgentRequest struct {
	StepID  string                                  `json:"step_id"`
	Prompt  string                                  `json:"prompt"`
	Outputs map[string]*config.WorkflowPromptOutput `json:"outputs"`
	Example map[string]interface{}                  `json:"example,omitempty"`
}

type RunStatus string

const (
	RunStatusRunning       RunStatus = "running"
	RunStatusAwaitingAgent RunStatus = "awaiting_agent"
	RunStatusCompleted     RunStatus = "completed"
	RunStatusFailed        RunStatus = "failed"
	RunStatusCancelled     RunStatus = "cancelled"
)

type RunFailure struct {
	Code    string    `json:"code"`
	Message string    `json:"message"`
	StepID  string    `json:"step_id,omitempty"`
	At      time.Time `json:"at"`
}

type RunHistoryEvent struct {
	StepID   string    `json:"step_id"`
	StepType string    `json:"step_type"`
	Status   string    `json:"status"`
	At       time.Time `json:"at"`
}

type RunExecutionSummary struct {
	ToolsExecuted          int `json:"tools_executed"`
	AgentBoundariesCrossed int `json:"agent_boundaries_crossed"`
}

// RunStepSummary is a lightweight status view for a workflow step.
type RunStepSummary struct {
	StepID    string `json:"step_id"`
	StepType  string `json:"step_type"`
	Status    string `json:"status"`
	HasOutput bool   `json:"has_output"`
}

type RunSummary struct {
	TerminalStepID string               `json:"terminal_step_id,omitempty"`
	Summary        *RunExecutionSummary `json:"summary,omitempty"`
}

// WorkflowRunState is the persisted checkpoint format for workflow runs.
type WorkflowRunState struct {
	Version      int                    `json:"version"`
	RunID        string                 `json:"run_id"`
	WorkflowName string                 `json:"workflow_name"`
	WorkflowHash string                 `json:"workflow_hash"`
	Status       RunStatus              `json:"status"`
	Cursor       int                    `json:"cursor"`
	AwaitingStep string                 `json:"awaiting_step_id,omitempty"`
	Inputs       map[string]interface{} `json:"inputs"`
	Steps        map[string]interface{} `json:"steps"`
	History      []RunHistoryEvent      `json:"history,omitempty"`
	Failure      *RunFailure            `json:"failure,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	ExpiresAt    *time.Time             `json:"expires_at,omitempty"`
	Revision     int                    `json:"revision"`
	LockedBy     *string                `json:"locked_by,omitempty"`
	LockedAt     *time.Time             `json:"locked_at,omitempty"`
}

// RunResult is the output of running a workflow until an agent step or completion.
type RunResult struct {
	RunID          string                 `json:"run_id"`
	WorkflowName   string                 `json:"workflow_name"`
	Status         RunStatus              `json:"status"`
	Revision       int                    `json:"revision"`
	Cursor         int                    `json:"cursor"`
	AwaitingStepID string                 `json:"awaiting_step_id,omitempty"`
	Inputs         map[string]interface{} `json:"inputs"`
	StepSummaries  []RunStepSummary       `json:"step_summaries"`
	Next           *AgentRequest          `json:"next,omitempty"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
	ExpiresAt      string                 `json:"expires_at,omitempty"`
	CompletedAt    string                 `json:"completed_at,omitempty"`
	Result         *RunSummary            `json:"result,omitempty"`
	Failure        *RunFailure            `json:"failure,omitempty"`
}

// ListItem represents a workflow in the list output.
type ListItem struct {
	Name        string                           `json:"name"`
	Description string                           `json:"description"`
	Inputs      map[string]*config.WorkflowInput `json:"inputs,omitempty"`
}
