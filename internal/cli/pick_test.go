package cli

import (
	"strings"
	"testing"
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
