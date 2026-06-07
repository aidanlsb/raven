package query

import "fmt"

type predicateKind int

const (
	predicateKindObject predicateKind = iota
	predicateKindTrait
	predicateKindAsset
	predicateKindSection
)

func (e *Executor) buildPredicateSQL(kind predicateKind, pred Predicate, alias, typeName string) (string, []interface{}, error) {
	// Shared recursion wiring for OR/group.
	recurse := func(p Predicate, alias string) (string, []interface{}, error) {
		return e.buildPredicateSQL(kind, p, alias, typeName)
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
		if kind == predicateKindAsset {
			return e.buildAssetRefdPredicateSQL(p, alias)
		}
		if kind == predicateKindTrait {
			return "", nil, fmt.Errorf("refd() predicate is only supported for type queries")
		}
		return e.buildRefdPredicateSQL(p, alias, false)
	case *ContentPredicate:
		if kind == predicateKindAsset {
			return "", nil, fmt.Errorf("content() predicate is not valid for asset queries")
		}
		if kind == predicateKindTrait {
			return e.buildTraitContentPredicateSQL(p, alias)
		}
		if kind == predicateKindSection {
			return e.buildContentPredicateSQL(p, alias)
		}
		return e.buildContentPredicateSQL(p, alias)
	case *RefsPredicate:
		if kind == predicateKindAsset {
			return "", nil, fmt.Errorf("refs() predicate is not valid for asset queries")
		}
		if kind == predicateKindTrait {
			return e.buildTraitRefsPredicateSQL(p, alias)
		}
		if kind == predicateKindSection {
			return e.buildRefsPredicateSQL(p, alias)
		}
		return e.buildRefsPredicateSQL(p, alias)
	case *StringFuncPredicate:
		if kind == predicateKindAsset {
			return e.buildAssetStringFuncPredicateSQL(p, alias)
		}
		if kind == predicateKindTrait {
			return e.buildTraitStringFuncPredicateSQL(p, alias)
		}
		if kind == predicateKindSection {
			return e.buildSectionStringFuncPredicateSQL(p, alias)
		}
		return e.buildStringFuncPredicateSQL(p, alias)

	// Object-only predicate nodes (except .value is allowed for traits).
	case *FieldPredicate:
		if kind == predicateKindAsset {
			return e.buildAssetFieldPredicateSQL(p, alias)
		}
		if kind == predicateKindTrait {
			// Allow .value for traits
			if p.Field == "value" {
				return e.buildTraitValueFieldPredicateSQL(p, alias)
			}
			return "", nil, fmt.Errorf("unsupported trait field predicate: .%s (only .value is allowed for traits)", p.Field)
		}
		if kind == predicateKindSection {
			return e.buildSectionFieldPredicateSQL(p, alias)
		}
		return e.buildFieldPredicateSQL(p, alias, typeName)
	case *ArrayQuantifierPredicate:
		if kind == predicateKindAsset {
			return "", nil, fmt.Errorf("array predicates are not valid for asset queries")
		}
		if kind == predicateKindTrait {
			return e.buildTraitArrayQuantifierPredicateSQL(p, alias)
		}
		return e.buildArrayQuantifierPredicateSQL(p, alias, typeName)
	case *HasPredicate:
		if kind == predicateKindAsset {
			return "", nil, fmt.Errorf("has() predicate is not valid for asset queries")
		}
		if kind != predicateKindObject && kind != predicateKindSection {
			return "", nil, fmt.Errorf("unsupported predicate type for has(): %T", pred)
		}
		return e.buildHasPredicateSQL(p, alias)
	case *InPredicate:
		if kind == predicateKindAsset {
			return "", nil, fmt.Errorf("scope predicates are not valid for asset queries")
		}
		if kind == predicateKindObject {
			return "", nil, fmt.Errorf("in() is not valid for root object queries")
		}
		return e.buildInPredicateSQL(p, alias, kind)
	case *ContainsPredicate:
		if kind == predicateKindAsset {
			return "", nil, fmt.Errorf("contains() predicate is not valid for asset queries")
		}
		if kind != predicateKindObject && kind != predicateKindSection {
			return "", nil, fmt.Errorf("unsupported predicate type for contains(): %T", pred)
		}
		return e.buildContainsPredicateSQL(p, alias)

	// Trait-only predicate nodes.
	case *ValuePredicate:
		if kind == predicateKindAsset {
			return "", nil, fmt.Errorf("value predicates are not valid for asset queries")
		}
		if kind != predicateKindTrait {
			return "", nil, fmt.Errorf("unsupported object predicate type: %T", pred)
		}
		return e.buildValuePredicateSQL(p, alias)
	case *WithinPredicate:
		if kind == predicateKindAsset {
			return "", nil, fmt.Errorf("scope predicates are not valid for asset queries")
		}
		if kind == predicateKindObject {
			return "", nil, fmt.Errorf("within() is not valid for root object queries")
		}
		return e.buildWithinPredicateSQL(p, alias, kind)
	case *AtPredicate:
		if kind == predicateKindAsset {
			return "", nil, fmt.Errorf("trait-location predicates are not valid for asset queries")
		}
		if kind != predicateKindTrait {
			return "", nil, fmt.Errorf("unsupported object predicate type: %T", pred)
		}
		return e.buildAtPredicateSQL(p, alias)

	default:
		if kind == predicateKindAsset {
			return "", nil, fmt.Errorf("unsupported asset predicate type: %T", pred)
		}
		if kind == predicateKindTrait {
			return "", nil, fmt.Errorf("unsupported trait predicate type: %T", pred)
		}
		return "", nil, fmt.Errorf("unsupported object predicate type: %T", pred)
	}
}

// buildObjectPredicateSQL builds SQL for an object predicate.
func (e *Executor) buildObjectPredicateSQL(pred Predicate, alias, typeName string) (string, []interface{}, error) {
	return e.buildPredicateSQL(predicateKindObject, pred, alias, typeName)
}

// buildTraitPredicateSQL builds SQL for a trait predicate.
func (e *Executor) buildTraitPredicateSQL(pred Predicate, alias string) (string, []interface{}, error) {
	return e.buildPredicateSQL(predicateKindTrait, pred, alias, "")
}

func (e *Executor) buildSectionPredicateSQL(pred Predicate, alias string) (string, []interface{}, error) {
	return e.buildPredicateSQL(predicateKindSection, pred, alias, "")
}

// buildAssetPredicateSQL builds SQL for an asset predicate.
func (e *Executor) buildAssetPredicateSQL(pred Predicate, alias string) (string, []interface{}, error) {
	return e.buildPredicateSQL(predicateKindAsset, pred, alias, "")
}
