package query

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// Executor executes queries against the database.
type Executor struct {
	db *sql.DB
}

// NewExecutor creates a new query executor.
func NewExecutor(db *sql.DB) *Executor {
	return &Executor{db: db}
}

// ObjectResult represents an object returned from a query.
type ObjectResult struct {
	ID        string
	Type      string
	Fields    map[string]interface{}
	FilePath  string
	LineStart int
	ParentID  *string
}

// TraitResult represents a trait returned from a query.
type TraitResult struct {
	ID             string
	TraitType      string
	Value          *string
	Content        string
	FilePath       string
	Line           int
	ParentObjectID string
	Source         string // "inline" or "frontmatter"
}

// ExecuteObjectQuery executes an object query and returns matching objects.
func (e *Executor) ExecuteObjectQuery(q *Query) ([]ObjectResult, error) {
	if q.Type != QueryTypeObject {
		return nil, fmt.Errorf("expected object query, got trait query")
	}

	sql, args, err := e.buildObjectSQL(q)
	if err != nil {
		return nil, err
	}

	rows, err := e.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w (SQL: %s)", err, sql)
	}
	defer rows.Close()

	var results []ObjectResult
	for rows.Next() {
		var r ObjectResult
		var fieldsJSON string
		if err := rows.Scan(&r.ID, &r.Type, &fieldsJSON, &r.FilePath, &r.LineStart, &r.ParentID); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(fieldsJSON), &r.Fields); err != nil {
			r.Fields = make(map[string]interface{})
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// ExecuteTraitQuery executes a trait query and returns matching traits.
func (e *Executor) ExecuteTraitQuery(q *Query) ([]TraitResult, error) {
	if q.Type != QueryTypeTrait {
		return nil, fmt.Errorf("expected trait query, got object query")
	}

	sql, args, err := e.buildTraitSQL(q)
	if err != nil {
		return nil, err
	}

	rows, err := e.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w (SQL: %s)", err, sql)
	}
	defer rows.Close()

	var results []TraitResult
	for rows.Next() {
		var r TraitResult
		if err := rows.Scan(&r.ID, &r.TraitType, &r.Value, &r.Content, &r.FilePath, &r.Line, &r.ParentObjectID, &r.Source); err != nil {
			return nil, err
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// buildObjectSQL builds SQL for an object query.
func (e *Executor) buildObjectSQL(q *Query) (string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	// Type filter
	conditions = append(conditions, "o.type = ?")
	args = append(args, q.TypeName)

	// Build predicate conditions
	for _, pred := range q.Predicates {
		cond, predArgs, err := e.buildObjectPredicateSQL(pred, "o")
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, cond)
		args = append(args, predArgs...)
	}

	sql := fmt.Sprintf(`
		SELECT o.id, o.type, o.fields, o.file_path, o.line_start, o.parent_id
		FROM objects o
		WHERE %s
		ORDER BY o.file_path, o.line_start
	`, strings.Join(conditions, " AND "))

	return sql, args, nil
}

// buildTraitSQL builds SQL for a trait query.
func (e *Executor) buildTraitSQL(q *Query) (string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	// Trait type filter
	conditions = append(conditions, "t.trait_type = ?")
	args = append(args, q.TypeName)

	// Build predicate conditions
	for _, pred := range q.Predicates {
		cond, predArgs, err := e.buildTraitPredicateSQL(pred, "t")
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, cond)
		args = append(args, predArgs...)
	}

	sql := fmt.Sprintf(`
		SELECT t.id, t.trait_type, t.value, t.content, t.file_path, t.line_number, t.parent_object_id,
		       CASE WHEN t.value IS NULL AND EXISTS(
		           SELECT 1 FROM objects o WHERE o.id = t.parent_object_id 
		           AND json_extract(o.fields, '$.' || t.trait_type) IS NOT NULL
		       ) THEN 'frontmatter' ELSE 'inline' END as source
		FROM traits t
		WHERE %s
		ORDER BY t.file_path, t.line_number
	`, strings.Join(conditions, " AND "))

	return sql, args, nil
}

// buildObjectPredicateSQL builds SQL for an object predicate.
func (e *Executor) buildObjectPredicateSQL(pred Predicate, alias string) (string, []interface{}, error) {
	switch p := pred.(type) {
	case *FieldPredicate:
		return e.buildFieldPredicateSQL(p, alias)
	case *HasPredicate:
		return e.buildHasPredicateSQL(p, alias)
	case *ParentPredicate:
		return e.buildParentPredicateSQL(p, alias)
	case *AncestorPredicate:
		return e.buildAncestorPredicateSQL(p, alias)
	case *ChildPredicate:
		return e.buildChildPredicateSQL(p, alias)
	case *DescendantPredicate:
		return e.buildDescendantPredicateSQL(p, alias)
	case *ContainsPredicate:
		return e.buildContainsPredicateSQL(p, alias)
	case *RefsPredicate:
		return e.buildRefsPredicateSQL(p, alias)
	case *ContentPredicate:
		return e.buildContentPredicateSQL(p, alias)
	case *OrPredicate:
		return e.buildOrPredicateSQL(p, alias, e.buildObjectPredicateSQL)
	case *GroupPredicate:
		return e.buildGroupPredicateSQL(p, alias, e.buildObjectPredicateSQL)
	default:
		return "", nil, fmt.Errorf("unsupported object predicate type: %T", pred)
	}
}

// buildTraitPredicateSQL builds SQL for a trait predicate.
func (e *Executor) buildTraitPredicateSQL(pred Predicate, alias string) (string, []interface{}, error) {
	switch p := pred.(type) {
	case *ValuePredicate:
		return e.buildValuePredicateSQL(p, alias)
	case *SourcePredicate:
		return e.buildSourcePredicateSQL(p, alias)
	case *OnPredicate:
		return e.buildOnPredicateSQL(p, alias)
	case *WithinPredicate:
		return e.buildWithinPredicateSQL(p, alias)
	case *RefsPredicate:
		return e.buildTraitRefsPredicateSQL(p, alias)
	case *OrPredicate:
		return e.buildOrPredicateSQL(p, alias, e.buildTraitPredicateSQL)
	case *GroupPredicate:
		return e.buildGroupPredicateSQL(p, alias, e.buildTraitPredicateSQL)
	default:
		return "", nil, fmt.Errorf("unsupported trait predicate type: %T", pred)
	}
}

// buildFieldPredicateSQL builds SQL for .field:value predicates.
func (e *Executor) buildFieldPredicateSQL(p *FieldPredicate, alias string) (string, []interface{}, error) {
	jsonPath := fmt.Sprintf("$.%s", p.Field)

	var cond string
	var args []interface{}

	if p.IsExists {
		// .field:* means field exists
		cond = fmt.Sprintf("json_extract(%s.fields, ?) IS NOT NULL", alias)
		args = append(args, jsonPath)
	} else {
		// Check if value is in array or equals scalar
		// Use json_each to check array membership, fall back to direct comparison
		cond = fmt.Sprintf(`(
			json_extract(%s.fields, ?) = ? OR
			EXISTS (
				SELECT 1 FROM json_each(%s.fields, ?)
				WHERE json_each.value = ?
			)
		)`, alias, alias)
		args = append(args, jsonPath, p.Value, jsonPath, p.Value)
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, args, nil
}

// buildHasPredicateSQL builds SQL for has:{trait:...} predicates.
func (e *Executor) buildHasPredicateSQL(p *HasPredicate, alias string) (string, []interface{}, error) {
	// Build subquery conditions for the trait
	var traitConditions []string
	var args []interface{}

	traitConditions = append(traitConditions, "trait_type = ?")
	args = append(args, p.SubQuery.TypeName)

	for _, pred := range p.SubQuery.Predicates {
		switch tp := pred.(type) {
		case *ValuePredicate:
			if tp.Negated() {
				traitConditions = append(traitConditions, "value != ?")
			} else {
				traitConditions = append(traitConditions, "value = ?")
			}
			args = append(args, tp.Value)
		case *SourcePredicate:
			// Source filtering would require more complex logic
			// For now, we'll skip it in subqueries
		}
	}

	cond := fmt.Sprintf(`EXISTS (
		SELECT 1 FROM traits
		WHERE parent_object_id = %s.id AND %s
	)`, alias, strings.Join(traitConditions, " AND "))

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
}

// buildParentPredicateSQL builds SQL for parent:{object:...} predicates.
func (e *Executor) buildParentPredicateSQL(p *ParentPredicate, alias string) (string, []interface{}, error) {
	var parentConditions []string
	var args []interface{}

	parentConditions = append(parentConditions, "type = ?")
	args = append(args, p.SubQuery.TypeName)

	for _, pred := range p.SubQuery.Predicates {
		cond, predArgs, err := e.buildObjectPredicateSQL(pred, "parent_obj")
		if err != nil {
			return "", nil, err
		}
		parentConditions = append(parentConditions, cond)
		args = append(args, predArgs...)
	}

	cond := fmt.Sprintf(`EXISTS (
		SELECT 1 FROM objects parent_obj
		WHERE parent_obj.id = %s.parent_id AND %s
	)`, alias, strings.Join(parentConditions, " AND "))

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
}

// buildAncestorPredicateSQL builds SQL for ancestor:{object:...} predicates.
func (e *Executor) buildAncestorPredicateSQL(p *AncestorPredicate, alias string) (string, []interface{}, error) {
	var ancestorConditions []string
	var args []interface{}

	ancestorConditions = append(ancestorConditions, "anc.type = ?")
	args = append(args, p.SubQuery.TypeName)

	// Build ancestor query using recursive CTE
	cond := fmt.Sprintf(`EXISTS (
		WITH RECURSIVE ancestors AS (
			SELECT id, parent_id, type, fields FROM objects WHERE id = %s.parent_id
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

// buildChildPredicateSQL builds SQL for child:{object:...} predicates.
func (e *Executor) buildChildPredicateSQL(p *ChildPredicate, alias string) (string, []interface{}, error) {
	var childConditions []string
	var args []interface{}

	childConditions = append(childConditions, "type = ?")
	args = append(args, p.SubQuery.TypeName)

	for _, pred := range p.SubQuery.Predicates {
		cond, predArgs, err := e.buildObjectPredicateSQL(pred, "child_obj")
		if err != nil {
			return "", nil, err
		}
		childConditions = append(childConditions, cond)
		args = append(args, predArgs...)
	}

	cond := fmt.Sprintf(`EXISTS (
		SELECT 1 FROM objects child_obj
		WHERE child_obj.parent_id = %s.id AND %s
	)`, alias, strings.Join(childConditions, " AND "))

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
}

// buildDescendantPredicateSQL builds SQL for descendant:{object:...} predicates.
// Uses a recursive CTE to find all descendants at any depth.
func (e *Executor) buildDescendantPredicateSQL(p *DescendantPredicate, alias string) (string, []interface{}, error) {
	var descendantConditions []string
	var args []interface{}

	descendantConditions = append(descendantConditions, "desc_obj.type = ?")
	args = append(args, p.SubQuery.TypeName)

	for _, pred := range p.SubQuery.Predicates {
		cond, predArgs, err := e.buildObjectPredicateSQL(pred, "desc_obj")
		if err != nil {
			return "", nil, err
		}
		descendantConditions = append(descendantConditions, cond)
		args = append(args, predArgs...)
	}

	// Build descendant query using recursive CTE
	cond := fmt.Sprintf(`EXISTS (
		WITH RECURSIVE descendants AS (
			SELECT id, parent_id, type, fields FROM objects WHERE parent_id = %s.id
			UNION ALL
			SELECT o.id, o.parent_id, o.type, o.fields FROM objects o
			JOIN descendants d ON o.parent_id = d.id
		)
		SELECT 1 FROM descendants desc_obj WHERE %s
	)`, alias, strings.Join(descendantConditions, " AND "))

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
}

// buildContainsPredicateSQL builds SQL for contains:{trait:...} predicates.
// Finds objects that have a matching trait anywhere in their subtree (self or descendants).
func (e *Executor) buildContainsPredicateSQL(p *ContainsPredicate, alias string) (string, []interface{}, error) {
	var traitConditions []string
	var args []interface{}

	traitConditions = append(traitConditions, "t.trait_type = ?")
	args = append(args, p.SubQuery.TypeName)

	for _, pred := range p.SubQuery.Predicates {
		switch tp := pred.(type) {
		case *ValuePredicate:
			if tp.Negated() {
				traitConditions = append(traitConditions, "t.value != ?")
			} else {
				traitConditions = append(traitConditions, "t.value = ?")
			}
			args = append(args, tp.Value)
		case *SourcePredicate:
			if tp.Source == "frontmatter" {
				traitConditions = append(traitConditions, "t.line_number <= 1")
			} else {
				traitConditions = append(traitConditions, "t.line_number > 1")
			}
		}
	}

	// Build a query that checks for traits on self OR any descendant
	// Use recursive CTE to find all descendants, then check traits on any of them
	cond := fmt.Sprintf(`EXISTS (
		WITH RECURSIVE subtree AS (
			SELECT id FROM objects WHERE id = %s.id
			UNION ALL
			SELECT o.id FROM objects o
			JOIN subtree s ON o.parent_id = s.id
		)
		SELECT 1 FROM traits t
		WHERE t.parent_object_id IN (SELECT id FROM subtree) AND %s
	)`, alias, strings.Join(traitConditions, " AND "))

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
}

// buildRefsPredicateSQL builds SQL for refs:[[target]] or refs:{object:...} predicates.
func (e *Executor) buildRefsPredicateSQL(p *RefsPredicate, alias string) (string, []interface{}, error) {
	var cond string
	var args []interface{}

	if p.Target != "" {
		// Direct reference to specific target
		// Prefer target_id (resolved at index time), fall back to target_raw
		cond = fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs r
			WHERE r.source_id = %s.id AND (r.target_id = ? OR (r.target_id IS NULL AND r.target_raw = ?))
		)`, alias)
		args = append(args, p.Target, p.Target)
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

		// Prefer target_id (resolved at index time), fall back to target_raw for unresolved refs
		cond = fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs r
			JOIN objects target_obj ON (
				r.target_id = target_obj.id OR 
				(r.target_id IS NULL AND r.target_raw = target_obj.id)
			)
			WHERE r.source_id = %s.id AND %s
		)`, alias, strings.Join(targetConditions, " AND "))
	} else {
		return "", nil, fmt.Errorf("refs predicate must have target or subquery")
	}

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
}

// buildContentPredicateSQL builds SQL for content:"search terms" predicates.
// Uses FTS5 full-text search to filter objects by their content.
func (e *Executor) buildContentPredicateSQL(p *ContentPredicate, alias string) (string, []interface{}, error) {
	// Use FTS5 to search content
	// The fts_content table has: object_id, title, content, file_path
	cond := fmt.Sprintf(`EXISTS (
		SELECT 1 FROM fts_content
		WHERE fts_content.object_id = %s.id
		  AND fts_content MATCH ?
	)`, alias)

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, []interface{}{p.SearchTerm}, nil
}

// buildTraitRefsPredicateSQL builds SQL for refs:[[target]] or refs:{object:...} predicates on traits.
//
// CONTENT SCOPE RULE: This matches refs that appear on the same line as the trait.
// This is the same rule used by parser.IsRefOnTraitLine and parser.ExtractTraitContent -
// a trait's associated content (including references) is defined as everything on the
// same line as the trait annotation.
func (e *Executor) buildTraitRefsPredicateSQL(p *RefsPredicate, alias string) (string, []interface{}, error) {
	var cond string
	var args []interface{}

	if p.Target != "" {
		// Direct reference to specific target
		// Match refs on the same line as the trait
		cond = fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs r
			WHERE r.file_path = %s.file_path 
			  AND r.line_number = %s.line_number
			  AND (r.target_id = ? OR (r.target_id IS NULL AND r.target_raw = ?))
		)`, alias, alias)
		args = append(args, p.Target, p.Target)
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

// buildValuePredicateSQL builds SQL for value:val predicates.
func (e *Executor) buildValuePredicateSQL(p *ValuePredicate, alias string) (string, []interface{}, error) {
	var cond string
	if p.Negated() {
		cond = fmt.Sprintf("%s.value != ?", alias)
	} else {
		cond = fmt.Sprintf("%s.value = ?", alias)
	}
	return cond, []interface{}{p.Value}, nil
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

// buildOnPredicateSQL builds SQL for on:{object:...} predicates.
func (e *Executor) buildOnPredicateSQL(p *OnPredicate, alias string) (string, []interface{}, error) {
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

// buildWithinPredicateSQL builds SQL for within:{object:...} predicates.
func (e *Executor) buildWithinPredicateSQL(p *WithinPredicate, alias string) (string, []interface{}, error) {
	var ancestorConditions []string
	var args []interface{}

	ancestorConditions = append(ancestorConditions, "anc.type = ?")
	args = append(args, p.SubQuery.TypeName)

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

// buildOrPredicateSQL builds SQL for OR predicates.
func (e *Executor) buildOrPredicateSQL(p *OrPredicate, alias string,
	buildFn func(Predicate, string) (string, []interface{}, error)) (string, []interface{}, error) {

	leftCond, leftArgs, err := buildFn(p.Left, alias)
	if err != nil {
		return "", nil, err
	}

	rightCond, rightArgs, err := buildFn(p.Right, alias)
	if err != nil {
		return "", nil, err
	}

	cond := fmt.Sprintf("(%s OR %s)", leftCond, rightCond)
	args := append(leftArgs, rightArgs...)

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
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

	cond := "(" + strings.Join(conditions, " AND ") + ")"

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
}

// Execute parses and executes a query string, returning either object or trait results.
func (e *Executor) Execute(queryStr string) (interface{}, error) {
	q, err := Parse(queryStr)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	if q.Type == QueryTypeObject {
		return e.ExecuteObjectQuery(q)
	}
	return e.ExecuteTraitQuery(q)
}
