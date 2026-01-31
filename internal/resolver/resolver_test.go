package resolver

import (
	"testing"
)

func TestResolver(t *testing.T) {
	objectIDs := []string{
		"people/freya",
		"people/thor",
		"projects/bifrost",
		"daily/2025-02-01",
		"daily/2025-02-01#standup",
	}

	r := New(objectIDs, Options{})

	t.Run("resolve full path", func(t *testing.T) {
		result := r.Resolve("people/freya")
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
		if result.Ambiguous {
			t.Error("expected not ambiguous")
		}
	})

	t.Run("resolve short name", func(t *testing.T) {
		result := r.Resolve("bifrost")
		if result.TargetID != "projects/bifrost" {
			t.Errorf("got %q, want %q", result.TargetID, "projects/bifrost")
		}
	})

	t.Run("resolve embedded object", func(t *testing.T) {
		result := r.Resolve("daily/2025-02-01#standup")
		if result.TargetID != "daily/2025-02-01#standup" {
			t.Errorf("got %q, want %q", result.TargetID, "daily/2025-02-01#standup")
		}
	})

	t.Run("short name for embedded object", func(t *testing.T) {
		result := r.Resolve("standup")
		if result.TargetID != "daily/2025-02-01#standup" {
			t.Errorf("got %q, want %q", result.TargetID, "daily/2025-02-01#standup")
		}
	})

	t.Run("short name with embedded fragment resolves", func(t *testing.T) {
		// Should resolve via suffix matching: "website#tasks" -> "projects/website#tasks"
		r2 := New([]string{"projects/website#tasks"}, Options{})
		result := r2.Resolve("website#tasks")
		if result.TargetID != "projects/website#tasks" {
			t.Errorf("got %q, want %q", result.TargetID, "projects/website#tasks")
		}
		if result.Ambiguous {
			t.Errorf("expected not ambiguous, got matches: %v", result.Matches)
		}
	})

	t.Run("not found", func(t *testing.T) {
		result := r.Resolve("nonexistent")
		if result.TargetID != "" {
			t.Errorf("expected empty target, got %q", result.TargetID)
		}
		if result.Error == "" {
			t.Error("expected error message")
		}
	})
}

func TestResolverAmbiguous(t *testing.T) {
	// Two objects with the same short name
	objectIDs := []string{
		"people/freya",
		"clients/freya",
	}

	r := New(objectIDs, Options{})

	result := r.Resolve("freya")
	if !result.Ambiguous {
		t.Error("expected ambiguous")
	}
	if len(result.Matches) != 2 {
		t.Errorf("expected 2 matches, got %d", len(result.Matches))
	}
}

func TestResolverSuffixMatching(t *testing.T) {
	// When objects have a directory prefix (e.g., "objects/") but the user
	// references by a partial path, suffix matching should find them.
	t.Run("partial path matches full path with prefix", func(t *testing.T) {
		objectIDs := []string{
			"objects/companies/cursor",
			"objects/companies/cursor#notes",
			"objects/people/freya",
		}

		r := New(objectIDs, Options{})

		// "companies/cursor" should resolve to "objects/companies/cursor"
		result := r.Resolve("companies/cursor")
		if result.Ambiguous {
			t.Errorf("expected non-ambiguous, got ambiguous with matches: %v", result.Matches)
		}
		if result.TargetID != "objects/companies/cursor" {
			t.Errorf("got %q, want %q", result.TargetID, "objects/companies/cursor")
		}
	})

	t.Run("short name still works with prefix", func(t *testing.T) {
		objectIDs := []string{
			"objects/companies/cursor",
			"objects/companies/cursor#cursor",
		}

		r := New(objectIDs, Options{})

		// "cursor" should still resolve via short name matching
		result := r.Resolve("cursor")
		if result.Ambiguous {
			t.Errorf("expected non-ambiguous, got ambiguous with matches: %v", result.Matches)
		}
		if result.TargetID != "objects/companies/cursor" {
			t.Errorf("got %q, want %q", result.TargetID, "objects/companies/cursor")
		}
	})

	t.Run("exact match takes priority over suffix match", func(t *testing.T) {
		objectIDs := []string{
			"objects/companies/cursor",
			"companies/cursor", // Also exists as a standalone
		}

		r := New(objectIDs, Options{})

		// "companies/cursor" should resolve to the exact match
		result := r.Resolve("companies/cursor")
		if result.Ambiguous {
			t.Errorf("expected non-ambiguous, got ambiguous with matches: %v", result.Matches)
		}
		if result.TargetID != "companies/cursor" {
			t.Errorf("got %q, want %q", result.TargetID, "companies/cursor")
		}
	})

	t.Run("ambiguous suffix matches are detected", func(t *testing.T) {
		objectIDs := []string{
			"objects/companies/cursor",
			"pages/companies/cursor", // Two different prefixes with same suffix
		}

		r := New(objectIDs, Options{})

		// "companies/cursor" matches both, should be ambiguous
		result := r.Resolve("companies/cursor")
		if !result.Ambiguous {
			t.Error("expected ambiguous when multiple suffix matches exist")
		}
		if len(result.Matches) != 2 {
			t.Errorf("expected 2 matches, got %d", len(result.Matches))
		}
	})
}

