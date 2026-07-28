package query

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
)

// buildLinkPredicateSQL builds the shared field predicate grammar over a link
// row. Both links(...) and the link query root consume this implementation.
func (e *Executor) buildLinkPredicateSQL(pred Predicate, alias string) (string, []interface{}, error) {
	if err := validateLinkPredicate(pred); err != nil {
		return "", nil, err
	}
	return e.buildValidLinkPredicateSQL(pred, alias)
}

func (e *Executor) buildValidLinkPredicateSQL(pred Predicate, alias string) (string, []interface{}, error) {
	recurse := func(child Predicate, childAlias string) (string, []interface{}, error) {
		return e.buildValidLinkPredicateSQL(child, childAlias)
	}

	switch p := pred.(type) {
	case *OrPredicate:
		return e.buildOrPredicateSQL(p, alias, recurse)
	case *NotPredicate:
		return e.buildNotPredicateSQL(p, alias, recurse)
	case *GroupPredicate:
		return e.buildGroupPredicateSQL(p, alias, recurse)
	case *FieldPredicate:
		return buildLinkFieldPredicateSQL(p, alias)
	case *StringFuncPredicate:
		return buildLinkStringFuncPredicateSQL(p, alias)
	default:
		return "", nil, fmt.Errorf("unsupported link predicate type: %T", pred)
	}
}

func buildLinkFieldPredicateSQL(p *FieldPredicate, alias string) (string, []interface{}, error) {
	column, ok := linkColumnExpr(alias, p.Field)
	if !ok {
		return "", nil, unknownLinkFieldError(p.Field)
	}

	if p.IsExists {
		cond := column + " IS NOT NULL"
		if p.CompareOp == CompareNeq {
			cond = column + " IS NULL"
		}
		if p.Negated() {
			cond = "NOT (" + cond + ")"
		}
		return cond, nil, nil
	}

	var cond string
	var args []interface{}
	if linkFieldTypes[p.Field] == schema.FieldTypeBool {
		value := 0
		if strings.EqualFold(p.Value, "true") {
			value = 1
		}
		cond = fmt.Sprintf("%s %s ?", column, compareOpToSQL(p.CompareOp))
		args = []interface{}{value}
	} else if p.CompareOp == CompareEq || p.CompareOp == CompareNeq {
		cond = fmt.Sprintf("LOWER(%s) %s LOWER(?)", column, compareOpToSQL(p.CompareOp))
		args = []interface{}{p.Value}
	} else {
		cond = fmt.Sprintf("%s %s ?", column, compareOpToSQL(p.CompareOp))
		args = []interface{}{p.Value}
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}
	return cond, args, nil
}

func buildLinkStringFuncPredicateSQL(p *StringFuncPredicate, alias string) (string, []interface{}, error) {
	column, ok := linkColumnExpr(alias, p.Field)
	if !ok {
		return "", nil, unknownLinkFieldError(p.Field)
	}
	cond, args, err := buildStringFuncCondition(p.FuncType, column, p.Value, p.CaseSensitive)
	if err != nil {
		return "", nil, err
	}
	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}
	return cond, args, nil
}

// buildLinksPredicateSQL matches each supported source root to link rows:
// objects own every link in their file, traits own links on their source line,
// and sections own links in their complete subtree line range.
func (e *Executor) buildLinksPredicateSQL(p *LinksPredicate, alias string, root QueryType) (string, []interface{}, error) {
	if p.LinkPredicate == nil {
		return "", nil, fmt.Errorf("links() requires a link field predicate")
	}
	linkCond, args, err := e.buildLinkPredicateSQL(p.LinkPredicate, "l")
	if err != nil {
		return "", nil, err
	}

	var sourceCond string
	switch root {
	case QueryTypeObject:
		sourceCond = fmt.Sprintf("l.source_id = %s.id", alias)
	case QueryTypeTrait:
		sourceCond = fmt.Sprintf("l.file_path = %[1]s.file_path AND l.line_number = %[1]s.line_number", alias)
	case QueryTypeSection:
		sourceCond = fmt.Sprintf(
			"l.file_path = %[1]s.file_path AND l.line_number >= %[1]s.line_start AND (%[1]s.subtree_line_end IS NULL OR l.line_number <= %[1]s.subtree_line_end)",
			alias,
		)
	default:
		return "", nil, fmt.Errorf("links() predicate is not supported for %s queries", queryTypeName(root))
	}

	cond := fmt.Sprintf(`EXISTS (
		SELECT 1 FROM links l
		WHERE %s
		  AND %s
	)`, sourceCond, linkCond)
	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}
	return cond, args, nil
}

// buildLinkWithinPredicateSQL treats the source object and every section whose
// subtree contains the indexed link line as ancestor scopes. This uses the
// edge index directly; it does not re-parse Markdown or infer a target.
func (e *Executor) buildLinkWithinPredicateSQL(p *WithinPredicate, alias string) (string, []interface{}, error) {
	targetCond, args, err := e.scopeMatcherCondition(p.Target, p.SubQuery, "anc")
	if err != nil {
		return "", nil, err
	}
	cond := fmt.Sprintf(`EXISTS (
		WITH link_scopes(id) AS (
			SELECT %[1]s.source_id
			UNION ALL
			SELECT scope_sec.id
			FROM sections scope_sec
			WHERE scope_sec.file_object_id = %[1]s.source_id
			  AND %[1]s.line_number >= scope_sec.line_start
			  AND (scope_sec.subtree_line_end IS NULL OR %[1]s.line_number <= scope_sec.subtree_line_end)
		)
		SELECT 1 FROM link_scopes anc WHERE %[2]s
	)`, alias, targetCond)
	if p.Negated() {
		cond = "NOT " + cond
	}
	return cond, args, nil
}
