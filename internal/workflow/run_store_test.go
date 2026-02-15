package workflow

import (
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/config"
)

func TestRunStore_SaveLoadAndList(t *testing.T) {
	vault := t.TempDir()
	cfg := config.ResolvedWorkflowRunsConfig{
		StoragePath:               ".raven/workflow-runs",
		AutoPrune:                 true,
		KeepCompletedForDays:      7,
		KeepFailedForDays:         7,
		KeepAwaitingForDays:       7,
		MaxRuns:                   100,
		PreserveLatestPerWorkflow: 2,
	}

	state := &WorkflowRunState{
		Version:      1,
		RunID:        "wrf_test_1",
		WorkflowName: "daily-brief",
		WorkflowHash: "sha256:abc",
		Status:       RunStatusAwaitingAgent,
		Cursor:       1,
		Inputs:       map[string]interface{}{"date": "2026-02-14"},
		Steps:        map[string]interface{}{"todos": map[string]interface{}{"ok": true}},
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		Revision:     1,
	}

	if err := SaveRunState(vault, cfg, state); err != nil {
		t.Fatalf("SaveRunState error: %v", err)
	}
	got, err := LoadRunState(vault, cfg, state.RunID)
	if err != nil {
		t.Fatalf("LoadRunState error: %v", err)
	}
	if got.RunID != state.RunID {
		t.Fatalf("run id mismatch: got %s want %s", got.RunID, state.RunID)
	}

	runs, err := ListRunStates(vault, cfg, RunListFilter{Workflow: "daily-brief"})
	if err != nil {
		t.Fatalf("ListRunStates error: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %d", len(runs))
	}
}

func TestRunStore_PruneByStatus(t *testing.T) {
	vault := t.TempDir()
	cfg := config.ResolvedWorkflowRunsConfig{
		StoragePath:               ".raven/workflow-runs",
		AutoPrune:                 true,
		KeepCompletedForDays:      365,
		KeepFailedForDays:         365,
		KeepAwaitingForDays:       365,
		MaxRuns:                   100,
		PreserveLatestPerWorkflow: 1,
	}

	create := func(id string, status RunStatus) {
		t.Helper()
		err := SaveRunState(vault, cfg, &WorkflowRunState{
			Version:      1,
			RunID:        id,
			WorkflowName: "daily-brief",
			WorkflowHash: "sha256:abc",
			Status:       status,
			Inputs:       map[string]interface{}{},
			Steps:        map[string]interface{}{},
			CreatedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
			Revision:     1,
		})
		if err != nil {
			t.Fatalf("SaveRunState(%s) error: %v", id, err)
		}
	}

	create("wrf_completed", RunStatusCompleted)
	create("wrf_awaiting", RunStatusAwaitingAgent)

	statuses := map[RunStatus]bool{RunStatusCompleted: true}
	result, err := PruneRunStates(vault, cfg, RunPruneOptions{
		Statuses: statuses,
		Apply:    true,
	})
	if err != nil {
		t.Fatalf("PruneRunStates error: %v", err)
	}
	if result.Deleted != 1 {
		t.Fatalf("expected 1 deletion, got %d", result.Deleted)
	}

	runs, err := ListRunStates(vault, cfg, RunListFilter{})
	if err != nil {
		t.Fatalf("ListRunStates error: %v", err)
	}
	if len(runs) != 1 || runs[0].RunID != "wrf_awaiting" {
		t.Fatalf("unexpected remaining runs: %#v", runs)
	}
}
