package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/paths"
)

type RunListFilter struct {
	Workflow string
	Statuses map[RunStatus]bool
}

type RunPruneOptions struct {
	Statuses  map[RunStatus]bool
	OlderThan *time.Duration
	Now       time.Time
	Apply     bool
}

type RunPruneResult struct {
	Scanned int      `json:"scanned"`
	Matched int      `json:"matched"`
	Deleted int      `json:"deleted"`
	RunIDs  []string `json:"run_ids,omitempty"`
}

func SaveRunState(vaultPath string, cfg config.ResolvedWorkflowRunsConfig, state *WorkflowRunState) error {
	if state == nil {
		return fmt.Errorf("run state is nil")
	}
	dir, err := runStoreDir(vaultPath, cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create workflow run storage: %w", err)
	}

	state.UpdatedAt = time.Now().UTC()
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal run state: %w", err)
	}

	path := filepath.Join(dir, state.RunID+".json")
	if err := atomicfile.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write run state: %w", err)
	}
	return nil
}

func LoadRunState(vaultPath string, cfg config.ResolvedWorkflowRunsConfig, runID string) (*WorkflowRunState, error) {
	if runID == "" {
		return nil, fmt.Errorf("run id is required")
	}
	dir, err := runStoreDir(vaultPath, cfg)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, runID+".json")
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("run state not found: %s", runID)
		}
		return nil, fmt.Errorf("read run state: %w", err)
	}
	var state WorkflowRunState
	if err := json.Unmarshal(content, &state); err != nil {
		return nil, fmt.Errorf("parse run state: %w", err)
	}
	if state.RunID == "" {
		state.RunID = runID
	}
	return &state, nil
}

func DeleteRunState(vaultPath string, cfg config.ResolvedWorkflowRunsConfig, runID string) error {
	dir, err := runStoreDir(vaultPath, cfg)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, runID+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete run state: %w", err)
	}
	return nil
}

func ListRunStates(vaultPath string, cfg config.ResolvedWorkflowRunsConfig, filter RunListFilter) ([]*WorkflowRunState, error) {
	dir, err := runStoreDir(vaultPath, cfg)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read run storage: %w", err)
	}

	runs := make([]*WorkflowRunState, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var state WorkflowRunState
		if err := json.Unmarshal(content, &state); err != nil {
			continue
		}
		if !matchesRunFilter(&state, filter) {
			continue
		}
		runs = append(runs, &state)
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].UpdatedAt.After(runs[j].UpdatedAt)
	})
	return runs, nil
}

func AutoPruneRunStates(vaultPath string, cfg config.ResolvedWorkflowRunsConfig) (*RunPruneResult, error) {
	if !cfg.AutoPrune {
		return &RunPruneResult{}, nil
	}
	now := time.Now().UTC()
	runs, err := ListRunStates(vaultPath, cfg, RunListFilter{})
	if err != nil {
		return nil, err
	}
	toDelete := chooseRunsForAutoPrune(runs, cfg, now)
	if len(toDelete) == 0 {
		return &RunPruneResult{Scanned: len(runs), Matched: 0, Deleted: 0}, nil
	}

	deleted := 0
	for _, run := range toDelete {
		if err := DeleteRunState(vaultPath, cfg, run.RunID); err == nil {
			deleted++
		}
	}
	return &RunPruneResult{
		Scanned: len(runs),
		Matched: len(toDelete),
		Deleted: deleted,
		RunIDs:  runIDs(toDelete),
	}, nil
}

func PruneRunStates(vaultPath string, cfg config.ResolvedWorkflowRunsConfig, opts RunPruneOptions) (*RunPruneResult, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	runs, err := ListRunStates(vaultPath, cfg, RunListFilter{Statuses: opts.Statuses})
	if err != nil {
		return nil, err
	}

	candidates := make([]*WorkflowRunState, 0, len(runs))
	for _, run := range runs {
		if opts.OlderThan != nil {
			age := opts.Now.Sub(run.UpdatedAt)
			if age < *opts.OlderThan {
				continue
			}
		}
		candidates = append(candidates, run)
	}

	result := &RunPruneResult{
		Scanned: len(runs),
		Matched: len(candidates),
		RunIDs:  runIDs(candidates),
	}
	if !opts.Apply {
		return result, nil
	}

	for _, run := range candidates {
		if err := DeleteRunState(vaultPath, cfg, run.RunID); err == nil {
			result.Deleted++
		}
	}
	return result, nil
}

