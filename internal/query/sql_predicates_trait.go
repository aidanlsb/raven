package query

import (
	"fmt"
	"strconv"
	"strings"
)

// buildTraitContentPredicateSQL builds SQL for content:"search terms" predicates on traits.
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

// buildTraitRefsPredicateSQL builds SQL for refs:[[target]] or refs:{object:...} predicates on traits.
//
// CONTENT SCOPE RULE: This matches refs that appear on the same line as the trait.
// This is the same rule used by parser.IsRefOnTraitLine and parser.StripTraitAnnotations -
// a trait's associated content (including references) is defined as everything on the
// same line as the trait annotation.
func (e *Executor) buildTraitRefsPredicateSQL(p *RefsPredicate, alias string) (string, []interface{}, error) {
	var cond string
	var args []interface{}

	if p.Target != "" {
		// Direct reference to specific target
		// Resolve the target to its canonical object ID (like backlinks does)
		resolvedTarget := p.Target
		if resolved, err := e.resolveTarget(p.Target); err == nil && resolved != "" {
			resolvedTarget = resolved
		}

		// Match refs on the same line as the trait
		cond = fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs r
			WHERE r.file_path = %s.file_path 
			  AND r.line_number = %s.line_number
			  AND (r.target_id = ? OR (r.target_id IS NULL AND r.target_raw = ?))
		)`, alias, alias)
		args = append(args, resolvedTarget, p.Target)
	} else if p.SubQuery != nil {
		// Subquery - reference to objects matching the subquery
		var targetConditions []string
		targetConditions = append(targetConditions, "target_obj.type = ?")
		args = append(args, p.SubQuery.TypeName)

		for _, pred := range p.SubQuery.Predicates {
			predCond, predArgs, err := e.buildObjectPredicateSQL(pred, "target_obj")
			if err != nil {
				return "", nil, err
			}
			targetConditions = append(targetConditions, predCond)
			args = append(args, predArgs...)
		}

		// Match refs on the same line as the trait that point to matching objects
		cond = fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs r
			JOIN objects target_obj ON (
				r.target_id = target_obj.id OR 
				(r.target_id IS NULL AND r.target_raw = target_obj.id)
			)
			WHERE r.file_path = %s.file_path 
			  AND r.line_number = %s.line_number
			  AND %s
		)`, alias, alias, strings.Join(targetConditions, " AND "))
	} else {
		return "", nil, fmt.Errorf("refs predicate must have target or subquery")
	}

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
}