func TestResolverPrefersParentOverSection(t *testing.T) {
	// When a file and its section have the same short name (e.g., "cursor"),
	// the resolver should prefer the parent file object.
	t.Run("parent object is preferred over section with same short name", func(t *testing.T) {
		objectIDs := []string{
			"companies/cursor",        // parent file object
			"companies/cursor#cursor", // section "# Cursor" inside the file
		}

		r := New(objectIDs, Options{})

		// [[cursor]] should resolve to companies/cursor, not be ambiguous
		result := r.Resolve("cursor")
		if result.Ambiguous {
			t.Errorf("expected non-ambiguous, got ambiguous with matches: %v", result.Matches)
		}
		if result.TargetID != "companies/cursor" {
			t.Errorf("got %q, want %q", result.TargetID, "companies/cursor")
		}
	})

	t.Run("section can still be resolved with full path", func(t *testing.T) {
		objectIDs := []string{
			"companies/cursor",
			"companies/cursor#cursor",
		}

		r := New(objectIDs, Options{})

		result := r.Resolve("companies/cursor#cursor")
		if result.Ambiguous {
			t.Error("full path should not be ambiguous")
		}
		if result.TargetID != "companies/cursor#cursor" {
			t.Errorf("got %q, want %q", result.TargetID, "companies/cursor#cursor")
		}
	})

	t.Run("section with unique short name still works", func(t *testing.T) {
		objectIDs := []string{
			"companies/cursor",
			"companies/cursor#notes", // section with different short name
		}

		r := New(objectIDs, Options{})

		// [[notes]] should resolve to the section
		result := r.Resolve("notes")
		if result.Ambiguous {
			t.Error("unique section short name should not be ambiguous")
		}
		if result.TargetID != "companies/cursor#notes" {
			t.Errorf("got %q, want %q", result.TargetID, "companies/cursor#notes")
		}
	})

	t.Run("two unrelated sections with same name are still ambiguous", func(t *testing.T) {
		objectIDs := []string{
			"companies/cursor#notes",
			"people/freya#notes",
		}

		r := New(objectIDs, Options{})

		result := r.Resolve("notes")
		if !result.Ambiguous {
			t.Error("expected ambiguous when two unrelated sections have same name")
		}
		if len(result.Matches) != 2 {
			t.Errorf("expected 2 matches, got %d", len(result.Matches))
		}
	})

	t.Run("parent preferred even with multiple sections", func(t *testing.T) {
		objectIDs := []string{
			"companies/cursor",
			"companies/cursor#cursor",
			"companies/cursor#notes",
			"companies/cursor#history",
		}

		r := New(objectIDs, Options{})

		// [[cursor]] should resolve to companies/cursor
		result := r.Resolve("cursor")
		if result.Ambiguous {
			t.Errorf("expected non-ambiguous, got ambiguous with matches: %v", result.Matches)
		}
		if result.TargetID != "companies/cursor" {
			t.Errorf("got %q, want %q", result.TargetID, "companies/cursor")
		}
	})
}

