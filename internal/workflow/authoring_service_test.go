package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/config"
)

func TestAuthoringService_MutateStep(t *testing.T) {
	vault := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vault, "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	workflowPath := filepath.Join(vault, "workflows", "demo.yaml")
	initial := `description: demo
steps:
  - id: fetch
    type: tool
    tool: raven_query
  - id: compose
    type: agent
    prompt: "Summarize"
`
	if err := os.WriteFile(workflowPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write workflow file: %v", err)
	}

	cfg := &config.VaultConfig{
		Workflows: map[string]*config.WorkflowRef{
			"demo": {File: "workflows/demo.yaml"},
		},
	}
	svc := NewAuthoringService(vault, cfg)

	t.Run("add with before hint", func(t *testing.T) {
		res, err := svc.MutateStep(StepMutationRequest{
			WorkflowName: "demo",
			Action:       StepMutationAdd,
			Position: PositionHint{
				BeforeStepID: "compose",
			},
			Step: &config.WorkflowStep{
				ID:   "expand",
				Type: "tool",
				Tool: "raven_read",
			},
		})
		if err != nil {
			t.Fatalf("MutateStep(add) error: %v", err)
		}
		if res.Index != 1 {
			t.Fatalf("insert index = %d, want 1", res.Index)
		}
		got := readWorkflowStepIDs(t, workflowPath)
		want := []string{"fetch", "expand", "compose"}
		assertStringSliceEqual(t, got, want)
	})

	t.Run("update rename step", func(t *testing.T) {
		res, err := svc.MutateStep(StepMutationRequest{
			WorkflowName: "demo",
			Action:       StepMutationUpdate,
			TargetStepID: "expand",
			Step: &config.WorkflowStep{
				ID:   "enrich",
				Type: "tool",
				Tool: "raven_search",
			},
		})
		if err != nil {
			t.Fatalf("MutateStep(update) error: %v", err)
		}
		if res.PreviousID != "expand" || res.StepID != "enrich" {
			t.Fatalf("unexpected update result: %#v", res)
		}
		got := readWorkflowStepIDs(t, workflowPath)
		want := []string{"fetch", "enrich", "compose"}
		assertStringSliceEqual(t, got, want)
	})

	t.Run("remove step", func(t *testing.T) {
		res, err := svc.MutateStep(StepMutationRequest{
			WorkflowName: "demo",
			Action:       StepMutationRemove,
			TargetStepID: "enrich",
		})
		if err != nil {
			t.Fatalf("MutateStep(remove) error: %v", err)
		}
		if res.StepID != "enrich" {
			t.Fatalf("removed step id = %s, want enrich", res.StepID)
		}
		got := readWorkflowStepIDs(t, workflowPath)
		want := []string{"fetch", "compose"}
		assertStringSliceEqual(t, got, want)
	})
}

func TestAuthoringService_MutateStep_NotFound(t *testing.T) {
	vault := t.TempDir()
	cfg := &config.VaultConfig{Workflows: map[string]*config.WorkflowRef{}}
	svc := NewAuthoringService(vault, cfg)

	_, err := svc.MutateStep(StepMutationRequest{
		WorkflowName: "missing",
		Action:       StepMutationRemove,
		TargetStepID: "x",
	})
	if err == nil {
		t.Fatal("expected error for missing workflow")
	}
	de, ok := AsDomainError(err)
	if !ok || de.Code != CodeWorkflowNotFound {
		t.Fatalf("expected CodeWorkflowNotFound, got %#v", err)
	}
}

func readWorkflowStepIDs(t *testing.T, path string) []string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workflow file: %v", err)
	}
	var def externalWorkflowDef
	if err := yaml.Unmarshal(content, &def); err != nil {
		t.Fatalf("yaml unmarshal workflow file: %v", err)
	}
	ids := make([]string, 0, len(def.Steps))
	for _, step := range def.Steps {
		if step == nil {
			continue
		}
		ids = append(ids, step.ID)
	}
	return ids
}

func assertStringSliceEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d len(want)=%d; got=%v want=%v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%q want[%d]=%q; got=%v want=%v", i, got[i], i, want[i], got, want)
		}
	}
}
