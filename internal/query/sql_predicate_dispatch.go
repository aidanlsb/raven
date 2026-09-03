package query

import (
	"fmt"
	"reflect"
)

// predicateBuilderFunc is the signature for predicate SQL builders.
type predicateBuilderFunc func(*Executor, Predicate, string, string) (string, []interface{}, error)

// predicateBuilderKey identifies a (predicate type, root) combination.
type predicateBuilderKey struct {
	predType interface{}
	root     QueryType
}

// predicateBuilderRegistry maps (predicate type, root) to builder functions.
var predicateBuilderRegistry map[predicateBuilderKey]predicateBuilderFunc

func init() {
	predicateBuilderRegistry = map[predicateBuilderKey]predicateBuilderFunc{
		{reflect.TypeOf((*FieldPredicate)(nil)), QueryTypeLink}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildLinkPredicateSQL(p.(*FieldPredicate), alias)
		},
		{reflect.TypeOf((*FieldPredicate)(nil)), QueryTypeSection}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildSectionFieldPredicateSQL(p.(*FieldPredicate), alias)
		},
		{reflect.TypeOf((*FieldPredicate)(nil)), QueryTypeTrait}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			fp := p.(*FieldPredicate)
			if fp.Field == "value" {
				return e.buildTraitValueFieldPredicateSQL(fp, alias)
			}
			return "", nil, fmt.Errorf("unsupported trait field predicate: .%s (only .value is allowed for traits)", fp.Field)
		},
		{reflect.TypeOf((*FieldPredicate)(nil)), QueryTypeObject}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildFieldPredicateSQL(p.(*FieldPredicate), alias, typeName)
		},

		{reflect.TypeOf((*StringFuncPredicate)(nil)), QueryTypeLink}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildLinkPredicateSQL(p.(*StringFuncPredicate), alias)
		},
		{reflect.TypeOf((*StringFuncPredicate)(nil)), QueryTypeTrait}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildTraitStringFuncPredicateSQL(p.(*StringFuncPredicate), alias)
		},
		{reflect.TypeOf((*StringFuncPredicate)(nil)), QueryTypeSection}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildSectionStringFuncPredicateSQL(p.(*StringFuncPredicate), alias)
		},
		{reflect.TypeOf((*StringFuncPredicate)(nil)), QueryTypeObject}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildStringFuncPredicateSQL(p.(*StringFuncPredicate), alias)
		},

		{reflect.TypeOf((*ArrayQuantifierPredicate)(nil)), QueryTypeTrait}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildTraitArrayQuantifierPredicateSQL(p.(*ArrayQuantifierPredicate), alias)
		},

		{reflect.TypeOf((*WithinPredicate)(nil)), QueryTypeLink}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildLinkWithinPredicateSQL(p.(*WithinPredicate), alias)
		},

		{reflect.TypeOf((*ContentPredicate)(nil)), QueryTypeTrait}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildTraitContentPredicateSQL(p.(*ContentPredicate), alias)
		},

		{reflect.TypeOf((*HasPredicate)(nil)), QueryTypeObject}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildHasPredicateSQL(p.(*HasPredicate), alias)
		},
		{reflect.TypeOf((*ContainsPredicate)(nil)), QueryTypeObject}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildContainsPredicateSQL(p.(*ContainsPredicate), alias)
		},
		{reflect.TypeOf((*InPredicate)(nil)), QueryTypeObject}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildInPredicateSQL(p.(*InPredicate), alias, QueryTypeObject)
		},
		{reflect.TypeOf((*InPredicate)(nil)), QueryTypeTrait}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildInPredicateSQL(p.(*InPredicate), alias, QueryTypeTrait)
		},
		{reflect.TypeOf((*InPredicate)(nil)), QueryTypeSection}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildInPredicateSQL(p.(*InPredicate), alias, QueryTypeSection)
		},
		{reflect.TypeOf((*RefsPredicate)(nil)), QueryTypeObject}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildRefsPredicateSQL(p.(*RefsPredicate), alias, QueryTypeObject)
		},
		{reflect.TypeOf((*RefsPredicate)(nil)), QueryTypeTrait}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildRefsPredicateSQL(p.(*RefsPredicate), alias, QueryTypeTrait)
		},
		{reflect.TypeOf((*RefsPredicate)(nil)), QueryTypeSection}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildRefsPredicateSQL(p.(*RefsPredicate), alias, QueryTypeSection)
		},
		{reflect.TypeOf((*LinksPredicate)(nil)), QueryTypeObject}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildLinksPredicateSQL(p.(*LinksPredicate), alias, QueryTypeObject)
		},
		{reflect.TypeOf((*LinksPredicate)(nil)), QueryTypeTrait}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildLinksPredicateSQL(p.(*LinksPredicate), alias, QueryTypeTrait)
		},
		{reflect.TypeOf((*LinksPredicate)(nil)), QueryTypeSection}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildLinksPredicateSQL(p.(*LinksPredicate), alias, QueryTypeSection)
		},
		{reflect.TypeOf((*RefdPredicate)(nil)), QueryTypeObject}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildRefdPredicateSQL(p.(*RefdPredicate), alias, false)
		},
		{reflect.TypeOf((*RefdPredicate)(nil)), QueryTypeTrait}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildRefdPredicateSQL(p.(*RefdPredicate), alias, false)
		},
		{reflect.TypeOf((*RefdPredicate)(nil)), QueryTypeSection}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildRefdPredicateSQL(p.(*RefdPredicate), alias, false)
		},
		{reflect.TypeOf((*ValuePredicate)(nil)), QueryTypeObject}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildValuePredicateSQL(p.(*ValuePredicate), alias)
		},
		{reflect.TypeOf((*AtPredicate)(nil)), QueryTypeObject}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildAtPredicateSQL(p.(*AtPredicate), alias)
		},
		{reflect.TypeOf((*AtPredicate)(nil)), QueryTypeSection}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildAtPredicateSQL(p.(*AtPredicate), alias)
		},
		{reflect.TypeOf((*AtPredicate)(nil)), QueryTypeTrait}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildAtPredicateSQL(p.(*AtPredicate), alias)
		},
		{reflect.TypeOf((*HasPredicate)(nil)), QueryTypeSection}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildHasPredicateSQL(p.(*HasPredicate), alias)
		},
		{reflect.TypeOf((*ContainsPredicate)(nil)), QueryTypeSection}: func(e *Executor, p Predicate, alias, typeName string) (string, []interface{}, error) {
			return e.buildContainsPredicateSQL(p.(*ContainsPredicate), alias)
		},
	}
}

