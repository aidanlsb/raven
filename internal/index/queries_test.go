package index

import (
	"fmt"
	"testing"

	"github.com/aidanlsb/raven/internal/model"
)

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func TestBuildFTSContentQuery_SanitizesHyphenatedTokens(t *testing.T) {
	t.Parallel()
	q := BuildFTSContentQuery(`michael-truell OR "Michael Truell"`)
	if q != `content: ("michael-truell" OR "Michael Truell")` {
		t.Fatalf("unexpected fts query:\n got: %q\nwant: %q", q, `content: ("michael-truell" OR "Michael Truell")`)
	}
}

func TestBuildFTSContentQuery_SanitizesDottedTokens(t *testing.T) {
	t.Parallel()
	q := BuildFTSContentQuery(`inputs.project OR "optional input"`)
	if q != `content: ("inputs.project" OR "optional input")` {
		t.Fatalf("unexpected fts query:\n got: %q\nwant: %q", q, `content: ("inputs.project" OR "optional input")`)
	}
}

func TestBuildFTSContentQuery_SanitizesSlashAndFunctionLikeTokens(t *testing.T) {
	t.Parallel()

	q := BuildFTSContentQuery(`reference/ OR content()`)
	want := `content: ("reference/" OR "content()")`
	if q != want {
		t.Fatalf("unexpected fts query:\n got: %q\nwant: %q", q, want)
	}
}

func TestBuildFTSSearchQuery_ScopesTitleAndContent(t *testing.T) {
	t.Parallel()
	q := BuildFTSSearchQuery("hello world")
	want := `{title content}: (hello world)`
	if q != want {
		t.Fatalf("unexpected fts query:\n got: %q\nwant: %q", q, want)
	}

	q = BuildFTSSearchQuery(`michael-truell OR "Michael Truell"`)
	want = `{title content}: ("michael-truell" OR "Michael Truell")`
	if q != want {
		t.Fatalf("unexpected fts query:\n got: %q\nwant: %q", q, want)
	}

	q = BuildFTSSearchQuery(`inputs.project OR "optional input"`)
	want = `{title content}: ("inputs.project" OR "optional input")`
	if q != want {
		t.Fatalf("unexpected fts query:\n got: %q\nwant: %q", q, want)
	}

	q = BuildFTSSearchQuery(`reference/ OR content()`)
	want = `{title content}: ("reference/" OR "content()")`
	if q != want {
		t.Fatalf("unexpected fts query:\n got: %q\nwant: %q", q, want)
	}

	q = BuildFTSSearchQuery("")
	want = `{title content}:""`
	if q != want {
		t.Fatalf("unexpected fts query:\n got: %q\nwant: %q", q, want)
	}
}

func TestSearch_AllowsHyphenatedTokenWithOR(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert a minimal object row for SearchWithType joins (and good hygiene).
	_, err = db.db.Exec(`INSERT INTO objects (id, file_path, type, line_start, fields) VALUES ('people/michael-truell', 'people/michael-truell.md', 'person', 1, '{}')`)
	if err != nil {
		t.Fatalf("failed to insert object: %v", err)
	}

	// Index a search row. FTS tokenization will split on '-', which is fine.
	_, err = db.db.Exec(`INSERT INTO fts_content (object_id, title, content, file_path) VALUES (?, ?, ?, ?)`,
		"people/michael-truell",
		"Michael Truell",
		`Meeting with [[michael-truell]]`,
		"daily/2026-01-29.md",
	)
	if err != nil {
		t.Fatalf("failed to insert fts row: %v", err)
	}

	// This used to fail with "no such column: truell" due to FTS parsing.
	results, err := db.Search(`michael-truell OR "Michael Truell"`, 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one result")
	}
}

func TestSearch_AllowsDottedToken(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	_, err = db.db.Exec(`INSERT INTO objects (id, file_path, type, line_start, fields) VALUES ('notes/workflow', 'notes/workflow.md', 'note', 1, '{}')`)
	if err != nil {
		t.Fatalf("failed to insert object: %v", err)
	}

	_, err = db.db.Exec(`INSERT INTO fts_content (object_id, title, content, file_path) VALUES (?, ?, ?, ?)`,
		"notes/workflow",
		"Workflow Notes",
		"Optional interpolation failed for inputs.project in the workflow.",
		"notes/workflow.md",
	)
	if err != nil {
		t.Fatalf("failed to insert fts row: %v", err)
	}

	results, err := db.Search("inputs.project", 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one result")
	}
}

