package query

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func sortTestSchema() *schema.Schema {
	return &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"status":   {Type: schema.FieldTypeEnum},
					"priority": {Type: schema.FieldTypeString},
					"title":    {Type: schema.FieldTypeString},
				},
			},
			"person": {
				Fields: map[string]*schema.FieldDefinition{
					"name":  {Type: schema.FieldTypeString},
					"email": {Type: schema.FieldTypeString},
				},
			},
			"task": {
				Fields: map[string]*schema.FieldDefinition{
					"title":    {Type: schema.FieldTypeString},
					"priority": {Type: schema.FieldTypeNumber},
					"status":   {Type: schema.FieldTypeString},
				},
			},
			"date": {Fields: map[string]*schema.FieldDefinition{}},
		},
		Traits: map[string]*schema.TraitDefinition{
			"due":       {Type: schema.FieldTypeDate},
			"highlight": {Type: schema.FieldTypeBool},
			"score":     {Type: schema.FieldTypeNumber},
		},
	}
}

func runSorted(t *testing.T, db *sql.DB, queryStr string, req RunRequest) *RunResult {
	t.Helper()
	q, err := Parse(queryStr)
	if err != nil {
		t.Fatalf("Parse(%q): %v", queryStr, err)
	}
	if err := NewValidator(sortTestSchema()).Validate(q); err != nil {
		t.Fatalf("Validate(%q): %v", queryStr, err)
	}
	exec := NewExecutor(db)
	exec.SetSchema(sortTestSchema())
	result, err := exec.Run(q, req)
	if err != nil {
		t.Fatalf("Run(%q): %v", queryStr, err)
	}
	return result
}

func objectIDs(result *RunResult) []string {
	ids := make([]string, len(result.Objects))
	for i, obj := range result.Objects {
		ids[i] = obj.ID
	}
	return ids
}

