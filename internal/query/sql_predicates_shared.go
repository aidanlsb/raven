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
	return wrapBooleanComplement(cond), args, nil
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
// identExpr is inlined twice; when it contains placeholders, bind its args
// twice via refResolvedIDPreferredMatchExpr.
func refResolvedIDPreferredMatchSQL(refAlias, identExpr string) string {
	return fmt.Sprintf(
		"(%s.target_id = %s OR (%s.target_id IS NULL AND %s.target_raw = %s))",
		refAlias, identExpr, refAlias, refAlias, identExpr,
	)
}

// refResolvedIDPreferredMatchExpr is the expression form of
// refResolvedIDPreferredMatchSQL. identArgs belong to identExpr and are
// duplicated because that expression is inlined twice.
func refResolvedIDPreferredMatchExpr(refAlias, identExpr string, identArgs []interface{}) (string, []interface{}) {
	return refResolvedIDPreferredMatchSQL(refAlias, identExpr), repeatIdentExprArgs(identExpr, 2, identArgs)
}

func repeatIdentExprArgs(identExpr string, times int, args []interface{}) []interface{} {
	if times <= 1 || len(args) == 0 || !strings.Contains(identExpr, "?") {
		return args
	}
	out := make([]interface{}, 0, len(args)*times)
	for i := 0; i < times; i++ {
		out = append(out, args...)
	}
	return out
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
	_ = isTrait
	if p.Target != "" {
		sourceID, err := e.resolveTarget(p.Target)
		if err != nil {
			return "", nil, err
		}
		rAlias := e.nextAlias("r")
		cond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs %s
			WHERE (%s.source_id = ? OR %s.source_id LIKE ? || '#%%')
			  AND %s
		)`, rAlias, rAlias, rAlias, refResolvedIDPreferredMatchSQL(rAlias, alias+".id"))
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, []interface{}{sourceID, sourceID}, nil
	}

	if p.SubQuery == nil {
		return "", nil, fmt.Errorf("refd predicate must have target or subquery")
	}

	// Named-edge reverse uses this/path correlation. Skip the untyped refs
	// join so multi-hop reverse can flatten: type:company refd(type:task
	// .project.company==this) does not require a direct task→company edge.
	if queryCorrelatesViaThis(p.SubQuery) {
		cond, args, err := e.buildCorrelatedRefdSubquerySQL(p.SubQuery)
		if err != nil {
			return "", nil, err
		}
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, args, nil
	}

	rAlias := e.nextAlias("r")
	targetMatch := refResolvedIDPreferredMatchSQL(rAlias, alias+".id")

	switch p.SubQuery.Type {
	case QueryTypeObject:
		srcAlias := e.nextAlias("src")
		where, args, err := e.buildObjectWhereForAlias(p.SubQuery, srcAlias)
		if err != nil {
			return "", nil, err
		}
		cond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs %s
			JOIN objects %s ON (%s.source_id = %s.id OR %s.source_id LIKE %s.id || '#%%')
			WHERE %s AND %s
		)`, rAlias, srcAlias, rAlias, srcAlias, rAlias, srcAlias, targetMatch, where)
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, args, nil
	case QueryTypeSection:
		srcAlias := e.nextAlias("srcs")
		where, args, err := e.sectionSubqueryCondition(p.SubQuery, srcAlias)
		if err != nil {
			return "", nil, err
		}
		cond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs %s
			JOIN sections %s ON %s.source_id = %s.id
			WHERE %s AND %s
		)`, rAlias, srcAlias, rAlias, srcAlias, targetMatch, where)
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, args, nil
	case QueryTypeTrait:
		srcAlias := e.nextAlias("srct")
		where, args, err := e.traitSubqueryCondition(p.SubQuery, srcAlias)
		if err != nil {
			return "", nil, err
		}
		cond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs %s
			JOIN traits %s ON %s.file_path = %s.file_path AND %s.line_number = %s.line_number
			WHERE %s AND %s
		)`, rAlias, srcAlias, rAlias, srcAlias, rAlias, srcAlias, targetMatch, where)
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, args, nil
	default:
		return "", nil, fmt.Errorf("refd() subquery must be a type, trait, or section query")
	}
}

func (e *Executor) buildCorrelatedRefdSubquerySQL(sub *Query) (string, []interface{}, error) {
	switch sub.Type {
	case QueryTypeObject:
		srcAlias := e.nextAlias("src")
		where, args, err := e.buildObjectWhereForAlias(sub, srcAlias)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("EXISTS (SELECT 1 FROM objects %s WHERE %s)", srcAlias, where), args, nil
	case QueryTypeSection:
		srcAlias := e.nextAlias("srcs")
		where, args, err := e.sectionSubqueryCondition(sub, srcAlias)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("EXISTS (SELECT 1 FROM sections %s WHERE %s)", srcAlias, where), args, nil
	case QueryTypeTrait:
		srcAlias := e.nextAlias("srct")
		where, args, err := e.traitSubqueryCondition(sub, srcAlias)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("EXISTS (SELECT 1 FROM traits %s WHERE %s)", srcAlias, where), args, nil
	default:
		return "", nil, fmt.Errorf("refd() subquery must be a type, trait, or section query")
	}
}
