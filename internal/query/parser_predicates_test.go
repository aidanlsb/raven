package query

import (
	"testing"
)

func TestParseTraitPredicates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		predType  string
		wantValue string
	}{
		{
			name:      "value predicate",
			input:     "trait:due .value==past",
			predType:  "value",
			wantValue: "past",
		},
		{
			name:      "value predicate with quoted string",
			input:     `trait:status .value=="in progress"`,
			predType:  "value",
			wantValue: "in progress",
		},
		{
			name:      "value predicate with spaces",
			input:     `trait:priority .value=="very high"`,
			predType:  "value",
			wantValue: "very high",
		},
		{
			name:      "value predicate with ref",
			input:     "trait:assignee .value==[[people/freya]]",
			predType:  "value",
			wantValue: "people/freya",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}

			switch p := q.Predicate.(type) {
			case *FieldPredicate:
				if tt.predType != "value" {
					t.Fatalf("expected %s, got field", tt.predType)
				}
				if p.Field != "value" {
					t.Errorf("Field = %v, want value", p.Field)
				}
				if p.Value != tt.wantValue {
					t.Errorf("Value = %v, want %v", p.Value, tt.wantValue)
				}
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicate)
			}
		})
	}
}
func TestParseBooleanComposition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "multiple predicates AND",
			input: "type:project .status==active has(trait:due)",
		},
		{
			name:  "OR predicate",
			input: "type:project .status==active | .status==done",
		},
		{
			name:  "grouped predicates",
			input: "type:project (.status==active | .status==done)",
		},
		{
			name:  "OR with function predicate",
			input: "type:project .status==active | has(trait:due)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			if tt.name == "multiple predicates AND" {
				gp, ok := q.Predicate.(*GroupPredicate)
				if !ok || len(gp.Predicates) != 2 {
					t.Errorf("expected GroupPredicate with 2 predicates, got %T", q.Predicate)
				}
			}
		})
	}
}
func TestParseRefsPredicate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		wantTarget string
		wantSubQ   bool
		wantNeg    bool
	}{
		{
			name:       "refs with target",
			input:      "type:meeting refs([[projects/website]])",
			wantTarget: "projects/website",
		},
		{
			name:       "refs with shorthand target",
			input:      "type:meeting refs(cursor)",
			wantTarget: "cursor",
		},
		{
			name:     "refs with subquery",
			input:    "type:meeting refs(type:project)",
			wantSubQ: true,
		},
		{
			name:       "negated refs",
			input:      "type:meeting !refs([[projects/website]])",
			wantTarget: "projects/website",
			wantNeg:    true,
		},
		{
			name:     "refs with complex subquery",
			input:    "type:meeting refs(type:project .status==active)",
			wantSubQ: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			rp, ok := q.Predicate.(*RefsPredicate)
			if !ok {
				t.Fatalf("expected RefsPredicate, got %T", q.Predicate)
			}
			if tt.wantTarget != "" && rp.Target != tt.wantTarget {
				t.Errorf("Target = %v, want %v", rp.Target, tt.wantTarget)
			}
			if tt.wantSubQ && rp.SubQuery == nil {
				t.Error("expected SubQuery, got nil")
			}
			if !tt.wantSubQ && rp.SubQuery != nil {
				t.Error("expected no SubQuery")
			}
			if rp.Negated() != tt.wantNeg {
				t.Errorf("Negated = %v, want %v", rp.Negated(), tt.wantNeg)
			}
		})
	}
}

