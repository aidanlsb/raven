package index

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestTryParseTemporalComparisonWithOptions(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.March, 4, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		filter        string
		op            string
		wantCondition string
		wantArg       interface{}
		wantOK        bool
		wantErr       bool
	}{
		{
			name:          "relative today equals compares by date",
			filter:        "today",
			op:            "=",
			wantCondition: "date(value) = date(?)",
			wantArg:       "2026-03-04",
			wantOK:        true,
		},
		{
			name:          "relative tomorrow less-than",
			filter:        "tomorrow",
			op:            "<",
			wantCondition: "date(value) < date(?)",
			wantArg:       "2026-03-05",
			wantOK:        true,
		},
		{
			name:          "absolute date greater-or-equal",
			filter:        "2025-02-01",
			op:            ">=",
			wantCondition: "date(value) >= date(?)",
			wantArg:       "2025-02-01",
			wantOK:        true,
		},
		{
			name:          "datetime literal compares by datetime",
			filter:        "2026-04-05T12:30",
			op:            "=",
			wantCondition: "datetime(value) = datetime(?)",
			wantArg:       "2026-04-05T12:30",
			wantOK:        true,
		},
		{
			name:   "non-temporal value is not a date filter",
			filter: "active",
			op:     "=",
			wantOK: false,
		},
		{
			name:    "invalid date literal errors",
			filter:  "2025-13-45",
			op:      "=",
			wantErr: true,
		},
		{
			name:    "unsupported operator errors",
			filter:  "today",
			op:      "LIKE",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition, args, ok, err := TryParseTemporalComparisonWithOptions(tt.filter, tt.op, "value", DateFilterOptions{
				Now: now,
			})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if condition != tt.wantCondition {
				t.Errorf("condition = %q, want %q", condition, tt.wantCondition)
			}
			if len(args) != 1 {
				t.Fatalf("args len = %d, want 1", len(args))
			}
			if args[0] != tt.wantArg {
				t.Errorf("arg = %v, want %v", args[0], tt.wantArg)
			}
		})
	}
}

func TestTryParseTemporalComparisonWithOptionsDefaultsNow(t *testing.T) {
	t.Parallel()

	// A zero Now falls back to time.Now(); the parse should still succeed and
	// yield a date comparison for a relative keyword.
	condition, args, ok, err := TryParseTemporalComparisonWithOptions("today", "=", "value", DateFilterOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected relative keyword to parse")
	}
	if condition != "date(value) = date(?)" {
		t.Errorf("condition = %q", condition)
	}
	if len(args) != 1 {
		t.Fatalf("args len = %d, want 1", len(args))
	}
}

func TestExtractDateString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value schema.FieldValue
		want  string
	}{
		{
			name:  "date value",
			value: schema.Date("2026-04-09"),
			want:  "2026-04-09",
		},
		{
			name:  "datetime value returns date prefix",
			value: schema.Datetime("2026-04-09T12:34:56Z"),
			want:  "2026-04-09",
		},
		{
			name:  "date-shaped junk string is rejected",
			value: schema.String("ABCD-EF-GH trailing text"),
			want:  "",
		},
		{
			name:  "short string is rejected",
			value: schema.String("2026-04"),
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractDateString(tt.value); got != tt.want {
				t.Fatalf("extractDateString() = %q, want %q", got, tt.want)
			}
		})
	}
}
func TestIndexDatesUsesSchemaFieldTypes(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.New()
	sch.Types["task"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"due":   {Type: schema.FieldTypeDate},
			"title": {Type: schema.FieldTypeString},
		},
	}

	doc := &parser.ParsedDocument{
		FilePath: "task/a.md",
		Objects: []*model.Object{
			{
				ID:   "task/a",
				Type: "task",
				Fields: map[string]schema.FieldValue{
					"due":   schema.Date("2026-04-05"),
					"title": schema.String("2026-04-05"),
				},
				LineStart: 1,
			},
		},
	}

	if err := db.IndexDocument(doc, sch); err != nil {
		t.Fatalf("failed to index document: %v", err)
	}

	rows, err := db.db.Query(`SELECT field_name, date FROM date_index WHERE source_id = 'task/a' ORDER BY field_name`)
	if err != nil {
		t.Fatalf("query date_index: %v", err)
	}
	defer rows.Close()

	got := map[string]string{}
	for rows.Next() {
		var fieldName, dateStr string
		if err := rows.Scan(&fieldName, &dateStr); err != nil {
			t.Fatalf("scan date_index: %v", err)
		}
		got[fieldName] = dateStr
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate date_index: %v", err)
	}

	if len(got) != 1 || got["due"] != "2026-04-05" {
		t.Fatalf("date_index rows = %#v, want only due=2026-04-05", got)
	}
}
func TestIndexDatesIndexesDateTargetRefs(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.New()
	sch.Types["brief"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"date":      {Type: schema.FieldTypeRef, Target: "date"},
			"next_date": {Type: schema.FieldTypeRef, Target: "date"},
		},
	}

	doc := &parser.ParsedDocument{
		FilePath: "brief/a.md",
		Objects: []*model.Object{
			{
				ID:   "brief/a",
				Type: "brief",
				Fields: map[string]schema.FieldValue{
					"date":      schema.String("2026-04-05"),
					"next_date": schema.String("daily/2026-04-06"),
				},
				LineStart: 1,
			},
		},
	}

	if err := db.IndexDocument(doc, sch); err != nil {
		t.Fatalf("failed to index document: %v", err)
	}

	rows, err := db.db.Query(`SELECT field_name, date FROM date_index WHERE source_id = 'brief/a' ORDER BY field_name`)
	if err != nil {
		t.Fatalf("query date_index: %v", err)
	}
	defer rows.Close()

	got := map[string]string{}
	for rows.Next() {
		var fieldName, dateStr string
		if err := rows.Scan(&fieldName, &dateStr); err != nil {
			t.Fatalf("scan date_index: %v", err)
		}
		got[fieldName] = dateStr
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate date_index: %v", err)
	}

	want := map[string]string{"date": "2026-04-05", "next_date": "2026-04-06"}
	if len(got) != len(want) {
		t.Fatalf("date_index rows = %#v, want %#v", got, want)
	}
	for fieldName, wantDate := range want {
		if got[fieldName] != wantDate {
			t.Fatalf("date_index[%s] = %q, want %q (all rows %#v)", fieldName, got[fieldName], wantDate, got)
		}
	}
}

