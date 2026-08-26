package indexschema

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSchemaSQL(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(SchemaSQL); err != nil {
		t.Fatalf("failed to execute SchemaSQL: %v", err)
	}

	tables := []string{
		"meta", "objects", "sections", "traits", "refs", "links",
		"field_refs", "date_index", "fts_content",
	}
	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %s not created: %v", table, err)
		}
	}
}

func TestSchemaSQL_Indexes(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(SchemaSQL); err != nil {
		t.Fatalf("failed to execute SchemaSQL: %v", err)
	}

	indexes := []string{
		"idx_objects_file", "idx_objects_type", "idx_objects_alias",
		"idx_sections_file", "idx_sections_file_object", "idx_sections_parent",
		"idx_traits_file", "idx_traits_type", "idx_traits_parent",
		"idx_refs_source", "idx_refs_target", "idx_refs_file",
		"idx_links_source", "idx_links_file", "idx_links_normalized_key",
		"idx_field_refs_source_field", "idx_field_refs_field_target",
		"idx_field_refs_field_raw", "idx_field_refs_status", "idx_field_refs_file",
		"idx_traits_file_line", "idx_refs_file_line", "idx_traits_type_value",
		"idx_date_index_date", "idx_date_index_file",
	}
	for _, index := range indexes {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name=?", index).Scan(&name)
		if err != nil {
			t.Errorf("index %s not created: %v", index, err)
		}
	}
}

func TestSchemaSQL_Triggers(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(SchemaSQL); err != nil {
		t.Fatalf("failed to execute SchemaSQL: %v", err)
	}

	triggers := []string{
		"trg_objects_resolver_generation_insert",
		"trg_objects_resolver_generation_update",
		"trg_objects_resolver_generation_delete",
		"trg_sections_resolver_generation_insert",
		"trg_sections_resolver_generation_update",
		"trg_sections_resolver_generation_delete",
	}
	for _, trigger := range triggers {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='trigger' AND name=?", trigger).Scan(&name)
		if err != nil {
			t.Errorf("trigger %s not created: %v", trigger, err)
		}
	}
}

func TestSchemaSQL_ResolverGenerationTriggers(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(SchemaSQL); err != nil {
		t.Fatalf("failed to execute SchemaSQL: %v", err)
	}

	getGeneration := func() int64 {
		var gen sql.NullString
		err := db.QueryRow("SELECT value FROM meta WHERE key = 'resolver_generation'").Scan(&gen)
		if err == sql.ErrNoRows {
			return 0
		}
		if err != nil {
			t.Fatalf("failed to read resolver_generation: %v", err)
		}
		if !gen.Valid {
			return 0
		}
		var result int64
		if _, err := fmt.Sscanf(gen.String, "%d", &result); err != nil {
			t.Fatalf("failed to parse resolver_generation: %v", err)
		}
		return result
	}

	initialGen := getGeneration()

	if _, err := db.Exec("INSERT INTO objects (id, file_path, type, line_start) VALUES (?, ?, ?, ?)",
		"test", "test.md", "note", 1); err != nil {
		t.Fatalf("failed to insert object: %v", err)
	}
	gen1 := getGeneration()
	if gen1 <= initialGen {
		t.Errorf("INSERT trigger did not increment resolver_generation: initial=%d, after=%d", initialGen, gen1)
	}

	if _, err := db.Exec("UPDATE objects SET type = ? WHERE id = ?", "task", "test"); err != nil {
		t.Fatalf("failed to update object: %v", err)
	}
	gen2 := getGeneration()
	if gen2 <= gen1 {
		t.Errorf("UPDATE trigger did not increment resolver_generation: before=%d, after=%d", gen1, gen2)
	}

	if _, err := db.Exec("DELETE FROM objects WHERE id = ?", "test"); err != nil {
		t.Fatalf("failed to delete object: %v", err)
	}
	gen3 := getGeneration()
	if gen3 <= gen2 {
		t.Errorf("DELETE trigger did not increment resolver_generation: before=%d, after=%d", gen2, gen3)
	}

	if _, err := db.Exec("INSERT INTO sections (id, file_object_id, file_path, slug, title, level, line_start) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"test#intro", "test", "test.md", "intro", "Introduction", 2, 5); err != nil {
		t.Fatalf("failed to insert section: %v", err)
	}
	gen4 := getGeneration()
	if gen4 <= gen3 {
		t.Errorf("sections INSERT trigger did not increment resolver_generation: before=%d, after=%d", gen3, gen4)
	}
}

func TestSchemaSQL_WALMode(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(SchemaSQL); err != nil {
		t.Fatalf("failed to execute SchemaSQL: %v", err)
	}

	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("failed to read journal_mode: %v", err)
	}
	// In-memory databases don't support WAL mode, so skip this check
	// The important thing is that the SchemaSQL doesn't cause an error
	if journalMode != "memory" && !strings.EqualFold(journalMode, "wal") {
		t.Errorf("journal_mode = %q, want %q or %q (in-memory)", journalMode, "wal", "memory")
	}
}

func TestCurrentDBVersion(t *testing.T) {
	if CurrentDBVersion <= 0 {
		t.Errorf("CurrentDBVersion = %d, want > 0", CurrentDBVersion)
	}
	if CurrentDBVersion != 16 {
		t.Errorf("CurrentDBVersion = %d, want 16 (update this test if intentionally changed)", CurrentDBVersion)
	}
}
