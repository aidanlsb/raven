// Package workflow provides workflow definition, loading, and running.
package workflow

import "github.com/aidanlsb/raven/internal/config"

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

// PromptRequest is emitted when a workflow reaches a prompt step.
type PromptRequest struct {
	StepID   string                                  `json:"step_id"`
	Prompt   string                                  `json:"prompt"`
	Outputs  map[string]*config.WorkflowPromptOutput `json:"outputs"`
	Template string                                  `json:"template,omitempty"`
}

// RunResult is the output of running a workflow until a prompt step or completion.
type RunResult struct {
	Name   string                 `json:"name"`
	Inputs map[string]string      `json:"inputs"`
	Steps  map[string]interface{} `json:"steps"`
	Next   *PromptRequest         `json:"next,omitempty"`
}

// ListItem represents a workflow in the list output.
type ListItem struct {
	Name        string                           `json:"name"`
	Description string                           `json:"description"`
	Inputs      map[string]*config.WorkflowInput `json:"inputs,omitempty"`
}
