package query

import "fmt"

type predicateKind int

const (
	predicateKindObject predicateKind = iota
	predicateKindTrait
)

func (e *Executor) buildPredicateSQL(kind predicateKind, pred Predicate, alias string) (string, []interface{}, error) {
	// Shared recursion wiring for OR/group.
	recurse := func(p Predicate, alias string) (string, []interface{}, error) {
		return e.buildPredicateSQL(kind, p, alias)
	}

	switch p := pred.(type) {
	// Shared predicate nodes (exist in both object and trait query contexts).
	case *OrPredicate:
		return e.buildOrPredicateSQL(p, alias, recurse)
	case *NotPredicate:
		return e.buildNotPredicateSQL(p, alias, recurse)
	case *GroupPredicate:
		return e.buildGroupPredicateSQL(p, alias, recurse)
	case *RefdPredicate:
		if kind == predicateKindTrait {
			return "", nil, fmt.Errorf("refd: predicate is only supported for object queries")
		}
		return e.buildRefdPredicateSQL(p, alias, false)
	case *ContentPredicate:
		if kind == predicateKindTrait {
			return e.buildTraitContentPredicateSQL(p, alias)
		}
		return e.buildContentPredicateSQL(p, alias)
	case *RefsPredicate:
		if kind == predicateKindTrait {
			return e.buildTraitRefsPredicateSQL(p, alias)
		}
		return e.buildRefsPredicateSQL(p, alias)
	case *StringFuncPredicate:
		if kind == predicateKindTrait {
			return e.buildTraitStringFuncPredicateSQL(p, alias)
		}
		return e.buildStringFuncPredicateSQL(p, alias)

	// Object-only predicate nodes (except .value is allowed for traits).
	case *FieldPredicate:
		if kind == predicateKindTrait {
			// Allow .value for traits
			if p.Field == "value" {
				return e.buildTraitValueFieldPredicateSQL(p, alias)
			}
			return "", nil, fmt.Errorf("unsupported trait field predicate: .%s (only .value is allowed for traits)", p.Field)
		}
		return e.buildFieldPredicateSQL(p, alias)
	case *ArrayQuantifierPredicate:
		if kind != predicateKindObject {
			return "", nil, fmt.Errorf("unsupported trait predicate type: %T", pred)
		}
		return e.buildArrayQuantifierPredicateSQL(p, alias)
	case *HasPredicate:
		if kind != predicateKindObject {
			return "", nil, fmt.Errorf("unsupported trait predicate type: %T", pred)
		}
		return e.buildHasPredicateSQL(p, alias)
	case *ParentPredicate:
		if kind != predicateKindObject {
			return "", nil, fmt.Errorf("unsupported trait predicate type: %T", pred)
		}
		return e.buildParentPredicateSQL(p, alias)
	case *AncestorPredicate:
		if kind != predicateKindObject {
			return "", nil, fmt.Errorf("unsupported trait predicate type: %T", pred)
		}
		return e.buildAncestorPredicateSQL(p, alias)
	case *ChildPredicate:
		if kind != predicateKindObject {
			return "", nil, fmt.Errorf("unsupported trait predicate type: %T", pred)
		}
		return e.buildChildPredicateSQL(p, alias)
	case *DescendantPredicate:
		if kind != predicateKindObject {
			return "", nil, fmt.Errorf("unsupported trait predicate type: %T", pred)
		}
		return e.buildDescendantPredicateSQL(p, alias)
	case *ContainsPredicate:
		if kind != predicateKindObject {
			return "", nil, fmt.Errorf("unsupported trait predicate type: %T", pred)
		}
		return e.buildContainsPredicateSQL(p, alias)

	// Trait-only predicate nodes.
	case *ValuePredicate:
		if kind != predicateKindTrait {
			return "", nil, fmt.Errorf("unsupported object predicate type: %T", pred)
		}
		return e.buildValuePredicateSQL(p, alias)
	case *OnPredicate:
		if kind != predicateKindTrait {
			return "", nil, fmt.Errorf("unsupported object predicate type: %T", pred)
		}
		return e.buildOnPredicateSQL(p, alias)
	case *WithinPredicate:
		if kind != predicateKindTrait {
			return "", nil, fmt.Errorf("unsupported object predicate type: %T", pred)
		}
		return e.buildWithinPredicateSQL(p, alias)
	case *AtPredicate:
		if kind != predicateKindTrait {
			return "", nil, fmt.Errorf("unsupported object predicate type: %T", pred)
		}
		return e.buildAtPredicateSQL(p, alias)

	default:
		if kind == predicateKindTrait {
			return "", nil, fmt.Errorf("unsupported trait predicate type: %T", pred)
		}
		return "", nil, fmt.Errorf("unsupported object predicate type: %T", pred)
	}
}

// buildObjectPredicateSQL builds SQL for an object predicate.
func (e *Executor) buildObjectPredicateSQL(pred Predicate, alias string) (string, []interface{}, error) {
	return e.buildPredicateSQL(predicateKindObject, pred, alias)
}

// buildTraitPredicateSQL builds SQL for a trait predicate.
func (e *Executor) buildTraitPredicateSQL(pred Predicate, alias string) (string, []interface{}, error) {
	return e.buildPredicateSQL(predicateKindTrait, pred, alias)
}
