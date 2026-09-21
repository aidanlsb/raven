package query

import (
	"strings"
	"testing"
)

func TestRefResolvedIDPreferredMatchSQL(t *testing.T) {
	t.Parallel()

	got := refResolvedIDPreferredMatchSQL("r", "?")
	want := "(r.target_id = ? OR (r.target_id IS NULL AND r.target_raw = ?))"
	if got != want {
		t.Fatalf("bound form = %q, want %q", got, want)
	}

	got = refResolvedIDPreferredMatchSQL("r", "o.id")
	want = "(r.target_id = o.id OR (r.target_id IS NULL AND r.target_raw = o.id))"
	if got != want {
		t.Fatalf("join form = %q, want %q", got, want)
	}

	cond, args := refResolvedIDPreferredMatch("fr", "companies/acme", "acme")
	if cond != refResolvedIDPreferredMatchSQL("fr", "?") {
		t.Fatalf("bound helper SQL = %q, want the shared fragment", cond)
	}
	if len(args) != 2 || args[0] != "companies/acme" || args[1] != "acme" {
		t.Fatalf("args = %#v, want (resolvedID, rawQuery)", args)
	}
}

func TestEdgeSourceCondition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		edgeAlias string
		rootAlias string
		root      QueryType
		predName  string
		want      string
		wantErr   string
	}{
		{
			name:      "object includes section fragments",
			edgeAlias: "l",
			rootAlias: "o",
			root:      QueryTypeObject,
			predName:  "links",
			want:      "(l.source_id = o.id OR l.source_id LIKE o.id || '#%')",
		},
		{
			name:      "refs object uses the same fragment-inclusive condition",
			edgeAlias: "r",
			rootAlias: "o",
			root:      QueryTypeObject,
			predName:  "refs",
			want:      "(r.source_id = o.id OR r.source_id LIKE o.id || '#%')",
		},
		{
			name:      "trait uses source line",
			edgeAlias: "l",
			rootAlias: "t",
			root:      QueryTypeTrait,
			predName:  "links",
			want:      "l.file_path = t.file_path AND l.line_number = t.line_number",
		},
		{
			name:      "section uses subtree line range",
			edgeAlias: "l",
			rootAlias: "s",
			root:      QueryTypeSection,
			predName:  "links",
			want:      "l.file_path = s.file_path AND l.line_number >= s.line_start AND (s.subtree_line_end IS NULL OR l.line_number <= s.subtree_line_end)",
		},
		{
			name:      "unsupported root keeps predicate name",
			edgeAlias: "l",
			rootAlias: "x",
			root:      QueryTypeLink,
			predName:  "links",
			wantErr:   "links() predicate is not supported for link queries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := edgeSourceCondition(tt.edgeAlias, tt.rootAlias, tt.root, tt.predName)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("edgeSourceCondition: %v", err)
			}
			if got != tt.want {
				t.Fatalf("condition = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEdgeSourceConditionObjectRootSharedShape(t *testing.T) {
	t.Parallel()

	refsCond, err := edgeSourceCondition("r", "o", QueryTypeObject, "refs")
	if err != nil {
		t.Fatalf("refs: %v", err)
	}
	linksCond, err := edgeSourceCondition("l", "o", QueryTypeObject, "links")
	if err != nil {
		t.Fatalf("links: %v", err)
	}
	normalizedLinks := strings.ReplaceAll(linksCond, "l.source_id", "r.source_id")
	if refsCond != normalizedLinks {
		t.Fatalf("object-root links condition %q does not match refs condition %q", linksCond, refsCond)
	}
}
