package query

import (
	"strings"
	"testing"
)

func TestParseSortClauses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantOrder []SortField
		wantPred  bool
	}{
		{
			name:      "single field defaults to asc",
			input:     "type:project sort(.title)",
			wantOrder: []SortField{{Field: "title", Direction: SortAsc}},
		},
		{
			name:      "explicit asc",
			input:     "type:project sort(.title, asc)",
			wantOrder: []SortField{{Field: "title", Direction: SortAsc}},
		},
		{
			name:      "desc is case-insensitive",
			input:     "type:date sort(.date, DESC)",
			wantOrder: []SortField{{Field: "date", Direction: SortDesc}},
		},
		{
			name:      "sort among predicates",
			input:     "type:project .status==active sort(.title, desc)",
			wantOrder: []SortField{{Field: "title", Direction: SortDesc}},
			wantPred:  true,
		},
		{
			name:      "sort before field predicate",
			input:     "type:project sort(.date, desc) .status==active",
			wantOrder: []SortField{{Field: "date", Direction: SortDesc}},
			wantPred:  true,
		},
		{
			name:  "repeatable multi-field",
			input: "type:issue .status==open sort(.priority, desc) sort(.title, asc)",
			wantOrder: []SortField{
				{Field: "priority", Direction: SortDesc},
				{Field: "title", Direction: SortAsc},
			},
			wantPred: true,
		},
		{
			name:      "section root",
			input:     "section sort(.title)",
			wantOrder: []SortField{{Field: "title", Direction: SortAsc}},
		},
		{
			name:      "trait root",
			input:     "trait:due sort(.value, desc)",
			wantOrder: []SortField{{Field: "value", Direction: SortDesc}},
		},
		{
			name:      "link root",
			input:     "link sort(.ext)",
			wantOrder: []SortField{{Field: "ext", Direction: SortAsc}},
		},
		{
			name:      "sort after grouped predicates",
			input:     "type:project (.status==active | .status==paused) sort(.title)",
			wantOrder: []SortField{{Field: "title", Direction: SortAsc}},
			wantPred:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.input, err)
			}
			if len(q.OrderBy) != len(tt.wantOrder) {
				t.Fatalf("OrderBy = %#v, want %#v", q.OrderBy, tt.wantOrder)
			}
			for i, got := range q.OrderBy {
				want := tt.wantOrder[i]
				if got != want {
					t.Errorf("OrderBy[%d] = %#v, want %#v", i, got, want)
				}
			}
			if tt.wantPred && q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			if !tt.wantPred && q.Predicate != nil {
				t.Fatalf("expected nil predicate, got %T", q.Predicate)
			}
		})
	}
}

func TestParseSortErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "missing field",
			input:   "type:project sort()",
			wantErr: "expected .field as first argument to sort()",
		},
		{
			name:    "first arg not a field",
			input:   "type:project sort(title)",
			wantErr: "expected .field as first argument to sort()",
		},
		{
			name:    "invalid direction",
			input:   "type:project sort(.title, upwards)",
			wantErr: "expected 'asc' or 'desc' as second argument to sort(), got upwards",
		},
		{
			name:    "extra argument",
			input:   "type:project sort(.title, asc, extra)",
			wantErr: "expected 'asc' or 'desc' as second argument to sort(), got extra",
		},
		{
			name:    "negated",
			input:   "type:project !sort(.title)",
			wantErr: "sort() cannot be negated",
		},
		{
			name:    "inside group",
			input:   "type:project (.status==active sort(.title))",
			wantErr: "sort() is only allowed at the top level",
		},
		{
			name:    "inside OR branch",
			input:   "type:project .a==1 | sort(.title)",
			wantErr: "sort() is only allowed at the top level",
		},
		{
			name:    "sort then OR",
			input:   "type:project sort(.title) | .status==open",
			wantErr: "sort() is only allowed at the top level",
		},
		{
			name:    "inside has subquery",
			input:   "type:project has(trait:due sort(.value))",
			wantErr: "sort() is only allowed at the top level",
		},
		{
			name:    "inside contains subquery",
			input:   "type:project contains(section sort(.title))",
			wantErr: "sort() is only allowed at the top level",
		},
		{
			name:    "inside in subquery",
			input:   "trait:due in(type:project sort(.title))",
			wantErr: "sort() is only allowed at the top level",
		},
		{
			name:    "inside within subquery",
			input:   "section within(type:project sort(.title))",
			wantErr: "sort() is only allowed at the top level",
		},
		{
			name:    "inside refs subquery",
			input:   "type:project refs(type:person sort(.name))",
			wantErr: "sort() is only allowed at the top level",
		},
		{
			name:    "inside refd subquery",
			input:   "type:project refd(type:meeting sort(.title))",
			wantErr: "sort() is only allowed at the top level",
		},
		{
			name:    "inside at subquery",
			input:   "trait:due at(trait:todo sort(.value))",
			wantErr: "sort() is only allowed at the top level",
		},
		{
			name:    "inside links predicate",
			input:   "type:project links(sort(.ext))",
			wantErr: "sort() is only allowed at the top level",
		},
		{
			name:    "inside any()",
			input:   `type:project any(.tags, sort(.title))`,
			wantErr: "sort() is only allowed at the top level",
		},
		{
			name:    "bare sort without paren",
			input:   "type:project sort",
			wantErr: "use sort(.field) or sort(.field, desc)",
		},
		{
			name:    "order hint points at sort()",
			input:   "type:project order",
			wantErr: "use sort(.field) or sort(.field, desc)",
		},
		{
			name:    "limit still pages with flags",
			input:   "type:project limit",
			wantErr: "RQL has no limit clause; page with --limit/--offset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			if err == nil {
				t.Fatal("expected parse error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseShellPipeSortWithoutParen(t *testing.T) {
	t.Parallel()

	_, err := Parse(`type:experiment_review | sort -field:created_at`)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "'|' (pipe) is not a shell pipe") {
		t.Fatalf("expected shell-pipe guidance, got %q", msg)
	}
}
