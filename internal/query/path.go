package query

import (
	"fmt"
	"strings"
)

// String renders a field path in RQL syntax: `.a.b`, `this`, or `this.a.b`.
func (p FieldPath) String() string {
	switch {
	case p.This && len(p.Fields) == 0:
		return "this"
	case p.This:
		return "this." + strings.Join(p.Fields, ".")
	case len(p.Fields) == 0:
		return "."
	default:
		return "." + strings.Join(p.Fields, ".")
	}
}

// IsThisIdentity reports whether the path is the bare enclosing subject (`this`).
func (p FieldPath) IsThisIdentity() bool {
	return p.This && len(p.Fields) == 0
}

// IsIdentity reports whether the path names the root itself (`this` with no hops).
func (p FieldPath) IsIdentity() bool {
	return len(p.Fields) == 0
}

func (p FieldPath) clone() FieldPath {
	if len(p.Fields) == 0 {
		return FieldPath{This: p.This}
	}
	fields := make([]string, len(p.Fields))
	copy(fields, p.Fields)
	return FieldPath{This: p.This, Fields: fields}
}

func (p FieldPath) first() string {
	if len(p.Fields) == 0 {
		return ""
	}
	return p.Fields[0]
}

func (p FieldPath) hasHops() bool {
	return len(p.Fields) > 1
}

func fieldPathFromName(name string) FieldPath {
	if name == "" {
		return FieldPath{}
	}
	return FieldPath{Fields: []string{name}}
}

func (p *FieldPredicate) path() FieldPath {
	if p == nil {
		return FieldPath{}
	}
	if p.Path.This || len(p.Path.Fields) > 0 {
		return p.Path
	}
	return fieldPathFromName(p.Field)
}

func (p *FieldPredicate) rhs() CompareRHS {
	if p == nil {
		return CompareRHS{}
	}
	if p.RHS.Path != nil || p.RHS.Literal != "" || p.RHS.IsRefValue {
		return p.RHS
	}
	return CompareRHS{Literal: p.Value, IsRefValue: p.IsRefValue}
}

func (p *FieldPredicate) isSimpleField() bool {
	if p == nil {
		return false
	}
	path := p.path()
	if path.This || len(path.Fields) != 1 {
		return false
	}
	return p.rhs().Path == nil
}

func (p *StringFuncPredicate) path() FieldPath {
	if p == nil {
		return FieldPath{}
	}
	if p.IsElementRef {
		return FieldPath{}
	}
	if p.Path.This || len(p.Path.Fields) > 0 {
		return p.Path
	}
	return fieldPathFromName(p.Field)
}

func (p *StringFuncPredicate) isSimpleField() bool {
	if p == nil || p.IsElementRef {
		return false
	}
	path := p.path()
	return !path.This && len(path.Fields) == 1
}

func (p *ArrayQuantifierPredicate) path() FieldPath {
	if p == nil {
		return FieldPath{}
	}
	if p.Path.This || len(p.Path.Fields) > 0 {
		return p.Path
	}
	return fieldPathFromName(p.Field)
}

func (p *ArrayQuantifierPredicate) isSimpleField() bool {
	if p == nil {
		return false
	}
	path := p.path()
	return !path.This && len(path.Fields) == 1
}

func (p *ElementEqualityPredicate) rhs() CompareRHS {
	if p == nil {
		return CompareRHS{}
	}
	if p.RHS.Path != nil || p.RHS.Literal != "" || p.RHS.IsRefValue {
		return p.RHS
	}
	return CompareRHS{Literal: p.Value, IsRefValue: p.IsRefValue}
}

func (r CompareRHS) isThis() bool {
	return r.Path != nil && r.Path.This
}

func literalRHS(value string, isRef bool) CompareRHS {
	return CompareRHS{Literal: value, IsRefValue: isRef}
}

func pathRHS(path FieldPath) CompareRHS {
	cloned := path.clone()
	return CompareRHS{Path: &cloned}
}

func isThisKeyword(tok Token) bool {
	if tok.Type != TokenIdent {
		return false
	}
	val := strings.ToLower(tok.Value)
	return val == "this" || strings.HasPrefix(val, "this.")
}

func splitDottedIdent(value string) ([]string, error) {
	parts := strings.Split(value, ".")
	if len(parts) == 0 {
		return nil, fmt.Errorf("expected field name after '.'")
	}
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("expected field name after '.'")
		}
	}
	return parts, nil
}

// queryCorrelatesViaThis reports whether a query's own predicates (not nested
// subqueries) mention `this`. Nested queries rebind `this` to their own
// enclosing subject, so they do not correlate this query to its outer subject.
func queryCorrelatesViaThis(q *Query) bool {
	if q == nil {
		return false
	}
	return predicateCorrelatesViaThis(q.Predicate)
}

func predicateCorrelatesViaThis(pred Predicate) bool {
	if pred == nil {
		return false
	}
	switch p := pred.(type) {
	case *FieldPredicate:
		path := p.path()
		rhs := p.rhs()
		return path.This || (rhs.Path != nil && rhs.Path.This)
	case *StringFuncPredicate:
		return p.path().This
	case *ArrayQuantifierPredicate:
		if p.path().This {
			return true
		}
		return predicateCorrelatesViaThis(p.ElementPred)
	case *ElementEqualityPredicate:
		rhs := p.rhs()
		return rhs.Path != nil && rhs.Path.This
	case *OrPredicate:
		for _, sub := range p.Predicates {
			if predicateCorrelatesViaThis(sub) {
				return true
			}
		}
	case *GroupPredicate:
		for _, sub := range p.Predicates {
			if predicateCorrelatesViaThis(sub) {
				return true
			}
		}
	case *NotPredicate:
		return predicateCorrelatesViaThis(p.Inner)
	}
	return false
}
