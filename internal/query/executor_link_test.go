package query

import (
	"reflect"
	"testing"
)

func TestExecuteLinkQuery(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)
	tests := []struct {
		name       string
		query      string
		wantTarget []string
	}{
		{
			name:       "extension equality within type",
			query:      "link .ext==pdf within(type:project)",
			wantTarget: []string{"../assets/spec.pdf", "../assets/report.pdf"},
		},
		{
			name:       "boolean image field",
			query:      "link .is_image==true",
			wantTarget: []string{"../assets/diagram.png"},
		},
		{
			name:       "URL scheme",
			query:      "link .scheme==url",
			wantTarget: []string{"https://example.com"},
		},
		{
			name:       "raw target string function",
			query:      `link endswith(.raw_target, ".pdf")`,
			wantTarget: []string{"../assets/spec.pdf", "../assets/report.pdf"},
		},
		{
			name:       "display string function",
			query:      `link includes(.display, "gram")`,
			wantTarget: []string{"../assets/diagram.png"},
		},
		{
			name:       "section scope",
			query:      "link within(section .title==Tasks)",
			wantTarget: []string{"../assets/spec.pdf", "../assets/diagram.png"},
		},
		{
			name:       "section scope and extension",
			query:      "link .ext==pdf within(section .title==Tasks)",
			wantTarget: []string{"../assets/spec.pdf"},
		},
		{
			name:       "nested type predicate",
			query:      "link within(type:project .status==active)",
			wantTarget: []string{"../assets/report.pdf", "../assets/diagram.png", "https://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.query, err)
			}
			rows, err := executor.ExecuteLinkQuery(q)
			if err != nil {
				t.Fatalf("ExecuteLinkQuery(%q): %v", tt.query, err)
			}
			got := make([]string, 0, len(rows))
			for _, row := range rows {
				got = append(got, row.RawTarget)
			}
			if !reflect.DeepEqual(got, tt.wantTarget) {
				t.Fatalf("targets = %#v, want %#v", got, tt.wantTarget)
			}
		})
	}
}

func TestExecuteLinkQueryModes(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	defer db.Close()

	executor := NewExecutor(db)
	q, err := Parse("link .ext==pdf within(type:project)")
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}

	ids, err := executor.ExecuteLinkIDQuery(q, 2, 0)
	if err != nil {
		t.Fatalf("ExecuteLinkIDQuery(): %v", err)
	}
	if want := []string{"projects/mobile", "projects/website"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("source IDs = %#v, want %#v", ids, want)
	}

	count, err := executor.ExecuteLinkCountQuery(q)
	if err != nil {
		t.Fatalf("ExecuteLinkCountQuery(): %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	page, err := executor.ExecuteLinkPageQuery(q, 1, 1)
	if err != nil {
		t.Fatalf("ExecuteLinkPageQuery(): %v", err)
	}
	if len(page) != 1 || page[0].RawTarget != "../assets/report.pdf" {
		t.Fatalf("page = %#v, want report PDF edge", page)
	}
}
