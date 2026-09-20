package query

import (
	"testing"
)

func TestRunObjectPage(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)
	q, err := Parse("type:project")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	t.Run("all results with no limit", func(t *testing.T) {
		result, err := exec.Run(q, RunRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Objects) != 2 {
			t.Errorf("expected 2 results, got %d", len(result.Objects))
		}
	})

	t.Run("limit restricts count", func(t *testing.T) {
		result, err := exec.Run(q, RunRequest{Limit: 1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Objects) != 1 {
			t.Errorf("expected 1 result, got %d", len(result.Objects))
		}
	})

	t.Run("offset skips results", func(t *testing.T) {
		result, err := exec.Run(q, RunRequest{Limit: 1, Offset: 1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Objects) != 1 {
			t.Errorf("expected 1 result, got %d", len(result.Objects))
		}
	})

	t.Run("offset beyond results returns empty", func(t *testing.T) {
		result, err := exec.Run(q, RunRequest{Limit: 10, Offset: 100})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Objects) != 0 {
			t.Errorf("expected 0 results, got %d", len(result.Objects))
		}
	})
}

func TestRunObjectCount(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)

	t.Run("counts all of type", func(t *testing.T) {
		q, _ := Parse("type:project")
		result, err := exec.Run(q, RunRequest{CountOnly: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Total != 2 {
			t.Errorf("expected count 2, got %d", result.Total)
		}
	})

	t.Run("counts with predicate", func(t *testing.T) {
		q, _ := Parse(`type:project .status==active`)
		result, err := exec.Run(q, RunRequest{CountOnly: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Total != 1 {
			t.Errorf("expected count 1, got %d", result.Total)
		}
	})

	t.Run("zero count for no matches", func(t *testing.T) {
		q, _ := Parse(`type:project .status==archived`)
		result, err := exec.Run(q, RunRequest{CountOnly: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Total != 0 {
			t.Errorf("expected count 0, got %d", result.Total)
		}
	})
}

func TestRunObjectIDs(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)

	t.Run("returns IDs only", func(t *testing.T) {
		q, _ := Parse("type:project")
		result, err := exec.Run(q, RunRequest{IDsOnly: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.IDs) != 2 {
			t.Errorf("expected 2 IDs, got %d", len(result.IDs))
		}
		for _, id := range result.IDs {
			if id == "" {
				t.Error("got empty ID")
			}
		}
	})

	t.Run("limit restricts IDs", func(t *testing.T) {
		q, _ := Parse("type:project")
		result, err := exec.Run(q, RunRequest{IDsOnly: true, Limit: 1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.IDs) != 1 {
			t.Errorf("expected 1 ID, got %d", len(result.IDs))
		}
	})

	t.Run("offset beyond results returns empty", func(t *testing.T) {
		q, _ := Parse("type:project")
		result, err := exec.Run(q, RunRequest{IDsOnly: true, Limit: 10, Offset: 100})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.IDs) != 0 {
			t.Errorf("expected 0 IDs, got %d", len(result.IDs))
		}
	})
}

func TestRunTraitPage(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)
	q, err := Parse("trait:due")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	t.Run("all results with no limit", func(t *testing.T) {
		result, err := exec.Run(q, RunRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Traits) != 3 {
			t.Errorf("expected 3 results, got %d", len(result.Traits))
		}
	})

	t.Run("limit restricts count", func(t *testing.T) {
		result, err := exec.Run(q, RunRequest{Limit: 1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Traits) != 1 {
			t.Errorf("expected 1 result, got %d", len(result.Traits))
		}
	})

	t.Run("offset beyond results", func(t *testing.T) {
		result, err := exec.Run(q, RunRequest{Limit: 10, Offset: 100})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Traits) != 0 {
			t.Errorf("expected 0 results, got %d", len(result.Traits))
		}
	})
}

func TestRunTraitCount(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)

	t.Run("counts all of type", func(t *testing.T) {
		q, _ := Parse("trait:due")
		result, err := exec.Run(q, RunRequest{CountOnly: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Total != 3 {
			t.Errorf("expected count 3, got %d", result.Total)
		}
	})

	t.Run("counts with value predicate", func(t *testing.T) {
		q, _ := Parse("trait:todo .value==todo")
		result, err := exec.Run(q, RunRequest{CountOnly: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Total != 2 {
			t.Errorf("expected count 2, got %d", result.Total)
		}
	})
}

func TestRunTraitIDs(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)

	t.Run("returns IDs only", func(t *testing.T) {
		q, _ := Parse("trait:todo")
		result, err := exec.Run(q, RunRequest{IDsOnly: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.IDs) != 3 {
			t.Errorf("expected 3 IDs, got %d", len(result.IDs))
		}
		for _, id := range result.IDs {
			if id == "" {
				t.Error("got empty ID")
			}
		}
	})

	t.Run("limit restricts IDs", func(t *testing.T) {
		q, _ := Parse("trait:todo")
		result, err := exec.Run(q, RunRequest{IDsOnly: true, Limit: 1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.IDs) != 1 {
			t.Errorf("expected 1 ID, got %d", len(result.IDs))
		}
	})
}
