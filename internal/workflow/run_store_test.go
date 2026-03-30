package workflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/config"
)

func TestRunStore_SaveLoadAndList(t *testing.T) {
	t.Parallel()
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

	runs, warnings, err := ListRunStates(vault, cfg, RunListFilter{Workflow: "daily-brief"})
	if err != nil {
		t.Fatalf("ListRunStates error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %d", len(runs))
	}
}

func TestRunStore_PruneByStatus(t *testing.T) {
	t.Parallel()
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

	runs, warnings, err := ListRunStates(vault, cfg, RunListFilter{})
	if err != nil {
		t.Fatalf("ListRunStates error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(runs) != 1 || runs[0].RunID != "wrf_awaiting" {
		t.Fatalf("unexpected remaining runs: %#v", runs)
	}
}

func TestRunStore_ListRunStatesReturnsWarningsForCorruptFiles(t *testing.T) {
	t.Parallel()
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

	if err := SaveRunState(vault, cfg, &WorkflowRunState{
		Version:      1,
		RunID:        "wrf_valid",
		WorkflowName: "daily-brief",
		WorkflowHash: "sha256:abc",
		Status:       RunStatusAwaitingAgent,
		Inputs:       map[string]interface{}{},
		Steps:        map[string]interface{}{},
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		Revision:     1,
	}); err != nil {
		t.Fatalf("SaveRunState error: %v", err)
	}

	storeDir := filepath.Join(vault, cfg.StoragePath)
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatalf("mkdir store dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storeDir, "wrf_corrupt.json"), []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("write corrupt run file: %v", err)
	}

	runs, warnings, err := ListRunStates(vault, cfg, RunListFilter{})
	if err != nil {
		t.Fatalf("ListRunStates error: %v", err)
	}
	if len(runs) != 1 || runs[0].RunID != "wrf_valid" {
		t.Fatalf("expected valid run to remain visible, got %#v", runs)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for corrupt file, got %d (%#v)", len(warnings), warnings)
	}
	if warnings[0].Code != "RUN_STATE_PARSE_ERROR" {
		t.Fatalf("unexpected warning code: %s", warnings[0].Code)
	}
	if warnings[0].RunID != "wrf_corrupt" {
		t.Fatalf("unexpected warning run id: %s", warnings[0].RunID)
	}
}

func TestRunStore_SavePreservesUpdatedAtAndExpiresAt(t *testing.T) {
	t.Parallel()
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

	updatedAt := time.Date(2026, 3, 28, 15, 4, 5, 0, time.UTC)
	expiresAt := updatedAt.Add(72 * time.Hour)
	state := &WorkflowRunState{
		Version:      1,
		RunID:        "wrf_test_preserve_timestamps",
		WorkflowName: "daily-brief",
		WorkflowHash: "sha256:abc",
		Status:       RunStatusAwaitingAgent,
		Inputs:       map[string]interface{}{},
		Steps:        map[string]interface{}{},
		CreatedAt:    updatedAt.Add(-1 * time.Hour),
		UpdatedAt:    updatedAt,
		ExpiresAt:    &expiresAt,
		Revision:     1,
	}

	if err := SaveRunState(vault, cfg, state); err != nil {
		t.Fatalf("SaveRunState error: %v", err)
	}
	got, err := LoadRunState(vault, cfg, state.RunID)
	if err != nil {
		t.Fatalf("LoadRunState error: %v", err)
	}
	if !got.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("updated_at mismatch: got %s want %s", got.UpdatedAt, updatedAt)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expires_at mismatch: got %#v want %s", got.ExpiresAt, expiresAt)
	}
}

func TestRunStore_AutoPruneHonorsStoredExpiry(t *testing.T) {
	t.Parallel()
	vault := t.TempDir()
	cfg := config.ResolvedWorkflowRunsConfig{
		StoragePath:               ".raven/workflow-runs",
		AutoPrune:                 true,
		KeepCompletedForDays:      365,
		KeepFailedForDays:         365,
		KeepAwaitingForDays:       365,
		MaxRuns:                   100,
		PreserveLatestPerWorkflow: 2,
	}

	now := time.Now().UTC()
	expiredAt := now.Add(-2 * time.Hour)
	state := &WorkflowRunState{
		Version:      1,
		RunID:        "wrf_expired",
		WorkflowName: "daily-brief",
		WorkflowHash: "sha256:abc",
		Status:       RunStatusCompleted,
		Inputs:       map[string]interface{}{},
		Steps:        map[string]interface{}{},
		CreatedAt:    now.Add(-48 * time.Hour),
		UpdatedAt:    now.Add(-1 * time.Hour),
		CompletedAt:  ptrTime(now.Add(-1 * time.Hour)),
		ExpiresAt:    &expiredAt,
		Revision:     1,
	}
	if err := SaveRunState(vault, cfg, state); err != nil {
		t.Fatalf("SaveRunState error: %v", err)
	}

	result, err := AutoPruneRunStates(vault, cfg)
	if err != nil {
		t.Fatalf("AutoPruneRunStates error: %v", err)
	}
	if result.Deleted != 1 {
		t.Fatalf("expected 1 deletion, got %#v", result)
	}

	runs, warnings, err := ListRunStates(vault, cfg, RunListFilter{})
	if err != nil {
		t.Fatalf("ListRunStates error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
	if len(runs) != 0 {
		t.Fatalf("expected expired run to be pruned, got %#v", runs)
	}
}

func ptrTime(v time.Time) *time.Time {
	return &v
}
