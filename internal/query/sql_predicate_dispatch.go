package query

import "fmt"

// buildPredicateSQL builds the SQL condition for a predicate at the given query
// root. Legality of a predicate kind at a root is decided by the shared
// capability matrix (capabilities.go) so that the executor cannot accept a
// combination the validator rejects (or vice versa). Once a predicate kind is
// known to be legal, the switch below is pure routing to the entity-specific
// builder.
func (e *Executor) buildPredicateSQL(root QueryType, pred Predicate, alias, typeName string) (string, []interface{}, error) {
	// Shared recursion wiring for OR/NOT/group composition.
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

	// Coarse legality: shared with the validator via the capability matrix.
	// The validator normally rejects illegal combinations first; this keeps SQL
	// generation from diverging if validation was somehow skipped.
	if verr := predicateAllowedAtRoot(root, pred); verr != nil {
		return "", nil, verr
	}

	switch p := pred.(type) {
	case *FieldPredicate:
		switch root {
		case QueryTypeLink:
			return e.buildLinkPredicateSQL(p, alias)
		case QueryTypeSection:
			return e.buildSectionFieldPredicateSQL(p, alias)
		case QueryTypeTrait:
			// Only .value is meaningful for traits; other fields are rejected
			// structurally by the validator, this is the defensive mirror.
			if p.Field == "value" {
				return e.buildTraitValueFieldPredicateSQL(p, alias)
			}
			return "", nil, fmt.Errorf("unsupported trait field predicate: .%s (only .value is allowed for traits)", p.Field)
		default:
			return e.buildFieldPredicateSQL(p, alias, typeName)
		}

	case *StringFuncPredicate:
		switch root {
		case QueryTypeLink:
			return e.buildLinkPredicateSQL(p, alias)
		case QueryTypeTrait:
			return e.buildTraitStringFuncPredicateSQL(p, alias)
		case QueryTypeSection:
			return e.buildSectionStringFuncPredicateSQL(p, alias)
		default:
			return e.buildStringFuncPredicateSQL(p, alias)
		}

	case *ArrayQuantifierPredicate:
		if root == QueryTypeTrait {
			return e.buildTraitArrayQuantifierPredicateSQL(p, alias)
		}
		return e.buildArrayQuantifierPredicateSQL(p, alias, typeName)

	case *HasPredicate:
		return e.buildHasPredicateSQL(p, alias)

	case *ContainsPredicate:
		return e.buildContainsPredicateSQL(p, alias)

	case *InPredicate:
		return e.buildInPredicateSQL(p, alias, root)

	case *WithinPredicate:
		if root == QueryTypeLink {
			return e.buildLinkWithinPredicateSQL(p, alias)
		}
		return e.buildWithinPredicateSQL(p, alias, root)

	case *RefsPredicate:
		return e.buildRefsPredicateSQL(p, alias, root)

	case *LinksPredicate:
		return e.buildLinksPredicateSQL(p, alias, root)

	case *RefdPredicate:
		return e.buildRefdPredicateSQL(p, alias, false)

	case *ContentPredicate:
		if root == QueryTypeTrait {
			return e.buildTraitContentPredicateSQL(p, alias)
		}
		return e.buildContentPredicateSQL(p, alias)

	case *ValuePredicate:
		return e.buildValuePredicateSQL(p, alias)

	case *AtPredicate:
		return e.buildAtPredicateSQL(p, alias)

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
