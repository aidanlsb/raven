package query

import (
	"strings"
	"testing"
)

func TestBuildContentPredicateSQLUsesListSubquery(t *testing.T) {
	t.Parallel()
	exec := &Executor{}
	pred := &ContentPredicate{SearchTerm: "type:project"}
	cond, args, err := exec.buildContentPredicateSQL(pred, "s")
	if err != nil {
		t.Fatalf("buildContentPredicateSQL: %v", err)
	}
	if strings.Contains(cond, "fts_content.object_id =") {
		t.Fatalf("correlated object_id predicate still present: %s", cond)
	}
	if !strings.Contains(cond, "s.id IN (") || !strings.Contains(cond, "SELECT object_id FROM fts_content") {
		t.Fatalf("expected list subquery, got %s", cond)
	}
	if len(args) != 1 {
		t.Fatalf("args = %#v", args)
	}
	ftsQuery, ok := args[0].(string)
	if !ok || !strings.Contains(ftsQuery, `"type:project"`) {
		t.Fatalf("FTS query = %#v, want quoted type:project", args[0])
	}
}

func TestContentPredicatePunctuationDoesNotError(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('notes/search', 'notes/search.md', 'note', '{}', 1);
		INSERT INTO fts_content (object_id, title, content, file_path) VALUES
			('notes/search', 'Search Notes', 'See type:project and https://github.com and -todo and foo (bar) and @todo', 'notes/search.md');
	`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	exec := NewExecutor(db)
	queries := []string{
		`type:note content("type:project")`,
		`type:note content("https://github.com")`,
		`type:note content("-todo")`,
		`type:note content("foo (bar)")`,
		`type:note content("@todo")`,
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			parsed, parseErr := Parse(q)
			if parseErr != nil {
				t.Fatalf("parse: %v", parseErr)
			}
			run, runErr := exec.Run(parsed, RunRequest{})
			if runErr != nil {
				t.Fatalf("run: %v", runErr)
			}
			if len(run.Objects) != 1 || run.Objects[0].ID != "notes/search" {
				t.Fatalf("result = %#v, want notes/search", run.Objects)
			}
		})
	}
}

func TestWithinTypeDoesNotUseRecursiveCTE(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()
	exec := NewExecutor(db)

	pred := &WithinPredicate{SubQuery: &Query{Type: QueryTypeObject, TypeName: "project"}}
	cond, _, err := exec.buildWithinPredicateSQL(pred, "s", QueryTypeSection)
	if err != nil {
		t.Fatalf("buildWithinPredicateSQL: %v", err)
	}
	if strings.Contains(strings.ToUpper(cond), "RECURSIVE") {
		t.Fatalf("within(type:...) still uses a recursive CTE: %s", cond)
	}
	if !strings.Contains(cond, "file_object_id") {
		t.Fatalf("expected direct file_object_id join, got %s", cond)
	}

	sectionPred := &WithinPredicate{SubQuery: &Query{Type: QueryTypeSection}}
	sectionCond, _, err := exec.buildWithinPredicateSQL(sectionPred, "s", QueryTypeSection)
	if err != nil {
		t.Fatalf("section within: %v", err)
	}
	if !strings.Contains(strings.ToUpper(sectionCond), "RECURSIVE") {
		t.Fatalf("within(section ...) should stay recursive, got %s", sectionCond)
	}
}

func TestLinkWithinTypeDoesNotWalkAncestors(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()
	exec := NewExecutor(db)

	pred := &WithinPredicate{SubQuery: &Query{Type: QueryTypeObject, TypeName: "project"}}
	cond, _, err := exec.buildLinkWithinPredicateSQL(pred, "l")
	if err != nil {
		t.Fatalf("buildLinkWithinPredicateSQL: %v", err)
	}
	if strings.Contains(cond, "link_scopes") {
		t.Fatalf("within(type:...) on links still uses link_scopes: %s", cond)
	}
	if !strings.Contains(cond, "file_path") {
		t.Fatalf("expected file_path object scope, got %s", cond)
	}
}