func TestRunSort_ObjectStringAndMissing(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	t.Run("status asc", func(t *testing.T) {
		got := objectIDs(runSorted(t, db, "type:project sort(.status)", RunRequest{}))
		want := []string{"projects/website", "projects/mobile"} // active, paused
		if !equalStrings(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("status desc", func(t *testing.T) {
		got := objectIDs(runSorted(t, db, "type:project sort(.status, desc)", RunRequest{}))
		want := []string{"projects/mobile", "projects/website"}
		if !equalStrings(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("missing email nulls last asc", func(t *testing.T) {
		got := objectIDs(runSorted(t, db, "type:person sort(.email, asc)", RunRequest{}))
		if len(got) != 2 {
			t.Fatalf("got %v", got)
		}
		if got[0] != "people/freya" {
			t.Fatalf("present value should sort first, got %v", got)
		}
		if got[1] != "people/loki" {
			t.Fatalf("missing email should sort last, got %v", got)
		}
	})

	t.Run("missing email nulls last desc", func(t *testing.T) {
		got := objectIDs(runSorted(t, db, "type:person sort(.email, desc)", RunRequest{}))
		if got[len(got)-1] != "people/loki" {
			t.Fatalf("missing email should sort last in desc too, got %v", got)
		}
	})

	t.Run("name case-insensitive", func(t *testing.T) {
		got := objectIDs(runSorted(t, db, "type:person sort(.name, asc)", RunRequest{}))
		want := []string{"people/freya", "people/loki"}
		if !equalStrings(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
}

func TestRunSort_NumericVsString(t *testing.T) {
	t.Parallel()
	db := setupSortNumericDB(t)
	defer db.Close()

	got := objectIDs(runSorted(t, db, "type:task sort(.priority, asc)", RunRequest{}))
	want := []string{"tasks/low", "tasks/mid", "tasks/high"} // 1, 2, 10 not 1, 10, 2
	if !equalStrings(got, want) {
		t.Fatalf("numeric order = %v, want %v", got, want)
	}

	got = objectIDs(runSorted(t, db, "type:task sort(.priority, desc)", RunRequest{}))
	want = []string{"tasks/high", "tasks/mid", "tasks/low"}
	if !equalStrings(got, want) {
		t.Fatalf("numeric desc = %v, want %v", got, want)
	}
}

func TestRunSort_MultiFieldAndPagination(t *testing.T) {
	t.Parallel()
	db := setupSortNumericDB(t)
	defer db.Close()

	got := objectIDs(runSorted(t, db, "type:task sort(.status, asc) sort(.title, desc)", RunRequest{}))
	// open: zebra, alpha; done: mid
	want := []string{"tasks/mid", "tasks/high", "tasks/low"}
	if !equalStrings(got, want) {
		t.Fatalf("multi-field = %v, want %v", got, want)
	}

	page := runSorted(t, db, "type:task sort(.priority, asc)", RunRequest{Limit: 1, Offset: 1})
	if len(page.Objects) != 1 || page.Objects[0].ID != "tasks/mid" {
		t.Fatalf("page offset 1 = %#v, want tasks/mid", objectIDs(page))
	}

	ids := runSorted(t, db, "type:task sort(.priority, desc)", RunRequest{IDsOnly: true, Limit: 2})
	if !equalStrings(ids.IDs, []string{"tasks/high", "tasks/mid"}) {
		t.Fatalf("ids page = %v", ids.IDs)
	}
}

func TestRunSort_MissingNumberNullsLast(t *testing.T) {
	t.Parallel()
	db := setupSortNumericDB(t)
	defer db.Close()
	if _, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('tasks/none', 'nn-tasks/none.md', 'task', '{"title":"none","status":"open"}', 1);
	`); err != nil {
		t.Fatalf("insert missing-priority task: %v", err)
	}

	got := objectIDs(runSorted(t, db, "type:task sort(.priority, asc)", RunRequest{}))
	if got[len(got)-1] != "tasks/none" {
		t.Fatalf("missing number should sort last in asc, got %v", got)
	}
	got = objectIDs(runSorted(t, db, "type:task sort(.priority, desc)", RunRequest{}))
	if got[len(got)-1] != "tasks/none" {
		t.Fatalf("missing number should sort last in desc, got %v", got)
	}
}

func TestRunSort_DateVirtualField(t *testing.T) {
	t.Parallel()
	db := setupSortDateDB(t)
	defer db.Close()

	got := objectIDs(runSorted(t, db, "type:date sort(.date, asc)", RunRequest{}))
	want := []string{"daily/2025-01-15", "daily/2025-02-01", "daily/2025-03-01"}
	if !equalStrings(got, want) {
		t.Fatalf("date asc = %v, want %v", got, want)
	}

	got = objectIDs(runSorted(t, db, "type:date sort(.date, desc)", RunRequest{}))
	want = []string{"daily/2025-03-01", "daily/2025-02-01", "daily/2025-01-15"}
	if !equalStrings(got, want) {
		t.Fatalf("date desc = %v, want %v", got, want)
	}
}

func TestRunSort_SectionTraitLink(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	t.Run("section title nocase", func(t *testing.T) {
		q := "section within(type:project) sort(.title, asc)"
		result := runSorted(t, db, q, RunRequest{})
		if len(result.Sections) == 0 {
			t.Fatal("expected sections")
		}
		for i := 1; i < len(result.Sections); i++ {
			prev := strings.ToLower(result.Sections[i-1].Title)
			cur := strings.ToLower(result.Sections[i].Title)
			if prev > cur {
				t.Fatalf("section titles not sorted: %q then %q", result.Sections[i-1].Title, result.Sections[i].Title)
			}
		}
	})

	t.Run("section numeric level desc", func(t *testing.T) {
		result := runSorted(t, db, "section sort(.level, desc)", RunRequest{Limit: 1})
		if len(result.Sections) != 1 || result.Sections[0].Level != 4 {
			t.Fatalf("expected highest level first, got %#v", result.Sections)
		}
	})

	t.Run("trait value nulls last", func(t *testing.T) {
		result := runSorted(t, db, "trait:highlight sort(.value, asc)", RunRequest{})
		if len(result.Traits) != 1 {
			t.Fatalf("highlight count = %d", len(result.Traits))
		}
		result = runSorted(t, db, "trait:due sort(.value, desc)", RunRequest{})
		if len(result.Traits) < 2 {
			t.Fatalf("due count = %d", len(result.Traits))
		}
		// Highest date first: 2025-06-30 (trait1)
		if result.Traits[0].ID != "trait1" {
			t.Fatalf("due desc first = %s", result.Traits[0].ID)
		}
	})

	t.Run("link ext nocase", func(t *testing.T) {
		result := runSorted(t, db, "link sort(.ext, asc) sort(.file_path, asc)", RunRequest{})
		if len(result.Links) == 0 {
			t.Fatal("expected links")
		}
		for i := 1; i < len(result.Links); i++ {
			prev := strings.ToLower(result.Links[i-1].Ext)
			cur := strings.ToLower(result.Links[i].Ext)
			if prev > cur {
				t.Fatalf("link ext not sorted: %q then %q", result.Links[i-1].Ext, result.Links[i].Ext)
			}
		}
	})

	t.Run("link numeric line", func(t *testing.T) {
		result := runSorted(t, db, "link within(type:project) sort(.line, asc)", RunRequest{})
		for i := 1; i < len(result.Links); i++ {
			if result.Links[i-1].Line > result.Links[i].Line {
				t.Fatalf("link lines not sorted: %d then %d", result.Links[i-1].Line, result.Links[i].Line)
			}
		}
	})
}

func TestBuildEntityPageSQL_SortArgsFollowWhereThenLimit(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(nil)
	exec.SetSchema(sortTestSchema())
	q, err := Parse("type:project .status==active sort(.title, desc)")
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	sqlStr, args, err := exec.buildEntityPageSQL(q, objectSpec, 10, 5)
	if err != nil {
		t.Fatalf("buildEntityPageSQL: %v", err)
	}
	if !strings.Contains(sqlStr, "NULLS LAST") {
		t.Fatalf("ORDER BY missing NULLS LAST: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "COLLATE NOCASE") {
		t.Fatalf("string sort missing COLLATE NOCASE: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "DESC NULLS LAST") {
		t.Fatalf("desc missing NULLS LAST: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, objectSpec.orderBy) {
		t.Fatalf("missing deterministic tie-breaker %q in %s", objectSpec.orderBy, sqlStr)
	}
	// WHERE type + equality (json path, value), ORDER BY json path, LIMIT, OFFSET
	if len(args) != 6 {
		t.Fatalf("args = %#v, want 6 (type, where path, where value, sort path, limit, offset)", args)
	}
	if args[0] != "project" {
		t.Fatalf("args[0] = %#v, want type name", args[0])
	}
	if args[1] != "$.status" || args[2] != "active" {
		t.Fatalf("where args = %#v, want json path + status", args[1:3])
	}
	if args[3] != "$.title" {
		t.Fatalf("args[3] = %#v, want sort json path", args[3])
	}
	if args[4] != 10 || args[5] != 5 {
		t.Fatalf("limit/offset args = %#v", args[4:])
	}
}

func TestBuildEntityPageSQL_NumericCast(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(nil)
	exec.SetSchema(sortTestSchema())
	q, err := Parse("type:task sort(.priority)")
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	sqlStr, args, err := exec.buildEntityPageSQL(q, objectSpec, 0, 0)
	if err != nil {
		t.Fatalf("buildEntityPageSQL: %v", err)
	}
	if !strings.Contains(sqlStr, "CAST(json_extract(o.fields, ?) AS REAL)") {
		t.Fatalf("numeric sort missing CAST: %s", sqlStr)
	}
	if len(args) != 2 || args[0] != "task" || args[1] != "$.priority" {
		t.Fatalf("args = %#v", args)
	}
}

func TestBuildEntityIDSQL_IncludesSort(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(nil)
	q, err := Parse("section sort(.title, desc)")
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	sqlStr, args, err := exec.buildEntityIDSQL(q, sectionSpec, 3, 1)
	if err != nil {
		t.Fatalf("buildEntityIDSQL: %v", err)
	}
	if !strings.Contains(sqlStr, "s.title COLLATE NOCASE DESC NULLS LAST") {
		t.Fatalf("section string sort SQL = %s", sqlStr)
	}
	if !strings.Contains(sqlStr, sectionSpec.orderBy) {
		t.Fatalf("missing tie-breaker in %s", sqlStr)
	}
	if len(args) != 2 || args[0] != 3 || args[1] != 1 {
		t.Fatalf("limit/offset args = %#v", args)
	}
}

func TestBuildEntityPageSQL_TraitNumericAndEmptyNulls(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(nil)
	exec.SetSchema(sortTestSchema())
	q, err := Parse("trait:score sort(.value, desc)")
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	sqlStr, _, err := exec.buildEntityPageSQL(q, traitSpec, 0, 0)
	if err != nil {
		t.Fatalf("buildEntityPageSQL: %v", err)
	}
	if !strings.Contains(sqlStr, "CAST(t.value AS REAL)") {
		t.Fatalf("numeric trait sort missing CAST: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "DESC NULLS LAST") {
		t.Fatalf("trait sort missing NULLS LAST: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, traitSpec.orderBy) {
		t.Fatalf("missing trait tie-breaker in %s", sqlStr)
	}
}

func TestBuildEntityCountSQL_OmitsOrderBy(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(nil)
	q, err := Parse("type:project sort(.title)")
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	sqlStr, _, err := exec.buildEntityCountSQL(q, objectSpec)
	if err != nil {
		t.Fatalf("buildEntityCountSQL: %v", err)
	}
	if strings.Contains(sqlStr, "ORDER BY") {
		t.Fatalf("count SQL should not ORDER BY: %s", sqlStr)
	}
}

func TestBuildEntityPageSQL_UnspecifiedOrderByUnchanged(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(nil)
	q, err := Parse("type:project")
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	sqlStr, _, err := exec.buildEntityPageSQL(q, objectSpec, 0, 0)
	if err != nil {
		t.Fatalf("buildEntityPageSQL: %v", err)
	}
	if !strings.Contains(sqlStr, "ORDER BY "+objectSpec.orderBy) {
		t.Fatalf("default ORDER BY missing: %s", sqlStr)
	}
	if strings.Contains(sqlStr, "NULLS LAST") {
		t.Fatalf("default ORDER BY should be unchanged: %s", sqlStr)
	}
}

func setupSortNumericDB(t *testing.T) *sql.DB {
	t.Helper()
	db := setupTestDB(t)
	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('tasks/high', 'zz-tasks/high.md', 'task', '{"title":"zebra","priority":10,"status":"open"}', 1),
			('tasks/low', 'aa-tasks/low.md', 'task', '{"title":"alpha","priority":1,"status":"open"}', 1),
			('tasks/mid', 'mm-tasks/mid.md', 'task', '{"title":"mid","priority":2,"status":"done"}', 1);
	`)
	if err != nil {
		t.Fatalf("insert numeric tasks: %v", err)
	}
	return db
}

func setupSortDateDB(t *testing.T) *sql.DB {
	t.Helper()
	db := setupTestDB(t)
	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('daily/2025-01-15', 'daily/2025-01-15.md', 'date', '{}', 1),
			('daily/2025-03-01', 'daily/2025-03-01.md', 'date', '{}', 1);
	`)
	if err != nil {
		t.Fatalf("insert date objects: %v", err)
	}
	return db
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