func TestResolverDateShorthand(t *testing.T) {
	objectIDs := []string{
		"daily/2025-02-01",
		"people/freya",
	}

	r := New(objectIDs, Options{})

	t.Run("date reference to existing daily note", func(t *testing.T) {
		result := r.Resolve("2025-02-01")
		if result.TargetID != "daily/2025-02-01" {
			t.Errorf("got %q, want %q", result.TargetID, "daily/2025-02-01")
		}
	})

	t.Run("date reference to non-existent daily note", func(t *testing.T) {
		// Date references should resolve even if the daily note doesn't exist
		result := r.Resolve("2025-03-15")
		if result.TargetID != "daily/2025-03-15" {
			t.Errorf("got %q, want %q", result.TargetID, "daily/2025-03-15")
		}
	})

	t.Run("custom daily directory", func(t *testing.T) {
		r2 := New([]string{"journal/2025-02-01"}, Options{DailyDirectory: "journal"})
		result := r2.Resolve("2025-02-01")
		if result.TargetID != "journal/2025-02-01" {
			t.Errorf("got %q, want %q", result.TargetID, "journal/2025-02-01")
		}
	})

	t.Run("non-date string not treated as date", func(t *testing.T) {
		result := r.Resolve("freya")
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
	})
}

func TestResolverSlugifiedMatching(t *testing.T) {
	// Files are stored with slugified names
	objectIDs := []string{
		"people/sif",
		"people/freya",
		"projects/my-awesome-project",
	}

	r := New(objectIDs, Options{})

	t.Run("proper noun resolves to slugified file", func(t *testing.T) {
		// User writes [[people/Sif]] but file is sif.md
		result := r.Resolve("people/Sif")
		if result.TargetID != "people/sif" {
			t.Errorf("got %q, want %q", result.TargetID, "people/sif")
		}
	})

	t.Run("mixed case resolves to slugified file", func(t *testing.T) {
		result := r.Resolve("people/SIF")
		if result.TargetID != "people/sif" {
			t.Errorf("got %q, want %q", result.TargetID, "people/sif")
		}
	})

	t.Run("spaces and caps in project name", func(t *testing.T) {
		result := r.Resolve("projects/My Awesome Project")
		if result.TargetID != "projects/my-awesome-project" {
			t.Errorf("got %q, want %q", result.TargetID, "projects/my-awesome-project")
		}
	})

	t.Run("short name with spaces resolves", func(t *testing.T) {
		result := r.Resolve("Sif")
		if result.TargetID != "people/sif" {
			t.Errorf("got %q, want %q", result.TargetID, "people/sif")
		}
	})

	t.Run("exact match still works", func(t *testing.T) {
		result := r.Resolve("people/freya")
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
	})
}

func TestResolverAliases(t *testing.T) {
	objectIDs := []string{
		"people/freya",
		"people/thor",
		"companies/acme-corp",
	}

	aliases := map[string]string{
		"goddess":   "people/freya",
		"thunder":   "people/thor",
		"ACME":      "companies/acme-corp",
		"Acme Corp": "companies/acme-corp",
	}

	r := New(objectIDs, Options{Aliases: aliases})

	t.Run("resolve by alias", func(t *testing.T) {
		result := r.Resolve("goddess")
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
	})

	t.Run("alias takes priority over short name lookup", func(t *testing.T) {
		result := r.Resolve("thunder")
		if result.TargetID != "people/thor" {
			t.Errorf("got %q, want %q", result.TargetID, "people/thor")
		}
	})

	t.Run("case-insensitive alias matching", func(t *testing.T) {
		result := r.Resolve("GODDESS")
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
	})

	t.Run("alias with spaces", func(t *testing.T) {
		result := r.Resolve("Acme Corp")
		if result.TargetID != "companies/acme-corp" {
			t.Errorf("got %q, want %q", result.TargetID, "companies/acme-corp")
		}
	})

	t.Run("short form alias", func(t *testing.T) {
		result := r.Resolve("acme")
		if result.TargetID != "companies/acme-corp" {
			t.Errorf("got %q, want %q", result.TargetID, "companies/acme-corp")
		}
	})

	t.Run("original ID still works", func(t *testing.T) {
		result := r.Resolve("freya")
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
	})

	t.Run("full path still works with alias defined", func(t *testing.T) {
		result := r.Resolve("people/freya")
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
	})
}