func TestSearch_AllowsSlashAndFunctionLikeTokens(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	_, err = db.db.Exec(`INSERT INTO objects (id, file_path, type, line_start, fields) VALUES ('notes/search', 'notes/search.md', 'note', 1, '{}')`)
	if err != nil {
		t.Fatalf("failed to insert object: %v", err)
	}

	_, err = db.db.Exec(`INSERT INTO fts_content (object_id, title, content, file_path) VALUES (?, ?, ?, ?)`,
		"notes/search",
		"Search Notes",
		"Paths like reference/ and code examples like content() should be searchable.",
		"notes/search.md",
	)
	if err != nil {
		t.Fatalf("failed to insert fts row: %v", err)
	}

	for _, query := range []string{"reference/", "content()"} {
		results, err := db.Search(query, 10)
		if err != nil {
			t.Fatalf("search %q failed: %v", query, err)
		}
		if len(results) == 0 {
			t.Fatalf("expected at least one result for %q", query)
		}
	}
}

func TestSearch_MatchesTitle(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	_, err = db.db.Exec(`INSERT INTO objects (id, file_path, type, line_start, fields) VALUES ('project/raven', 'project/raven.md', 'project', 1, '{}')`)
	if err != nil {
		t.Fatalf("failed to insert object: %v", err)
	}

	// Index with a distinctive title but no matching content
	_, err = db.db.Exec(`INSERT INTO fts_content (object_id, title, content, file_path) VALUES (?, ?, ?, ?)`,
		"project/raven",
		"Raven Knowledge Base",
		"This is a project about structured notes.",
		"project/raven.md",
	)
	if err != nil {
		t.Fatalf("failed to insert fts row: %v", err)
	}

	// Search for a term that only appears in the title
	results, err := db.Search("Knowledge", 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected search to match title, got no results")
	}
	if results[0].ObjectID != "project/raven" {
		t.Fatalf("expected object_id 'project/raven', got %q", results[0].ObjectID)
	}
}

func TestBacklinks(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test objects
	_, err = db.db.Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields)
		VALUES 
			('people/freya', 'people/freya.md', 'person', 1, '{}'),
			('daily/2025-02-01', 'daily/2025-02-01.md', 'date', 1, '{}'),
			('projects/bifrost', 'projects/bifrost.md', 'project', 1, '{}')
	`)
	if err != nil {
		t.Fatalf("failed to insert test objects: %v", err)
	}

	// Insert test refs
	_, err = db.db.Exec(`
		INSERT INTO refs (source_id, target_id, target_raw, file_path, line_number, position_start, position_end)
		VALUES 
			('daily/2025-02-01', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 5, 4, 20),
			('projects/bifrost', 'people/freya', 'freya', 'projects/bifrost.md', 10, NULL, NULL),
			('projects/bifrost', 'people/freya#notes', 'freya#notes', 'projects/bifrost.md', 11, 0, 15)
	`)
	if err != nil {
		t.Fatalf("failed to insert test refs: %v", err)
	}

	t.Run("find backlinks to person", func(t *testing.T) {
		results, err := db.Backlinks("people/freya")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		// Includes a section backlink via target_id LIKE 'people/freya#%'.
		if len(results) != 3 {
			t.Errorf("expected 3 backlinks, got %d", len(results))
		}
	})

	t.Run("backlinks include column positions when stored", func(t *testing.T) {
		results, err := db.Backlinks("people/freya")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		byPath := map[string]model.Reference{}
		for _, ref := range results {
			byPath[fmt.Sprintf("%s:%d", ref.FilePath, derefInt(ref.Line))] = ref
		}

		withPos, ok := byPath["daily/2025-02-01.md:5"]
		if !ok {
			t.Fatal("missing expected backlink daily/2025-02-01.md:5")
		}
		if withPos.PositionStart == nil || withPos.PositionEnd == nil {
			t.Fatalf("expected position data, got start=%v end=%v", withPos.PositionStart, withPos.PositionEnd)
		}
		if *withPos.PositionStart != 4 || *withPos.PositionEnd != 20 {
			t.Errorf("positions = [%d, %d), want [4, 20)", *withPos.PositionStart, *withPos.PositionEnd)
		}

		withoutPos, ok := byPath["projects/bifrost.md:10"]
		if !ok {
			t.Fatal("missing expected backlink projects/bifrost.md:10")
		}
		if withoutPos.PositionStart != nil || withoutPos.PositionEnd != nil {
			t.Errorf("expected nil positions, got start=%v end=%v", withoutPos.PositionStart, withoutPos.PositionEnd)
		}
	})

	t.Run("no backlinks", func(t *testing.T) {
		results, err := db.Backlinks("projects/bifrost")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 backlinks, got %d", len(results))
		}
	})
}

func TestOutlinks(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test objects
	_, err = db.db.Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields)
		VALUES
			('people/freya', 'people/freya.md', 'person', 1, '{}'),
			('daily/2025-02-01', 'daily/2025-02-01.md', 'date', 1, '{}'),
			('projects/bifrost', 'projects/bifrost.md', 'project', 1, '{}')
	`)
	if err != nil {
		t.Fatalf("failed to insert test objects: %v", err)
	}

	// Insert test refs (including a section outlink via source_id LIKE 'projects/bifrost#%').
	_, err = db.db.Exec(`
		INSERT INTO refs (source_id, target_id, target_raw, file_path, line_number, position_start)
		VALUES
			('daily/2025-02-01', 'people/freya', 'people/freya', 'daily/2025-02-01.md', 5, 1),
			('projects/bifrost', 'people/freya', 'freya', 'projects/bifrost.md', 10, 1),
			('projects/bifrost#notes', 'people/freya', 'freya', 'projects/bifrost.md', 11, 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert test refs: %v", err)
	}

	t.Run("find outlinks from object (includes section outlinks)", func(t *testing.T) {
		results, err := db.Outlinks("projects/bifrost")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 outlinks, got %d", len(results))
		}
	})

	t.Run("find outlinks from daily note", func(t *testing.T) {
		results, err := db.Outlinks("daily/2025-02-01")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 outlink, got %d", len(results))
		}
	})
}

func TestGetObject(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test object
	_, err = db.db.Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields)
		VALUES ('people/freya', 'people/freya.md', 'person', 1, '{"name": "Freya"}')
	`)
	if err != nil {
		t.Fatalf("failed to insert test object: %v", err)
	}

	t.Run("find existing object", func(t *testing.T) {
		obj, err := db.GetObject("people/freya")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if obj == nil {
			t.Fatal("expected object, got nil")
		}
		if obj.Type != "person" {
			t.Errorf("expected type 'person', got '%s'", obj.Type)
		}
	})

	t.Run("object not found", func(t *testing.T) {
		obj, err := db.GetObject("people/nonexistent")
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if obj != nil {
			t.Errorf("expected nil for nonexistent object, got %v", obj)
		}
	})
}