func chooseRunsForAutoPrune(runs []*WorkflowRunState, cfg config.ResolvedWorkflowRunsConfig, now time.Time) []*WorkflowRunState {
	var toDelete []*WorkflowRunState

	for _, run := range runs {
		if shouldExpireByTTL(run, cfg, now) {
			toDelete = append(toDelete, run)
		}
	}

	remaining := filterOutRuns(runs, toDelete)
	if len(remaining) <= cfg.MaxRuns {
		return toDelete
	}

	protected := newestRunsByWorkflow(remaining, cfg.PreserveLatestPerWorkflow)

	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].UpdatedAt.Before(remaining[j].UpdatedAt)
	})

	capDeletes := 0
	for _, run := range remaining {
		if len(remaining)-capDeletes <= cfg.MaxRuns {
			break
		}
		if protected[run.RunID] {
			continue
		}
		if containsRunID(toDelete, run.RunID) {
			continue
		}
		toDelete = append(toDelete, run)
		capDeletes++
	}
	return toDelete
}

func shouldExpireByTTL(run *WorkflowRunState, cfg config.ResolvedWorkflowRunsConfig, now time.Time) bool {
	switch run.Status {
	case RunStatusCompleted:
		return now.Sub(run.UpdatedAt) > (time.Duration(cfg.KeepCompletedForDays) * 24 * time.Hour)
	case RunStatusFailed:
		return now.Sub(run.UpdatedAt) > (time.Duration(cfg.KeepFailedForDays) * 24 * time.Hour)
	case RunStatusAwaitingAgent:
		return now.Sub(run.UpdatedAt) > (time.Duration(cfg.KeepAwaitingForDays) * 24 * time.Hour)
	default:
		return false
	}
}

func newestRunsByWorkflow(runs []*WorkflowRunState, preserve int) map[string]bool {
	protected := map[string]bool{}
	if preserve <= 0 {
		return protected
	}

	byWorkflow := map[string][]*WorkflowRunState{}
	for _, run := range runs {
		byWorkflow[run.WorkflowName] = append(byWorkflow[run.WorkflowName], run)
	}

	for _, group := range byWorkflow {
		sort.Slice(group, func(i, j int) bool {
			return group[i].UpdatedAt.After(group[j].UpdatedAt)
		})
		limit := preserve
		if limit > len(group) {
			limit = len(group)
		}
		for i := 0; i < limit; i++ {
			protected[group[i].RunID] = true
		}
	}
	return protected
}

func runStoreDir(vaultPath string, cfg config.ResolvedWorkflowRunsConfig) (string, error) {
	rel := filepath.ToSlash(filepath.Clean(cfg.StoragePath))
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || rel == "." {
		return "", fmt.Errorf("invalid workflow run storage path")
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("workflow run storage path escapes vault")
	}
	abs := filepath.Join(vaultPath, rel)
	if err := paths.ValidateWithinVault(vaultPath, abs); err != nil {
		return "", fmt.Errorf("invalid workflow run storage path: %w", err)
	}
	return abs, nil
}

func matchesRunFilter(run *WorkflowRunState, filter RunListFilter) bool {
	if run == nil {
		return false
	}
	if filter.Workflow != "" && run.WorkflowName != filter.Workflow {
		return false
	}
	if len(filter.Statuses) > 0 && !filter.Statuses[run.Status] {
		return false
	}
	return true
}

func runIDs(runs []*WorkflowRunState) []string {
	out := make([]string, 0, len(runs))
	for _, run := range runs {
		if run != nil && run.RunID != "" {
			out = append(out, run.RunID)
		}
	}
	return out
}

func containsRunID(runs []*WorkflowRunState, runID string) bool {
	for _, run := range runs {
		if run != nil && run.RunID == runID {
			return true
		}
	}
	return false
}

func filterOutRuns(all []*WorkflowRunState, removed []*WorkflowRunState) []*WorkflowRunState {
	removedSet := map[string]bool{}
	for _, run := range removed {
		if run != nil {
			removedSet[run.RunID] = true
		}
	}
	kept := make([]*WorkflowRunState, 0, len(all))
	for _, run := range all {
		if run == nil {
			continue
		}
		if removedSet[run.RunID] {
			continue
		}
		kept = append(kept, run)
	}
	return kept
}
