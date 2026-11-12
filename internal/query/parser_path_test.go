package query

import (
	"strings"
	"testing"
)

func TestParseFieldPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantFields  []string
		wantThis    bool
		wantRHS     string
		wantRHSPath *FieldPath
		wantExists  bool
	}{
		{
			name:       "one hop",
			input:      "type:task .project==[[project/foo]]",
			wantFields: []string{"project"},
			wantRHS:    "project/foo",
		},
		{
			name:       "two hop forward path",
			input:      "type:task .project.status==active",
			wantFields: []string{"project", "status"},
			wantRHS:    "active",
		},
		{
			name:       "three hop",
			input:      "type:task .project.company.tier==1",
			wantFields: []string{"project", "company", "tier"},
			wantRHS:    "1",
		},
		{
			name:        "rhs current-subject path",
			input:       "type:task .due>.created",
			wantFields:  []string{"due"},
			wantRHSPath: &FieldPath{Fields: []string{"created"}},
		},
		{
			name:        "rhs two-hop path",
			input:       "type:task .due==.project.created",
			wantFields:  []string{"due"},
			wantRHSPath: &FieldPath{Fields: []string{"project", "created"}},
		},
		{
			name:        "rhs this identity",
			input:       "type:person refd(type:task .owner==this)",
			wantFields:  []string{"owner"},
			wantRHSPath: &FieldPath{This: true},
		},
		{
			name:        "rhs this field",
			input:       "type:person refd(type:task .owner==this.lead)",
			wantFields:  []string{"owner"},
			wantRHSPath: &FieldPath{This: true, Fields: []string{"lead"}},
		},
		{
			name:       "exists multi-hop",
			input:      "type:task exists(.project.email)",
			wantFields: []string{"project", "email"},
			wantExists: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.input, err)
			}
			fp := findFieldPredicate(q.Predicate)
			if fp == nil {
				t.Fatalf("no FieldPredicate in %T", q.Predicate)
			}
			path := fp.path()
			if path.This != tt.wantThis {
				t.Errorf("Path.This = %v, want %v", path.This, tt.wantThis)
			}
			if got := strings.Join(path.Fields, "."); got != strings.Join(tt.wantFields, ".") {
				t.Errorf("Path.Fields = %v, want %v", path.Fields, tt.wantFields)
			}
			if tt.wantExists != fp.IsExists {
				t.Errorf("IsExists = %v, want %v", fp.IsExists, tt.wantExists)
			}
			rhs := fp.rhs()
			if tt.wantRHSPath != nil {
				if rhs.Path == nil {
					t.Fatalf("RHS.Path = nil, want %s", tt.wantRHSPath.String())
				}
				if rhs.Path.This != tt.wantRHSPath.This || strings.Join(rhs.Path.Fields, ".") != strings.Join(tt.wantRHSPath.Fields, ".") {
					t.Errorf("RHS.Path = %s, want %s", rhs.Path.String(), tt.wantRHSPath.String())
				}
				return
			}
			if fp.IsExists {
				return
			}
			if rhs.Literal != tt.wantRHS {
				t.Errorf("RHS.Literal = %q, want %q", rhs.Literal, tt.wantRHS)
			}
		})
	}
}

func TestParseThisPredicate(t *testing.T) {
	t.Parallel()

	q, err := Parse("type:task refd(type:person this.lead==ada)")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fp := findFieldPredicate(q.Predicate)
	if fp == nil {
		t.Fatal("expected FieldPredicate")
	}
	path := fp.path()
	if !path.This || strings.Join(path.Fields, ".") != "lead" {
		t.Fatalf("path = %s, want this.lead", path.String())
	}
	if fp.Value != "ada" {
		t.Fatalf("Value = %q, want ada", fp.Value)
	}
}

