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
			input:     "object:project .status==active",
			wantField: "status",
			wantValue: "active",
		},
		{
			name:      "negated field",
			input:     "object:project !.status==done",
			wantField: "status",
			wantValue: "done",
			wantNeg:   true,
		},
		{
			name:      "quoted string value",
			input:     `object:project .title=="My Project"`,
			wantField: "title",
			wantValue: "My Project",
		},
		{
			name:      "quoted string with spaces",
			input:     `object:book .author=="J.R.R. Tolkien"`,
			wantField: "author",
			wantValue: "J.R.R. Tolkien",
		},
		{
			name:      "negated quoted string",
			input:     `object:project !.status=="in progress"`,
			wantField: "status",
			wantValue: "in progress",
			wantNeg:   true,
		},
		{
			name:      "field ref value uses wikilink target",
			input:     `object:meeting .attendees==[[people/freya|Freya]]`,
			wantField: "attendees",
			wantValue: "people/freya",
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
			input:         "object:meeting has(trait:due)",
			wantTraitName: "due",
		},
		{
			name:          "full has subquery",
			input:         "object:meeting has(trait:due)",
			wantTraitName: "due",
		},
		{
			name:          "negated has",
			input:         "object:meeting !has(trait:due)",
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
			input:        "object:meeting parent(object:date)",
			predType:     "parent",
			wantTypeName: "date",
		},
		{
			name:         "parent full",
			input:        "object:meeting parent(object:date)",
			predType:     "parent",
			wantTypeName: "date",
		},
		{
			name:         "ancestor shorthand",
			input:        "object:meeting ancestor(object:date)",
			predType:     "ancestor",
			wantTypeName: "date",
		},
		{
			name:         "child shorthand",
			input:        "object:date child(object:meeting)",
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
			if len(q.Predicates) != 1 {
				t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
			}

			switch p := q.Predicates[0].(type) {
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
			input:        "trait:due on(object:meeting)",
			predType:     "on",
			wantTypeName: "meeting",
		},
		{
			name:         "on full",
			input:        "trait:due on(object:meeting)",
			predType:     "on",
			wantTypeName: "meeting",
		},
		{
			name:         "within shorthand",
			input:        "trait:highlight within(object:date)",
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
			input:          "object:project .status==active has(trait:due)",
			wantPredicates: 1,
		},
		{
			name:           "OR predicate",
			input:          "object:project .status==active | .status==done",
			wantPredicates: 1, // Becomes single OrPredicate
		},
		{
			name:           "grouped predicates",
			input:          "object:project (.status==active | .status==done)",
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
			if tt.name == "multiple predicates AND" {
				gp, ok := q.Predicates[0].(*GroupPredicate)
				if !ok || len(gp.Predicates) != 2 {
					t.Errorf("expected GroupPredicate with 2 predicates, got %T", q.Predicates[0])
				}
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
			input:      "object:meeting refs([[projects/website]])",
			wantTarget: "projects/website",
		},
		{
			name:     "refs with subquery",
			input:    "object:meeting refs(object:project)",
			wantSubQ: true,
		},
		{
			name:       "negated refs",
			input:      "object:meeting !refs([[projects/website]])",
			wantTarget: "projects/website",
			wantNeg:    true,
		},
		{
			name:     "refs with complex subquery",
			input:    "object:meeting refs(object:project .status==active)",
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
		name     string
		input    string
		wantTerm string
		wantNeg  bool
		wantErr  bool
	}{
		{
			name:     "simple content search",
			input:    `object:person content("colleague")`,
			wantTerm: "colleague",
		},
		{
			name:     "content with multiple words",
			input:    `object:project content("api design")`,
			wantTerm: "api design",
		},
		{
			name:     "negated content search",
			input:    `object:person !content("contractor")`,
			wantTerm: "contractor",
			wantNeg:  true,
		},
		{
			name:    "content without quotes",
			input:   `object:person content(colleague)`,
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

func TestParseDescendantContains(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		predType     string
		wantTypeName string
		wantNeg      bool
	}{
		{
			name:         "descendant shorthand",
			input:        "object:project descendant(object:section)",
			predType:     "descendant",
			wantTypeName: "section",
		},
		{
			name:         "descendant full",
			input:        "object:project descendant(object:section)",
			predType:     "descendant",
			wantTypeName: "section",
		},
		{
			name:         "negated descendant",
			input:        "object:project !descendant(object:section)",
			predType:     "descendant",
			wantTypeName: "section",
			wantNeg:      true,
		},
		{
			name:         "encloses shorthand",
			input:        "object:project encloses(trait:todo)",
			predType:     "encloses",
			wantTypeName: "todo",
		},
		{
			name:         "encloses full",
			input:        "object:project encloses(trait:todo)",
			predType:     "encloses",
			wantTypeName: "todo",
		},
		{
			name:         "encloses with value",
			input:        "object:project encloses(trait:todo .value==done)",
			predType:     "encloses",
			wantTypeName: "todo",
		},
		{
			name:         "negated encloses",
			input:        "object:project !encloses(trait:todo)",
			predType:     "encloses",
			wantTypeName: "todo",
			wantNeg:      true,
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
			var negated bool
			switch p := q.Predicates[0].(type) {
			case *DescendantPredicate:
				if tt.predType != "descendant" {
					t.Fatalf("expected %s, got descendant", tt.predType)
				}
				subQuery = p.SubQuery
				negated = p.Negated()
			case *ContainsPredicate:
				if tt.predType != "encloses" {
					t.Fatalf("expected %s, got encloses", tt.predType)
				}
				subQuery = p.SubQuery
				negated = p.Negated()
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicates[0])
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
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:  "nested has with value",
			input: "object:meeting has(trait:due .value==past)",
		},
		{
			name:  "on with field",
			input: "trait:highlight on(object:book .status==reading)",
		},
		{
			name:  "ancestor chain",
			input: "object:topic ancestor(object:meeting ancestor(object:date))",
		},
		{
			name:  "complex with OR",
			input: "trait:highlight (on(object:book .status==reading) | on(object:article .status==reading))",
		},
		{
			name:  "multiple field predicates",
			input: "object:project .status==active .priority==high",
		},
		{
			name:  "encloses with value predicate",
			input: "object:project encloses(trait:todo .value==todo)",
		},
		{
			name:  "descendant with field predicate",
			input: "object:project descendant(object:section .title==Tasks)",
		},
		{
			name:  "combined contains and field",
			input: "object:project .status==active encloses(trait:todo)",
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

func TestParseAtPredicate(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantTraitName string
		wantNeg       bool
	}{
		{
			name:          "at with shorthand trait",
			input:         "trait:due at(trait:todo)",
			wantTraitName: "todo",
		},
		{
			name:          "at with full trait subquery",
			input:         "trait:due at(trait:todo)",
			wantTraitName: "todo",
		},
		{
			name:          "at with trait subquery and value",
			input:         "trait:due at(trait:priority .value==high)",
			wantTraitName: "priority",
		},
		{
			name:          "negated at",
			input:         "trait:due !at(trait:todo)",
			wantTraitName: "todo",
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
			ap, ok := q.Predicates[0].(*AtPredicate)
			if !ok {
				t.Fatalf("expected AtPredicate, got %T", q.Predicates[0])
			}
			if ap.SubQuery.TypeName != tt.wantTraitName {
				t.Errorf("trait name = %v, want %v", ap.SubQuery.TypeName, tt.wantTraitName)
			}
			if ap.Negated() != tt.wantNeg {
				t.Errorf("Negated = %v, want %v", ap.Negated(), tt.wantNeg)
			}
		})
	}
}

func TestParseRefdPredicate(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTarget string
		wantSubQ   bool
		wantNeg    bool
	}{
		{
			name:       "refd with target",
			input:      "object:project refd([[meetings/standup]])",
			wantTarget: "meetings/standup",
		},
		{
			name:     "refd with object subquery",
			input:    "object:project refd(object:meeting)",
			wantSubQ: true,
		},
		{
			name:       "negated refd",
			input:      "object:project !refd([[meetings/standup]])",
			wantTarget: "meetings/standup",
			wantNeg:    true,
		},
		{
			name:     "refd with complex subquery",
			input:    "object:person refd(object:project .status==active)",
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
			rp, ok := q.Predicates[0].(*RefdPredicate)
			if !ok {
				t.Fatalf("expected RefdPredicate, got %T", q.Predicates[0])
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

func TestParseComparisonOperators(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantCompareOp CompareOp
		wantValue     string
	}{
		{
			name:          "value less than",
			input:         "trait:due .value<2025-01-01",
			wantCompareOp: CompareLt,
			wantValue:     "2025-01-01",
		},
		{
			name:          "value greater than",
			input:         "trait:priority .value>5",
			wantCompareOp: CompareGt,
			wantValue:     "5",
		},
		{
			name:          "value less than or equal",
			input:         "trait:due .value<=2025-12-31",
			wantCompareOp: CompareLte,
			wantValue:     "2025-12-31",
		},
		{
			name:          "value greater than or equal",
			input:         "trait:score .value>=100",
			wantCompareOp: CompareGte,
			wantValue:     "100",
		},
		{
			name:          "value equals (default)",
			input:         "trait:status .value==active",
			wantCompareOp: CompareEq,
			wantValue:     "active",
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
			if fp.Field != "value" {
				t.Errorf("Field = %v, want value", fp.Field)
			}
			if fp.CompareOp != tt.wantCompareOp {
				t.Errorf("CompareOp = %v, want %v", fp.CompareOp, tt.wantCompareOp)
			}
			if fp.Value != tt.wantValue {
				t.Errorf("Value = %v, want %v", fp.Value, tt.wantValue)
			}
		})
	}
}

func TestParseInPredicates(t *testing.T) {
	t.Run("trait value in list", func(t *testing.T) {
		q, err := Parse(`trait:todo in(.value, [todo,done])`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(q.Predicates) != 1 {
			t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
		}
		if _, ok := q.Predicates[0].(*OrPredicate); !ok {
			t.Fatalf("expected OrPredicate, got %T", q.Predicates[0])
		}
	})

	t.Run("trait value not in list via negation", func(t *testing.T) {
		q, err := Parse(`trait:todo !in(.value, [todo,done])`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(q.Predicates) != 1 {
			t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
		}
		op, ok := q.Predicates[0].(*OrPredicate)
		if !ok {
			t.Fatalf("expected OrPredicate, got %T", q.Predicates[0])
		}
		if !op.Negated() {
			t.Fatal("expected negated predicate")
		}
	})

	t.Run("object field in list", func(t *testing.T) {
		q, err := Parse(`object:project in(.status, [active,backlog])`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(q.Predicates) != 1 {
			t.Fatalf("expected 1 predicate, got %d", len(q.Predicates))
		}
		if _, ok := q.Predicates[0].(*OrPredicate); !ok {
			t.Fatalf("expected OrPredicate, got %T", q.Predicates[0])
		}
	})

	t.Run("in() errors on empty list", func(t *testing.T) {
		_, err := Parse(`trait:todo in(.value, [])`)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("bracket list is not valid after ==", func(t *testing.T) {
		_, err := Parse(`trait:todo .value==[todo,done]`)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestParseFieldComparisonOperators(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantField     string
		wantCompareOp CompareOp
		wantValue     string
	}{
		{
			name:          "field less than",
			input:         "object:project .priority<5",
			wantField:     "priority",
			wantCompareOp: CompareLt,
			wantValue:     "5",
		},
		{
			name:          "field greater than or equal",
			input:         "object:task .count>=10",
			wantField:     "count",
			wantCompareOp: CompareGte,
			wantValue:     "10",
		},
		{
			name:          "field equals (default)",
			input:         "object:project .status==active",
			wantField:     "status",
			wantCompareOp: CompareEq,
			wantValue:     "active",
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
			if fp.CompareOp != tt.wantCompareOp {
				t.Errorf("CompareOp = %v, want %v", fp.CompareOp, tt.wantCompareOp)
			}
			if fp.Value != tt.wantValue {
				t.Errorf("Value = %v, want %v", fp.Value, tt.wantValue)
			}
		})
	}
}

func TestParseRefdShorthand(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantTypeName string
	}{
		{
			name:         "refd shorthand",
			input:        "object:project refd(object:meeting)",
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
			rp, ok := q.Predicates[0].(*RefdPredicate)
			if !ok {
				t.Fatalf("expected RefdPredicate, got %T", q.Predicates[0])
			}
			if rp.SubQuery == nil {
				t.Fatal("expected SubQuery, got nil")
			}
			if rp.SubQuery.TypeName != tt.wantTypeName {
				t.Errorf("TypeName = %v, want %v", rp.SubQuery.TypeName, tt.wantTypeName)
			}
		})
	}
}

func TestParseDirectTargetPredicates(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		predType   string
		wantTarget string
		wantNeg    bool
	}{
		{
			name:       "parent with direct target",
			input:      "object:section parent([[projects/website]])",
			predType:   "parent",
			wantTarget: "projects/website",
		},
		{
			name:       "ancestor with direct target",
			input:      "object:section ancestor([[projects/website]])",
			predType:   "ancestor",
			wantTarget: "projects/website",
		},
		{
			name:       "child with direct target",
			input:      "object:project child([[projects/website#overview]])",
			predType:   "child",
			wantTarget: "projects/website#overview",
		},
		{
			name:       "descendant with direct target",
			input:      "object:project descendant([[projects/website#tasks]])",
			predType:   "descendant",
			wantTarget: "projects/website#tasks",
		},
		{
			name:       "on with direct target (trait query)",
			input:      "trait:todo on([[projects/website]])",
			predType:   "on",
			wantTarget: "projects/website",
		},
		{
			name:       "within with direct target (trait query)",
			input:      "trait:todo within([[projects/website]])",
			predType:   "within",
			wantTarget: "projects/website",
		},
		{
			name:       "negated parent with direct target",
			input:      "object:section !parent([[projects/website]])",
			predType:   "parent",
			wantTarget: "projects/website",
			wantNeg:    true,
		},
		{
			name:       "short reference",
			input:      "trait:todo within([[website]])",
			predType:   "within",
			wantTarget: "website",
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

			var target string
			var negated bool
			switch p := q.Predicates[0].(type) {
			case *ParentPredicate:
				if tt.predType != "parent" {
					t.Fatalf("expected %s, got parent", tt.predType)
				}
				target = p.Target
				negated = p.Negated()
			case *AncestorPredicate:
				if tt.predType != "ancestor" {
					t.Fatalf("expected %s, got ancestor", tt.predType)
				}
				target = p.Target
				negated = p.Negated()
			case *ChildPredicate:
				if tt.predType != "child" {
					t.Fatalf("expected %s, got child", tt.predType)
				}
				target = p.Target
				negated = p.Negated()
			case *DescendantPredicate:
				if tt.predType != "descendant" {
					t.Fatalf("expected %s, got descendant", tt.predType)
				}
				target = p.Target
				negated = p.Negated()
			case *OnPredicate:
				if tt.predType != "on" {
					t.Fatalf("expected %s, got on", tt.predType)
				}
				target = p.Target
				negated = p.Negated()
			case *WithinPredicate:
				if tt.predType != "within" {
					t.Fatalf("expected %s, got within", tt.predType)
				}
				target = p.Target
				negated = p.Negated()
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicates[0])
			}

			if target != tt.wantTarget {
				t.Errorf("Target = %v, want %v", target, tt.wantTarget)
			}
			if negated != tt.wantNeg {
				t.Errorf("Negated = %v, want %v", negated, tt.wantNeg)
			}
		})
	}
}
