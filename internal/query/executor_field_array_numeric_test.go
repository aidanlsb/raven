package query

import "testing"

func TestObjectFieldEquality_NumericArrayMembership(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('nums/a', 'nums/a.md', 'nums', '{"scores":[10,2]}', 1),
			('nums/b', 'nums/b.md', 'nums', '{"scores":["10"]}', 1),
			('nums/c', 'nums/c.md', 'nums', '{"scores":[3]}', 1);
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	e := NewExecutor(db)
	q, err := Parse("object:nums .scores==10")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	results, err := e.ExecuteObjectQuery(q)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}

	got := make(map[string]bool)
	for _, r := range results {
		got[r.ID] = true
	}

	if len(got) != 2 || !got["nums/a"] || !got["nums/b"] {
		t.Fatalf("unexpected ids: %#v", got)
	}
}
