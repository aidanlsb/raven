package query

import (
	"testing"
)

func TestParseScopeNavigationPredicates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		input        string
		predType     string
		wantTypeName string
	}{
		{
			name:         "section in type",
			input:        "section in(type:project)",
			predType:     "in",
			wantTypeName: "project",
		},
		{
			name:         "section within type",
			input:        "section within(type:project)",
			predType:     "within",
			wantTypeName: "project",
		},
		{
			name:         "trait within type",
			input:        "trait:todo within(type:project)",
			predType:     "within",
			wantTypeName: "project",
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
			switch p := q.Predicate.(type) {
			case *InPredicate:
				if tt.predType != "in" {
					t.Fatalf("expected %s, got in", tt.predType)
				}
				subQuery = p.SubQuery
			case *WithinPredicate:
				if tt.predType != "within" {
					t.Fatalf("expected %s, got within", tt.predType)
				}
				subQuery = p.SubQuery
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicate)
			}

			if subQuery.TypeName != tt.wantTypeName {
				t.Errorf("TypeName = %v, want %v", subQuery.TypeName, tt.wantTypeName)
			}
		})
	}
}
func TestParseScopeNavigationTargets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		predType   string
		wantTarget string
	}{
		{
			name:       "in target",
			input:      "section in(website)",
			predType:   "in",
			wantTarget: "website",
		},
		{
			name:       "within target",
			input:      "section within(projects/website)",
			predType:   "within",
			wantTarget: "projects/website",
		},
		{
			name:       "trait in target",
			input:      "trait:todo in(projects/website#tasks)",
			predType:   "in",
			wantTarget: "projects/website#tasks",
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
			case *InPredicate:
				if tt.predType != "in" {
					t.Fatalf("expected %s, got in", tt.predType)
				}
				if p.Target != tt.wantTarget {
					t.Errorf("Target = %v, want %v", p.Target, tt.wantTarget)
				}
				if p.SubQuery != nil {
					t.Error("expected no SubQuery")
				}
			case *WithinPredicate:
				if tt.predType != "within" {
					t.Fatalf("expected %s, got within", tt.predType)
				}
				if p.Target != tt.wantTarget {
					t.Errorf("Target = %v, want %v", p.Target, tt.wantTarget)
				}
				if p.SubQuery != nil {
					t.Error("expected no SubQuery")
				}
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicate)
			}
		})
	}
}
func TestParseInWithin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		input        string
		predType     string
		wantTypeName string
	}{
		{
			name:         "in shorthand",
			input:        "trait:due in(type:meeting)",
			predType:     "in",
			wantTypeName: "meeting",
		},
		{
			name:         "in full",
			input:        "trait:due in(type:meeting)",
			predType:     "in",
			wantTypeName: "meeting",
		},
		{
			name:         "within shorthand",
			input:        "trait:highlight within(type:date)",
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
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}

			var subQuery *Query
			switch p := q.Predicate.(type) {
			case *InPredicate:
				if tt.predType != "in" {
					t.Fatalf("expected %s, got in", tt.predType)
				}
				subQuery = p.SubQuery
			case *WithinPredicate:
				if tt.predType != "within" {
					t.Fatalf("expected %s, got within", tt.predType)
				}
				subQuery = p.SubQuery
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicate)
			}

			if subQuery.TypeName != tt.wantTypeName {
				t.Errorf("TypeName = %v, want %v", subQuery.TypeName, tt.wantTypeName)
			}
		})
	}
}
func TestParseInWithinTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		predType   string
		wantTarget string
	}{
		{
			name:       "in target",
			input:      "trait:due in(website)",
			predType:   "in",
			wantTarget: "website",
		},
		{
			name:       "within target",
			input:      "trait:highlight within(projects/website)",
			predType:   "within",
			wantTarget: "projects/website",
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
			case *InPredicate:
				if tt.predType != "in" {
					t.Fatalf("expected %s, got in", tt.predType)
				}
				if p.Target != tt.wantTarget {
					t.Errorf("Target = %v, want %v", p.Target, tt.wantTarget)
				}
				if p.SubQuery != nil {
					t.Error("expected no SubQuery")
				}
			case *WithinPredicate:
				if tt.predType != "within" {
					t.Fatalf("expected %s, got within", tt.predType)
				}
				if p.Target != tt.wantTarget {
					t.Errorf("Target = %v, want %v", p.Target, tt.wantTarget)
				}
				if p.SubQuery != nil {
					t.Error("expected no SubQuery")
				}
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicate)
			}
		})
	}
}
func TestParseAtPredicate(t *testing.T) {
	t.Parallel()
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
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}
			ap, ok := q.Predicate.(*AtPredicate)
			if !ok {
				t.Fatalf("expected AtPredicate, got %T", q.Predicate)
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
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		wantTarget string
		wantSubQ   bool
		wantNeg    bool
	}{
		{
			name:       "refd with target",
			input:      "type:project refd([[meetings/standup]])",
			wantTarget: "meetings/standup",
		},
		{
			name:       "refd with shorthand target",
			input:      "type:project refd(cursor)",
			wantTarget: "cursor",
		},
		{
			name:     "refd with type subquery",
			input:    "type:project refd(type:meeting)",
			wantSubQ: true,
		},
		{
			name:       "negated refd",
			input:      "type:project !refd([[meetings/standup]])",
			wantTarget: "meetings/standup",
			wantNeg:    true,
		},
		{
			name:     "refd with complex subquery",
			input:    "type:person refd(type:project .status==active)",
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
			rp, ok := q.Predicate.(*RefdPredicate)
			if !ok {
				t.Fatalf("expected RefdPredicate, got %T", q.Predicate)
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
func TestParseRefdShorthand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		input        string
		wantTypeName string
	}{
		{
			name:         "refd shorthand",
			input:        "type:project refd(type:meeting)",
			wantTypeName: "meeting",
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
			rp, ok := q.Predicate.(*RefdPredicate)
			if !ok {
				t.Fatalf("expected RefdPredicate, got %T", q.Predicate)
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
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		predType   string
		wantTarget string
		wantNeg    bool
	}{
		{
			name:       "in with direct target",
			input:      "section in([[projects/website]])",
			predType:   "in",
			wantTarget: "projects/website",
		},
		{
			name:       "within with direct target",
			input:      "section within([[projects/website]])",
			predType:   "within",
			wantTarget: "projects/website",
		},
		{
			name:       "contains with direct target",
			input:      "type:project contains(section .id==[[projects/website#overview]])",
			predType:   "contains",
			wantTarget: "projects/website#overview",
		},
		{
			name:       "has with direct target",
			input:      "type:project has(section .id==[[projects/website#tasks]])",
			predType:   "has",
			wantTarget: "projects/website#tasks",
		},
		{
			name:       "in with direct target (trait query)",
			input:      "trait:todo in([[projects/website]])",
			predType:   "in",
			wantTarget: "projects/website",
		},
		{
			name:       "within with direct target (trait query)",
			input:      "trait:todo within([[projects/website]])",
			predType:   "within",
			wantTarget: "projects/website",
		},
		{
			name:       "negated in with direct target",
			input:      "section !in([[projects/website]])",
			predType:   "in",
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
			if q.Predicate == nil {
				t.Fatal("expected predicate, got nil")
			}

			var target string
			var negated bool
			switch p := q.Predicate.(type) {
			case *InPredicate:
				if tt.predType != "in" {
					t.Fatalf("expected %s, got in", tt.predType)
				}
				target = p.Target
				negated = p.Negated()
			case *WithinPredicate:
				if tt.predType != "within" {
					t.Fatalf("expected %s, got within", tt.predType)
				}
				target = p.Target
				negated = p.Negated()
			case *HasPredicate:
				if tt.predType != "has" {
					t.Fatalf("expected %s, got has", tt.predType)
				}
				target = refTargetFromSectionIDPredicate(t, p.SubQuery)
				negated = p.Negated()
			case *ContainsPredicate:
				if tt.predType != "contains" {
					t.Fatalf("expected %s, got contains", tt.predType)
				}
				target = refTargetFromSectionIDPredicate(t, p.SubQuery)
				negated = p.Negated()
			default:
				t.Fatalf("unexpected predicate type: %T", q.Predicate)
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
func refTargetFromSectionIDPredicate(t *testing.T, q *Query) string {
	t.Helper()
	if q == nil || q.Predicate == nil {
		t.Fatal("expected section subquery predicate")
	}
	if q.Type != QueryTypeSection {
		t.Fatalf("subquery type = %v, want section", q.Type)
	}
	fp, ok := q.Predicate.(*FieldPredicate)
	if !ok {
		t.Fatalf("subquery predicate = %T, want FieldPredicate", q.Predicate)
	}
	if fp.Field != "id" || !fp.IsRefValue {
		t.Fatalf("field predicate = %#v, want .id ref value", fp)
	}
	return fp.Value
}
func TestParseNavigationPredicateErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "scope nav rejects brace subqueries",
			input:   "section in({type:date})",
			wantErr: "brace subqueries are no longer supported; use in(type:...) or in([[target]])",
		},
		{
			name:    "trait scope nav rejects brace subqueries",
			input:   "trait:due in({type:meeting})",
			wantErr: "brace subqueries are no longer supported; use in(type:...) or in([[target]])",
		},
		{
			name:    "scope nav rejects self reference",
			input:   "section in(_)",
			wantErr: "self-reference '_' is no longer supported; write an explicit target or subquery instead",
		},
		{
			name:    "trait scope nav rejects self reference",
			input:   "trait:due in(_)",
			wantErr: "self-reference '_' is no longer supported; write an explicit target or subquery instead",
		},
		{
			name:    "scope nav requires target or subquery",
			input:   "section in()",
			wantErr: "expected scope query or target in in()",
		},
		{
			name:    "trait scope nav requires target or subquery",
			input:   "trait:due in()",
			wantErr: "expected scope query or target in in()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}