// TestTraitIDConsistency is a regression test for the bug where indexDates used
// the raw loop index (idx) while indexInlineTraits used a counter that only
// incremented for defined traits. When undefined traits preceded defined ones,
// the two functions produced different IDs for the same physical trait, causing
// date queries to reference non-existent trait IDs.
func TestTraitIDConsistency(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Schema defines "due" but NOT "undefined" — so "undefined" must be skipped.
	testSchema := schema.New()
	testSchema.Traits["due"] = &schema.TraitDefinition{
		Type: schema.FieldTypeDate,
	}

	dueValue := schema.Date("2025-03-15")
	doc := &parser.ParsedDocument{
		FilePath: "test.md",
		Objects: []*model.Object{
			{
				ID:        "test",
				Type:      "page",
				Fields:    make(map[string]schema.FieldValue),
				LineStart: 1,
			},
		},
		Traits: []*model.Trait{
			{
				// raw index 0 — undefined, must be skipped
				TraitType:     "undefined",
				Value:         nil,
				Content:       "some note",
				Line:          3,
				ParentScopeID: "test",
			},
			{
				// raw index 1, but first defined trait → traitIdx=0 in both functions
				TraitType:     "due",
				Value:         &dueValue,
				Content:       "finish by",
				Line:          4,
				ParentScopeID: "test",
			},
		},
	}

	if err := db.IndexDocument(doc, testSchema); err != nil {
		t.Fatalf("failed to index document: %v", err)
	}

	// The "due" trait should be stored with ID "test.md:trait:0" in the traits table.
	var traitID string
	if err := db.db.QueryRow(`SELECT id FROM traits WHERE trait_type = 'due'`).Scan(&traitID); err != nil {
		t.Fatalf("failed to query traits table: %v", err)
	}
	if traitID != "test.md:trait:0" {
		t.Errorf("traits table: got id %q, want %q", traitID, "test.md:trait:0")
	}

	// The date_index entry for the same trait must reference the same ID.
	var dateSourceID string
	if err := db.db.QueryRow(`SELECT source_id FROM date_index WHERE source_type = 'trait'`).Scan(&dateSourceID); err != nil {
		t.Fatalf("failed to query date_index table: %v", err)
	}
	if dateSourceID != traitID {
		t.Errorf("date_index source_id %q does not match traits.id %q — trait ID mismatch bug", dateSourceID, traitID)
	}
}
func TestDateIndexTraitIDsTrackIndexedTraitOrder(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.New()
	sch.Traits["due"] = &schema.TraitDefinition{Type: schema.FieldTypeDate}
	sch.Traits["review"] = &schema.TraitDefinition{Type: schema.FieldTypeDate}
	sch.Traits["status"] = &schema.TraitDefinition{Type: schema.FieldTypeString}

	dueValue := schema.Date("2025-03-15")
	reviewValue := schema.Date("2025-03-20")
	statusValue := schema.String("active")
	doc := &parser.ParsedDocument{
		FilePath: "notes/plan.md",
		Objects: []*model.Object{
			{
				ID:        "notes/plan",
				Type:      "note",
				Fields:    map[string]schema.FieldValue{},
				LineStart: 1,
			},
		},
		Traits: []*model.Trait{
			{
				TraitType:     "undefined",
				Content:       "skip me",
				Line:          2,
				ParentScopeID: "notes/plan",
			},
			{
				TraitType:     "due",
				Value:         &dueValue,
				Content:       "ship it",
				Line:          3,
				ParentScopeID: "notes/plan",
			},
			{
				TraitType:     "status",
				Value:         &statusValue,
				Content:       "state",
				Line:          4,
				ParentScopeID: "notes/plan",
			},
			{
				TraitType:     "review",
				Value:         &reviewValue,
				Content:       "check it",
				Line:          5,
				ParentScopeID: "notes/plan",
			},
		},
	}

	if err := db.IndexDocument(doc, sch); err != nil {
		t.Fatalf("failed to index document: %v", err)
	}

	traitIDsByType := map[string]string{}
	rows, err := db.db.Query(`SELECT trait_type, id FROM traits ORDER BY line_number`)
	if err != nil {
		t.Fatalf("query traits: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var traitType, traitID string
		if err := rows.Scan(&traitType, &traitID); err != nil {
			t.Fatalf("scan trait row: %v", err)
		}
		traitIDsByType[traitType] = traitID
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate trait rows: %v", err)
	}

	wantTraitIDs := map[string]string{
		"due":    "notes/plan.md:trait:0",
		"status": "notes/plan.md:trait:1",
		"review": "notes/plan.md:trait:2",
	}
	for traitType, wantID := range wantTraitIDs {
		if got := traitIDsByType[traitType]; got != wantID {
			t.Fatalf("%s trait id = %q, want %q", traitType, got, wantID)
		}
	}

	dateIDsByType := map[string]string{}
	dateRows, err := db.db.Query(`
		SELECT field_name, source_id
		FROM date_index
		WHERE source_type = 'trait'
		ORDER BY date
	`)
	if err != nil {
		t.Fatalf("query date_index: %v", err)
	}
	defer dateRows.Close()
	for dateRows.Next() {
		var fieldName, sourceID string
		if err := dateRows.Scan(&fieldName, &sourceID); err != nil {
			t.Fatalf("scan date_index row: %v", err)
		}
		dateIDsByType[fieldName] = sourceID
	}
	if err := dateRows.Err(); err != nil {
		t.Fatalf("iterate date_index rows: %v", err)
	}

	if len(dateIDsByType) != 2 {
		t.Fatalf("got %d trait-backed date rows, want 2", len(dateIDsByType))
	}
	if got := dateIDsByType["due"]; got != traitIDsByType["due"] {
		t.Fatalf("due date_index source_id = %q, want %q", got, traitIDsByType["due"])
	}
	if got := dateIDsByType["review"]; got != traitIDsByType["review"] {
		t.Fatalf("review date_index source_id = %q, want %q", got, traitIDsByType["review"])
	}
}
func TestTraitIDsStableAcrossReindexForMultilineParagraph(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.New()
	sch.Traits["todo"] = &schema.TraitDefinition{Type: schema.FieldTypeString}

	content := "First task @todo one\nSecond task @todo two\nThird task @todo three\n"
	expectedIDs := []string{
		"notes/unstable.md:trait:0",
		"notes/unstable.md:trait:1",
		"notes/unstable.md:trait:2",
	}

	for i := 0; i < 20; i++ {
		if err := db.ClearAllData(); err != nil {
			t.Fatalf("iteration %d: failed to clear database: %v", i, err)
		}

		doc, err := parser.ParseDocument(content, "/vault/notes/unstable.md", "/vault")
		if err != nil {
			t.Fatalf("iteration %d: failed to parse document: %v", i, err)
		}
		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("iteration %d: failed to index document: %v", i, err)
		}

		rows, err := db.db.Query(`SELECT id FROM traits ORDER BY line_number`)
		if err != nil {
			t.Fatalf("iteration %d: failed to query traits: %v", i, err)
		}

		var gotIDs []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				t.Fatalf("iteration %d: failed to scan trait id: %v", i, err)
			}
			gotIDs = append(gotIDs, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatalf("iteration %d: row iteration error: %v", i, err)
		}
		rows.Close()

		if len(gotIDs) != len(expectedIDs) {
			t.Fatalf("iteration %d: got %d traits, want %d", i, len(gotIDs), len(expectedIDs))
		}
		for j, gotID := range gotIDs {
			if gotID != expectedIDs[j] {
				t.Fatalf(
					"iteration %d: trait id at line-order index %d = %q, want %q (unstable trait ordering)",
					i, j, gotID, expectedIDs[j],
				)
			}
		}

		var dateSourceID string
		if err := db.db.QueryRow(`
			SELECT source_id FROM date_index
			WHERE source_type='trait'
			ORDER BY date
			LIMIT 1
		`).Scan(&dateSourceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("iteration %d: failed to query date index source id: %v", i, err)
		}
		if dateSourceID != "" {
			wantPrefix := "notes/unstable.md:trait:"
			if len(dateSourceID) < len(wantPrefix) || dateSourceID[:len(wantPrefix)] != wantPrefix {
				t.Fatalf("iteration %d: unexpected date_index trait source id %q", i, dateSourceID)
			}
		}
	}
}