func TestResolverWithOptions(t *testing.T) {
	objectIDs := []string{
		"people/freya",
		"projects/bifrost",
	}

	opts := Options{
		DailyDirectory: "journal",
		Aliases: map[string]string{
			"goddess": "people/freya",
		},
	}

	r := New(objectIDs, opts)

	t.Run("alias works with config", func(t *testing.T) {
		result := r.Resolve("goddess")
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
	})

	t.Run("daily directory from config", func(t *testing.T) {
		result := r.Resolve("2025-02-01")
		if result.TargetID != "journal/2025-02-01" {
			t.Errorf("got %q, want %q", result.TargetID, "journal/2025-02-01")
		}
	})

	t.Run("invalid date does not resolve as date shorthand", func(t *testing.T) {
		result := r.Resolve("2025-13-45")
		if result.TargetID == "journal/2025-13-45" {
			t.Errorf("expected invalid date not to be treated as date shorthand, got %q", result.TargetID)
		}
	})
}

func TestAliasConflicts(t *testing.T) {
	t.Run("alias conflicting with short name is ambiguous", func(t *testing.T) {
		// "thor" is both a short name for people/thor and an alias for people/freya
		objectIDs := []string{
			"people/freya",
			"people/thor",
		}
		aliases := map[string]string{
			"thor": "people/freya", // alias "thor" points to freya, but thor also exists!
		}

		r := New(objectIDs, Options{Aliases: aliases})

		// Should be ambiguous - not silently resolved
		result := r.Resolve("thor")
		if !result.Ambiguous {
			t.Error("expected ambiguous result when alias conflicts with short name")
		}
		if len(result.Matches) != 2 {
			t.Errorf("expected 2 matches, got %d", len(result.Matches))
		}

		// Full path should still work unambiguously for the actual thor
		result = r.Resolve("people/thor")
		if result.Ambiguous {
			t.Error("full path should not be ambiguous")
		}
		if result.TargetID != "people/thor" {
			t.Errorf("got %q, want %q", result.TargetID, "people/thor")
		}
	})

	t.Run("alias conflicting with object ID is ambiguous", func(t *testing.T) {
		// An alias that matches an actual object ID
		objectIDs := []string{
			"people/freya",
			"people/thor",
		}
		aliases := map[string]string{
			"people/thor": "people/freya", // alias "people/thor" points to freya!
		}

		r := New(objectIDs, Options{Aliases: aliases})

		// Should be ambiguous
		result := r.Resolve("people/thor")
		if !result.Ambiguous {
			t.Error("expected ambiguous result when alias conflicts with object ID")
		}
		if len(result.Matches) != 2 {
			t.Errorf("expected 2 matches, got %d", len(result.Matches))
		}
	})

	t.Run("detect alias conflicts with short names", func(t *testing.T) {
		objectIDs := []string{
			"people/freya",
			"people/thor",
		}
		aliases := map[string]string{
			"thor": "people/freya", // conflicts with people/thor's short name
		}

		r := New(objectIDs, Options{Aliases: aliases})
		collisions := r.FindAliasCollisions()

		if len(collisions) != 1 {
			t.Errorf("expected 1 collision, got %d", len(collisions))
		}
		if len(collisions) > 0 {
			if collisions[0].Alias != "thor" {
				t.Errorf("expected collision on alias 'thor', got %q", collisions[0].Alias)
			}
			if collisions[0].ConflictsWith != "short_name" {
				t.Errorf("expected conflict type 'short_name', got %q", collisions[0].ConflictsWith)
			}
		}
	})

	t.Run("detect alias conflicts with object IDs", func(t *testing.T) {
		objectIDs := []string{
			"people/freya",
			"people/thor",
		}
		aliases := map[string]string{
			"people/thor": "people/freya", // conflicts with actual object ID
		}

		r := New(objectIDs, Options{Aliases: aliases})
		collisions := r.FindAliasCollisions()

		found := false
		for _, c := range collisions {
			if c.Alias == "people/thor" && c.ConflictsWith == "object_id" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find collision with object_id")
		}
	})

	t.Run("no collision when alias is unique", func(t *testing.T) {
		objectIDs := []string{
			"people/freya",
			"people/thor",
		}
		aliases := map[string]string{
			"goddess": "people/freya", // unique alias
			"thunder": "people/thor",  // unique alias
		}

		r := New(objectIDs, Options{Aliases: aliases})
		collisions := r.FindAliasCollisions()

		if len(collisions) != 0 {
			t.Errorf("expected 0 collisions for unique aliases, got %d", len(collisions))
		}

		// Both should resolve unambiguously
		result := r.Resolve("goddess")
		if result.Ambiguous {
			t.Error("unique alias should not be ambiguous")
		}
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
	})

	t.Run("alias resolves when no conflict exists", func(t *testing.T) {
		objectIDs := []string{
			"people/freya",
			"people/thor",
		}
		aliases := map[string]string{
			"goddess": "people/freya", // unique - no object named "goddess"
		}

		r := New(objectIDs, Options{Aliases: aliases})

		result := r.Resolve("goddess")
		if result.Ambiguous {
			t.Error("unique alias should resolve unambiguously")
		}
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
	})
}

