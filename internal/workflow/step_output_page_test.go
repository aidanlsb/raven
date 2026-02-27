package workflow

import (
	"strings"
	"testing"
)

func TestPaginateStepOutput(t *testing.T) {
	t.Run("array root", func(t *testing.T) {
		page, err := PaginateStepOutput([]interface{}{"a", "b", "c"}, "", 1, 2)
		if err != nil {
			t.Fatalf("PaginateStepOutput returned error: %v", err)
		}
		if page.Kind != "array" {
			t.Fatalf("kind = %q, want %q", page.Kind, "array")
		}
		if page.Total != 3 || page.Returned != 2 {
			t.Fatalf("total/returned = %d/%d, want 3/2", page.Total, page.Returned)
		}
		if page.NextOffset != 0 || page.HasMore {
			t.Fatalf("next_offset/has_more = %d/%v, want 0/false", page.NextOffset, page.HasMore)
		}
		if len(page.Items) != 2 || page.Items[0] != "b" || page.Items[1] != "c" {
			t.Fatalf("items = %#v, want [b c]", page.Items)
		}
	})

	t.Run("object root sorted by key", func(t *testing.T) {
		root := map[string]interface{}{
			"z": 1,
			"a": 2,
			"m": 3,
		}
		page, err := PaginateStepOutput(root, "", 0, 2)
		if err != nil {
			t.Fatalf("PaginateStepOutput returned error: %v", err)
		}
		if page.Kind != "object" {
			t.Fatalf("kind = %q, want %q", page.Kind, "object")
		}
		if page.Total != 3 || page.Returned != 2 {
			t.Fatalf("total/returned = %d/%d, want 3/2", page.Total, page.Returned)
		}
		if !page.HasMore || page.NextOffset != 2 {
			t.Fatalf("has_more/next_offset = %v/%d, want true/2", page.HasMore, page.NextOffset)
		}
		if len(page.Entries) != 2 || page.Entries[0].Key != "a" || page.Entries[1].Key != "m" {
			t.Fatalf("entries = %#v, want keys [a m]", page.Entries)
		}
	})

	t.Run("string rune safe paging", func(t *testing.T) {
		page, err := PaginateStepOutput("a🙂b", "", 1, 1)
		if err != nil {
			t.Fatalf("PaginateStepOutput returned error: %v", err)
		}
		if page.Kind != "string" {
			t.Fatalf("kind = %q, want %q", page.Kind, "string")
		}
		if page.Text != "🙂" {
			t.Fatalf("text = %q, want %q", page.Text, "🙂")
		}
		if page.Total != 3 || page.Returned != 1 {
			t.Fatalf("total/returned = %d/%d, want 3/1", page.Total, page.Returned)
		}
	})

	t.Run("nested path", func(t *testing.T) {
		root := map[string]interface{}{
			"data": map[string]interface{}{
				"results": []interface{}{"x", "y", "z"},
			},
		}
		page, err := PaginateStepOutput(root, "data.results", 1, 1)
		if err != nil {
			t.Fatalf("PaginateStepOutput returned error: %v", err)
		}
		if page.Kind != "array" || len(page.Items) != 1 || page.Items[0] != "y" {
			t.Fatalf("unexpected page: %#v", page)
		}
		if page.Path != "data.results" {
			t.Fatalf("path = %q, want %q", page.Path, "data.results")
		}
	})
}

func TestPaginateStepOutputErrors(t *testing.T) {
	tests := []struct {
		name    string
		root    interface{}
		path    string
		offset  int
		limit   int
		wantErr string
	}{
		{
			name:    "negative offset",
			root:    []interface{}{},
			offset:  -1,
			limit:   10,
			wantErr: "offset must be >= 0",
		},
		{
			name:    "invalid limit",
			root:    []interface{}{},
			offset:  0,
			limit:   0,
			wantErr: "limit must be > 0",
		},
		{
			name: "missing path key",
			root: map[string]interface{}{
				"data": map[string]interface{}{},
			},
			path:    "data.results",
			offset:  0,
			limit:   10,
			wantErr: "path \"data.results\" not found",
		},
		{
			name:    "invalid empty segment",
			root:    map[string]interface{}{"data": map[string]interface{}{}},
			path:    "data..results",
			offset:  0,
			limit:   10,
			wantErr: "empty path segment",
		},
		{
			name:    "unsupported type",
			root:    map[string]interface{}{"count": 42},
			path:    "count",
			offset:  0,
			limit:   10,
			wantErr: "unsupported type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := PaginateStepOutput(tt.root, tt.path, tt.offset, tt.limit)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