func TestUntypedPages(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test objects
	_, err = db.db.Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields)
		VALUES 
			('notes/random', 'notes/random.md', 'page', 1, '{}'),
			('people/freya', 'people/freya.md', 'person', 1, '{}'),
			('notes/another', 'notes/another.md', 'page', 1, '{}')
	`)
	if err != nil {
		t.Fatalf("failed to insert test objects: %v", err)
	}

	t.Run("find untyped pages", func(t *testing.T) {
		results, err := db.UntypedPages()
		if err != nil {
			t.Fatalf("query failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 untyped pages, got %d", len(results))
		}
	})
}

func TestAllObjects(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test objects
	_, err = db.db.Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields)
		VALUES 
			('people/freya', 'people/freya.md', 'person', 1, '{"name": "Freya"}'),
			('people/thor', 'people/thor.md', 'person', 1, '{"name": "Thor"}'),
			('projects/bifrost', 'projects/bifrost.md', 'project', 1, '{}')
	`)
	if err != nil {
		t.Fatalf("failed to insert test objects: %v", err)
	}

	results, err := db.AllObjects()
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 objects, got %d", len(results))
	}
	if results[0].ID != "people/freya" {
		t.Fatalf("first object ID = %q, want people/freya", results[0].ID)
	}
	if got, ok := results[0].Fields["name"].AsString(); !ok || got != "Freya" {
		t.Fatalf("first object name = %#v, want Freya", results[0].Fields["name"])
	}
}

func TestAllSections(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	_, err = db.db.Exec(`
		INSERT INTO sections (id, file_object_id, file_path, slug, title, level, line_start, line_end, subtree_line_end, parent_section_id)
		VALUES
			('notes/alpha#overview', 'notes/alpha', 'notes/alpha.md', 'overview', 'Overview', 1, 1, 4, 8, NULL),
			('notes/alpha#details', 'notes/alpha', 'notes/alpha.md', 'details', 'Details', 2, 5, NULL, NULL, 'notes/alpha#overview')
	`)
	if err != nil {
		t.Fatalf("failed to insert test sections: %v", err)
	}

	results, err := db.AllSections()
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(results))
	}
	if results[0].ID != "notes/alpha#overview" {
		t.Fatalf("first section ID = %q, want notes/alpha#overview", results[0].ID)
	}
	if results[1].ParentSectionID == nil || *results[1].ParentSectionID != "notes/alpha#overview" {
		t.Fatalf("parent section ID = %#v, want notes/alpha#overview", results[1].ParentSectionID)
	}
}
