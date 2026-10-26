package indexschema

import (
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestTryResolveExactID(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	insertObject(t, db, "people/ada", "people/ada.md", "person")
	insertObjectWithAlias(t, db, "people/grace", "people/grace.md", "person", "The Admiral")
	insertSection(t, db, "people/ada#notes", "people/ada", "people/ada.md", "notes", "Notes", 2, 8)

	sch := schema.New()
	sch.Types["person"] = &schema.TypeDefinition{NameField: "name"}

	t.Run("canonical object id", func(t *testing.T) {
		id, ok, err := TryResolveExactID(db, sch, "daily", "people/ada")
		if err != nil {
			t.Fatalf("TryResolveExactID: %v", err)
		}
		if !ok || id != "people/ada" {
			t.Fatalf("got id=%q ok=%v, want people/ada", id, ok)
		}
	})

	t.Run("canonical section id", func(t *testing.T) {
		id, ok, err := TryResolveExactID(db, sch, "daily", "people/ada#notes")
		if err != nil {
			t.Fatalf("TryResolveExactID: %v", err)
		}
		if !ok || id != "people/ada#notes" {
			t.Fatalf("got id=%q ok=%v, want people/ada#notes", id, ok)
		}
	})

	t.Run("short name falls back", func(t *testing.T) {
		_, ok, err := TryResolveExactID(db, sch, "daily", "ada")
		if err != nil {
			t.Fatalf("TryResolveExactID: %v", err)
		}
		if ok {
			t.Fatal("short name should not resolve via exact-id lookup")
		}
	})

	t.Run("missing id falls back", func(t *testing.T) {
		_, ok, err := TryResolveExactID(db, sch, "daily", "people/missing")
		if err != nil {
			t.Fatalf("TryResolveExactID: %v", err)
		}
		if ok {
			t.Fatal("missing id should not resolve")
		}
	})

	t.Run("name field collision falls back", func(t *testing.T) {
		if _, err := db.Exec(
			`INSERT INTO objects (id, file_path, type, fields, line_start) VALUES (?, ?, ?, ?, ?)`,
			"people/other", "people/other.md", "person", `{"name":"people/ada"}`, 1,
		); err != nil {
			t.Fatalf("insert name-field collision: %v", err)
		}
		_, ok, err := TryResolveExactID(db, sch, "daily", "people/ada")
		if err != nil {
			t.Fatalf("TryResolveExactID: %v", err)
		}
		if ok {
			t.Fatal("name-field collision should not take the exact-id fast path")
		}
	})
}

func TestAllAliasDataSingleQuery(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	insertObjectWithAlias(t, db, "people/ada", "people/ada.md", "person", "Ada")
	insertObjectWithAlias(t, db, "people/grace", "people/grace.md", "person", "Ada")

	aliases, matches, err := AllAliasData(db)
	if err != nil {
		t.Fatalf("AllAliasData: %v", err)
	}
	if aliases["Ada"] != "people/ada" {
		t.Fatalf("first alias = %q, want people/ada", aliases["Ada"])
	}
	if len(matches["Ada"]) != 2 {
		t.Fatalf("alias matches = %#v, want 2", matches["Ada"])
	}
}
