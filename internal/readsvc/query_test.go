package readsvc

import (
	"errors"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/query"
)

func TestExecuteQuery_InvalidInput(t *testing.T) {
	t.Parallel()
	_, err := ExecuteQuery(nil, ExecuteQueryRequest{QueryString: "type:project"})
	if err == nil {
		t.Fatalf("expected error for nil runtime")
	}

	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rt := &Runtime{DB: db}

	_, err = ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "type:project", Limit: -1})
	if err == nil || err.Error() != "limit must be >= 0" {
		t.Fatalf("expected limit validation error, got: %v", err)
	}

	_, err = ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "type:project", Offset: -1})
	if err == nil || err.Error() != "offset must be >= 0" {
		t.Fatalf("expected offset validation error, got: %v", err)
	}
}

func TestExecuteQuery_ObjectModes(t *testing.T) {
	t.Parallel()
	rt := seededRuntime(t)

	result, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "type:project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.QueryKind != "type" || result.TypeName != "project" {
		t.Fatalf("unexpected query metadata: %#v", result)
	}
	if result.Total != 2 || len(result.Objects) != 2 || result.Returned != 2 {
		t.Fatalf("unexpected object results: %#v", result)
	}

	idsOnly, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "type:project", IDsOnly: true, Limit: 1})
	if err != nil {
		t.Fatalf("unexpected IDsOnly error: %v", err)
	}
	if len(idsOnly.IDs) != 1 || idsOnly.Returned != 1 {
		t.Fatalf("unexpected IDsOnly result: %#v", idsOnly)
	}
	if idsOnly.Total != 2 {
		t.Fatalf("expected total 2, got %d", idsOnly.Total)
	}

	countOnly, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "type:project", CountOnly: true})
	if err != nil {
		t.Fatalf("unexpected CountOnly error: %v", err)
	}
	if countOnly.Total != 2 || countOnly.Returned != 0 {
		t.Fatalf("unexpected CountOnly result: %#v", countOnly)
	}
	if len(countOnly.Objects) != 0 || len(countOnly.IDs) != 0 {
		t.Fatalf("count-only should not include rows or ids: %#v", countOnly)
	}

	paged, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "type:project", Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("unexpected paged type query error: %v", err)
	}
	if paged.Total != 2 || paged.Returned != 1 || len(paged.Objects) != 1 {
		t.Fatalf("unexpected paged object result: %#v", paged)
	}
	if paged.Objects[0].ID != "project/raven" {
		t.Fatalf("unexpected paged object ID: %#v", paged.Objects[0])
	}
}

func TestExecuteQueryResult_HasMoreAndNextOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		result         *ExecuteQueryResult
		wantHasMore    bool
		wantNextOffset int
	}{
		{
			name:           "nil result",
			result:         nil,
			wantHasMore:    false,
			wantNextOffset: 0,
		},
		{
			name:           "unlimited full result set",
			result:         &ExecuteQueryResult{Total: 5, Returned: 5, Offset: 0, Limit: 0},
			wantHasMore:    false,
			wantNextOffset: 5,
		},
		{
			name:           "first page with more",
			result:         &ExecuteQueryResult{Total: 120, Returned: 50, Offset: 0, Limit: 50},
			wantHasMore:    true,
			wantNextOffset: 50,
		},
		{
			name:           "middle page with more",
			result:         &ExecuteQueryResult{Total: 120, Returned: 50, Offset: 50, Limit: 50},
			wantHasMore:    true,
			wantNextOffset: 100,
		},
		{
			name:           "last partial page",
			result:         &ExecuteQueryResult{Total: 120, Returned: 20, Offset: 100, Limit: 50},
			wantHasMore:    false,
			wantNextOffset: 120,
		},
		{
			name:           "offset past end",
			result:         &ExecuteQueryResult{Total: 120, Returned: 0, Offset: 200, Limit: 50},
			wantHasMore:    false,
			wantNextOffset: 200,
		},
		{
			name:           "exact last page",
			result:         &ExecuteQueryResult{Total: 100, Returned: 50, Offset: 50, Limit: 50},
			wantHasMore:    false,
			wantNextOffset: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.HasMore(); got != tt.wantHasMore {
				t.Errorf("HasMore() = %v, want %v", got, tt.wantHasMore)
			}
			if got := tt.result.NextOffset(); got != tt.wantNextOffset {
				t.Errorf("NextOffset() = %d, want %d", got, tt.wantNextOffset)
			}
		})
	}
}

