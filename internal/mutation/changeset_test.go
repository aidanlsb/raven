package mutation

import (
	"reflect"
	"testing"
)

func TestChangeSetMergeAndProjectionPaths(t *testing.T) {
	t.Parallel()

	first := NewChangeSet()
	first.AddChanged("./notes/one.md", "notes/one.md")
	first.AddDeleted("archive/gone.md")
	first.AddMoved("people/old.md", "people/new.md")

	second := NewChangeSet()
	second.AddChanged("notes/two.md", "people/new.md", "assets/old.pdf")
	second.AddDeleted("archive/gone.md", "archive/other.md")
	second.AddMoved("people/old.md", "people/new.md")
	second.AddMoved("assets/old.pdf", "assets/new.pdf")

	first.Merge(second)

	if got, want := first.Changed, []string{"notes/one.md", "notes/two.md", "people/new.md", "assets/old.pdf"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Changed = %#v, want %#v", got, want)
	}
	if got, want := first.Deleted, []string{"archive/gone.md", "archive/other.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Deleted = %#v, want %#v", got, want)
	}
	if got, want := first.Moved, []Move{
		{From: "people/old.md", To: "people/new.md"},
		{From: "assets/old.pdf", To: "assets/new.pdf"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Moved = %#v, want %#v", got, want)
	}
	if got, want := first.IndexPaths(), []string{
		"notes/one.md",
		"notes/two.md",
		"people/new.md",
		"assets/old.pdf",
		"assets/new.pdf",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("IndexPaths() = %#v, want %#v", got, want)
	}
	if got, want := first.RemovedPaths(), []string{
		"archive/gone.md",
		"archive/other.md",
		"people/old.md",
		"assets/old.pdf",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("RemovedPaths() = %#v, want %#v", got, want)
	}
}

func TestChangeSetIgnoresInvalidPaths(t *testing.T) {
	t.Parallel()

	changes := NewChangeSet()
	changes.AddChanged("", ".", "../outside.md")
	changes.AddDeleted(" ")
	changes.AddMoved("valid.md", "../outside.md")

	if !changes.Empty() {
		t.Fatalf("ChangeSet = %#v, want empty", changes)
	}
}

func TestChangeSetRejectsCrossPlatformAbsolutePaths(t *testing.T) {
	t.Parallel()

	tests := []string{
		"/absolute.md",
		`\absolute.md`,
		`C:\absolute.md`,
		"C:/absolute.md",
		`\\server\share\file.md`,
		"//server/share/file.md",
	}
	for _, filePath := range tests {
		t.Run(filePath, func(t *testing.T) {
			t.Parallel()

			changes := NewChangeSet()
			changes.AddChanged(filePath)
			if !changes.Empty() {
				t.Fatalf("ChangeSet = %#v, want empty for absolute path %q", changes, filePath)
			}
		})
	}
}

func TestChangeSetIndexPathsIncludesReusedMoveDestination(t *testing.T) {
	t.Parallel()

	changes := NewChangeSet()
	changes.AddMoved("b.md", "c.md")
	changes.AddMoved("a.md", "b.md")

	if got, want := changes.IndexPaths(), []string{"c.md", "b.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("IndexPaths() = %#v, want %#v", got, want)
	}
}

func TestChangeSetIndexPathsIncludesRecreatedMoveSource(t *testing.T) {
	t.Parallel()

	changes := NewChangeSet()
	changes.AddMoved("a.md", "b.md")
	changes.AddChanged("a.md")

	if got, want := changes.IndexPaths(), []string{"a.md", "b.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("IndexPaths() = %#v, want %#v", got, want)
	}
}
