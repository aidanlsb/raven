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

// buildRefdPredicateSQL builds SQL for refd:{...} predicates.
// Matches objects/traits that are referenced by the subquery matches.
// isTrait indicates if we're building for a trait query (uses different columns).
func (e *Executor) buildRefdPredicateSQL(p *RefdPredicate, alias string, isTrait bool) (string, []interface{}, error) {
	if p.Target != "" {
		// Check for trait line marker: __trait_line:filepath:line
		if strings.HasPrefix(p.Target, "__trait_line:") {
			// Parse file:line from the marker
			rest := strings.TrimPrefix(p.Target, "__trait_line:")
			lastColon := strings.LastIndex(rest, ":")
			if lastColon > 0 {
				filePath := rest[:lastColon]
				lineStr := rest[lastColon+1:]
				// Find refs on that specific line
				cond := fmt.Sprintf(`EXISTS (
					SELECT 1 FROM refs r
					WHERE r.file_path = ?
					  AND r.line_number = ?
					  AND (r.target_id = %s.id OR r.target_raw = %s.id)
				)`, alias, alias)
				if p.Negated() {
					cond = "NOT " + cond
				}
				return cond, []interface{}{filePath, lineStr}, nil
			}
		}

		// Referenced by a specific source
		sourceID, err := e.resolveTarget(p.Target)
		if err != nil {
			return "", nil, err
		}
		cond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs r
			WHERE r.source_id = ?
			  AND (r.target_id = %s.id OR r.target_raw = %s.id)
		)`, alias, alias)
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, []interface{}{sourceID}, nil
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
			JOIN objects src ON r.source_id = src.id
			WHERE (r.target_id = %s.id OR r.target_raw = %s.id)
			  AND %s
		)`, alias, alias, strings.Join(sourceConditions, " AND "))

		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, args, nil
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
		WHERE (r.target_id = %s.id OR r.target_raw = %s.id)
		  AND %s
	)`, alias, alias, strings.Join(sourceConditions, " AND "))

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
}