// buildTraitStringFuncPredicateSQL builds SQL for string function predicates on trait values.
// For traits, the field is implicitly the trait's value column.
func (e *Executor) buildTraitStringFuncPredicateSQL(p *StringFuncPredicate, alias string) (string, []interface{}, error) {
	fieldExpr := fmt.Sprintf("%s.value", alias)

	var cond string
	var args []interface{}

	wrapLower := !p.CaseSensitive

	switch p.FuncType {
	case StringFuncIncludes:
		cond = likeCond(fieldExpr, wrapLower)
		args = append(args, "%"+escapeLikePattern(p.Value)+"%")

	case StringFuncStartsWith:
		cond = likeCond(fieldExpr, wrapLower)
		args = append(args, escapeLikePattern(p.Value)+"%")

	case StringFuncEndsWith:
		cond = likeCond(fieldExpr, wrapLower)
		args = append(args, "%"+escapeLikePattern(p.Value))

	case StringFuncMatches:
		if wrapLower {
			cond = fmt.Sprintf("%s REGEXP ?", fieldExpr)
			args = append(args, "(?i)"+p.Value)
		} else {
			cond = fmt.Sprintf("%s REGEXP ?", fieldExpr)
			args = append(args, p.Value)
		}
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, args, nil
}

// buildValuePredicateSQL builds SQL for value==val predicates.
// Comparisons are case-insensitive for equality, but case-sensitive for ordering comparisons.
func (e *Executor) buildValuePredicateSQL(p *ValuePredicate, alias string) (string, []interface{}, error) {
	cond, args := buildValueCondition(p, fmt.Sprintf("%s.value", alias))
	return cond, args, nil
}

// buildValueCondition builds a SQL condition for a ValuePredicate.
// This is a helper for use in subqueries where we don't have the full executor context.
func buildValueCondition(p *ValuePredicate, column string) (string, []interface{}) {
	// Pick operator for the predicate.
	op := "="
	switch p.CompareOp {
	case CompareNeq:
		op = "!="
	case CompareLt:
		op = "<"
	case CompareGt:
		op = ">"
	case CompareLte:
		op = "<="
	case CompareGte:
		op = ">="
	}

	// Prefer numeric comparisons when RHS parses as a number.
	if n, err := strconv.ParseFloat(strings.TrimSpace(p.Value), 64); err == nil {
		cond := fmt.Sprintf("CAST(%s AS REAL) %s ?", column, op)
		if p.Negated() {
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

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, []interface{}{p.Value}
}

// buildSourcePredicateSQL builds SQL for source:inline predicates.
func (e *Executor) buildSourcePredicateSQL(p *SourcePredicate, alias string) (string, []interface{}, error) {
	// All traits are inline (in content). source:inline filters by line position.
	var cond string
	if p.Source == "frontmatter" {
		cond = fmt.Sprintf("%s.line_number <= 1", alias)
	} else {
		cond = fmt.Sprintf("%s.line_number > 1", alias)
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, nil, nil
}

// buildOnPredicateSQL builds SQL for on:{object:...} or on:[[target]] predicates.
func (e *Executor) buildOnPredicateSQL(p *OnPredicate, alias string) (string, []interface{}, error) {
	// Handle direct target reference: on:[[target]]
	if p.Target != "" {
		resolvedTarget, err := e.resolveTarget(p.Target)
		if err != nil {
			return "", nil, err
		}
		cond := fmt.Sprintf("%s.parent_object_id = ?", alias)
		if p.Negated() {
			cond = fmt.Sprintf("(%s.parent_object_id IS NULL OR %s.parent_object_id != ?)", alias, alias)
			return cond, []interface{}{resolvedTarget}, nil
		}
		return cond, []interface{}{resolvedTarget}, nil
	}

	var objConditions []string
	var args []interface{}

	objConditions = append(objConditions, "type = ?")
	args = append(args, p.SubQuery.TypeName)

	for _, pred := range p.SubQuery.Predicates {
		cond, predArgs, err := e.buildObjectPredicateSQL(pred, "parent_obj")
		if err != nil {
			return "", nil, err
		}
		objConditions = append(objConditions, cond)
		args = append(args, predArgs...)
	}

	cond := fmt.Sprintf(`EXISTS (
		SELECT 1 FROM objects parent_obj
		WHERE parent_obj.id = %s.parent_object_id AND %s
	)`, alias, strings.Join(objConditions, " AND "))

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
}

// buildWithinPredicateSQL builds SQL for within:{object:...} or within:[[target]] predicates.
func (e *Executor) buildWithinPredicateSQL(p *WithinPredicate, alias string) (string, []interface{}, error) {
	// Handle direct target reference: within:[[target]]
	if p.Target != "" {
		resolvedTarget, err := e.resolveTarget(p.Target)
		if err != nil {
			return "", nil, err
		}
		// Check if target is the trait's parent or any ancestor of the trait's parent
		cond := fmt.Sprintf(`EXISTS (
			WITH RECURSIVE ancestors AS (
				SELECT id, parent_id FROM objects WHERE id = %s.parent_object_id
				UNION ALL
				SELECT o.id, o.parent_id FROM objects o
				JOIN ancestors a ON o.id = a.parent_id
			)
			SELECT 1 FROM ancestors WHERE id = ?
		)`, alias)
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, []interface{}{resolvedTarget}, nil
	}

	var ancestorConditions []string
	var args []interface{}

	ancestorConditions = append(ancestorConditions, "anc.type = ?")
	args = append(args, p.SubQuery.TypeName)

	// Process predicates from the subquery
	for _, pred := range p.SubQuery.Predicates {
		predCond, predArgs, err := e.buildObjectPredicateSQL(pred, "anc")
		if err != nil {
			return "", nil, err
		}
		ancestorConditions = append(ancestorConditions, predCond)
		args = append(args, predArgs...)
	}

	// Build ancestor query using recursive CTE
	cond := fmt.Sprintf(`EXISTS (
		WITH RECURSIVE ancestors AS (
			SELECT id, parent_id, type, fields FROM objects WHERE id = %s.parent_object_id
			UNION ALL
			SELECT o.id, o.parent_id, o.type, o.fields FROM objects o
			JOIN ancestors a ON o.id = a.parent_id
		)
		SELECT 1 FROM ancestors anc WHERE %s
	)`, alias, strings.Join(ancestorConditions, " AND "))

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
}

// buildAtPredicateSQL builds SQL for at:{trait:...} predicates.
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

	for _, pred := range p.SubQuery.Predicates {
		// Build predicate conditions for the co-located traits
		cond, predArgs, err := e.buildTraitPredicateSQL(pred, "co")
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
