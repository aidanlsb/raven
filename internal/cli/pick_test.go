package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/picker"
)

func TestReadPickItemsParsesPipeableRows(t *testing.T) {
	input := strings.Join([]string{
		"1\tproject/raven\tRaven\tprojects/raven.md:1",
		"2\tproject/cursor\tCursor\tprojects/cursor.md:2",
		"",
	}, "\n")

	items, err := readPickItems(strings.NewReader(input))
	if err != nil {
		t.Fatalf("readPickItems() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("item count = %d, want 2", len(items))
	}
	if items[0].ID != "project/raven" {
		t.Fatalf("id = %q, want project/raven", items[0].ID)
	}
	if items[0].Label != "Raven" {
		t.Fatalf("label = %q, want Raven", items[0].Label)
	}
	if items[0].Location != "projects/raven.md:1" {
		t.Fatalf("location = %q, want projects/raven.md:1", items[0].Location)
	}
	if strings.Join(items[0].Columns, "|") != "Raven|project/raven|projects/raven.md:1" {
		t.Fatalf("columns = %#v", items[0].Columns)
	}
	if items[0].FilePath != "projects/raven.md" || items[0].Line != 1 {
		t.Fatalf("preview location = %q:%d, want projects/raven.md:1", items[0].FilePath, items[0].Line)
	}
}

func TestPickItemFromLineAcceptsRawID(t *testing.T) {
	item, ok := pickItemFromLine("project/raven")
	if !ok {
		t.Fatalf("expected raw ID item")
	}
	if item.ID != "project/raven" || item.Label != "project/raven" {
		t.Fatalf("item = %#v, want raw ID label", item)
	}
}

func TestPickItemFromLineSkipsEmptyIDs(t *testing.T) {
	_, ok := pickItemFromLine("1\t\tcontent\tlocation")
	if ok {
		t.Fatalf("expected empty ID row to be skipped")
	}
}

func TestParsePreviewLocation(t *testing.T) {
	path, line := parsePreviewLocation("notes/todo.md:42")
	if path != "notes/todo.md" || line != 42 {
		t.Fatalf("location = %q:%d, want notes/todo.md:42", path, line)
	}

	path, line = parsePreviewLocation("notes/todo.md")
	if path != "notes/todo.md" || line != 0 {
		t.Fatalf("location without line = %q:%d, want notes/todo.md:0", path, line)
	}
}

func TestVaultFilePreviewReturnsLineExcerpt(t *testing.T) {
	vaultPath := t.TempDir()
	relPath := filepath.Join("notes", "todo.md")
	absPath := filepath.Join(vaultPath, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(absPath, []byte("one\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatalf("write preview file: %v", err)
	}

	preview, err := vaultFilePreview(vaultPath)(pickerItemForPreview(relPath, 3))
	if err != nil {
		t.Fatalf("vaultFilePreview() error = %v", err)
	}
	if preview.Title != relPath+":3" {
		t.Fatalf("preview title = %q, want %q", preview.Title, relPath+":3")
	}
	for _, want := range []string{" 1 │ one", ">    3 │ three"} {
		if !strings.Contains(preview.Content, want) {
			t.Fatalf("preview missing %q:\n%s", want, preview.Content)
		}
	}
}

func pickerItemForPreview(path string, line int) picker.Item {
	return picker.Item{FilePath: path, Line: line}
}
