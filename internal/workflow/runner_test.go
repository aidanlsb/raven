package workflow

import (
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestRunner_StopsAtAgentStep(t *testing.T) {
	wf := &Workflow{
		Name: "x",
		Steps: []*config.WorkflowStep{
			{
				ID:   "fetch",
				Type: "tool",
				Tool: "raven_stats",
			},
			{
				ID:     "agent",
				Type:   "agent",
				Prompt: "Status:\n{{steps.fetch.data.status}}",
				Outputs: map[string]*config.WorkflowPromptOutput{
					"markdown": {Type: "markdown", Required: true},
				},
			},
			{
				ID:   "after",
				Type: "tool",
				Tool: "raven_query",
				Arguments: map[string]interface{}{
					"query_string": "object:project",
				},
			},
		},
	}

	callCount := 0
	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		callCount++
		return map[string]interface{}{
			"ok": true,
			"data": map[string]interface{}{
				"status": "green",
			},
		}, nil
	}

	result, err := r.Run(wf, map[string]string{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Next == nil {
		t.Fatal("expected agent step in result.Next")
	}
	if result.Next.StepID != "agent" {
		t.Fatalf("unexpected Next.StepID: %s", result.Next.StepID)
	}
	if callCount != 1 {
		t.Fatalf("expected exactly one tool call before agent stop, got %d", callCount)
	}
}

func TestRunner_ReturnsStepSummariesInsteadOfStepPayloads(t *testing.T) {
	wf := &Workflow{
		Name: "x",
		Steps: []*config.WorkflowStep{
			{
				ID:   "fetch",
				Type: "tool",
				Tool: "raven_stats",
			},
			{
				ID:     "compose",
				Type:   "agent",
				Prompt: "Status:\n{{steps.fetch.data.status}}",
				Outputs: map[string]*config.WorkflowPromptOutput{
					"markdown": {Type: "markdown", Required: true},
				},
			},
		},
	}

	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{
			"ok": true,
			"data": map[string]interface{}{
				"status": "green",
			},
		}, nil
	}

	result, err := r.Run(wf, map[string]string{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Next == nil {
		t.Fatal("expected awaiting agent boundary")
	}
	if len(result.StepSummaries) != 2 {
		t.Fatalf("expected 2 step summaries, got %d", len(result.StepSummaries))
	}

	byID := map[string]RunStepSummary{}
	for _, s := range result.StepSummaries {
		byID[s.StepID] = s
	}

	fetch, ok := byID["fetch"]
	if !ok {
		t.Fatal("missing fetch step summary")
	}
	if fetch.Status != "ok" || !fetch.HasOutput {
		t.Fatalf("unexpected fetch summary: %#v", fetch)
	}

	compose, ok := byID["compose"]
	if !ok {
		t.Fatal("missing compose step summary")
	}
	if compose.Status != string(RunStatusAwaitingAgent) {
		t.Fatalf("unexpected compose status: %s", compose.Status)
	}
	if !compose.HasOutput {
		t.Fatalf("expected compose step to have stored prompt state: %#v", compose)
	}
}

func TestRunner_ToolStepTypedInterpolation(t *testing.T) {
	wf := &Workflow{
		Name: "typed",
		Steps: []*config.WorkflowStep{
			{
				ID:   "fetch",
				Type: "tool",
				Tool: "raven_query",
				Arguments: map[string]interface{}{
					"query_string": "object:project",
				},
			},
			{
				ID:   "summarize",
				Type: "tool",
				Tool: "raven_upsert",
				Arguments: map[string]interface{}{
					"title": "Daily Brief",
					"type":  "brief",
					"field": map[string]interface{}{
						"count": "{{steps.fetch.data.count}}",
						"ids":   "{{steps.fetch.data.ids}}",
					},
					"content": "count={{steps.fetch.data.count}}",
				},
			},
			{
				ID:     "agent",
				Type:   "agent",
				Prompt: "Done",
			},
		},
	}

	var secondCallArgs map[string]interface{}
	callIndex := 0

	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		callIndex++
		if callIndex == 1 {
			return map[string]interface{}{
				"ok": true,
				"data": map[string]interface{}{
					"count": 3,
					"ids":   []interface{}{"p1", "p2", "p3"},
				},
			}, nil
		}
		secondCallArgs = args
		return map[string]interface{}{"ok": true, "data": map[string]interface{}{}}, nil
	}

	_, err := r.Run(wf, map[string]string{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if secondCallArgs == nil {
		t.Fatal("expected second tool call args to be captured")
	}

	field, ok := secondCallArgs["field"].(map[string]interface{})
	if !ok {
		t.Fatalf("field should be map[string]interface{}, got %T", secondCallArgs["field"])
	}
	if _, ok := field["count"].(int); !ok {
		t.Fatalf("field.count should preserve numeric type, got %T", field["count"])
	}
	if _, ok := field["ids"].([]interface{}); !ok {
		t.Fatalf("field.ids should preserve array type, got %T", field["ids"])
	}
	if secondCallArgs["content"] != "count=3" {
		t.Fatalf("content interpolation mismatch: got %#v", secondCallArgs["content"])
	}
}

func TestRunner_OptionalInputOmittedIsAddressable(t *testing.T) {
	wf := &Workflow{
		Name: "optional-inputs",
		Inputs: map[string]*config.WorkflowInput{
			"title":   {Type: "string", Required: true},
			"project": {Type: "string", Required: false},
		},
		Steps: []*config.WorkflowStep{
			{
				ID:   "create",
				Type: "tool",
				Tool: "raven_upsert",
				Arguments: map[string]interface{}{
					"type":    "analysis-plan",
					"title":   "{{inputs.title}}",
					"project": "{{inputs.project}}",
				},
			},
		},
	}

	var createArgs map[string]interface{}
	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		createArgs = args
		return map[string]interface{}{"ok": true}, nil
	}

	result, err := r.Run(wf, map[string]string{"title": "New model order"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != RunStatusCompleted {
		t.Fatalf("expected completed status, got %s", result.Status)
	}
	if createArgs == nil {
		t.Fatal("expected create tool args")
	}
	if _, ok := createArgs["project"]; !ok {
		t.Fatal("expected optional project input key to be present")
	}
	if createArgs["project"] != nil {
		t.Fatalf("expected omitted optional project to resolve as nil, got %#v", createArgs["project"])
	}
}
