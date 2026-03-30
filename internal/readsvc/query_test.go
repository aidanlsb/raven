package readsvc

import (
	"testing"

	"github.com/aidanlsb/raven/internal/index"
)

func TestExecuteQuery_InvalidInput(t *testing.T) {
	t.Parallel()
	_, err := ExecuteQuery(nil, ExecuteQueryRequest{QueryString: "object:project"})
	if err == nil {
		t.Fatalf("expected error for nil runtime")
	}

	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rt := &Runtime{DB: db}

	_, err = ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "object:project", Limit: -1})
	if err == nil || err.Error() != "limit must be >= 0" {
		t.Fatalf("expected limit validation error, got: %v", err)
	}

	_, err = ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "object:project", Offset: -1})
	if err == nil || err.Error() != "offset must be >= 0" {
		t.Fatalf("expected offset validation error, got: %v", err)
	}
}

func TestExecuteQuery_ObjectModes(t *testing.T) {
	t.Parallel()
	rt := seededRuntime(t)

	result, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "object:project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.QueryType != "object" || result.TypeName != "project" {
		t.Fatalf("unexpected query metadata: %#v", result)
	}
	if result.Total != 2 || len(result.Objects) != 2 || result.Returned != 2 {
		t.Fatalf("unexpected object results: %#v", result)
	}

	idsOnly, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "object:project", IDsOnly: true, Limit: 1})
	if err != nil {
		t.Fatalf("unexpected IDsOnly error: %v", err)
	}
	if len(idsOnly.IDs) != 1 || idsOnly.Returned != 1 {
		t.Fatalf("unexpected IDsOnly result: %#v", idsOnly)
	}
	if idsOnly.Total != 2 {
		t.Fatalf("expected total 2, got %d", idsOnly.Total)
	}

	countOnly, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "object:project", CountOnly: true})
	if err != nil {
		t.Fatalf("unexpected CountOnly error: %v", err)
	}
	if countOnly.Total != 2 || countOnly.Returned != 0 {
		t.Fatalf("unexpected CountOnly result: %#v", countOnly)
	}
	if len(countOnly.Objects) != 0 || len(countOnly.IDs) != 0 {
		t.Fatalf("count-only should not include rows or ids: %#v", countOnly)
	}

	paged, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "object:project", Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("unexpected paged object query error: %v", err)
	}
	if paged.Total != 2 || paged.Returned != 1 || len(paged.Objects) != 1 {
		t.Fatalf("unexpected paged object result: %#v", paged)
	}
	if paged.Objects[0].ID != "project/raven" {
		t.Fatalf("unexpected paged object ID: %#v", paged.Objects[0])
	}
}

func TestExecuteQuery_TraitModes(t *testing.T) {
	t.Parallel()
	rt := seededRuntime(t)

	result, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "trait:todo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.QueryType != "trait" || result.TypeName != "todo" {
		t.Fatalf("unexpected query metadata: %#v", result)
	}
	if result.Total != 2 || len(result.Traits) != 2 || result.Returned != 2 {
		t.Fatalf("unexpected trait results: %#v", result)
	}

	idsOnly, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "trait:todo", IDsOnly: true, Offset: 1})
	if err != nil {
		t.Fatalf("unexpected IDsOnly error: %v", err)
	}
	if len(idsOnly.IDs) != 1 || idsOnly.Returned != 1 || idsOnly.Total != 2 {
		t.Fatalf("unexpected IDsOnly result: %#v", idsOnly)
	}

	countOnly, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "trait:todo", CountOnly: true})
	if err != nil {
		t.Fatalf("unexpected CountOnly error: %v", err)
	}
	if countOnly.Total != 2 || countOnly.Returned != 0 {
		t.Fatalf("unexpected CountOnly result: %#v", countOnly)
	}
	if len(countOnly.Traits) != 0 || len(countOnly.IDs) != 0 {
		t.Fatalf("count-only should not include rows or ids: %#v", countOnly)
	}

	paged, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "trait:todo", Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("unexpected paged trait query error: %v", err)
	}
	if paged.Total != 2 || paged.Returned != 1 || len(paged.Traits) != 1 {
		t.Fatalf("unexpected paged trait result: %#v", paged)
	}
	if paged.Traits[0].ID != "projects/raven.md:trait:0" {
		t.Fatalf("unexpected paged trait ID: %#v", paged.Traits[0])
	}
}

func TestExecuteQuery_RefPredicateUsesLazyResolver(t *testing.T) {
	t.Parallel()
	rt := seededRuntime(t)

	_, err := rt.DB.DB().Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields) VALUES
			('note/standup', 'notes/standup.md', 'note', 1, '{}')
	`)
	if err != nil {
		t.Fatalf("failed to seed note object: %v", err)
	}

	_, err = rt.DB.DB().Exec(`
		INSERT INTO refs (source_id, target_raw, target_id, file_path, line_number) VALUES
			('note/standup', 'project/raven', 'project/raven', 'notes/standup.md', 3)
	`)
	if err != nil {
		t.Fatalf("failed to seed refs: %v", err)
	}

	result, err := ExecuteQuery(rt, ExecuteQueryRequest{
		QueryString: "object:note refs([[raven]])",
	})
	if err != nil {
		t.Fatalf("unexpected ref query error: %v", err)
	}
	if result.Total != 1 || result.Returned != 1 {
		t.Fatalf("unexpected ref query result: %#v", result)
	}
	if len(result.Objects) != 1 || result.Objects[0].ID != "note/standup" {
		t.Fatalf("unexpected ref query objects: %#v", result.Objects)
	}
}

func seededRuntime(t *testing.T) *Runtime {
	t.Helper()

	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.DB().Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields) VALUES
			('project/raven', 'projects/raven.md', 'project', 1, '{}'),
			('project/atlas', 'projects/atlas.md', 'project', 1, '{}'),
			('person/alex', 'people/alex.md', 'person', 1, '{}')
	`)
	if err != nil {
		t.Fatalf("failed to seed objects: %v", err)
	}

	_, err = db.DB().Exec(`
		INSERT INTO traits (id, trait_type, value, content, file_path, line_number, parent_object_id) VALUES
			('projects/raven.md:trait:0', 'todo', 'open', 'Task A', 'projects/raven.md', 5, 'project/raven'),
			('projects/atlas.md:trait:0', 'todo', 'done', 'Task B', 'projects/atlas.md', 6, 'project/atlas')
	`)
	if err != nil {
		t.Fatalf("failed to seed traits: %v", err)
	}

	return &Runtime{
		VaultPath: t.TempDir(),
		DB:        db,
	}
}