func TestExecuteQuery_TraitModes(t *testing.T) {
	t.Parallel()
	rt := seededRuntime(t)

	result, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "trait:todo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.QueryKind != "trait" || result.TypeName != "todo" {
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

func TestExecuteQuery_AssetModes(t *testing.T) {
	t.Parallel()
	rt := seededRuntime(t)

	result, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "asset"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.QueryKind != "asset" {
		t.Fatalf("unexpected query kind: %#v", result)
	}
	if result.Total != 2 || len(result.Assets) != 2 || result.Returned != 2 {
		t.Fatalf("unexpected asset results: %#v", result)
	}

	idsOnly, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "asset", IDsOnly: true, Limit: 1})
	if err != nil {
		t.Fatalf("unexpected IDsOnly error: %v", err)
	}
	if len(idsOnly.IDs) != 1 || idsOnly.Returned != 1 || idsOnly.Total != 2 {
		t.Fatalf("unexpected asset IDsOnly result: %#v", idsOnly)
	}
	if len(idsOnly.Assets) != 0 {
		t.Fatalf("ids-only should not include rows: %#v", idsOnly)
	}

	countOnly, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "asset", CountOnly: true})
	if err != nil {
		t.Fatalf("unexpected CountOnly error: %v", err)
	}
	if countOnly.Total != 2 || countOnly.Returned != 0 || len(countOnly.Assets) != 0 {
		t.Fatalf("unexpected asset CountOnly result: %#v", countOnly)
	}

	paged, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "asset", Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("unexpected paged asset query error: %v", err)
	}
	if paged.Total != 2 || paged.Returned != 1 || len(paged.Assets) != 1 {
		t.Fatalf("unexpected paged asset result: %#v", paged)
	}
	if paged.Assets[0].ID != "assets/paper.pdf" {
		t.Fatalf("unexpected paged asset ID: %#v", paged.Assets[0])
	}
}

func TestExecuteQuery_SectionModes(t *testing.T) {
	t.Parallel()
	rt := seededRuntime(t)

	result, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "section"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.QueryKind != "section" {
		t.Fatalf("unexpected query kind: %#v", result)
	}
	if result.Total != 3 || len(result.Sections) != 3 || result.Returned != 3 {
		t.Fatalf("unexpected section results: %#v", result)
	}

	idsOnly, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "section", IDsOnly: true, Limit: 2})
	if err != nil {
		t.Fatalf("unexpected IDsOnly error: %v", err)
	}
	if len(idsOnly.IDs) != 2 || idsOnly.Returned != 2 || idsOnly.Total != 3 {
		t.Fatalf("unexpected section IDsOnly result: %#v", idsOnly)
	}

	countOnly, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "section", CountOnly: true})
	if err != nil {
		t.Fatalf("unexpected CountOnly error: %v", err)
	}
	if countOnly.Total != 3 || countOnly.Returned != 0 || len(countOnly.Sections) != 0 {
		t.Fatalf("unexpected section CountOnly result: %#v", countOnly)
	}

	paged, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "section", Limit: 1})
	if err != nil {
		t.Fatalf("unexpected paged section query error: %v", err)
	}
	if paged.Total != 3 || paged.Returned != 1 || len(paged.Sections) != 1 {
		t.Fatalf("unexpected paged section result: %#v", paged)
	}
}

func TestExecuteQuery_LinkModes(t *testing.T) {
	t.Parallel()
	rt := seededRuntime(t)

	result, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "link .ext==pdf"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.QueryKind != "link" || result.Total != 1 || result.Returned != 1 || len(result.Links) != 1 {
		t.Fatalf("unexpected link results: %#v", result)
	}
	if result.Links[0].SourceID != "project/raven" || result.Links[0].RawTarget != "docs/spec.pdf" {
		t.Fatalf("unexpected link row: %#v", result.Links[0])
	}

	idsOnly, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "link", IDsOnly: true, Limit: 1})
	if err != nil {
		t.Fatalf("unexpected IDsOnly error: %v", err)
	}
	if len(idsOnly.IDs) != 1 || idsOnly.IDs[0] != "project/atlas" || idsOnly.Total != 2 {
		t.Fatalf("unexpected link IDsOnly result: %#v", idsOnly)
	}

	countOnly, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "link", CountOnly: true})
	if err != nil {
		t.Fatalf("unexpected CountOnly error: %v", err)
	}
	if countOnly.Total != 2 || countOnly.Returned != 0 || len(countOnly.Links) != 0 {
		t.Fatalf("unexpected link CountOnly result: %#v", countOnly)
	}

	paged, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "link", Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("unexpected paged link query error: %v", err)
	}
	if paged.Total != 2 || paged.Returned != 1 || len(paged.Links) != 1 {
		t.Fatalf("unexpected paged link result: %#v", paged)
	}
	if paged.Links[0].SourceID != "project/raven" {
		t.Fatalf("unexpected paged link source: %#v", paged.Links[0])
	}
}

