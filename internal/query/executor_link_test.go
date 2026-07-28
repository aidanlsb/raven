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
			wantTarget: []string{"../assets/spec.pdf", "../manual.pdf", "../assets/report.pdf", "docs/brief.pdf", "https://example.com/Guide.PDF"},
		},
		{
			name:       "boolean image field",
			query:      "link .is_image==true",
			wantTarget: []string{"../assets/diagram.png", "assets/wireframe.png"},
		},
		{
			name:       "URL scheme",
			query:      "link .scheme==url",
			wantTarget: []string{"https://example.com", "https://example.com", "https://example.com/Guide.PDF"},
		},
		{
			name:       "raw target string function",
			query:      `link endswith(.raw_target, ".pdf")`,
			wantTarget: []string{"../assets/spec.pdf", "../manual.pdf", "../assets/report.pdf", "docs/brief.pdf", "https://example.com/Guide.PDF"},
		},
		{
			name:       "display string function",
			query:      `link includes(.display, "gram")`,
			wantTarget: []string{"../assets/diagram.png"},
		},
		{
			name:       "section scope",
			query:      "link within(section .title==Tasks)",
			wantTarget: []string{"../assets/spec.pdf", "../manual.pdf", "../assets/diagram.png", "assets/wireframe.png"},
		},
		{
			name:       "section scope and extension",
			query:      "link .ext==pdf within(section .title==Tasks)",
			wantTarget: []string{"../assets/spec.pdf", "../manual.pdf"},
		},
		{
			name:       "nested type predicate",
			query:      "link within(type:project .status==active)",
			wantTarget: []string{"../assets/report.pdf", "docs/brief.pdf", "../assets/diagram.png", "assets/wireframe.png", "https://example.com", "https://example.com/Guide.PDF"},
		},
		{
			name:       "source type field",
			query:      "link .source_type==person",
			wantTarget: []string{"https://example.com"},
		},
		{
			name:       "numeric line field",
			query:      "link .line>=50",
			wantTarget: []string{"https://example.com", "https://example.com/Guide.PDF"},
		},
		{
			name:       "numeric position field",
			query:      "link .position_start>=8",
			wantTarget: []string{"https://example.com/Guide.PDF"},
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
	if want := []string{"projects/mobile", "projects/mobile"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("source IDs = %#v, want %#v", ids, want)
	}

	count, err := executor.ExecuteLinkCountQuery(q)
	if err != nil {
		t.Fatalf("ExecuteLinkCountQuery(): %v", err)
	}
	if count != 5 {
		t.Fatalf("count = %d, want 5", count)
	}

	page, err := executor.ExecuteLinkPageQuery(q, 1, 1)
	if err != nil {
		t.Fatalf("ExecuteLinkPageQuery(): %v", err)
	}
	if len(page) != 1 || page[0].RawTarget != "../manual.pdf" {
		t.Fatalf("page = %#v, want manual PDF edge", page)
	}
}

func TestExecuteLinkQuery_CaseSensitiveIdentityFields(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	defer db.Close()
	_, err := db.Exec(`
		INSERT INTO links (
			source_id, source_type, file_path, line_number, position_start, position_end,
			raw_target, display, is_image, scheme, ext, normalized_key
		) VALUES (
			'projects/website', 'project', 'projects/website.md', 56, 0, 51,
			'https://example.com/guide.pdf', 'lowercase guide', 0, 'url', 'pdf',
			'https://example.com/guide.pdf'
		)
	`)
	if err != nil {
		t.Fatalf("insert case-variant link: %v", err)
	}

	executor := NewExecutor(db)
	tests := []struct {
		field string
		value string
	}{
		{field: "normalized_key", value: "https://example.com/Guide.PDF"},
		{field: "raw_target", value: "https://example.com/Guide.PDF"},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			q, parseErr := Parse(`link .` + tt.field + `=="` + tt.value + `"`)
			if parseErr != nil {
				t.Fatalf("Parse(): %v", parseErr)
			}
			rows, execErr := executor.ExecuteLinkQuery(q)
			if execErr != nil {
				t.Fatalf("ExecuteLinkQuery(): %v", execErr)
			}
			if len(rows) != 1 || rows[0].RawTarget != "https://example.com/Guide.PDF" {
				t.Fatalf("rows = %#v, want only path-case-exact URL", rows)
			}
		})
	}
}
