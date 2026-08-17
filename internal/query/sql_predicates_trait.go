package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/indexschema"
)

// buildTraitContentPredicateSQL builds SQL for content("search terms") predicates on traits.
// Uses LIKE matching on the trait's content column (the line text where the trait appears).
// This is simpler than FTS5 since trait content is a single line.
func (e *Executor) buildTraitContentPredicateSQL(p *ContentPredicate, alias string) (string, []interface{}, error) {
	// Use case-insensitive LIKE to search the trait's line content
	// The content column stores the full line where the trait annotation appears
	cond := fmt.Sprintf("%s.content LIKE ? ESCAPE '\\'", alias)

	// Escape special LIKE characters and wrap with wildcards for substring match
	searchPattern := "%" + escapeLikePattern(p.SearchTerm) + "%"

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, []interface{}{searchPattern}, nil
}

// buildTraitStringFuncPredicateSQL builds SQL for string function predicates on trait values.
// For traits, the field is implicitly the trait's value column.
func (e *Executor) buildTraitStringFuncPredicateSQL(p *StringFuncPredicate, alias string) (string, []interface{}, error) {
	if p.IsElementRef || p.Field != "value" {
		fieldLabel := "." + p.Field
		if p.IsElementRef {
			fieldLabel = "_"
		}
		return "", nil, fmt.Errorf("unsupported trait string function field: %s (only .value is allowed for traits)", fieldLabel)
	}

	fieldExpr := fmt.Sprintf("%s.value", alias)

	cond, args, err := buildStringFuncCondition(p.FuncType, fieldExpr, p.Value, p.CaseSensitive)
	if err != nil {
		return "", nil, err
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, args, nil
}

// buildTraitArrayQuantifierPredicateSQL builds SQL for array quantifier predicates on trait values.
// Array-valued traits are indexed as JSON arrays in traits.value.
func (e *Executor) buildTraitArrayQuantifierPredicateSQL(p *ArrayQuantifierPredicate, alias string) (string, []interface{}, error) {
	elemCond, elemArgs, err := e.buildElementPredicateSQL(p.ElementPred)
	if err != nil {
		return "", nil, err
	}

	validJSON := fmt.Sprintf("json_valid(%[1]s.value)", alias)
	safeJSON := fmt.Sprintf("CASE WHEN %[1]s THEN %[2]s.value ELSE 'null' END", validJSON, alias)
	arrayGuard := fmt.Sprintf("%s AND json_type(%s) = 'array'", validJSON, safeJSON)
	arrayExpr := fmt.Sprintf("CASE WHEN %s THEN %s.value ELSE '[]' END", arrayGuard, alias)
	var cond string
	switch p.Quantifier {
	case ArrayQuantifierAny:
		cond = fmt.Sprintf(`%s AND EXISTS (
			SELECT 1 FROM json_each(%s)
			WHERE %s
		)`, arrayGuard, arrayExpr, elemCond)
	case ArrayQuantifierAll:
		cond = fmt.Sprintf(`%s AND NOT EXISTS (
			SELECT 1 FROM json_each(%s)
			WHERE NOT (%s)
		)`, arrayGuard, arrayExpr, elemCond)
	case ArrayQuantifierNone:
		cond = fmt.Sprintf(`%s AND NOT EXISTS (
			SELECT 1 FROM json_each(%s)
			WHERE %s
		)`, arrayGuard, arrayExpr, elemCond)
	default:
		return "", nil, fmt.Errorf("unknown array quantifier: %v", p.Quantifier)
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}
	return cond, elemArgs, nil
}

// buildValuePredicateSQL builds SQL for value==val predicates.
// Comparisons are case-insensitive for equality, but case-sensitive for ordering comparisons.
func (e *Executor) buildValuePredicateSQL(p *ValuePredicate, alias string) (string, []interface{}, error) {
	cond, args := e.buildValueCondition(p, fmt.Sprintf("%s.value", alias))
	return cond, args, nil
}

func (e *Executor) buildValueCondition(p *ValuePredicate, column string) (string, []interface{}) {
	return e.buildCompareCondition(p.Value, p.CompareOp, p.Negated(), column)
}

func (e *Executor) buildCompareCondition(value string, compareOp CompareOp, negated bool, column string) (string, []interface{}) {
	// Date filters (today/tomorrow/yesterday, YYYY-MM-DD, etc.)
	if cond, args, ok := buildDateFilterConditionForCompare(strings.TrimSpace(value), compareOp, column, e.queryNow()); ok {
		if negated {
			cond = "NOT (" + cond + ")"
		}
		return cond, args
	}

	// Pick operator for the predicate.
	op := compareOpToSQL(compareOp)

	// Prefer numeric comparisons when RHS parses as a number.
	if n, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
		cond := fmt.Sprintf("CAST(%s AS REAL) %s ?", column, op)
		if negated {
			cond = "NOT (" + cond + ")"
		}
		return cond, []interface{}{n}
	}

	// String comparisons:
	// - equality/inequality are case-insensitive (existing behavior)
	// - ordering comparisons are case-sensitive (existing behavior)
	cond := ""
	if op == "=" || op == "!=" {
		cond = fmt.Sprintf("LOWER(%s) %s LOWER(?)", column, op)
	} else {
		cond = fmt.Sprintf("%s %s ?", column, op)
	}

	if negated {
		cond = "NOT (" + cond + ")"
	}

	return cond, []interface{}{value}
}

