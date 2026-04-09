package traitsvc

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestResolveTraitIDs(t *testing.T) {
	t.Parallel()
	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.DB().Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields) VALUES
			('page/task-list', 'notes/tasks.md', 'page', 1, '{}')
	`)
	if err != nil {
		t.Fatalf("failed to seed object: %v", err)
	}
	_, err = db.DB().Exec(`
		INSERT INTO traits (id, trait_type, value, content, file_path, line_number, parent_object_id) VALUES
			('notes/tasks.md:trait:0', 'todo', 'open', 'Task A', 'notes/tasks.md', 1, 'page/task-list')
	`)
	if err != nil {
		t.Fatalf("failed to seed trait: %v", err)
	}

	traits, skipped, err := ResolveTraitIDs(db, []string{
		"bad-id",
		"notes/missing.md:trait:0",
		"notes/tasks.md:trait:0",
	})
	if err != nil {
		t.Fatalf("ResolveTraitIDs returned error: %v", err)
	}

	if len(traits) != 1 || traits[0].ID != "notes/tasks.md:trait:0" {
		t.Fatalf("unexpected resolved traits: %#v", traits)
	}
	if len(skipped) != 2 {
		t.Fatalf("expected 2 skipped ids, got %d (%#v)", len(skipped), skipped)
	}
	if skipped[0].Reason != "invalid trait ID format" {
		t.Fatalf("unexpected first skip reason: %q", skipped[0].Reason)
	}
	if !strings.Contains(skipped[1].Reason, "trait not found") {
		t.Fatalf("unexpected second skip reason: %q", skipped[1].Reason)
	}
}

func TestBuildPreviewSkipsUnchangedValues(t *testing.T) {
	t.Parallel()
	existing := "done"
	traits := []model.Trait{{
		ID:        "notes/tasks.md:trait:0",
		TraitType: "todo",
		Value:     &existing,
		Content:   "- [x] done",
		FilePath:  "notes/tasks.md",
		Line:      1,
	}}
	sch := schema.New()
	sch.Traits["todo"] = &schema.TraitDefinition{
		Type:   schema.FieldTypeEnum,
		Values: []string{"open", "done"},
	}

	preview, err := BuildPreview(traits, "done", sch, nil)
	if err != nil {
		t.Fatalf("BuildPreview returned error: %v", err)
	}
	if len(preview.Items) != 0 {
		t.Fatalf("expected no preview items, got %#v", preview.Items)
	}
	if len(preview.Skipped) != 1 {
		t.Fatalf("expected one skipped item, got %#v", preview.Skipped)
	}
	if preview.Skipped[0].Reason != "already has target value" {
		t.Fatalf("unexpected skip reason: %q", preview.Skipped[0].Reason)
	}
}

func TestBuildPreviewResolvesRelativeDate(t *testing.T) {
	t.Parallel()
	traits := []model.Trait{{
		ID:        "daily/2026-03-15.md:trait:0",
		TraitType: "due",
		Content:   "@due(today)",
		FilePath:  "daily/2026-03-15.md",
		Line:      1,
	}}
	sch := schema.New()
	sch.Traits["due"] = &schema.TraitDefinition{Type: schema.FieldTypeDate}

	preview, err := BuildPreview(traits, "today", sch, nil)
	if err != nil {
		t.Fatalf("BuildPreview returned error: %v", err)
	}
	if len(preview.Items) != 1 {
		t.Fatalf("expected one preview item, got %#v", preview.Items)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(preview.Items[0].NewValue) {
		t.Fatalf("expected resolved date, got %q", preview.Items[0].NewValue)
	}
}

func TestApplyUpdatesModifiesFile(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	filePath := filepath.Join(vaultPath, "notes", "tasks.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("failed to create fixture dir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("- [ ] Task @todo(open)\n"), 0o644); err != nil {
		t.Fatalf("failed to write fixture file: %v", err)
	}

	existing := "open"
	traits := []model.Trait{{
		ID:        "notes/tasks.md:trait:0",
		TraitType: "todo",
		Value:     &existing,
		Content:   "- [ ] Task",
		FilePath:  "notes/tasks.md",
		Line:      1,
	}}
	sch := schema.New()
	sch.Traits["todo"] = &schema.TraitDefinition{
		Type:   schema.FieldTypeEnum,
		Values: []string{"open", "done"},
	}

	summary, err := ApplyUpdates(vaultPath, traits, "done", sch, nil)
	if err != nil {
		t.Fatalf("ApplyUpdates returned error: %v", err)
	}
	if summary.Modified != 1 || summary.Errors != 0 || summary.Skipped != 0 {
		t.Fatalf("unexpected summary counters: %#v", summary)
	}
	if len(summary.ChangedFilePaths) != 1 {
		t.Fatalf("expected one changed file path, got %#v", summary.ChangedFilePaths)
	}
	if summary.Results[0].OldValue != "open" || summary.Results[0].NewValue != "done" {
		t.Fatalf("unexpected update result values: %#v", summary.Results[0])
	}

	updated, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed reading updated file: %v", err)
	}
	if !strings.Contains(string(updated), "@todo(done)") {
		t.Fatalf("expected updated trait value in file, got %q", string(updated))
	}
}

func TestResolvedAndValidatedTraitValueValidationError(t *testing.T) {
	t.Parallel()
	sch := schema.New()
	sch.Traits["priority"] = &schema.TraitDefinition{
		Type:   schema.FieldTypeEnum,
		Values: []string{"high", "low"},
	}

	_, err := resolvedAndValidatedTraitValue("medium", "priority", sch)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	var valueErr *ValueValidationError
	if !errors.As(err, &valueErr) {
		t.Fatalf("expected ValueValidationError, got %T: %v", err, err)
	}
	if valueErr.TraitType != "priority" {
		t.Fatalf("trait type = %q, want %q", valueErr.TraitType, "priority")
	}
	if !strings.Contains(valueErr.Suggestion(), "@priority") {
		t.Fatalf("unexpected suggestion: %q", valueErr.Suggestion())
	}
}

func TestTraitExistingValue_IgnoresNilTraitDefinition(t *testing.T) {
	t.Parallel()

	sch := schema.New()
	sch.Traits["todo"] = nil

	got := traitExistingValue(sch, model.Trait{TraitType: "todo"})
	if got != "" {
		t.Fatalf("traitExistingValue() = %q, want empty string", got)
	}
}
