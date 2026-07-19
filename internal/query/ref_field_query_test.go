package query

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestRefFieldQueryResolvesCanonicalTargets(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	personFields, _ := json.Marshal(map[string]interface{}{
		"company": "cursor",
	})

	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start)
		VALUES
			('companies/cursor', 'companies/cursor.md', 'company', '{}', 1),
			('people/ada', 'people/ada.md', 'person', ?, 1)
	`, string(personFields))
	if err != nil {
		t.Fatalf("failed to insert objects: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO field_refs (source_id, field_name, target_id, target_raw, resolution_status, file_path, line_number)
		VALUES ('people/ada', 'company', 'companies/cursor', 'cursor', 'resolved', 'people/ada.md', 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert field_refs: %v", err)
	}

	sch := schema.New()
	sch.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"company": {Type: schema.FieldTypeRef, Target: "company"},
		},
	}
	sch.Types["company"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}

	executor := NewExecutor(db)
	executor.SetSchema(sch)

	q, err := Parse("type:person .company==[[companies/cursor]]")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	results, err := executor.ExecuteObjectQuery(q)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(results) != 1 || results[0].ID != "people/ada" {
		t.Fatalf("expected people/ada, got %+v", results)
	}

	q, err = Parse("type:person .company==cursor")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	results, err = executor.ExecuteObjectQuery(q)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(results) != 1 || results[0].ID != "people/ada" {
		t.Fatalf("expected people/ada for shorthand query, got %+v", results)
	}
}

func TestRefFieldQueryRelativeDateKeywordsResolveToDailyRefs(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	// Daily notes have a bare-date object ID; the file lives under daily/.
	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start)
		VALUES
			('2026-04-04', 'daily/2026-04-04.md', 'date', '{}', 1),
			('2026-04-05', 'daily/2026-04-05.md', 'date', '{}', 1),
			('2026-04-06', 'daily/2026-04-06.md', 'date', '{}', 1),
			('brief/yesterday', 'brief/yesterday.md', 'brief', '{"date":"2026-04-04"}', 1),
			('brief/today', 'brief/today.md', 'brief', '{"date":"2026-04-05"}', 1),
			('brief/tomorrow', 'brief/tomorrow.md', 'brief', '{"date":"2026-04-06"}', 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert objects: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO field_refs (source_id, field_name, target_id, target_raw, resolution_status, file_path, line_number)
		VALUES
			('brief/yesterday', 'date', '2026-04-04', '2026-04-04', 'resolved', 'brief/yesterday.md', 1),
			('brief/today', 'date', '2026-04-05', '2026-04-05', 'resolved', 'brief/today.md', 1),
			('brief/tomorrow', 'date', '2026-04-06', '2026-04-06', 'resolved', 'brief/tomorrow.md', 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert field_refs: %v", err)
	}

	executor := NewExecutor(db)
	executor.SetSchema(briefDateRefSchema())
	executor.nowFn = func() time.Time {
		return time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
	}

	tests := []struct {
		name   string
		query  string
		wantID string
	}{
		{name: "yesterday", query: "type:brief .date==yesterday", wantID: "brief/yesterday"},
		{name: "today", query: "type:brief .date==today", wantID: "brief/today"},
		{name: "tomorrow", query: "type:brief .date==tomorrow", wantID: "brief/tomorrow"},
		{name: "wikilink keyword", query: "type:brief .date==[[today]]", wantID: "brief/today"},
		{name: "explicit date", query: "type:brief .date==2026-04-05", wantID: "brief/today"},
		{name: "explicit bare date id", query: "type:brief .date==2026-04-05", wantID: "brief/today"},
		{name: "legacy daily id compat", query: "type:brief .date==daily/2026-04-05", wantID: "brief/today"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			results, err := executor.ExecuteObjectQuery(q)
			if err != nil {
				t.Fatalf("query error: %v", err)
			}
			if len(results) != 1 || results[0].ID != tt.wantID {
				t.Fatalf("expected %s, got %+v", tt.wantID, results)
			}
		})
	}
}

func TestRefFieldQueryRelativeDateKeywordUsesSingleExecutionTimestamp(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start)
		VALUES
			('2026-04-05', 'daily/2026-04-05.md', 'date', '{}', 1),
			('brief/today', 'brief/today.md', 'brief', '{"date":"2026-04-05"}', 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert objects: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO field_refs (source_id, field_name, target_id, target_raw, resolution_status, file_path, line_number)
		VALUES ('brief/today', 'date', '2026-04-05', '2026-04-05', 'resolved', 'brief/today.md', 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert field_refs: %v", err)
	}

	callCount := 0
	executor := NewExecutor(db)
	executor.SetSchema(briefDateRefSchema())
	executor.nowFn = func() time.Time {
		callCount++
		if callCount == 1 {
			return time.Date(2026, 4, 5, 23, 59, 59, 0, time.UTC)
		}
		return time.Date(2026, 4, 6, 0, 0, 1, 0, time.UTC)
	}

	q, err := Parse("type:brief .date==today .date==today")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	results, err := executor.ExecuteObjectQuery(q)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(results) != 1 || results[0].ID != "brief/today" {
		t.Fatalf("expected brief/today, got %+v", results)
	}
	if callCount != 1 {
		t.Fatalf("nowFn callCount = %d, want 1", callCount)
	}
}

func TestRefFieldQueryErrorsOnAmbiguousStoredValue(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	personFields, _ := json.Marshal(map[string]interface{}{
		"company": "cursor",
	})

	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start)
		VALUES
			('companies/cursor', 'companies/cursor.md', 'company', '{}', 1),
			('orgs/cursor', 'orgs/cursor.md', 'company', '{}', 1),
			('people/ada', 'people/ada.md', 'person', ?, 1)
	`, string(personFields))
	if err != nil {
		t.Fatalf("failed to insert objects: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO field_refs (source_id, field_name, target_id, target_raw, resolution_status, file_path, line_number)
		VALUES ('people/ada', 'company', NULL, 'cursor', 'ambiguous', 'people/ada.md', 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert field_refs: %v", err)
	}

	sch := schema.New()
	sch.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"company": {Type: schema.FieldTypeRef, Target: "company"},
		},
	}
	sch.Types["company"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}

	executor := NewExecutor(db)
	executor.SetSchema(sch)

	q, err := Parse("type:person .company==[[companies/cursor]]")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if _, err := executor.ExecuteObjectQuery(q); err == nil {
		t.Fatal("expected error for ambiguous stored ref, got nil")
	}
}

