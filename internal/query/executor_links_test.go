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

			var got []string
			switch q.Type {
			case QueryTypeObject:
				results, execErr := executor.executeObjectQuery(q)
				err = execErr
				for _, result := range results {
					got = append(got, result.ID)
				}
			case QueryTypeTrait:
				results, execErr := executor.executeTraitQuery(q)
				err = execErr
				for _, result := range results {
					got = append(got, result.ID)
				}
			case QueryTypeSection:
				results, execErr := executor.executeSectionQuery(q)
				err = execErr
				for _, result := range results {
					got = append(got, result.ID)
				}
			default:
				t.Fatalf("unexpected root: %v", q.Type)
			}
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tt.wantIDs, ",") {
				t.Fatalf("IDs = %#v, want %#v", got, tt.wantIDs)
			}
		})
	}
}
