package readsvc

import (
	"testing"
)

func TestReadSectionsReturnsFullOutline(t *testing.T) {
	t.Parallel()

	rt := seededSectionRuntime(t)

	result, err := ReadSections(rt, "note/example")
	if err != nil {
		t.Fatalf("ReadSections failed: %v", err)
	}
	if result.ObjectID != "note/example" || result.Path != "note/example.md" {
		t.Fatalf("result = %#v, want note/example", result)
	}
	if len(result.Sections) != 3 {
		t.Fatalf("sections = %#v, want 3 entries", result.Sections)
	}
	if result.Sections[0].ID != "note/example#parent" || result.Sections[0].Level != 1 {
		t.Fatalf("first section = %#v, want parent at level 1", result.Sections[0])
	}
	if result.Sections[1].ID != "note/example#child" || result.Sections[1].ParentSectionID == nil || *result.Sections[1].ParentSectionID != "note/example#parent" {
		t.Fatalf("second section = %#v, want child of parent", result.Sections[1])
	}
	if result.Sections[2].ID != "note/example#next" {
		t.Fatalf("third section = %#v, want next", result.Sections[2])
	}
}

func TestReadSectionsScopesToSectionSubtree(t *testing.T) {
	t.Parallel()

	rt := seededSectionRuntime(t)

	result, err := ReadSections(rt, "note/example#parent")
	if err != nil {
		t.Fatalf("ReadSections failed: %v", err)
	}
	if len(result.Sections) != 2 {
		t.Fatalf("sections = %#v, want parent and child only", result.Sections)
	}
	if result.Sections[0].ID != "note/example#parent" || result.Sections[1].ID != "note/example#child" {
		t.Fatalf("sections = %#v, want [parent, child]", result.Sections)
	}
}