// TestExecuteQuery_StructuralValidationWithoutSchema verifies that structural
// (root/predicate) validation runs even when the runtime has no schema, while
// structurally valid queries still execute. This is the intended hardening from
// making validation mandatory.
func TestExecuteQuery_StructuralValidationWithoutSchema(t *testing.T) {
	t.Parallel()
	rt := seededRuntime(t)
	if rt.Schema != nil {
		t.Fatalf("expected seeded runtime to have no schema")
	}

	illegal := []struct {
		name    string
		query   string
		wantMsg string
	}{
		{
			name:    "in on object root",
			query:   "type:project in(type:project)",
			wantMsg: "in() predicate is only valid for trait and section queries",
		},
		{
			name:    "content on asset root",
			query:   `asset content("diagram")`,
			wantMsg: "content() predicate is not valid for asset queries",
		},
		{
			name:    "unknown asset field",
			query:   "asset .bogus==1",
			wantMsg: "asset has no field 'bogus'",
		},
		{
			name:    "refd on trait root",
			query:   "trait:todo refd([[project/raven]])",
			wantMsg: "refd() predicate is only valid for type queries",
		},
	}

	for _, tt := range illegal {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: tt.query})
			if err == nil {
				t.Fatalf("expected validation error for %q", tt.query)
			}
			var ve *query.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *query.ValidationError, got %T: %v", err, err)
			}
			if !strings.Contains(ve.Message, tt.wantMsg) {
				t.Fatalf("message = %q, want substring %q", ve.Message, tt.wantMsg)
			}
		})
	}

	// Structurally valid queries still execute without a schema.
	if _, err := ExecuteQuery(rt, ExecuteQueryRequest{QueryString: "type:project has(trait:todo)"}); err != nil {
		t.Fatalf("valid query unexpectedly failed without schema: %v", err)
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
		QueryString: "type:note refs([[raven]])",
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

func TestExecuteQuery_AmbiguousISODateRefReturnsError(t *testing.T) {
	t.Parallel()
	rt := seededRuntime(t)

	_, err := rt.DB.DB().Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields) VALUES
			('daily/2025-02-01', 'daily/2025-02-01.md', 'page', 1, '{}'),
			('2025-02-01', '2025-02-01.md', 'page', 1, '{}')
	`)
	if err != nil {
		t.Fatalf("failed to seed ISO date collision objects: %v", err)
	}

	_, err = ExecuteQuery(rt, ExecuteQueryRequest{
		QueryString: "type:project refs([[2025-02-01]])",
	})
	if err == nil {
		t.Fatal("expected ambiguous ISO date reference error")
	}
	if !strings.Contains(err.Error(), "ambiguous reference '2025-02-01'") {
		t.Fatalf("unexpected error: %v", err)
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

	_, err = db.DB().Exec(`
		INSERT INTO assets (id, file_path, media_type, extension, filename, size_bytes) VALUES
			('assets/diagram.png', 'assets/diagram.png', 'image/png', 'png', 'diagram.png', 2048),
			('assets/paper.pdf', 'assets/paper.pdf', 'application/pdf', 'pdf', 'paper.pdf', 4096)
	`)
	if err != nil {
		t.Fatalf("failed to seed assets: %v", err)
	}

	_, err = db.DB().Exec(`
		INSERT INTO sections (id, file_object_id, file_path, slug, title, level, line_start) VALUES
			('projects/raven.md#intro', 'project/raven', 'projects/raven.md', 'intro', 'Intro', 2, 3),
			('projects/raven.md#tasks', 'project/raven', 'projects/raven.md', 'tasks', 'Tasks', 2, 10),
			('projects/atlas.md#intro', 'project/atlas', 'projects/atlas.md', 'intro', 'Intro', 2, 3)
	`)
	if err != nil {
		t.Fatalf("failed to seed sections: %v", err)
	}

	_, err = db.DB().Exec(`
		INSERT INTO links (
			source_id, source_type, file_path, line_number, position_start, position_end,
			raw_target, display, is_image, scheme, ext, normalized_key
		) VALUES
			('project/raven', 'project', 'projects/raven.md', 5, 0, 21, 'docs/spec.pdf', 'Spec', 0, 'file', 'pdf', 'projects/docs/spec.pdf'),
			('project/atlas', 'project', 'projects/atlas.md', 6, 0, 28, 'https://example.com', 'Example', 0, 'url', '', 'https://example.com')
	`)
	if err != nil {
		t.Fatalf("failed to seed links: %v", err)
	}

	return &Runtime{
		VaultPath: t.TempDir(),
		DB:        db,
	}
}
