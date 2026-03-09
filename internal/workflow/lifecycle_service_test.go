package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestRunService_StartAndContinue(t *testing.T) {
	keep := t.TempDir()
	if err := os.MkdirAll(filepath.Join(keep, "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}

	content := `description: test workflow
inputs:
  date:
    type: string
    required: true
steps:
  - id: fetch
    type: tool
    tool: raven_query
    arguments:
      query_string: "object:project"
  - id: compose
    type: agent
    prompt: "Compose for {{inputs.date}} {{steps.fetch.data.results}}"
    outputs:
      markdown:
        type: markdown
        required: true
  - id: save
    type: tool
    tool: raven_upsert
    arguments:
      type: brief
      title: "Daily Brief {{inputs.date}}"
      content: "{{steps.compose.outputs.markdown}}"
`
	if err := os.WriteFile(filepath.Join(keep, "workflows", "daily.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow file: %v", err)
	}

	cfg := &config.KeepConfig{
		Workflows: map[string]*config.WorkflowRef{
			"daily": {File: "workflows/daily.yaml"},
		},
	}

	var savedContent interface{}
	callCount := 0
	svc := NewRunService(keep, cfg, func(tool string, args map[string]interface{}) (interface{}, error) {
		callCount++
		switch tool {
		case "raven_query":
			return map[string]interface{}{
				"ok": true,
				"data": map[string]interface{}{
					"results": []interface{}{"a", "b"},
				},
			}, nil
		case "raven_upsert":
			savedContent = args["content"]
			return map[string]interface{}{"ok": true}, nil
		default:
			return nil, nil
		}
	})

	start, err := svc.Start(StartRunRequest{
		WorkflowName: "daily",
		Inputs: map[string]interface{}{
			"date": "2026-03-08",
		},
	})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if start.Result == nil || start.Result.Status != RunStatusAwaitingAgent {
		t.Fatalf("expected awaiting agent status, got %#v", start.Result)
	}
	if callCount != 1 {
		t.Fatalf("expected 1 tool call before agent boundary, got %d", callCount)
	}

	cont, err := svc.Continue(ContinueRunRequest{
		RunID:            start.Result.RunID,
		ExpectedRevision: start.Result.Revision,
		AgentOutput: AgentOutputEnvelope{
			Outputs: map[string]interface{}{
				"markdown": "# Brief",
			},
		},
	})
	if err != nil {
		t.Fatalf("Continue returned error: %v", err)
	}
	if cont.Result == nil || cont.Result.Status != RunStatusCompleted {
		t.Fatalf("expected completed status, got %#v", cont.Result)
	}
	if savedContent != "# Brief" {
		t.Fatalf("expected save step to receive markdown output, got %#v", savedContent)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 total tool calls, got %d", callCount)
	}
}

func TestRunService_ContinueRevisionConflict(t *testing.T) {
	keep := t.TempDir()
	if err := os.MkdirAll(filepath.Join(keep, "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	content := `description: test
steps:
  - id: compose
    type: agent
    prompt: "x"
    outputs:
      markdown:
        type: markdown
        required: true
`
	if err := os.WriteFile(filepath.Join(keep, "workflows", "x.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow file: %v", err)
	}

	cfg := &config.KeepConfig{
		Workflows: map[string]*config.WorkflowRef{
			"x": {File: "workflows/x.yaml"},
		},
	}
	svc := NewRunService(keep, cfg, func(tool string, args map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"ok": true}, nil
	})

	start, err := svc.Start(StartRunRequest{WorkflowName: "x", Inputs: map[string]interface{}{}})
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	_, err = svc.Continue(ContinueRunRequest{
		RunID:            start.Result.RunID,
		ExpectedRevision: start.Result.Revision + 1,
		AgentOutput: AgentOutputEnvelope{
			Outputs: map[string]interface{}{"markdown": "x"},
		},
	})
	if err == nil {
		t.Fatal("expected revision conflict error")
	}
	de, ok := AsDomainError(err)
	if !ok {
		t.Fatalf("expected domain error, got %T", err)
	}
	if de.Code != CodeWorkflowConflict {
		t.Fatalf("expected CodeWorkflowConflict, got %s", de.Code)
	}
}
