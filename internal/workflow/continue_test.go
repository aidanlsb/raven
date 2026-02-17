package workflow

import (
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestRunner_ContinueAfterAgentOutput(t *testing.T) {
	wf := &Workflow{
		Name: "daily-brief",
		Steps: []*config.WorkflowStep{
			{
				ID:   "todos",
				Type: "tool",
				Tool: "raven_query",
				Arguments: map[string]interface{}{
					"query_string": "trait:todo .value==todo",
				},
			},
			{
				ID:   "compose",
				Type: "agent",
				Outputs: map[string]*config.WorkflowPromptOutput{
					"markdown": {Type: "markdown", Required: true},
				},
				Prompt: "Compose brief for {{inputs.date}} with {{steps.todos.data.results}}",
			},
			{
				ID:   "save",
				Type: "tool",
				Tool: "raven_upsert",
				Arguments: map[string]interface{}{
					"type":    "brief",
					"title":   "Daily Brief {{inputs.date}}",
					"content": "{{steps.compose.outputs.markdown}}",
				},
			},
		},
	}

	var saveArgs map[string]interface{}
	callCount := 0
	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		callCount++
		if callCount == 1 {
			return map[string]interface{}{
				"ok": true,
				"data": map[string]interface{}{
					"results": []interface{}{"a", "b"},
				},
			}, nil
		}
		saveArgs = args
		return map[string]interface{}{"ok": true}, nil
	}

	state, err := NewRunState(wf, map[string]interface{}{"date": "2026-02-14"})
	if err != nil {
		t.Fatalf("NewRunState error: %v", err)
	}

	first, err := r.RunWithState(wf, state)
	if err != nil {
		t.Fatalf("RunWithState error: %v", err)
	}
	if first.Status != RunStatusAwaitingAgent {
		t.Fatalf("expected awaiting_agent, got %s", first.Status)
	}
	if first.Next == nil || first.Next.StepID != "compose" {
		t.Fatalf("expected next agent compose, got %#v", first.Next)
	}

	err = ApplyAgentOutputs(wf, state, AgentOutputEnvelope{
		Outputs: map[string]interface{}{
			"markdown": "# Brief\n- item",
		},
	})
	if err != nil {
		t.Fatalf("ApplyAgentOutputs error: %v", err)
	}
	state.Revision++

	second, err := r.RunWithState(wf, state)
	if err != nil {
		t.Fatalf("resume RunWithState error: %v", err)
	}
	if second.Status != RunStatusCompleted {
		t.Fatalf("expected completed, got %s", second.Status)
	}
	if len(second.StepSummaries) != 3 {
		t.Fatalf("expected 3 step summaries, got %d", len(second.StepSummaries))
	}
	summaryByID := map[string]RunStepSummary{}
	for _, s := range second.StepSummaries {
		summaryByID[s.StepID] = s
	}
	if summaryByID["todos"].Status != "ok" {
		t.Fatalf("unexpected todos summary: %#v", summaryByID["todos"])
	}
	if summaryByID["compose"].Status != "accepted" {
		t.Fatalf("unexpected compose summary: %#v", summaryByID["compose"])
	}
	if summaryByID["save"].Status != "ok" {
		t.Fatalf("unexpected save summary: %#v", summaryByID["save"])
	}
	if saveArgs == nil {
		t.Fatal("expected save tool call args")
	}
	if saveArgs["content"] != "# Brief\n- item" {
		t.Fatalf("expected propagated agent output, got %#v", saveArgs["content"])
	}
}

func TestApplyAgentOutputs_Validation(t *testing.T) {
	wf := &Workflow{
		Name: "x",
		Steps: []*config.WorkflowStep{
			{
				ID:   "a",
				Type: "agent",
				Outputs: map[string]*config.WorkflowPromptOutput{
					"markdown": {Type: "markdown", Required: true},
				},
				Prompt: "x",
			},
		},
	}

	state, err := NewRunState(wf, map[string]interface{}{})
	if err != nil {
		t.Fatalf("NewRunState error: %v", err)
	}
	state.Status = RunStatusAwaitingAgent
	state.AwaitingStep = "a"
	state.Steps["a"] = map[string]interface{}{"prompt": "x"}

	if err := ApplyAgentOutputs(wf, state, AgentOutputEnvelope{Outputs: map[string]interface{}{}}); err == nil {
		t.Fatal("expected validation error for missing required output")
	}
}
