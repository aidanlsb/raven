package readsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
)

func TestReadSectionDefaultsToSubtreeRange(t *testing.T) {
	t.Parallel()

	rt := seededSectionRuntime(t)

	result, err := Read(rt, ReadRequest{Reference: "note/example#parent"})
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	want := "# Parent\nintro\n## Child\nchild\n"
	if result.Content != want {
		t.Fatalf("Content = %q, want %q", result.Content, want)
	}
	if result.StartLine != 1 || result.EndLine != 4 {
		t.Fatalf("range = %d-%d, want 1-4", result.StartLine, result.EndLine)
	}
	if result.ObjectID != "note/example#parent" {
		t.Fatalf("ObjectID = %q, want section ID", result.ObjectID)
	}
}

func TestReadSectionExplicitRangeWins(t *testing.T) {
	t.Parallel()

	rt := seededSectionRuntime(t)

	result, err := Read(rt, ReadRequest{
		Reference: "note/example#parent",
		Raw:       true,
		StartLine: 2,
		EndLine:   2,
	})
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if result.Content != "intro\n" {
		t.Fatalf("Content = %q, want explicit line", result.Content)
	}
	if result.StartLine != 2 || result.EndLine != 2 {
		t.Fatalf("range = %d-%d, want 2-2", result.StartLine, result.EndLine)
	}
}

func TestResolveOpenTargetIncludesSectionLine(t *testing.T) {
	t.Parallel()

	rt := seededSectionRuntime(t)

	target, err := ResolveOpenTarget(rt, "note/example#child")
	if err != nil {
		t.Fatalf("ResolveOpenTarget failed: %v", err)
	}
	if !target.IsSection || target.ObjectID != "note/example#child" || target.FileObjectID != "note/example" {
		t.Fatalf("target = %#v, want child section", target)
	}
	if target.LineStart != 3 {
		t.Fatalf("LineStart = %d, want 3", target.LineStart)
	}
}

func seededSectionRuntime(t *testing.T) *Runtime {
	t.Helper()

	vaultPath := t.TempDir()
	notePath := filepath.Join(vaultPath, "note", "example.md")
	if err := os.MkdirAll(filepath.Dir(notePath), 0o755); err != nil {
		t.Fatalf("create note directory: %v", err)
	}
	content := "# Parent\nintro\n## Child\nchild\n# Next\nnext\n"
	if err := os.WriteFile(notePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatalf("open in-memory index: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.DB().Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields) VALUES
			('note/example', 'note/example.md', 'note', 1, '{}');

		INSERT INTO sections (id, file_object_id, file_path, slug, title, level, line_start, line_end, subtree_line_end, parent_section_id) VALUES
			('note/example#parent', 'note/example', 'note/example.md', 'parent', 'Parent', 1, 1, 2, 4, NULL),
			('note/example#child', 'note/example', 'note/example.md', 'child', 'Child', 2, 3, 4, 4, 'note/example#parent'),
			('note/example#next', 'note/example', 'note/example.md', 'next', 'Next', 1, 5, NULL, NULL, NULL);
	`)
	if err != nil {
		t.Fatalf("seed index: %v", err)
	}

	return &Runtime{
		VaultPath: vaultPath,
		VaultCfg:  &config.VaultConfig{},
		DB:        db,
	}
}
