package query

import "fmt"

// predicateBuilderFunc is the signature for predicate SQL builders.
type predicateBuilderFunc func(*Executor, Predicate, string, string) (string, []interface{}, error)

// simpleBuildFunc is for predicates that don't vary by root.
type simpleBuildFunc func(*Executor, Predicate, string, QueryType, string) (string, []interface{}, error)

// predicateBuilderKey identifies a (predicate type, root) combination.
type predicateBuilderKey struct {
	predType string
	root     QueryType
}

// predicateBuilderRegistry maps (predicate type, root) to builder functions.
var predicateBuilderRegistry map[predicateBuilderKey]predicateBuilderFunc

// simplePredicateRegistry maps predicate types to root-independent builders.
var simplePredicateRegistry map[string]simpleBuildFunc

func init() {
	predicateBuilderRegistry = map[predicateBuilderKey]predicateBuilderFunc{
		{"FieldPredicate", QueryTypeLink}:    (*Executor).buildFieldPredicateForLink,
		{"FieldPredicate", QueryTypeSection}: (*Executor).buildFieldPredicateForSection,
		{"FieldPredicate", QueryTypeTrait}:   (*Executor).buildFieldPredicateForTrait,
		{"FieldPredicate", QueryTypeObject}:  (*Executor).buildFieldPredicateForObject,

		{"StringFuncPredicate", QueryTypeLink}:    (*Executor).buildStringFuncPredicateForLink,
		{"StringFuncPredicate", QueryTypeTrait}:   (*Executor).buildStringFuncPredicateForTrait,
		{"StringFuncPredicate", QueryTypeSection}: (*Executor).buildStringFuncPredicateForSection,
		{"StringFuncPredicate", QueryTypeObject}:  (*Executor).buildStringFuncPredicateForObject,

		{"ArrayQuantifierPredicate", QueryTypeTrait}:  (*Executor).buildArrayQuantifierPredicateForTrait,
		{"ArrayQuantifierPredicate", QueryTypeObject}: (*Executor).buildArrayQuantifierPredicateForObject,

		{"WithinPredicate", QueryTypeLink}:    (*Executor).buildWithinPredicateForLink,
		{"WithinPredicate", QueryTypeObject}:  (*Executor).buildWithinPredicateForObject,
		{"WithinPredicate", QueryTypeSection}: (*Executor).buildWithinPredicateForSection,
		{"WithinPredicate", QueryTypeTrait}:   (*Executor).buildWithinPredicateForTrait,

		{"ContentPredicate", QueryTypeTrait}:  (*Executor).buildContentPredicateForTrait,
		{"ContentPredicate", QueryTypeObject}: (*Executor).buildContentPredicateForObject,
	}

	simplePredicateRegistry = map[string]simpleBuildFunc{
		"HasPredicate":      (*Executor).buildHasPredicateSimple,
		"ContainsPredicate": (*Executor).buildContainsPredicateSimple,
		"InPredicate":       (*Executor).buildInPredicateSimple,
		"RefsPredicate":     (*Executor).buildRefsPredicateSimple,
		"LinksPredicate":    (*Executor).buildLinksPredicateSimple,
		"RefdPredicate":     (*Executor).buildRefdPredicateSimple,
		"ValuePredicate":    (*Executor).buildValuePredicateSimple,
		"AtPredicate":       (*Executor).buildAtPredicateSimple,
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

	predType := fmt.Sprintf("%T", pred)[7:]

	key := predicateBuilderKey{predType: predType, root: root}
	if builderFn, ok := predicateBuilderRegistry[key]; ok {
		return builderFn(e, pred, alias, typeName)
	}

	if simpleFn, ok := simplePredicateRegistry[predType]; ok {
		return simpleFn(e, pred, alias, root, typeName)
	}

	return "", nil, fmt.Errorf("unsupported predicate type: %T", pred)
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

// Registry adapter functions for (predicate type × root) dispatch.

func (e *Executor) buildFieldPredicateForLink(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildLinkPredicateSQL(pred.(*FieldPredicate), alias)
}

func (e *Executor) buildFieldPredicateForSection(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildSectionFieldPredicateSQL(pred.(*FieldPredicate), alias)
}

func (e *Executor) buildFieldPredicateForTrait(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	p := pred.(*FieldPredicate)
	if p.Field == "value" {
		return e.buildTraitValueFieldPredicateSQL(p, alias)
	}
	return "", nil, fmt.Errorf("unsupported trait field predicate: .%s (only .value is allowed for traits)", p.Field)
}

func (e *Executor) buildFieldPredicateForObject(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildFieldPredicateSQL(pred.(*FieldPredicate), alias, typeName)
}

func (e *Executor) buildStringFuncPredicateForLink(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildLinkPredicateSQL(pred.(*StringFuncPredicate), alias)
}

func (e *Executor) buildStringFuncPredicateForTrait(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildTraitStringFuncPredicateSQL(pred.(*StringFuncPredicate), alias)
}

func (e *Executor) buildStringFuncPredicateForSection(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildSectionStringFuncPredicateSQL(pred.(*StringFuncPredicate), alias)
}

func (e *Executor) buildStringFuncPredicateForObject(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildStringFuncPredicateSQL(pred.(*StringFuncPredicate), alias)
}

func (e *Executor) buildArrayQuantifierPredicateForTrait(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildTraitArrayQuantifierPredicateSQL(pred.(*ArrayQuantifierPredicate), alias)
}

func (e *Executor) buildArrayQuantifierPredicateForObject(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildArrayQuantifierPredicateSQL(pred.(*ArrayQuantifierPredicate), alias, typeName)
}

func (e *Executor) buildWithinPredicateForLink(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildLinkWithinPredicateSQL(pred.(*WithinPredicate), alias)
}

func (e *Executor) buildWithinPredicateForObject(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildWithinPredicateSQL(pred.(*WithinPredicate), alias, QueryTypeObject)
}

func (e *Executor) buildContentPredicateForTrait(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildTraitContentPredicateSQL(pred.(*ContentPredicate), alias)
}

func (e *Executor) buildContentPredicateForObject(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildContentPredicateSQL(pred.(*ContentPredicate), alias)
}

// Simple predicate adapters (root-independent).

func (e *Executor) buildHasPredicateSimple(pred Predicate, alias string, root QueryType, typeName string) (string, []interface{}, error) {
	return e.buildHasPredicateSQL(pred.(*HasPredicate), alias)
}

func (e *Executor) buildContainsPredicateSimple(pred Predicate, alias string, root QueryType, typeName string) (string, []interface{}, error) {
	return e.buildContainsPredicateSQL(pred.(*ContainsPredicate), alias)
}

func (e *Executor) buildInPredicateSimple(pred Predicate, alias string, root QueryType, typeName string) (string, []interface{}, error) {
	return e.buildInPredicateSQL(pred.(*InPredicate), alias, root)
}

func (e *Executor) buildRefsPredicateSimple(pred Predicate, alias string, root QueryType, typeName string) (string, []interface{}, error) {
	return e.buildRefsPredicateSQL(pred.(*RefsPredicate), alias, root)
}

func (e *Executor) buildLinksPredicateSimple(pred Predicate, alias string, root QueryType, typeName string) (string, []interface{}, error) {
	return e.buildLinksPredicateSQL(pred.(*LinksPredicate), alias, root)
}

func (e *Executor) buildRefdPredicateSimple(pred Predicate, alias string, root QueryType, typeName string) (string, []interface{}, error) {
	return e.buildRefdPredicateSQL(pred.(*RefdPredicate), alias, false)
}

func (e *Executor) buildValuePredicateSimple(pred Predicate, alias string, root QueryType, typeName string) (string, []interface{}, error) {
	return e.buildValuePredicateSQL(pred.(*ValuePredicate), alias)
}

func (e *Executor) buildAtPredicateSimple(pred Predicate, alias string, root QueryType, typeName string) (string, []interface{}, error) {
	return e.buildAtPredicateSQL(pred.(*AtPredicate), alias)
}

func (e *Executor) buildWithinPredicateForSection(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildWithinPredicateSQL(pred.(*WithinPredicate), alias, QueryTypeSection)
}

func (e *Executor) buildWithinPredicateForTrait(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildWithinPredicateSQL(pred.(*WithinPredicate), alias, QueryTypeTrait)
}


