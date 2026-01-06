package query

import (
	"testing"
)

func TestParseObjectQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType QueryType
		wantName string
		wantErr  bool
	}{
		{
			name:     "simple object query",
			input:    "object:project",
			wantType: QueryTypeObject,
			wantName: "project",
		},
		{
			name:     "simple trait query",
			input:    "trait:due",
			wantType: QueryTypeTrait,
			wantName: "due",
		},
		{
			name:    "invalid query type",
			input:   "foo:bar",
			wantErr: true,
		},
		{
			name:    "missing type name",
			input:   "object:",
			wantErr: true,
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
			if q.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", q.Type, tt.wantType)
			}
			if q.TypeName != tt.wantName {
				t.Errorf("TypeName = %v, want %v", q.TypeName, tt.wantName)
			}
		})
	}
}

func TestParseFieldPredicates(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantField  string
		wantValue  string
		wantExists bool
		wantNeg    bool
	}{
		{
			name:      "simple field",
			input:     "object:project .status:active",
			wantField: "status",
			wantValue: "active",
		},
		{
			name:       "field exists",
			input:      "object:person .email:*",
			wantField:  "email",
			wantExists: true,
		},
		{
			name:      "negated field",
			input:     "object:project !.status:done",
			wantField: "status",
			wantValue: "done",
			wantNeg:   true,
		},
		{
			name:       "negated exists",
			input:      "object:person !.email:*",
			wantField:  "email",
			wantExists: true,
			wantNeg:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(q.Predicates) != 1 {
				t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
			}
			fp, ok := q.Predicates[0].(*FieldPredicate)
			if !ok {
				t.Fatalf("expected FieldPredicate, got %T", q.Predicates[0])
			}
			if fp.Field != tt.wantField {
				t.Errorf("Field = %v, want %v", fp.Field, tt.wantField)
			}
			if !tt.wantExists && fp.Value != tt.wantValue {
				t.Errorf("Value = %v, want %v", fp.Value, tt.wantValue)
			}
			if fp.IsExists != tt.wantExists {
				t.Errorf("IsExists = %v, want %v", fp.IsExists, tt.wantExists)
			}
			if fp.Negated() != tt.wantNeg {
				t.Errorf("Negated = %v, want %v", fp.Negated(), tt.wantNeg)
			}
		})
	}
}

func TestParseHasPredicate(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantTraitName string
		wantNeg       bool
	}{
		{
			name:          "shorthand has",
			input:         "object:meeting has:due",
			wantTraitName: "due",
		},
		{
			name:          "full has subquery",
			input:         "object:meeting has:{trait:due}",
			wantTraitName: "due",
		},
		{
			name:          "negated has",
			input:         "object:meeting !has:due",
			wantTraitName: "due",
			wantNeg:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(q.Predicates) != 1 {
				t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
			}
			hp, ok := q.Predicates[0].(*HasPredicate)
			if !ok {
				t.Fatalf("expected HasPredicate, got %T", q.Predicates[0])
			}
			if hp.SubQuery.TypeName != tt.wantTraitName {
				t.Errorf("trait name = %v, want %v", hp.SubQuery.TypeName, tt.wantTraitName)
			}
			if hp.Negated() != tt.wantNeg {
				t.Errorf("Negated = %v, want %v", hp.Negated(), tt.wantNeg)
			}
		})
	}
}

func TestParseParentAncestorChild(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		predType     string
		wantTypeName string
	}{
		{
			name:         "parent shorthand",
			input:        "object:meeting parent:date",
			predType:     "parent",
			wantTypeName: "date",
		},
		{
			name:         "parent full",
			input:        "object:meeting parent:{object:date}",
			predType:     "parent",
			wantTypeName: "date",
		},
		{
			name:         "ancestor shorthand",
			input:        "object:meeting ancestor:date",
			predType:     "ancestor",
			wantTypeName: "date",
		},
		{
			name:         "child shorthand",
			input:        "object:date child:meeting",
			predType:     "child",
			wantTypeName: "meeting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(q.Predicates) != 1 {
				t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
			}

			var subQuery *Query
			switch p := q.Predicates[0].(type) {
			case *ParentPredicate:
				if tt.predType != "parent" {
					t.Fatalf("expected %s, got parent", tt.predType)
				}
				subQuery = p.SubQuery
			case *AncestorPredicate:
				if tt.predType != "ancestor" {
					t.Fatalf("expected %s, got ancestor", tt.predType)
				}
				subQuery = p.SubQuery
			case *ChildPredicate:
				if tt.predType != "child" {
					t.Fatalf("expected %s, got child", tt.predType)
				}
				subQuery = p.SubQuery
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicates[0])
			}

			if subQuery.TypeName != tt.wantTypeName {
				t.Errorf("TypeName = %v, want %v", subQuery.TypeName, tt.wantTypeName)
			}
		})
	}
}

