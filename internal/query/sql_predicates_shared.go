package query

import (
	"fmt"
	"strings"
)

// buildOrPredicateSQL builds SQL for OR predicates.
func (e *Executor) buildOrPredicateSQL(p *OrPredicate, alias string,
	buildFn func(Predicate, string) (string, []interface{}, error)) (string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	for _, pred := range p.Predicates {
		cond, predArgs, err := buildFn(pred, alias)
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, cond)
		args = append(args, predArgs...)
	}

	return "(" + strings.Join(conditions, " OR ") + ")", args, nil
}

// buildNotPredicateSQL builds SQL for NOT predicates.
func (e *Executor) buildNotPredicateSQL(p *NotPredicate, alias string,
	buildFn func(Predicate, string) (string, []interface{}, error)) (string, []interface{}, error) {
	cond, args, err := buildFn(p.Inner, alias)
	if err != nil {
		return "", nil, err
	}
	return "NOT (" + cond + ")", args, nil
}

// buildGroupPredicateSQL builds SQL for grouped predicates.
func (e *Executor) buildGroupPredicateSQL(p *GroupPredicate, alias string,
	buildFn func(Predicate, string) (string, []interface{}, error)) (string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	for _, pred := range p.Predicates {
		cond, predArgs, err := buildFn(pred, alias)
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, cond)
		args = append(args, predArgs...)
	}

	return "(" + strings.Join(conditions, " AND ") + ")", args, nil
}

// edgeSourceCondition scopes refs/links edge rows to a query root.
// Object roots include the file object and any section-fragment source IDs
// under it (source_id = id OR source_id LIKE id || '#%'). Trait roots use the
// source line; section roots use the complete subtree line range.
func edgeSourceCondition(edgeAlias, rootAlias string, root QueryType, predName string) (string, error) {
	switch root {
	case QueryTypeObject:
		return fmt.Sprintf("(%[1]s.source_id = %[2]s.id OR %[1]s.source_id LIKE %[2]s.id || '#%%')", edgeAlias, rootAlias), nil
	case QueryTypeTrait:
		return fmt.Sprintf("%[1]s.file_path = %[2]s.file_path AND %[1]s.line_number = %[2]s.line_number", edgeAlias, rootAlias), nil
	case QueryTypeSection:
		return fmt.Sprintf(
			"%[1]s.file_path = %[2]s.file_path AND %[1]s.line_number >= %[2]s.line_start AND (%[2]s.subtree_line_end IS NULL OR %[1]s.line_number <= %[2]s.subtree_line_end)",
			edgeAlias, rootAlias,
		), nil
	default:
		return "", fmt.Errorf("%s() predicate is not supported for %s queries", predName, queryTypeName(root))
	}
}

// buildStringFuncCondition builds SQL for string function predicates against a field expression.
func buildStringFuncCondition(funcType StringFuncType, fieldExpr string, value string, caseSensitive bool) (string, []interface{}, error) {
	wrapLower := !caseSensitive

	switch funcType {
	case StringFuncIncludes:
		return likeCond(fieldExpr, wrapLower), []interface{}{"%" + escapeLikePattern(value) + "%"}, nil

	case StringFuncStartsWith:
		return likeCond(fieldExpr, wrapLower), []interface{}{escapeLikePattern(value) + "%"}, nil

	case StringFuncEndsWith:
		return likeCond(fieldExpr, wrapLower), []interface{}{"%" + escapeLikePattern(value)}, nil

	case StringFuncMatches:
		cond := fmt.Sprintf("%s REGEXP ?", fieldExpr)
		if wrapLower {
			return cond, []interface{}{"(?i)" + value}, nil
		}
		return cond, []interface{}{value}, nil
	default:
		return "", nil, fmt.Errorf("unsupported string function: %v", funcType)
	}
}

// refResolvedIDPreferredMatchSQL matches a refs-table row against a target
// identity. Prefer canonical target_id when the row resolved; only compare
// target_raw when target_id is NULL.
//
// identExpr is "?" for bound arguments in order (resolvedID, rawQuery), or a
// SQL expression such as "o.id" when joining against another row's identity.
func refResolvedIDPreferredMatchSQL(refAlias, identExpr string) string {
	return fmt.Sprintf(
		"(%s.target_id = %s OR (%s.target_id IS NULL AND %s.target_raw = %s))",
		refAlias, identExpr, refAlias, refAlias, identExpr,
	)
}

