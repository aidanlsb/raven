package query

import "testing"

func runResultRowCount(qt QueryType, r *RunResult) int {
	switch qt {
	case QueryTypeObject:
		return len(r.Objects)
	case QueryTypeTrait:
		return len(r.Traits)
	case QueryTypeAsset:
		return len(r.Assets)
	case QueryTypeSection:
		return len(r.Sections)
	case QueryTypeLink:
		return len(r.Links)
	default:
		return 0
	}
}

// TestRun_AllRootsCountIDsPageFull exercises the single generic execution path
// (Run) across every query root and every mode: full, count-only, ids-only,
// and paginated. It guards the collapse of the previously copy-pasted per-root
// dispatch blocks.
func TestRun_AllRootsCountIDsPageFull(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()
	exec := NewExecutor(db)

	cases := []struct {
		name  string
		query string
		total int
	}{
		{"object", "type:project", 2},
		{"trait", "trait:due", 3},
		{"asset", "asset", 3},
		{"section", "section", 5},
		{"link", "link", 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := Parse(tc.query)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.query, err)
			}

			// Full: all rows, Total == Returned == total.
			full, err := exec.Run(q, RunRequest{})
			if err != nil {
				t.Fatalf("full run: %v", err)
			}
			if full.Total != tc.total || full.Returned != tc.total {
				t.Fatalf("full: total=%d returned=%d, want %d/%d", full.Total, full.Returned, tc.total, tc.total)
			}
			if got := runResultRowCount(q.Type, full); got != tc.total {
				t.Fatalf("full: row slice len=%d, want %d", got, tc.total)
			}
			if len(full.IDs) != 0 {
				t.Fatalf("full: expected no IDs, got %d", len(full.IDs))
			}

			// Count-only: Total set, no rows or IDs.
			count, err := exec.Run(q, RunRequest{CountOnly: true})
			if err != nil {
				t.Fatalf("count run: %v", err)
			}
			if count.Total != tc.total || count.Returned != 0 {
				t.Fatalf("count: total=%d returned=%d, want %d/0", count.Total, count.Returned, tc.total)
			}
			if runResultRowCount(q.Type, count) != 0 || len(count.IDs) != 0 {
				t.Fatalf("count: expected no rows/ids, got rows=%d ids=%d", runResultRowCount(q.Type, count), len(count.IDs))
			}

			// IDs-only with a limit: pagination forces a COUNT(*) for Total.
			wantReturned := tc.total
			if wantReturned > 2 {
				wantReturned = 2
			}
			ids, err := exec.Run(q, RunRequest{IDsOnly: true, Limit: 2})
			if err != nil {
				t.Fatalf("ids run: %v", err)
			}
			if len(ids.IDs) != wantReturned || ids.Returned != wantReturned {
				t.Fatalf("ids: len=%d returned=%d, want %d", len(ids.IDs), ids.Returned, wantReturned)
			}
			if ids.Total != tc.total {
				t.Fatalf("ids: total=%d, want %d (paginated count)", ids.Total, tc.total)
			}
			if runResultRowCount(q.Type, ids) != 0 {
				t.Fatalf("ids: expected no typed rows, got %d", runResultRowCount(q.Type, ids))
			}

			// Paginated rows: Total is the full count, Returned is the page size.
			paged, err := exec.Run(q, RunRequest{Limit: 1})
			if err != nil {
				t.Fatalf("paged run: %v", err)
			}
			if paged.Total != tc.total {
				t.Fatalf("paged: total=%d, want %d", paged.Total, tc.total)
			}
			if paged.Returned != 1 || runResultRowCount(q.Type, paged) != 1 {
				t.Fatalf("paged: returned=%d rows=%d, want 1/1", paged.Returned, runResultRowCount(q.Type, paged))
			}
		})
	}
}

// TestRun_MatchesTypedExecutors verifies Run yields the same rows/counts as the
// typed Execute* methods, i.e. the generic path did not change semantics.
func TestRun_MatchesTypedExecutors(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()
	exec := NewExecutor(db)

	t.Run("object", func(t *testing.T) {
		q, _ := Parse("type:project")
		run, err := exec.Run(q, RunRequest{})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		typed, err := exec.ExecuteObjectQuery(q)
		if err != nil {
			t.Fatalf("typed: %v", err)
		}
		if len(run.Objects) != len(typed) {
			t.Fatalf("object row counts differ: run=%d typed=%d", len(run.Objects), len(typed))
		}
		for i := range typed {
			if run.Objects[i].ID != typed[i].ID {
				t.Fatalf("object[%d] id mismatch: run=%q typed=%q", i, run.Objects[i].ID, typed[i].ID)
			}
		}
	})

	t.Run("trait", func(t *testing.T) {
		q, _ := Parse("trait:todo")
		run, err := exec.Run(q, RunRequest{})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		typed, err := exec.ExecuteTraitQuery(q)
		if err != nil {
			t.Fatalf("typed: %v", err)
		}
		if len(run.Traits) != len(typed) {
			t.Fatalf("trait row counts differ: run=%d typed=%d", len(run.Traits), len(typed))
		}
	})
}