// buildTraitValueFieldPredicateSQL builds SQL for .value==val predicates on traits.
// This is the newer syntax that replaces the bare value== syntax.
func (e *Executor) buildTraitValueFieldPredicateSQL(p *FieldPredicate, alias string) (string, []interface{}, error) {
	cond, args := e.buildCompareCondition(p.Value, p.CompareOp, p.Negated(), fmt.Sprintf("%s.value", alias))
	return cond, args, nil
}

func buildDateFilterConditionForCompare(value string, compareOp CompareOp, column string, now time.Time) (string, []interface{}, bool) {
	if value == "" {
		return "", nil, false
	}
	cond, args, ok, err := indexschema.TryParseTemporalComparisonWithOptions(value, compareOpToSQL(compareOp), column, indexschema.DateFilterOptions{
		Now: now,
	})
	if err != nil {
		return "", nil, false
	}
	if !ok {
		return "", nil, false
	}
	return cond, args, true
}

// buildAtPredicateSQL builds SQL for at(trait:...) predicates.
// Matches traits at the same file:line location as matching traits.
func (e *Executor) buildAtPredicateSQL(p *AtPredicate, alias string) (string, []interface{}, error) {
	if p.Target != "" {
		// Check for special self-reference marker from at:_ binding
		if strings.HasPrefix(p.Target, "__selfref_trait:") {
			// Parse file:line from the marker
			parts := strings.SplitN(strings.TrimPrefix(p.Target, "__selfref_trait:"), ":", 2)
			if len(parts) == 2 {
				cond := fmt.Sprintf(`(%s.file_path = ? AND %s.line_number = ?)`, alias, alias)
				if p.Negated() {
					cond = "NOT " + cond
				}
				return cond, []interface{}{parts[0], parts[1]}, nil
			}
		}

		// Direct reference to specific trait location
		// Need to look up the trait's file:line
		cond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM traits ref_t
			WHERE ref_t.id = ?
			  AND %s.file_path = ref_t.file_path
			  AND %s.line_number = ref_t.line_number
		)`, alias, alias)
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, []interface{}{p.Target}, nil
	}

	// Subquery - match traits at the same location as matching traits
	var traitConditions []string
	var args []interface{}

	traitConditions = append(traitConditions, "co.trait_type = ?")
	args = append(args, p.SubQuery.TypeName)

	if p.SubQuery.Predicate != nil {
		// Build predicate condition for the co-located traits
		cond, predArgs, err := e.buildTraitPredicateSQL(p.SubQuery.Predicate, "co")
		if err != nil {
			return "", nil, err
		}
		traitConditions = append(traitConditions, cond)
		args = append(args, predArgs...)
	}

	cond := fmt.Sprintf(`EXISTS (
		SELECT 1 FROM traits co
		WHERE co.file_path = %s.file_path
		  AND co.line_number = %s.line_number
		  AND co.id != %s.id
		  AND %s
	)`, alias, alias, alias, strings.Join(traitConditions, " AND "))

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
}