// refResolvedIDPreferredMatch is the bound-argument form of
// refResolvedIDPreferredMatchSQL. Args are (resolvedID, rawQuery).
func refResolvedIDPreferredMatch(refAlias, resolvedID, rawQuery string) (string, []interface{}) {
	return refResolvedIDPreferredMatchSQL(refAlias, "?"), []interface{}{resolvedID, rawQuery}
}

// buildRefStringFuncMatchSQL matches a string function against a refs-table
// row's resolved target_id or stored target_raw.
func buildRefStringFuncMatchSQL(p *StringFuncPredicate, refAlias string) (string, []interface{}, error) {
	idCond, idArgs, err := buildStringFuncCondition(p.FuncType, refAlias+".target_id", p.Value, p.CaseSensitive)
	if err != nil {
		return "", nil, err
	}
	rawCond, rawArgs, err := buildStringFuncCondition(p.FuncType, refAlias+".target_raw", p.Value, p.CaseSensitive)
	if err != nil {
		return "", nil, err
	}
	return "(" + idCond + " OR " + rawCond + ")", append(idArgs, rawArgs...), nil
}

// buildRefdPredicateSQL builds SQL for refd(...) predicates.
// Matches objects/traits that are referenced by the subquery matches.
// isTrait indicates if we're building for a trait query (uses different columns).
func (e *Executor) buildRefdPredicateSQL(p *RefdPredicate, alias string, isTrait bool) (string, []interface{}, error) {
	if p.Target != "" {
		// Referenced by a specific source
		sourceID, err := e.resolveTarget(p.Target)
		if err != nil {
			return "", nil, err
		}
		cond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs r
			WHERE (r.source_id = ? OR r.source_id LIKE ? || '#%%')
			  AND %s
		)`, refResolvedIDPreferredMatchSQL("r", alias+".id"))
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, []interface{}{sourceID, sourceID}, nil
	}

	// Subquery - referenced by objects/traits matching the subquery
	var sourceConditions []string
	var args []interface{}

	if p.SubQuery.Type == QueryTypeObject {
		sourceConditions = append(sourceConditions, "src.type = ?")
		args = append(args, p.SubQuery.TypeName)

		if p.SubQuery.Predicate != nil {
			cond, predArgs, err := e.buildObjectPredicateSQL(p.SubQuery.Predicate, "src", p.SubQuery.TypeName)
			if err != nil {
				return "", nil, err
			}
			sourceConditions = append(sourceConditions, cond)
			args = append(args, predArgs...)
		}

		cond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs r
			JOIN objects src ON (r.source_id = src.id OR r.source_id LIKE src.id || '#%%')
			WHERE %s
			  AND %s
		)`, refResolvedIDPreferredMatchSQL("r", alias+".id"), strings.Join(sourceConditions, " AND "))

		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, args, nil
	}

	if p.SubQuery.Type == QueryTypeSection {
		cond, args, err := e.sectionSubqueryCondition(p.SubQuery, "src_s")
		if err != nil {
			return "", nil, err
		}

		sqlCond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs r
			JOIN sections src_s ON r.source_id = src_s.id
			WHERE %s
			  AND %s
		)`, refResolvedIDPreferredMatchSQL("r", alias+".id"), cond)

		if p.Negated() {
			sqlCond = "NOT " + sqlCond
		}
		return sqlCond, args, nil
	}

	// Trait subquery - referenced by traits matching the subquery
	sourceConditions = append(sourceConditions, "src_t.trait_type = ?")
	args = append(args, p.SubQuery.TypeName)

	if p.SubQuery.Predicate != nil {
		cond, predArgs, err := e.buildTraitPredicateSQL(p.SubQuery.Predicate, "src_t")
		if err != nil {
			return "", nil, err
		}
		sourceConditions = append(sourceConditions, cond)
		args = append(args, predArgs...)
	}

	cond := fmt.Sprintf(`EXISTS (
		SELECT 1 FROM refs r
		JOIN traits src_t ON r.file_path = src_t.file_path 
		                 AND r.line_number = src_t.line_number
		WHERE %s
		  AND %s
	)`, refResolvedIDPreferredMatchSQL("r", alias+".id"), strings.Join(sourceConditions, " AND "))

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
}
