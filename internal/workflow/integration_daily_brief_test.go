package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestDailyBriefStepsWorkflowMigration(t *testing.T) {
	vaultDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultDir, "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}

	def := strings.TrimSpace(`
description: Create daily brief
inputs:
  date:
    type: string
    required: true
steps:
  - id: todos
    type: tool
    tool: raven_query
    arguments:
      query_string: "trait:todo .value==todo"

  - id: compose
    type: agent
    outputs:
      markdown:
        type: markdown
        required: true
    prompt: |
      Build the daily brief for {{inputs.date}}.

      ## Open Todos
      {{steps.todos.data.results}}

  - id: save
    type: tool
    tool: raven_upsert
    arguments:
      type: brief
      title: "Daily Brief {{inputs.date}}"
      content: "{{steps.compose.outputs.markdown}}"
`) + "\n"

	path := filepath.Join(vaultDir, "workflows", "daily-brief.yaml")
	if err := os.WriteFile(path, []byte(def), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	wf, err := Load(vaultDir, "daily-brief", &config.WorkflowRef{File: "workflows/daily-brief.yaml"})
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if len(wf.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(wf.Steps))
	}

	callCount := 0
	r := NewRunner(vaultDir, &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		callCount++
		if tool != "raven_query" {
			t.Fatalf("expected first tool to be raven_query, got %s", tool)
		}
		return map[string]interface{}{
			"ok": true,
			"data": map[string]interface{}{
				"results": []interface{}{"todo-a", "todo-b"},
			},
		}, nil
	}

	result, err := r.Run(wf, map[string]string{"date": "2026-02-14"})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.Next == nil {
		t.Fatal("expected run to stop at agent step")
	}
	if result.Next.StepID != "compose" {
		t.Fatalf("expected next step id compose, got %s", result.Next.StepID)
	}
	if !strings.Contains(result.Next.Prompt, "todo-a") {
		t.Fatalf("expected prompt to include tool output, got:\n%s", result.Next.Prompt)
	}
	if callCount != 1 {
		t.Fatalf("expected only pre-agent tool step to execute, got %d calls", callCount)
	}
}
