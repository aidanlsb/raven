// Package workflow provides workflow definition, loading, and rendering.
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

	// Context defines data to gather before rendering the prompt.
	Context map[string]*config.ContextQuery

	// Prompt is the task description with interpolated context.
	Prompt string
}

// RenderResult contains the rendered workflow ready for an agent.
type RenderResult struct {
	// Name is the workflow identifier.
	Name string `json:"name"`

	// Prompt is the rendered prompt text with variables substituted.
	Prompt string `json:"prompt"`

	// Context contains the gathered data from context queries.
	Context map[string]interface{} `json:"context"`
}

// ListItem represents a workflow in the list output.
type ListItem struct {
	Name        string                           `json:"name"`
	Description string                           `json:"description"`
	Inputs      map[string]*config.WorkflowInput `json:"inputs,omitempty"`
}