func TestRefFieldQueryMemoizesAmbiguityChecksWithinExecution(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	personFields, _ := json.Marshal(map[string]interface{}{
		"company": "cursor",
	})

	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start)
		VALUES
			('companies/cursor', 'companies/cursor.md', 'company', '{}', 1),
			('people/ada', 'people/ada.md', 'person', ?, 1)
	`, string(personFields))
	if err != nil {
		t.Fatalf("failed to insert objects: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO field_refs (source_id, field_name, target_id, target_raw, resolution_status, file_path, line_number)
		VALUES ('people/ada', 'company', 'companies/cursor', 'cursor', 'resolved', 'people/ada.md', 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert field_refs: %v", err)
	}

	sch := schema.New()
	sch.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"company": {Type: schema.FieldTypeRef, Target: "company"},
		},
	}
	sch.Types["company"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}

	executor := NewExecutor(db)
	executor.SetSchema(sch)
	checkCount := 0
	executor.ambiguousFieldRefQueryHook = func() {
		checkCount++
	}

	q, err := Parse("type:person .company==cursor .company==cursor")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	results, err := executor.ExecuteObjectQuery(q)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(results) != 1 || results[0].ID != "people/ada" {
		t.Fatalf("expected people/ada, got %+v", results)
	}
	if checkCount != 1 {
		t.Fatalf("ambiguity checks = %d, want 1", checkCount)
	}
}

func briefDateRefSchema() *schema.Schema {
	sch := schema.New()
	sch.Types["brief"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"date": {Type: schema.FieldTypeRef, Target: "date"},
		},
	}
	sch.Types["date"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}
	return sch
}

func TestRefFieldQueryBatchesDistinctAmbiguityChecksWithinExecution(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	personFields, _ := json.Marshal(map[string]interface{}{
		"company": "cursor",
		"manager": "freya",
	})

	_, err := db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start)
		VALUES
			('companies/cursor', 'companies/cursor.md', 'company', '{}', 1),
			('people/ada', 'people/ada.md', 'person', ?, 1)
	`, string(personFields))
	if err != nil {
		t.Fatalf("failed to insert objects: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO field_refs (source_id, field_name, target_id, target_raw, resolution_status, file_path, line_number)
		VALUES
			('people/ada', 'company', 'companies/cursor', 'cursor', 'resolved', 'people/ada.md', 1),
			('people/ada', 'manager', 'people/freya', 'freya', 'resolved', 'people/ada.md', 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert field_refs: %v", err)
	}

	sch := schema.New()
	sch.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"company": {Type: schema.FieldTypeRef, Target: "company"},
			"manager": {Type: schema.FieldTypeRef, Target: "person"},
		},
	}
	sch.Types["company"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}

	executor := NewExecutor(db)
	executor.SetSchema(sch)
	checkCount := 0
	executor.ambiguousFieldRefQueryHook = func() {
		checkCount++
	}

	q, err := Parse("type:person .company==cursor .manager==freya")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	results, err := executor.ExecuteObjectQuery(q)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(results) != 1 || results[0].ID != "people/ada" {
		t.Fatalf("expected people/ada, got %+v", results)
	}
	if checkCount != 1 {
		t.Fatalf("ambiguity checks = %d, want 1", checkCount)
	}
}