func TestAliasEdgeCases(t *testing.T) {
	t.Run("empty alias is ignored", func(t *testing.T) {
		objectIDs := []string{"people/freya"}
		aliases := map[string]string{
			"":        "people/freya", // empty alias should be ignored
			"goddess": "people/freya",
		}

		r := New(objectIDs, Options{Aliases: aliases})

		// Empty string should not resolve
		result := r.Resolve("")
		if result.TargetID != "" {
			t.Errorf("empty alias should not resolve, got %q", result.TargetID)
		}

		// Valid alias should work
		result = r.Resolve("goddess")
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
	})

	t.Run("alias with special characters", func(t *testing.T) {
		objectIDs := []string{"companies/acme-corp"}
		aliases := map[string]string{
			"ACME Inc.": "companies/acme-corp",
		}

		r := New(objectIDs, Options{Aliases: aliases})

		// Exact match should work
		result := r.Resolve("ACME Inc.")
		if result.TargetID != "companies/acme-corp" {
			t.Errorf("got %q, want %q", result.TargetID, "companies/acme-corp")
		}

		// Slugified version should also work
		result = r.Resolve("acme-inc")
		if result.TargetID != "companies/acme-corp" {
			t.Errorf("slugified alias should work, got %q, want %q", result.TargetID, "companies/acme-corp")
		}
	})

	t.Run("alias pointing to non-existent object", func(t *testing.T) {
		objectIDs := []string{"people/freya"}
		aliases := map[string]string{
			"ghost": "people/nonexistent", // target doesn't exist
		}

		r := New(objectIDs, Options{Aliases: aliases})

		// Alias should still resolve to the target even if target doesn't exist in objectIDs
		// (the alias map is independent - validation of target existence should happen elsewhere)
		result := r.Resolve("ghost")
		if result.Ambiguous {
			t.Error("alias to non-existent target should not be ambiguous")
		}
		if result.TargetID != "people/nonexistent" {
			t.Errorf("got %q, want %q", result.TargetID, "people/nonexistent")
		}
	})

	t.Run("nil aliases map is handled", func(t *testing.T) {
		objectIDs := []string{"people/freya"}

		r := New(objectIDs, Options{})

		// Should still resolve by short name
		result := r.Resolve("freya")
		if result.TargetID != "people/freya" {
			t.Errorf("got %q, want %q", result.TargetID, "people/freya")
		}
	})
}

