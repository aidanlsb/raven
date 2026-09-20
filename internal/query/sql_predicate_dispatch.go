package query

import "fmt"

// predicateBuilderFunc is the signature for leaf-predicate SQL builders.
type predicateBuilderFunc func(*Executor, Predicate, string, string) (string, []interface{}, error)

// buildPredicateSQL builds the SQL condition for a predicate at the given query
// root. Legality of a predicate kind at a root is decided by the shared
// capability matrix (capabilities.go) so that the executor cannot accept a
// combination the validator rejects (or vice versa). Once a predicate kind is
// known to be legal, leafPredicateSQLBuilder is pure routing to the
// entity-specific builder.
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

	builderFn := leafPredicateSQLBuilder(root, pred)
	if builderFn == nil {
		return "", nil, fmt.Errorf("unsupported predicate type: %T", pred)
	}
	return builderFn(e, pred, alias, typeName)
}

// leafPredicateSQLBuilder returns the SQL builder for a leaf predicate at
// root, or nil if that (kind, root) pair has no route. Composite nodes
// (OR/NOT/GROUP) are handled before this lookup. Capability checking is the
// caller's job; a nil result after a successful capability check is a routing
// bug rather than an illegal query.
func leafPredicateSQLBuilder(root QueryType, pred Predicate) predicateBuilderFunc {
	switch p := pred.(type) {
	case *FieldPredicate:
		switch root {
		case QueryTypeLink:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildLinkPredicateSQL(p, alias)
			}
		case QueryTypeSection:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildSectionFieldPredicateSQL(p, alias)
			}
		case QueryTypeTrait:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				if p.Field == "value" {
					return e.buildTraitValueFieldPredicateSQL(p, alias)
				}
				return "", nil, fmt.Errorf("unsupported trait field predicate: .%s (only .value is allowed for traits)", p.Field)
			}
		case QueryTypeObject:
			return func(e *Executor, _ Predicate, alias, typeName string) (string, []interface{}, error) {
				return e.buildFieldPredicateSQL(p, alias, typeName)
			}
		}

	case *StringFuncPredicate:
		switch root {
		case QueryTypeLink:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildLinkPredicateSQL(p, alias)
			}
		case QueryTypeTrait:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildTraitStringFuncPredicateSQL(p, alias)
			}
		case QueryTypeSection:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildSectionStringFuncPredicateSQL(p, alias)
			}
		case QueryTypeObject:
			return func(e *Executor, _ Predicate, alias, typeName string) (string, []interface{}, error) {
				return e.buildStringFuncPredicateSQL(p, alias, typeName)
			}
		}

	case *ArrayQuantifierPredicate:
		switch root {
		case QueryTypeObject:
			return func(e *Executor, _ Predicate, alias, typeName string) (string, []interface{}, error) {
				return e.buildArrayQuantifierPredicateSQL(p, alias, typeName)
			}
		case QueryTypeTrait:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildTraitArrayQuantifierPredicateSQL(p, alias)
			}
		}

	case *WithinPredicate:
		switch root {
		case QueryTypeLink:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildLinkWithinPredicateSQL(p, alias)
			}
		case QueryTypeTrait, QueryTypeSection:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildWithinPredicateSQL(p, alias, root)
			}
		}

	case *ContentPredicate:
		switch root {
		case QueryTypeTrait:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildTraitContentPredicateSQL(p, alias)
			}
		case QueryTypeObject, QueryTypeSection:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildContentPredicateSQL(p, alias)
			}
		}

	case *HasPredicate:
		switch root {
		case QueryTypeObject, QueryTypeSection:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildHasPredicateSQL(p, alias)
			}
		}

	case *ContainsPredicate:
		switch root {
		case QueryTypeObject, QueryTypeSection:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildContainsPredicateSQL(p, alias)
			}
		}

	case *InPredicate:
		switch root {
		case QueryTypeTrait, QueryTypeSection:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildInPredicateSQL(p, alias, root)
			}
		}

	case *RefsPredicate:
		switch root {
		case QueryTypeObject, QueryTypeTrait, QueryTypeSection:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildRefsPredicateSQL(p, alias, root)
			}
		}

	case *LinksPredicate:
		switch root {
		case QueryTypeObject, QueryTypeTrait, QueryTypeSection:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildLinksPredicateSQL(p, alias, root)
			}
		}

	case *RefdPredicate:
		switch root {
		case QueryTypeObject, QueryTypeSection:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildRefdPredicateSQL(p, alias, false)
			}
		}

	case *AtPredicate:
		switch root {
		case QueryTypeTrait:
			return func(e *Executor, _ Predicate, alias, _ string) (string, []interface{}, error) {
				return e.buildAtPredicateSQL(p, alias)
			}
		}
	}

	return nil
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
