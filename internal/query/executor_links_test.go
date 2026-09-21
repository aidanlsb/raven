package query

import (
	"strings"
	"testing"
)

func TestExecuteLinksPredicateByRoot(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	defer db.Close()
	executor := NewExecutor(db)

	tests := []struct {
		name    string
		query   string
		wantIDs []string
	}{
		{
			name:    "type root",
			query:   "type:project links(.ext==pdf)",
			wantIDs: []string{"projects/mobile", "projects/website"},
		},
		{
			name:    "type root boolean link field",
			query:   "type:project links(.is_image==true)",
			wantIDs: []string{"projects/website"},
		},
		{
			name:    "trait root uses same source line",
			query:   "trait:todo links(.ext==pdf)",
			wantIDs: []string{"trait7"},
		},
		{
			name:    "section root uses subtree range",
			query:   "section links(.is_image==true)",
			wantIDs: []string{"projects/website#tasks"},
		},
		{
			name:    "section root excludes preceding sections",
			query:   "section links(.scheme==url)",
			wantIDs: []string{"projects/website#design"},
		},
		{
			name:    "shared string field grammar",
			query:   `type:project links(includes(.display, "SPEC"))`,
			wantIDs: []string{"projects/mobile"},
		},
		{
			name:    "shared source field grammar",
			query:   "type:project links(.source_type==project)",
			wantIDs: []string{"projects/mobile", "projects/website"},
		},
		{
			name:    "shared numeric field grammar",
			query:   "trait:todo links(.line==20)",
			wantIDs: []string{"trait7"},
		},
		{
			name:    "shared numeric position grammar",
			query:   "type:project links(.position_start>=8)",
			wantIDs: []string{"projects/website"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			run, err := executor.Run(q, RunRequest{})
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			var got []string
			switch q.Type {
			case QueryTypeObject:
				for _, result := range run.Objects {
					got = append(got, result.ID)
				}
			case QueryTypeTrait:
				for _, result := range run.Traits {
					got = append(got, result.ID)
				}
			case QueryTypeSection:
				for _, result := range run.Sections {
					got = append(got, result.ID)
				}
			default:
				t.Fatalf("unexpected root: %v", q.Type)
			}
			if strings.Join(got, ",") != strings.Join(tt.wantIDs, ",") {
				t.Fatalf("IDs = %#v, want %#v", got, tt.wantIDs)
			}
		})
	}
}

func TestExecuteLinksPredicateObjectRootIncludesSectionFragmentSource(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	defer db.Close()
	_, err := db.Exec(`
		INSERT INTO links (
			source_id, source_type, file_path, line_number, position_start, position_end,
			raw_target, display, is_image, scheme, ext, normalized_key
		) VALUES (
			'projects/website#design', 'project', 'projects/website.md', 56, 0, 40,
			'https://cdn.example.com/sheet.csv', 'sheet', 0, 'url', 'csv',
			'https://cdn.example.com/sheet.csv'
		)
	`)
	if err != nil {
		t.Fatalf("insert section-sourced link: %v", err)
	}

	executor := NewExecutor(db)
	q, err := Parse(`type:project links(.ext==csv)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	run, err := executor.Run(q, RunRequest{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(run.Objects) != 1 || run.Objects[0].ID != "projects/website" {
		t.Fatalf("IDs = %#v, want [projects/website]", run.Objects)
	}
}