func TestParseArrayElementFieldPath(t *testing.T) {
	t.Parallel()

	q, err := Parse(`type:task any(.collaborators, .team==infra)`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	aq, ok := q.Predicate.(*ArrayQuantifierPredicate)
	if !ok {
		t.Fatalf("got %T, want ArrayQuantifierPredicate", q.Predicate)
	}
	fp, ok := aq.ElementPred.(*FieldPredicate)
	if !ok {
		t.Fatalf("element pred %T, want FieldPredicate", aq.ElementPred)
	}
	if got := strings.Join(fp.path().Fields, "."); got != "team" {
		t.Fatalf("element path = %q, want team", got)
	}
	if fp.Value != "infra" {
		t.Fatalf("Value = %q, want infra", fp.Value)
	}
}

func TestParseElementEqualsThis(t *testing.T) {
	t.Parallel()

	q, err := Parse(`type:person refd(type:task any(.collaborators, _==this))`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	refd, ok := q.Predicate.(*RefdPredicate)
	if !ok {
		t.Fatalf("got %T, want RefdPredicate", q.Predicate)
	}
	aq, ok := refd.SubQuery.Predicate.(*ArrayQuantifierPredicate)
	if !ok {
		t.Fatalf("nested pred %T, want ArrayQuantifierPredicate", refd.SubQuery.Predicate)
	}
	eq, ok := aq.ElementPred.(*ElementEqualityPredicate)
	if !ok {
		t.Fatalf("element pred %T, want ElementEqualityPredicate", aq.ElementPred)
	}
	rhs := eq.rhs()
	if rhs.Path == nil || !rhs.Path.IsThisIdentity() {
		t.Fatalf("RHS = %#v, want this", rhs)
	}
}

func TestParseDottedRHSIsPathNotLiteral(t *testing.T) {
	t.Parallel()

	q, err := Parse("link .ext==.pdf")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fp, ok := q.Predicate.(*FieldPredicate)
	if !ok {
		t.Fatalf("got %T", q.Predicate)
	}
	rhs := fp.rhs()
	if rhs.Path == nil || strings.Join(rhs.Path.Fields, ".") != "pdf" {
		t.Fatalf("RHS = %#v, want path .pdf", rhs)
	}
	if fp.Value == ".pdf" {
		t.Fatal("legacy literal \".pdf\" should no longer be stored as Value")
	}
}

func TestParseQuotedDottedLiteralStillWorks(t *testing.T) {
	t.Parallel()

	q, err := Parse(`link .ext==".pdf"`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fp := q.Predicate.(*FieldPredicate)
	if fp.Value != ".pdf" || fp.rhs().Path != nil {
		t.Fatalf("Value = %q path=%v, want quoted literal .pdf", fp.Value, fp.rhs().Path)
	}
}

func TestParseExistingProjectIDComparison(t *testing.T) {
	t.Parallel()

	q, err := Parse("type:task .project==[[project/foo]]")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fp := q.Predicate.(*FieldPredicate)
	if fp.Field != "project" || !fp.IsRefValue || fp.Value != "project/foo" {
		t.Fatalf("pred = field=%q value=%q ref=%v", fp.Field, fp.Value, fp.IsRefValue)
	}
}

func TestParseStringFuncAndOneofPaths(t *testing.T) {
	t.Parallel()

	q, err := Parse(`type:task includes(.project.name, "web")`)
	if err != nil {
		t.Fatalf("Parse includes: %v", err)
	}
	sf := q.Predicate.(*StringFuncPredicate)
	if got := strings.Join(sf.path().Fields, "."); got != "project.name" {
		t.Fatalf("includes path = %q", got)
	}

	q, err = Parse("type:task oneof(.project.status, [active, backlog])")
	if err != nil {
		t.Fatalf("Parse oneof: %v", err)
	}
	or := q.Predicate.(*OrPredicate)
	fp := or.Predicates[0].(*FieldPredicate)
	if got := strings.Join(fp.path().Fields, "."); got != "project.status" {
		t.Fatalf("oneof path = %q", got)
	}
}

func TestParseSortRejectsPath(t *testing.T) {
	t.Parallel()

	_, err := Parse("type:task sort(.project.status)")
	if err == nil {
		t.Fatal("expected sort path error")
	}
	if !strings.Contains(err.Error(), "sort() only supports a field of the query root") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseTopLevelThisIsSyntaxOK(t *testing.T) {
	t.Parallel()
	// Parser accepts this; the validator rejects top-level this.
	q, err := Parse("type:task .owner==this")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fp := q.Predicate.(*FieldPredicate)
	if !fp.rhs().isThis() {
		t.Fatalf("expected this RHS, got %#v", fp.rhs())
	}
}

func findFieldPredicate(pred Predicate) *FieldPredicate {
	switch p := pred.(type) {
	case *FieldPredicate:
		return p
	case *RefdPredicate:
		if p.SubQuery != nil {
			return findFieldPredicate(p.SubQuery.Predicate)
		}
	case *RefsPredicate:
		if p.SubQuery != nil {
			return findFieldPredicate(p.SubQuery.Predicate)
		}
	case *GroupPredicate:
		for _, sub := range p.Predicates {
			if fp := findFieldPredicate(sub); fp != nil {
				return fp
			}
		}
	case *OrPredicate:
		for _, sub := range p.Predicates {
			if fp := findFieldPredicate(sub); fp != nil {
				return fp
			}
		}
	case *ArrayQuantifierPredicate:
		return findFieldPredicate(p.ElementPred)
	}
	return nil
}