// buildPredicateSQL builds the SQL condition for a predicate at the given query
// root. Legality of a predicate kind at a root is decided by the shared
// capability matrix (capabilities.go) so that the executor cannot accept a
// combination the validator rejects (or vice versa). Once a predicate kind is
// known to be legal, the registry below is pure routing to the entity-specific
// builder.
func (e *Executor) buildPredicateSQL(root QueryType, pred Predicate, alias, typeName string) (string, []interface{}, error) {
	recurse := func(p Predicate, alias string) (string, []interface{}, error) {
		return e.buildPredicateSQL(root, p, alias, typeName)
	}

	switch p := pred.(type) {
	case *OrPredicate:
		return e.buildOrPredicateSQL(p, alias, recurse)
	case *NotPredicate:
		return e.buildNotPredicateSQL(p, alias, recurse)
	case *GroupPredicate:
		return e.buildGroupPredicateSQL(p, alias, recurse)
	}

	if verr := predicateAllowedAtRoot(root, pred); verr != nil {
		return "", nil, verr
	}

	key := predicateBuilderKey{predType: reflect.TypeOf(pred), root: root}
	if builderFn, ok := predicateBuilderRegistry[key]; ok {
		return builderFn(e, pred, alias, typeName)
	}

	switch p := pred.(type) {
	case *ArrayQuantifierPredicate:
		return e.buildArrayQuantifierPredicateSQL(p, alias, typeName)
	case *WithinPredicate:
		return e.buildWithinPredicateSQL(p, alias, root)
	case *ContentPredicate:
		return e.buildContentPredicateSQL(p, alias)
	default:
		return "", nil, fmt.Errorf("unsupported predicate type: %T", pred)
	}
}

// buildObjectPredicateSQL builds SQL for an object predicate.
func (e *Executor) buildObjectPredicateSQL(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildPredicateSQL(QueryTypeObject, pred, alias, typeName)
}

// buildTraitPredicateSQL builds SQL for a trait predicate.
func (e *Executor) buildTraitPredicateSQL(pred Predicate, alias string) (string, []interface{}, error) {
	return e.buildPredicateSQL(QueryTypeTrait, pred, alias, "")
}

// buildSectionPredicateSQL builds SQL for a section predicate.
func (e *Executor) buildSectionPredicateSQL(pred Predicate, alias string) (string, []interface{}, error) {
	return e.buildPredicateSQL(QueryTypeSection, pred, alias, "")
}
