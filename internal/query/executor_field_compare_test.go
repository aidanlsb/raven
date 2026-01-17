package query

import (
	"slices"
	"testing"
)

func TestObjectFieldComparison_NumericUsesNumericOrdering(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Add objects with numeric fields.
	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('metric/a', 'metric/a.md', 'metric', '{"score": 10}', 1),
			('metric/b', 'metric/b.md', 'metric', '{"score": 2}', 1),
			('metric/c', 'metric/c.md', 'metric', '{"score": "10"}', 1);
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	e := NewExecutor(db)

	q, err := Parse("object:metric .score>5")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	results, err := e.ExecuteObjectQueryWithPipeline(q)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}

	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.ID)
	}

	// Expect score 10 and "10" (stringified) to match, score 2 should not.
	// This asserts numeric ordering rather than lexicographic.
	if len(ids) != 2 {
		t.Fatalf("got %d results: %#v", len(ids), ids)
	}
	if !(slices.Contains(ids, "metric/a") && slices.Contains(ids, "metric/c")) {
		t.Fatalf("unexpected ids: %#v", ids)
	}
}