func TestParseTraitPredicates(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		predType  string
		wantValue string
	}{
		{
			name:      "value predicate",
			input:     "trait:due value:past",
			predType:  "value",
			wantValue: "past",
		},
		{
			name:      "source inline",
			input:     "trait:due source:inline",
			predType:  "source",
			wantValue: "inline",
		},
		{
			name:      "source frontmatter",
			input:     "trait:due source:frontmatter",
			predType:  "source",
			wantValue: "frontmatter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(q.Predicates) != 1 {
				t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
			}

			switch p := q.Predicates[0].(type) {
			case *ValuePredicate:
				if tt.predType != "value" {
					t.Fatalf("expected %s, got value", tt.predType)
				}
				if p.Value != tt.wantValue {
					t.Errorf("Value = %v, want %v", p.Value, tt.wantValue)
				}
			case *SourcePredicate:
				if tt.predType != "source" {
					t.Fatalf("expected %s, got source", tt.predType)
				}
				if p.Source != tt.wantValue {
					t.Errorf("Source = %v, want %v", p.Source, tt.wantValue)
				}
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicates[0])
			}
		})
	}
}

func TestParseOnWithin(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		predType     string
		wantTypeName string
	}{
		{
			name:         "on shorthand",
			input:        "trait:due on:meeting",
			predType:     "on",
			wantTypeName: "meeting",
		},
		{
			name:         "on full",
			input:        "trait:due on:{object:meeting}",
			predType:     "on",
			wantTypeName: "meeting",
		},
		{
			name:         "within shorthand",
			input:        "trait:highlight within:date",
			predType:     "within",
			wantTypeName: "date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(q.Predicates) != 1 {
				t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
			}

			var subQuery *Query
			switch p := q.Predicates[0].(type) {
			case *OnPredicate:
				if tt.predType != "on" {
					t.Fatalf("expected %s, got on", tt.predType)
				}
				subQuery = p.SubQuery
			case *WithinPredicate:
				if tt.predType != "within" {
					t.Fatalf("expected %s, got within", tt.predType)
				}
				subQuery = p.SubQuery
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicates[0])
			}

			if subQuery.TypeName != tt.wantTypeName {
				t.Errorf("TypeName = %v, want %v", subQuery.TypeName, tt.wantTypeName)
			}
		})
	}
}

func TestParseBooleanComposition(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantPredicates int
	}{
		{
			name:           "multiple predicates AND",
			input:          "object:project .status:active has:due",
			wantPredicates: 2,
		},
		{
			name:           "OR predicate",
			input:          "object:project .status:active | .status:done",
			wantPredicates: 1, // Becomes single OrPredicate
		},
		{
			name:           "grouped predicates",
			input:          "object:project (.status:active | .status:done)",
			wantPredicates: 1, // Becomes single GroupPredicate
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(q.Predicates) != tt.wantPredicates {
				t.Errorf("got %d predicates, want %d", len(q.Predicates), tt.wantPredicates)
			}
		})
	}
}

func TestParseRefsPredicate(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTarget string
		wantSubQ   bool
		wantNeg    bool
	}{
		{
			name:       "refs with target",
			input:      "object:meeting refs:[[projects/website]]",
			wantTarget: "projects/website",
		},
		{
			name:     "refs with subquery",
			input:    "object:meeting refs:{object:project}",
			wantSubQ: true,
		},
		{
			name:       "negated refs",
			input:      "object:meeting !refs:[[projects/website]]",
			wantTarget: "projects/website",
			wantNeg:    true,
		},
		{
			name:     "refs with complex subquery",
			input:    "object:meeting refs:{object:project .status:active}",
			wantSubQ: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(q.Predicates) != 1 {
				t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
			}
			rp, ok := q.Predicates[0].(*RefsPredicate)
			if !ok {
				t.Fatalf("expected RefsPredicate, got %T", q.Predicates[0])
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

func TestParseContentPredicate(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTerm   string
		wantNeg    bool
		wantErr    bool
	}{
		{
			name:     "simple content search",
			input:    `object:person content:"colleague"`,
			wantTerm: "colleague",
		},
		{
			name:     "content with multiple words",
			input:    `object:project content:"api design"`,
			wantTerm: "api design",
		},
		{
			name:     "negated content search",
			input:    `object:person !content:"contractor"`,
			wantTerm: "contractor",
			wantNeg:  true,
		},
		{
			name:    "content without quotes",
			input:   `object:person content:colleague`,
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
			if len(q.Predicates) != 1 {
				t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
			}
			cp, ok := q.Predicates[0].(*ContentPredicate)
			if !ok {
				t.Fatalf("expected ContentPredicate, got %T", q.Predicates[0])
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

func TestParseComplexQueries(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:  "nested has with value",
			input: "object:meeting has:{trait:due value:past}",
		},
		{
			name:  "on with field",
			input: "trait:highlight on:{object:book .status:reading}",
		},
		{
			name:  "ancestor chain",
			input: "object:topic ancestor:{object:meeting ancestor:date}",
		},
		{
			name:  "complex with OR",
			input: "trait:highlight (on:{object:book .status:reading} | on:{object:article .status:reading})",
		},
		{
			name:  "multiple field predicates",
			input: "object:project .status:active .priority:high",
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
