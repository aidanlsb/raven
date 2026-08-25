package indexschema

import (
	"database/sql"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
	_ "modernc.org/sqlite"
)

func TestDefaultDailyDirectory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty returns default", "", "daily"},
		{"explicit value preserved", "journal", "journal"},
		{"explicit daily preserved", "daily", "daily"},
		{"nested path preserved", "notes/daily", "notes/daily"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultDailyDirectory(tt.input)
			if got != tt.want {
				t.Errorf("DefaultDailyDirectory(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolverGeneration(t *testing.T) {
	t.Parallel()

	t.Run("returns 0 for missing meta key", func(t *testing.T) {
		db := setupTestDB(t)
		gen, err := ResolverGeneration(db)
		if err != nil {
			t.Fatalf("ResolverGeneration() error = %v", err)
		}
		if gen != 0 {
			t.Errorf("ResolverGeneration() = %d, want 0", gen)
		}
	})

	t.Run("reads stored generation", func(t *testing.T) {
		db := setupTestDB(t)
		if _, err := db.Exec("INSERT INTO meta (key, value) VALUES (?, ?)", ResolverGenerationMetaKey, "42"); err != nil {
			t.Fatalf("failed to insert meta: %v", err)
		}
		gen, err := ResolverGeneration(db)
		if err != nil {
			t.Fatalf("ResolverGeneration() error = %v", err)
		}
		if gen != 42 {
			t.Errorf("ResolverGeneration() = %d, want 42", gen)
		}
	})

	t.Run("returns 0 when meta table missing", func(t *testing.T) {
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Fatalf("failed to open database: %v", err)
		}
		defer db.Close()

		gen, err := ResolverGeneration(db)
		if err != nil {
			t.Fatalf("ResolverGeneration() error = %v", err)
		}
		if gen != 0 {
			t.Errorf("ResolverGeneration() = %d, want 0", gen)
		}
	})
}

func TestAllObjectIDs(t *testing.T) {
	t.Parallel()

	t.Run("returns empty for empty database", func(t *testing.T) {
		db := setupTestDB(t)
		ids, err := AllObjectIDs(db)
		if err != nil {
			t.Fatalf("AllObjectIDs() error = %v", err)
		}
		if len(ids) != 0 {
			t.Errorf("AllObjectIDs() = %v, want empty", ids)
		}
	})

	t.Run("returns all object IDs", func(t *testing.T) {
		db := setupTestDB(t)
		insertObject(t, db, "books/dune", "books/dune.md", "book")
		insertObject(t, db, "books/foundation", "books/foundation.md", "book")

		ids, err := AllObjectIDs(db)
		if err != nil {
			t.Fatalf("AllObjectIDs() error = %v", err)
		}
		if len(ids) != 2 {
			t.Fatalf("AllObjectIDs() returned %d IDs, want 2", len(ids))
		}
		if !contains(ids, "books/dune") || !contains(ids, "books/foundation") {
			t.Errorf("AllObjectIDs() = %v, want [books/dune, books/foundation]", ids)
		}
	})

	t.Run("includes section IDs when sections table exists", func(t *testing.T) {
		db := setupTestDB(t)
		insertObject(t, db, "notes/readme", "notes/readme.md", "note")
		insertSection(t, db, "notes/readme#intro", "notes/readme", "notes/readme.md", "intro", "Introduction", 2, 5)

		ids, err := AllObjectIDs(db)
		if err != nil {
			t.Fatalf("AllObjectIDs() error = %v", err)
		}
		if len(ids) != 2 {
			t.Fatalf("AllObjectIDs() returned %d IDs, want 2", len(ids))
		}
		if !contains(ids, "notes/readme") || !contains(ids, "notes/readme#intro") {
			t.Errorf("AllObjectIDs() = %v, want [notes/readme, notes/readme#intro]", ids)
		}
	})
}

func TestAllAliases(t *testing.T) {
	t.Parallel()

	t.Run("returns empty when no aliases", func(t *testing.T) {
		db := setupTestDB(t)
		insertObject(t, db, "books/dune", "books/dune.md", "book")

		aliases, err := AllAliases(db)
		if err != nil {
			t.Fatalf("AllAliases() error = %v", err)
		}
		if len(aliases) != 0 {
			t.Errorf("AllAliases() = %v, want empty", aliases)
		}
	})

	t.Run("returns deterministic first ID per alias", func(t *testing.T) {
		db := setupTestDB(t)
		insertObjectWithAlias(t, db, "people/freya", "people/freya.md", "person", "Freya")
		insertObjectWithAlias(t, db, "places/freya", "places/freya.md", "place", "Freya")

		aliases, err := AllAliases(db)
		if err != nil {
			t.Fatalf("AllAliases() error = %v", err)
		}
		if len(aliases) != 1 {
			t.Fatalf("AllAliases() returned %d aliases, want 1", len(aliases))
		}
		got, ok := aliases["Freya"]
		if !ok {
			t.Fatalf("AllAliases() missing alias 'Freya'")
		}
		if got != "people/freya" {
			t.Errorf("AllAliases()['Freya'] = %q, want %q (first alphabetically)", got, "people/freya")
		}
	})

	t.Run("ignores empty alias strings", func(t *testing.T) {
		db := setupTestDB(t)
		insertObjectWithAlias(t, db, "test", "test.md", "note", "")

		aliases, err := AllAliases(db)
		if err != nil {
			t.Fatalf("AllAliases() error = %v", err)
		}
		if len(aliases) != 0 {
			t.Errorf("AllAliases() = %v, want empty (empty alias ignored)", aliases)
		}
	})
}

func TestAllAliasMatches(t *testing.T) {
	t.Parallel()

	t.Run("returns all matches per alias", func(t *testing.T) {
		db := setupTestDB(t)
		insertObjectWithAlias(t, db, "people/freya", "people/freya.md", "person", "Freya")
		insertObjectWithAlias(t, db, "places/freya", "places/freya.md", "place", "Freya")
		insertObjectWithAlias(t, db, "books/dune", "books/dune.md", "book", "Dune")

		matches, err := AllAliasMatches(db)
		if err != nil {
			t.Fatalf("AllAliasMatches() error = %v", err)
		}
		if len(matches) != 2 {
			t.Fatalf("AllAliasMatches() returned %d aliases, want 2", len(matches))
		}

		freyaMatches := matches["Freya"]
		if len(freyaMatches) != 2 {
			t.Errorf("AllAliasMatches()['Freya'] = %v, want 2 matches", freyaMatches)
		}
		if !contains(freyaMatches, "people/freya") || !contains(freyaMatches, "places/freya") {
			t.Errorf("AllAliasMatches()['Freya'] = %v, want [people/freya, places/freya]", freyaMatches)
		}

		duneMatches := matches["Dune"]
		if len(duneMatches) != 1 || duneMatches[0] != "books/dune" {
			t.Errorf("AllAliasMatches()['Dune'] = %v, want [books/dune]", duneMatches)
		}
	})
}

func TestTableHasColumn(t *testing.T) {
	t.Parallel()

	t.Run("detects existing column", func(t *testing.T) {
		db := setupTestDB(t)
		has, err := TableHasColumn(db, "objects", "id")
		if err != nil {
			t.Fatalf("TableHasColumn() error = %v", err)
		}
		if !has {
			t.Errorf("TableHasColumn('objects', 'id') = false, want true")
		}
	})

	t.Run("detects missing column", func(t *testing.T) {
		db := setupTestDB(t)
		has, err := TableHasColumn(db, "objects", "nonexistent")
		if err != nil {
			t.Fatalf("TableHasColumn() error = %v", err)
		}
		if has {
			t.Errorf("TableHasColumn('objects', 'nonexistent') = true, want false")
		}
	})

	t.Run("detects alias column when present", func(t *testing.T) {
		db := setupTestDB(t)
		has, err := TableHasColumn(db, "objects", "alias")
		if err != nil {
			t.Fatalf("TableHasColumn() error = %v", err)
		}
		if !has {
			t.Errorf("TableHasColumn('objects', 'alias') = false, want true")
		}
	})
}

func TestBuildTypeNameFields(t *testing.T) {
	t.Parallel()

	t.Run("returns empty for nil schema", func(t *testing.T) {
		result := BuildTypeNameFields(nil)
		if len(result) != 0 {
			t.Errorf("BuildTypeNameFields(nil) = %v, want empty", result)
		}
	})

	t.Run("extracts name fields from schema", func(t *testing.T) {
		sch := &schema.Schema{
			Types: map[string]*schema.TypeDefinition{
				"book": {
					NameField: "title",
					Fields: map[string]*schema.FieldDefinition{
						"title": {Type: "string"},
					},
				},
				"person": {
					NameField: "full_name",
					Fields: map[string]*schema.FieldDefinition{
						"full_name": {Type: "string"},
					},
				},
				"note": {
					Fields: map[string]*schema.FieldDefinition{
						"content": {Type: "string"},
					},
				},
			},
		}

		result := BuildTypeNameFields(sch)
		if len(result) != 2 {
			t.Fatalf("BuildTypeNameFields() returned %d entries, want 2", len(result))
		}
		if result["book"] != "title" {
			t.Errorf("BuildTypeNameFields()['book'] = %q, want 'title'", result["book"])
		}
		if result["person"] != "full_name" {
			t.Errorf("BuildTypeNameFields()['person'] = %q, want 'full_name'", result["person"])
		}
		if _, ok := result["note"]; ok {
			t.Errorf("BuildTypeNameFields() included 'note' with no name_field")
		}
	})
}

func TestExtractNameFieldValue(t *testing.T) {
	t.Parallel()

	typeNameFields := map[string]string{
		"book": "title",
	}

	tests := []struct {
		name       string
		objType    string
		fieldsJSON string
		wantValue  string
		wantOk     bool
	}{
		{
			name:       "extracts valid name field",
			objType:    "book",
			fieldsJSON: `{"title":"Dune","author":"Frank Herbert"}`,
			wantValue:  "Dune",
			wantOk:     true,
		},
		{
			name:       "returns false for missing type",
			objType:    "person",
			fieldsJSON: `{"name":"Freya"}`,
			wantValue:  "",
			wantOk:     false,
		},
		{
			name:       "returns false for missing field",
			objType:    "book",
			fieldsJSON: `{"author":"Frank Herbert"}`,
			wantValue:  "",
			wantOk:     false,
		},
		{
			name:       "returns false for non-string value",
			objType:    "book",
			fieldsJSON: `{"title":42}`,
			wantValue:  "",
			wantOk:     false,
		},
		{
			name:       "returns false for empty string",
			objType:    "book",
			fieldsJSON: `{"title":""}`,
			wantValue:  "",
			wantOk:     false,
		},
		{
			name:       "returns false for invalid JSON",
			objType:    "book",
			fieldsJSON: `{invalid}`,
			wantValue:  "",
			wantOk:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotOk := ExtractNameFieldValue(typeNameFields, tt.objType, tt.fieldsJSON)
			if gotValue != tt.wantValue {
				t.Errorf("ExtractNameFieldValue() value = %q, want %q", gotValue, tt.wantValue)
			}
			if gotOk != tt.wantOk {
				t.Errorf("ExtractNameFieldValue() ok = %v, want %v", gotOk, tt.wantOk)
			}
		})
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(SchemaSQL); err != nil {
		t.Fatalf("failed to execute SchemaSQL: %v", err)
	}

	return db
}

func insertObject(t *testing.T, db *sql.DB, id, filePath, objType string) {
	t.Helper()

	if _, err := db.Exec(
		"INSERT INTO objects (id, file_path, type, line_start) VALUES (?, ?, ?, ?)",
		id, filePath, objType, 1,
	); err != nil {
		t.Fatalf("failed to insert object: %v", err)
	}
}

func insertObjectWithAlias(t *testing.T, db *sql.DB, id, filePath, objType, alias string) {
	t.Helper()

	if _, err := db.Exec(
		"INSERT INTO objects (id, file_path, type, alias, line_start) VALUES (?, ?, ?, ?, ?)",
		id, filePath, objType, alias, 1,
	); err != nil {
		t.Fatalf("failed to insert object with alias: %v", err)
	}
}

func insertSection(t *testing.T, db *sql.DB, id, fileObjectID, filePath, slug, title string, level, lineStart int) {
	t.Helper()

	if _, err := db.Exec(
		"INSERT INTO sections (id, file_object_id, file_path, slug, title, level, line_start) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, fileObjectID, filePath, slug, title, level, lineStart,
	); err != nil {
		t.Fatalf("failed to insert section: %v", err)
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