func TestParseLinksPredicate(t *testing.T) {
	t.Parallel()

	q, err := Parse(`type:project links(.ext==pdf | .is_image==true)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	linksPred, ok := q.Predicate.(*LinksPredicate)
	if !ok {
		t.Fatalf("expected LinksPredicate, got %T", q.Predicate)
	}
	inner, ok := linksPred.LinkPredicate.(*OrPredicate)
	if !ok || len(inner.Predicates) != 2 {
		t.Fatalf("expected two link field predicates, got %#v", linksPred.LinkPredicate)
	}

	negated, err := Parse(`trait:todo !links(includes(.display, "spec"))`)
	if err != nil {
		t.Fatalf("unexpected negated parse error: %v", err)
	}
	linksPred, ok = negated.Predicate.(*LinksPredicate)
	if !ok || !linksPred.Negated() {
		t.Fatalf("expected negated LinksPredicate, got %#v", negated.Predicate)
	}
}

func TestParseContentPredicate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantTerm string
		wantNeg  bool
		wantErr  bool
	}{
		{
			name:     "simple content search",
			input:    `type:person content("colleague")`,
			wantTerm: "colleague",
		},
		{
			name:     "content with multiple words",
			input:    `type:project content("api design")`,
			wantTerm: "api design",
		},
		{
			name:     "negated content search",
			input:    `type:person !content("contractor")`,
			wantTerm: "contractor",
			wantNeg:  true,
		},
		{
			name:    "content without quotes",
			input:   `type:person content(colleague)`,
			wantErr: true, // requires quoted string
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			cp, ok := q.Predicate.(*ContentPredicate)
			if !ok {
				t.Fatalf("expected ContentPredicate, got %T", q.Predicate)
			}
			if cp.SearchTerm != tt.wantTerm {
				t.Errorf("SearchTerm = %q, want %q", cp.SearchTerm, tt.wantTerm)
			}
			if cp.Negated() != tt.wantNeg {
				t.Errorf("Negated() = %v, want %v", cp.Negated(), tt.wantNeg)
			}
		})
	}
}
func TestParseHasContainsScopePredicates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		input        string
		predType     string
		wantTypeName string
		wantNeg      bool
	}{
		{
			name:         "has trait",
			input:        "type:project has(trait:todo)",
			predType:     "has",
			wantTypeName: "todo",
		},
		{
			name:         "has section",
			input:        "type:project has(section .title==Tasks)",
			predType:     "has",
			wantTypeName: "",
		},
		{
			name:         "contains trait",
			input:        "type:project contains(trait:todo .value==done)",
			predType:     "contains",
			wantTypeName: "todo",
		},
		{
			name:         "negated contains section",
			input:        "type:project !contains(section .title==Tasks)",
			predType:     "contains",
			wantTypeName: "",
			wantNeg:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}

			var subQuery *Query
			var negated bool
			switch p := q.Predicate.(type) {
			case *HasPredicate:
				if tt.predType != "has" {
					t.Fatalf("expected %s, got has", tt.predType)
				}
				subQuery = p.SubQuery
				negated = p.Negated()
			case *ContainsPredicate:
				if tt.predType != "contains" {
					t.Fatalf("expected %s, got contains", tt.predType)
				}
				subQuery = p.SubQuery
				negated = p.Negated()
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicate)
			}

			if subQuery.TypeName != tt.wantTypeName {
				t.Errorf("TypeName = %v, want %v", subQuery.TypeName, tt.wantTypeName)
			}
			if negated != tt.wantNeg {
				t.Errorf("Negated = %v, want %v", negated, tt.wantNeg)
			}
		})
	}
}
func TestParseComplexQueries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:  "nested has with value",
			input: "type:meeting has(trait:due .value==past)",
		},
		{
			name:  "in with field",
			input: "trait:highlight in(type:book .status==reading)",
		},
		{
			name:  "within section",
			input: "section within(type:topic .status==active)",
		},
		{
			name:  "complex with OR",
			input: "trait:highlight (in(type:book .status==reading) | in(type:article .status==reading))",
		},
		{
			name:  "multiple field predicates",
			input: "type:project .status==active .priority==high",
		},
		{
			name:  "contains with value predicate",
			input: "type:project contains(trait:todo .value==todo)",
		},
		{
			name:  "contains section with field predicate",
			input: "type:project contains(section .title==Tasks)",
		},
		{
			name:  "combined contains and field",
			input: "type:project .status==active contains(trait:todo)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q == nil {
				t.Fatal("expected non-nil query")
			}
		})
	}
}
func TestParseBooleanEdgeCases(t *testing.T) {
	t.Parallel()
	t.Run("chained OR produces flat OrPredicate", func(t *testing.T) {
		q, err := Parse("type:project .status==active | .status==paused | .status==done | .status==archived")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		op, ok := q.Predicate.(*OrPredicate)
		if !ok {
			t.Fatalf("expected OrPredicate, got %T", q.Predicate)
		}
		if len(op.Predicates) != 4 {
			t.Errorf("expected 4 OR branches, got %d", len(op.Predicates))
		}
	})

	t.Run("AND of two OR groups", func(t *testing.T) {
		q, err := Parse("type:project (.status==active | .status==paused) (.priority==high | .priority==medium)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		gp, ok := q.Predicate.(*GroupPredicate)
		if !ok {
			t.Fatalf("expected GroupPredicate, got %T", q.Predicate)
		}
		if len(gp.Predicates) != 2 {
			t.Fatalf("expected 2 AND branches, got %d", len(gp.Predicates))
		}
		or1, ok1 := gp.Predicates[0].(*OrPredicate)
		or2, ok2 := gp.Predicates[1].(*OrPredicate)
		if !ok1 || !ok2 {
			t.Fatalf("expected both branches to be OrPredicate, got %T and %T", gp.Predicates[0], gp.Predicates[1])
		}
		if len(or1.Predicates) != 2 || len(or2.Predicates) != 2 {
			t.Errorf("expected 2 branches in each OR, got %d and %d", len(or1.Predicates), len(or2.Predicates))
		}
	})

	t.Run("negated AND inside OR", func(t *testing.T) {
		q, err := Parse("type:project .status==active | !(.status==paused .priority==low)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		op, ok := q.Predicate.(*OrPredicate)
		if !ok {
			t.Fatalf("expected OrPredicate, got %T", q.Predicate)
		}
		if len(op.Predicates) != 2 {
			t.Fatalf("expected 2 OR branches, got %d", len(op.Predicates))
		}
		np, ok := op.Predicates[1].(*NotPredicate)
		if !ok {
			t.Fatalf("expected NotPredicate as second OR branch, got %T", op.Predicates[1])
		}
		gp, ok := np.Inner.(*GroupPredicate)
		if !ok {
			t.Fatalf("expected GroupPredicate inside NotPredicate, got %T", np.Inner)
		}
		if len(gp.Predicates) != 2 {
			t.Errorf("expected 2 AND branches inside negation, got %d", len(gp.Predicates))
		}
	})

	t.Run("negated single predicate", func(t *testing.T) {
		q, err := Parse("type:project !.status==done")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		fp, ok := q.Predicate.(*FieldPredicate)
		if !ok {
			t.Fatalf("expected FieldPredicate, got %T", q.Predicate)
		}
		if !fp.Negated() {
			t.Error("expected negated predicate")
		}
	})

	t.Run("NotPredicate from negated group", func(t *testing.T) {
		q, err := Parse("type:project !(.status==active | .status==paused)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		np, ok := q.Predicate.(*NotPredicate)
		if !ok {
			t.Fatalf("expected NotPredicate, got %T", q.Predicate)
		}
		op, ok := np.Inner.(*OrPredicate)
		if !ok {
			t.Fatalf("expected OrPredicate inside NotPredicate, got %T", np.Inner)
		}
		if len(op.Predicates) != 2 {
			t.Errorf("expected 2 OR branches, got %d", len(op.Predicates))
		}
	})

	t.Run("in() produces flat OrPredicate", func(t *testing.T) {
		q, err := Parse("type:project oneof(.status, [active,paused,done])")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		op, ok := q.Predicate.(*OrPredicate)
		if !ok {
			t.Fatalf("expected OrPredicate, got %T", q.Predicate)
		}
		if len(op.Predicates) != 3 {
			t.Errorf("expected 3 OR branches from in(), got %d", len(op.Predicates))
		}
	})
}
