package query

import (
	"testing"
)

func TestExecuteObjectPageQuery(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)
	q, err := Parse("object:project")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	t.Run("all results with no limit", func(t *testing.T) {
		results, err := exec.ExecuteObjectPageQuery(q, 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("limit restricts count", func(t *testing.T) {
		results, err := exec.ExecuteObjectPageQuery(q, 1, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}
	})

	t.Run("offset skips results", func(t *testing.T) {
		results, err := exec.ExecuteObjectPageQuery(q, 1, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}
	})

	t.Run("offset beyond results returns empty", func(t *testing.T) {
		results, err := exec.ExecuteObjectPageQuery(q, 10, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("rejects trait query", func(t *testing.T) {
		tq, _ := Parse("trait:todo")
		_, err := exec.ExecuteObjectPageQuery(tq, 0, 0)
		if err == nil {
			t.Error("expected error for trait query on object page executor")
		}
	})
}

func TestExecuteObjectCountQuery(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)

	t.Run("counts all of type", func(t *testing.T) {
		q, _ := Parse("object:project")
		count, err := exec.ExecuteObjectCountQuery(q)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 2 {
			t.Errorf("expected count 2, got %d", count)
		}
	})

	t.Run("counts with predicate", func(t *testing.T) {
		q, _ := Parse(`object:project .status==active`)
		count, err := exec.ExecuteObjectCountQuery(q)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 1 {
			t.Errorf("expected count 1, got %d", count)
		}
	})

	t.Run("zero count for no matches", func(t *testing.T) {
		q, _ := Parse(`object:project .status==archived`)
		count, err := exec.ExecuteObjectCountQuery(q)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 0 {
			t.Errorf("expected count 0, got %d", count)
		}
	})

	t.Run("rejects trait query", func(t *testing.T) {
		tq, _ := Parse("trait:todo")
		_, err := exec.ExecuteObjectCountQuery(tq)
		if err == nil {
			t.Error("expected error for trait query on object count executor")
		}
	})
}

func TestExecuteObjectIDQuery(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)

	t.Run("returns IDs only", func(t *testing.T) {
		q, _ := Parse("object:project")
		ids, err := exec.ExecuteObjectIDQuery(q, 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 2 {
			t.Errorf("expected 2 IDs, got %d", len(ids))
		}
		for _, id := range ids {
			if id == "" {
				t.Error("got empty ID")
			}
		}
	})

	t.Run("limit restricts IDs", func(t *testing.T) {
		q, _ := Parse("object:project")
		ids, err := exec.ExecuteObjectIDQuery(q, 1, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 1 {
			t.Errorf("expected 1 ID, got %d", len(ids))
		}
	})

	t.Run("offset beyond results returns empty", func(t *testing.T) {
		q, _ := Parse("object:project")
		ids, err := exec.ExecuteObjectIDQuery(q, 10, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 0 {
			t.Errorf("expected 0 IDs, got %d", len(ids))
		}
	})
}

func TestExecuteTraitPageQuery(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)
	q, err := Parse("trait:due")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	t.Run("all results with no limit", func(t *testing.T) {
		results, err := exec.ExecuteTraitPageQuery(q, 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("expected 3 results, got %d", len(results))
		}
	})

	t.Run("limit restricts count", func(t *testing.T) {
		results, err := exec.ExecuteTraitPageQuery(q, 1, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}
	})

	t.Run("offset beyond results", func(t *testing.T) {
		results, err := exec.ExecuteTraitPageQuery(q, 10, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("rejects object query", func(t *testing.T) {
		oq, _ := Parse("object:project")
		_, err := exec.ExecuteTraitPageQuery(oq, 0, 0)
		if err == nil {
			t.Error("expected error for object query on trait page executor")
		}
	})
}

func TestExecuteTraitCountQuery(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)

	t.Run("counts all of type", func(t *testing.T) {
		q, _ := Parse("trait:due")
		count, err := exec.ExecuteTraitCountQuery(q)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 3 {
			t.Errorf("expected count 3, got %d", count)
		}
	})

	t.Run("counts with value predicate", func(t *testing.T) {
		q, _ := Parse("trait:todo .value==todo")
		count, err := exec.ExecuteTraitCountQuery(q)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 2 {
			t.Errorf("expected count 2, got %d", count)
		}
	})

	t.Run("rejects object query", func(t *testing.T) {
		oq, _ := Parse("object:project")
		_, err := exec.ExecuteTraitCountQuery(oq)
		if err == nil {
			t.Error("expected error for object query on trait count executor")
		}
	})
}

func TestExecuteTraitIDQuery(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	exec := NewExecutor(db)

	t.Run("returns IDs only", func(t *testing.T) {
		q, _ := Parse("trait:todo")
		ids, err := exec.ExecuteTraitIDQuery(q, 0, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 3 {
			t.Errorf("expected 3 IDs, got %d", len(ids))
		}
		for _, id := range ids {
			if id == "" {
				t.Error("got empty ID")
			}
		}
	})

	t.Run("limit restricts IDs", func(t *testing.T) {
		q, _ := Parse("trait:todo")
		ids, err := exec.ExecuteTraitIDQuery(q, 1, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 1 {
			t.Errorf("expected 1 ID, got %d", len(ids))
		}
	})
}