func TestNameFieldResolution(t *testing.T) {
	objectIDs := []string{
		"books/the-prose-edda",
		"books/the-poetic-edda",
		"people/snorri-sturluson",
	}

	// Name field map: display name -> object ID
	nameFieldMap := map[string]string{
		"The Prose Edda":   "books/the-prose-edda",
		"The Poetic Edda":  "books/the-poetic-edda",
		"Snorri Sturluson": "people/snorri-sturluson",
	}

	r := New(objectIDs, Options{NameFieldMap: nameFieldMap})

	t.Run("resolve by name_field value - exact match", func(t *testing.T) {
		result := r.Resolve("The Prose Edda")
		if result.TargetID != "books/the-prose-edda" {
			t.Errorf("got %q, want %q", result.TargetID, "books/the-prose-edda")
		}
	})

	t.Run("resolve by name_field value - case insensitive", func(t *testing.T) {
		result := r.Resolve("the prose edda")
		if result.TargetID != "books/the-prose-edda" {
			t.Errorf("got %q, want %q", result.TargetID, "books/the-prose-edda")
		}
	})

	t.Run("resolve by name_field value - slugified", func(t *testing.T) {
		result := r.Resolve("the-poetic-edda")
		if result.TargetID != "books/the-poetic-edda" {
			t.Errorf("got %q, want %q", result.TargetID, "books/the-poetic-edda")
		}
	})

	t.Run("name_field takes precedence over filename for different names", func(t *testing.T) {
		// When name_field value is different from filename, name_field should match
		result := r.Resolve("Snorri Sturluson")
		if result.TargetID != "people/snorri-sturluson" {
			t.Errorf("got %q, want %q", result.TargetID, "people/snorri-sturluson")
		}
	})

	t.Run("fallback to filename when name_field doesn't match", func(t *testing.T) {
		// Short filename should still work
		result := r.Resolve("the-poetic-edda")
		if result.TargetID != "books/the-poetic-edda" {
			t.Errorf("got %q, want %q", result.TargetID, "books/the-poetic-edda")
		}
	})

	t.Run("nil name_field map is handled", func(t *testing.T) {
		r := New(objectIDs, Options{})

		// Should still resolve by short name
		result := r.Resolve("the-prose-edda")
		if result.TargetID != "books/the-prose-edda" {
			t.Errorf("got %q, want %q", result.TargetID, "books/the-prose-edda")
		}
	})
}

func TestFindCollisions(t *testing.T) {
	t.Run("file and its own section is not a collision", func(t *testing.T) {
		// people/freya and people/freya#freya both have short name "freya"
		// but this is not a real collision - [[freya]] resolves to the file
		objectIDs := []string{
			"people/freya",
			"people/freya#freya",
			"people/freya#notes",
		}
		r := New(objectIDs, Options{})

		collisions := r.FindCollisions()

		// Should not report "freya" as a collision
		for _, c := range collisions {
			if c.ShortName == "freya" {
				t.Errorf("should not report 'freya' as collision between file and its own section")
			}
		}
	})

	t.Run("sections in different files is a real collision", func(t *testing.T) {
		// notes section exists in both freya and thor files
		objectIDs := []string{
			"people/freya",
			"people/freya#notes",
			"people/thor",
			"people/thor#notes",
		}
		r := New(objectIDs, Options{})

		collisions := r.FindCollisions()

		// Should report "notes" as a collision
		foundNotes := false
		for _, c := range collisions {
			if c.ShortName == "notes" {
				foundNotes = true
				if len(c.ObjectIDs) != 2 {
					t.Errorf("expected 2 colliding IDs for 'notes', got %d", len(c.ObjectIDs))
				}
			}
		}
		if !foundNotes {
			t.Error("expected 'notes' collision between sections in different files")
		}
	})

	t.Run("two different files with same name is a collision", func(t *testing.T) {
		objectIDs := []string{
			"people/alex",
			"companies/alex",
		}
		r := New(objectIDs, Options{})

		collisions := r.FindCollisions()

		foundAlex := false
		for _, c := range collisions {
			if c.ShortName == "alex" {
				foundAlex = true
			}
		}
		if !foundAlex {
			t.Error("expected 'alex' collision between two different files")
		}
	})

	t.Run("section colliding with file in different path is a collision", func(t *testing.T) {
		objectIDs := []string{
			"projects/overview",
			"docs/readme#overview",
		}
		r := New(objectIDs, Options{})

		collisions := r.FindCollisions()

		foundOverview := false
		for _, c := range collisions {
			if c.ShortName == "overview" {
				foundOverview = true
			}
		}
		if !foundOverview {
			t.Error("expected 'overview' collision between file and section in different file")
		}
	})
}
