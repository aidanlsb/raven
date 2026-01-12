package query

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/resolver"
)

// Executor executes queries against the database.
type Executor struct {
	db       *sql.DB
	resolver *resolver.Resolver // Cached resolver for target resolution
}

// NewExecutor creates a new query executor.
func NewExecutor(db *sql.DB) *Executor {
	return &Executor{db: db}
}

// SetResolver injects a resolver for target resolution.
//
// This allows callers (CLI/LSP) to provide a canonical resolver that includes
// aliases and vault-specific settings like daily directory. If not set, the
// executor will fall back to building a resolver from the objects table.
func (e *Executor) SetResolver(r *resolver.Resolver) {
	e.resolver = r
}

// getResolver returns a resolver for target resolution, creating it if needed.
func (e *Executor) getResolver() (*resolver.Resolver, error) {
	if e.resolver != nil {
		return e.resolver, nil
	}

	// Query all object IDs from the database
	rows, err := e.db.Query("SELECT id FROM objects")
	if err != nil {
		return nil, fmt.Errorf("failed to get object IDs: %w", err)
	}
	defer rows.Close()

	var objectIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		objectIDs = append(objectIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	e.resolver = resolver.New(objectIDs)
	return e.resolver, nil
}

// resolveTarget resolves a reference to an object ID.
// Returns the resolved ID or an error if ambiguous.
func (e *Executor) resolveTarget(target string) (string, error) {
	res, err := e.getResolver()
	if err != nil {
		return "", err
	}

	result := res.Resolve(target)
	if result.Ambiguous {
		return "", fmt.Errorf("ambiguous reference '%s' - matches: %s",
			target, strings.Join(result.Matches, ", "))
	}
	if result.TargetID == "" {
		// Not found - return the original target (will match nothing)
		return target, nil
	}
	return result.TargetID, nil
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

// SortedTraitResult represents sorted/grouped trait query results.
type SortedTraitResult struct {
	Groups []TraitGroup
}

// TraitGroup represents a group of trait results.
type TraitGroup struct {
	Key     string        // Group key (object ID, field value, etc.)
	Label   string        // Human-readable label
	Results []TraitResult
}

// SortedObjectResult represents sorted/grouped object query results.
type SortedObjectResult struct {
	Groups []ObjectGroup
}

// ObjectGroup represents a group of object results.
type ObjectGroup struct {
	Key     string         // Group key
	Label   string         // Human-readable label
	Results []ObjectResult
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

	sqlStr := fmt.Sprintf(`
		SELECT o.id, o.type, o.fields, o.file_path, o.line_start, o.parent_id
		FROM objects o
		WHERE %s
		ORDER BY o.file_path, o.line_start
	`, strings.Join(conditions, " AND "))

	// Apply limit if specified
	if q.Limit > 0 {
		sqlStr += fmt.Sprintf(" LIMIT %d", q.Limit)
	}

	return sqlStr, args, nil
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

	sqlStr := fmt.Sprintf(`
		SELECT t.id, t.trait_type, t.value, t.content, t.file_path, t.line_number, t.parent_object_id,
		       CASE WHEN t.value IS NULL AND EXISTS(
		           SELECT 1 FROM objects o WHERE o.id = t.parent_object_id 
		           AND json_extract(o.fields, '$.' || t.trait_type) IS NOT NULL
		       ) THEN 'frontmatter' ELSE 'inline' END as source
		FROM traits t
		WHERE %s
		ORDER BY t.file_path, t.line_number
	`, strings.Join(conditions, " AND "))

	// Apply limit if specified
	if q.Limit > 0 {
		sqlStr += fmt.Sprintf(" LIMIT %d", q.Limit)
	}

	return sqlStr, args, nil
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
	case *RefdPredicate:
		return e.buildRefdPredicateSQL(p, alias, false)
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
	case *ContentPredicate:
		return e.buildTraitContentPredicateSQL(p, alias)
	case *AtPredicate:
		return e.buildAtPredicateSQL(p, alias)
	case *RefdPredicate:
		return e.buildRefdPredicateSQL(p, alias, true)
	case *OrPredicate:
		return e.buildOrPredicateSQL(p, alias, e.buildTraitPredicateSQL)
	case *GroupPredicate:
		return e.buildGroupPredicateSQL(p, alias, e.buildTraitPredicateSQL)
	default:
		return "", nil, fmt.Errorf("unsupported trait predicate type: %T", pred)
	}
}

// buildFieldPredicateSQL builds SQL for .field:value predicates.
// Comparisons are case-insensitive for equality, but case-sensitive for ordering comparisons.
func (e *Executor) buildFieldPredicateSQL(p *FieldPredicate, alias string) (string, []interface{}, error) {
	jsonPath := fmt.Sprintf("$.%s", p.Field)

	var cond string
	var args []interface{}

	if p.IsExists {
		// .field:* means field exists
		cond = fmt.Sprintf("json_extract(%s.fields, ?) IS NOT NULL", alias)
		args = append(args, jsonPath)
	} else if p.CompareOp != CompareEq {
		// Comparison operators: <, >, <=, >=
		var op string
		switch p.CompareOp {
		case CompareLt:
			op = "<"
		case CompareGt:
			op = ">"
		case CompareLte:
			op = "<="
		case CompareGte:
			op = ">="
		}
		cond = fmt.Sprintf("json_extract(%s.fields, ?) %s ?", alias, op)
		args = append(args, jsonPath, p.Value)
	} else {
		// Equality: Check if value is in array or equals scalar (case-insensitive)
		// Use json_each to check array membership, fall back to direct comparison
		cond = fmt.Sprintf(`(
			LOWER(json_extract(%s.fields, ?)) = LOWER(?) OR
			EXISTS (
				SELECT 1 FROM json_each(%s.fields, ?)
				WHERE LOWER(json_each.value) = LOWER(?)
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
			// Case-insensitive comparison for better UX
			if tp.Negated() {
				traitConditions = append(traitConditions, "LOWER(value) != LOWER(?)")
			} else {
				traitConditions = append(traitConditions, "LOWER(value) = LOWER(?)")
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
	// Handle direct target reference: parent:[[target]]
	if p.Target != "" {
		resolvedTarget, err := e.resolveTarget(p.Target)
		if err != nil {
			return "", nil, err
		}
		cond := fmt.Sprintf("%s.parent_id = ?", alias)
		if p.Negated() {
			cond = fmt.Sprintf("(%s.parent_id IS NULL OR %s.parent_id != ?)", alias, alias)
			return cond, []interface{}{resolvedTarget}, nil
		}
		return cond, []interface{}{resolvedTarget}, nil
	}

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

// buildAncestorPredicateSQL builds SQL for ancestor:{object:...} or ancestor:[[target]] predicates.
func (e *Executor) buildAncestorPredicateSQL(p *AncestorPredicate, alias string) (string, []interface{}, error) {
	// Handle direct target reference: ancestor:[[target]]
	if p.Target != "" {
		resolvedTarget, err := e.resolveTarget(p.Target)
		if err != nil {
			return "", nil, err
		}
		// Check if target is anywhere in the ancestor chain
		cond := fmt.Sprintf(`EXISTS (
			WITH RECURSIVE ancestors AS (
				SELECT id, parent_id FROM objects WHERE id = %s.parent_id
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

// buildChildPredicateSQL builds SQL for child:{object:...} or child:[[target]] predicates.
func (e *Executor) buildChildPredicateSQL(p *ChildPredicate, alias string) (string, []interface{}, error) {
	// Handle direct target reference: child:[[target]]
	if p.Target != "" {
		resolvedTarget, err := e.resolveTarget(p.Target)
		if err != nil {
			return "", nil, err
		}
		// Check if target is a direct child of this object
		cond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM objects WHERE id = ? AND parent_id = %s.id
		)`, alias)
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, []interface{}{resolvedTarget}, nil
	}

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

// buildDescendantPredicateSQL builds SQL for descendant:{object:...} or descendant:[[target]] predicates.
// Uses a recursive CTE to find all descendants at any depth.
func (e *Executor) buildDescendantPredicateSQL(p *DescendantPredicate, alias string) (string, []interface{}, error) {
	// Handle direct target reference: descendant:[[target]]
	if p.Target != "" {
		resolvedTarget, err := e.resolveTarget(p.Target)
		if err != nil {
			return "", nil, err
		}
		// Check if target is anywhere in the descendant tree
		cond := fmt.Sprintf(`EXISTS (
			WITH RECURSIVE descendants AS (
				SELECT id, parent_id FROM objects WHERE parent_id = %s.id
				UNION ALL
				SELECT o.id, o.parent_id FROM objects o
				JOIN descendants d ON o.parent_id = d.id
			)
			SELECT 1 FROM descendants WHERE id = ?
		)`, alias)
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, []interface{}{resolvedTarget}, nil
	}

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

// escapeLikePattern escapes special characters for LIKE pattern matching.
func escapeLikePattern(s string) string {
	// Escape backslash first, then % and _
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
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
// Comparisons are case-insensitive for equality, but case-sensitive for ordering comparisons.
func (e *Executor) buildValuePredicateSQL(p *ValuePredicate, alias string) (string, []interface{}, error) {
	var cond string

	switch p.CompareOp {
	case CompareLt:
		cond = fmt.Sprintf("%s.value < ?", alias)
	case CompareGt:
		cond = fmt.Sprintf("%s.value > ?", alias)
	case CompareLte:
		cond = fmt.Sprintf("%s.value <= ?", alias)
	case CompareGte:
		cond = fmt.Sprintf("%s.value >= ?", alias)
	default: // CompareEq
		if p.Negated() {
			cond = fmt.Sprintf("LOWER(%s.value) != LOWER(?)", alias)
		} else {
			cond = fmt.Sprintf("LOWER(%s.value) = LOWER(?)", alias)
		}
		return cond, []interface{}{p.Value}, nil
	}

	// For comparison operators, apply negation by wrapping
	if p.Negated() {
		cond = "NOT (" + cond + ")"
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

// buildAtPredicateSQL builds SQL for at:{trait:...} predicates.
// Matches traits at the same file:line location as matching traits.
func (e *Executor) buildAtPredicateSQL(p *AtPredicate, alias string) (string, []interface{}, error) {
	if p.Target != "" {
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

// buildRefdPredicateSQL builds SQL for refd:{...} predicates.
// Matches objects/traits that are referenced by the subquery matches.
// isTrait indicates if we're building for a trait query (uses different columns).
func (e *Executor) buildRefdPredicateSQL(p *RefdPredicate, alias string, isTrait bool) (string, []interface{}, error) {
	if p.Target != "" {
		// Referenced by a specific source
		cond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs r
			WHERE r.source_id = ?
			  AND (r.target_id = %s.id OR r.target_raw = %s.id)
		)`, alias, alias)
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, []interface{}{p.Target}, nil
	}

	// Subquery - referenced by objects/traits matching the subquery
	var sourceConditions []string
	var args []interface{}

	if p.SubQuery.Type == QueryTypeObject {
		sourceConditions = append(sourceConditions, "src.type = ?")
		args = append(args, p.SubQuery.TypeName)

		for _, pred := range p.SubQuery.Predicates {
			cond, predArgs, err := e.buildObjectPredicateSQL(pred, "src")
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

	for _, pred := range p.SubQuery.Predicates {
		cond, predArgs, err := e.buildTraitPredicateSQL(pred, "src_t")
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

// ExecuteTraitQuerySorted executes a trait query with sort/group.
func (e *Executor) ExecuteTraitQuerySorted(q *Query) (*SortedTraitResult, error) {
	// Get base results
	results, err := e.ExecuteTraitQuery(q)
	if err != nil {
		return nil, err
	}

	// Apply sorting
	if q.Sort != nil {
		results, err = e.sortTraitResults(results, q.Sort)
		if err != nil {
			return nil, err
		}
	}

	// Apply grouping
	if q.Group != nil {
		return e.groupTraitResults(results, q.Group)
	}

	return &SortedTraitResult{Groups: []TraitGroup{{Results: results}}}, nil
}

// ExecuteObjectQuerySorted executes an object query with sort/group.
func (e *Executor) ExecuteObjectQuerySorted(q *Query) (*SortedObjectResult, error) {
	// Get base results
	results, err := e.ExecuteObjectQuery(q)
	if err != nil {
		return nil, err
	}

	// Apply sorting
	if q.Sort != nil {
		results, err = e.sortObjectResults(results, q.Sort)
		if err != nil {
			return nil, err
		}
	}

	// Apply grouping
	if q.Group != nil {
		return e.groupObjectResults(results, q.Group)
	}

	return &SortedObjectResult{Groups: []ObjectGroup{{Results: results}}}, nil
}

// sortTraitResults computes sort keys and sorts the results.
func (e *Executor) sortTraitResults(results []TraitResult, spec *SortSpec) ([]TraitResult, error) {
	type sortableResult struct {
		result  TraitResult
		sortKey interface{}
	}

	sortable := make([]sortableResult, len(results))
	for i, r := range results {
		key, err := e.computeTraitSortKey(r, spec)
		if err != nil {
			return nil, err
		}
		sortable[i] = sortableResult{result: r, sortKey: key}
	}

	// Sort by key
	sort.Slice(sortable, func(i, j int) bool {
		return compareSortKeys(sortable[i].sortKey, sortable[j].sortKey, spec.Descending)
	})

	// Extract sorted results
	sorted := make([]TraitResult, len(results))
	for i, s := range sortable {
		sorted[i] = s.result
	}
	return sorted, nil
}

// sortObjectResults computes sort keys and sorts the results.
func (e *Executor) sortObjectResults(results []ObjectResult, spec *SortSpec) ([]ObjectResult, error) {
	type sortableResult struct {
		result  ObjectResult
		sortKey interface{}
	}

	sortable := make([]sortableResult, len(results))
	for i, r := range results {
		key, err := e.computeObjectSortKey(r, spec)
		if err != nil {
			return nil, err
		}
		sortable[i] = sortableResult{result: r, sortKey: key}
	}

	// Sort by key
	sort.Slice(sortable, func(i, j int) bool {
		return compareSortKeys(sortable[i].sortKey, sortable[j].sortKey, spec.Descending)
	})

	// Extract sorted results
	sorted := make([]ObjectResult, len(results))
	for i, s := range sortable {
		sorted[i] = s.result
	}
	return sorted, nil
}

// computeTraitSortKey computes the sort key for a single trait result.
func (e *Executor) computeTraitSortKey(r TraitResult, spec *SortSpec) (interface{}, error) {
	if spec.Path != nil && spec.SubQuery == nil {
		return e.evaluateTraitPath(r, spec.Path)
	}

	if spec.SubQuery != nil {
		return e.evaluateTraitSortSubquery(r, spec)
	}

	return nil, fmt.Errorf("sort spec has neither path nor subquery")
}

// computeObjectSortKey computes the sort key for a single object result.
func (e *Executor) computeObjectSortKey(r ObjectResult, spec *SortSpec) (interface{}, error) {
	if spec.Path != nil && spec.SubQuery == nil {
		return e.evaluateObjectPath(r, spec.Path)
	}

	if spec.SubQuery != nil {
		return e.evaluateObjectSortSubquery(r, spec)
	}

	return nil, fmt.Errorf("sort spec has neither path nor subquery")
}

// evaluateTraitPath evaluates a path expression against a trait result.
func (e *Executor) evaluateTraitPath(r TraitResult, path *PathExpr) (interface{}, error) {
	if len(path.Steps) == 0 {
		return nil, nil
	}

	// For traits, handle common paths
	step := path.Steps[0]
	switch step.Kind {
	case PathStepValue:
		if r.Value != nil {
			return *r.Value, nil
		}
		return nil, nil

	case PathStepParent:
		// Get the parent object and continue with remaining steps
		if len(path.Steps) == 1 {
			return r.ParentObjectID, nil
		}
		// Need to look up the parent object for further navigation
		obj, err := e.getObjectByID(r.ParentObjectID)
		if err != nil {
			return nil, nil // Object not found, return nil key
		}
		return e.evaluateObjectPath(*obj, &PathExpr{Steps: path.Steps[1:]})

	case PathStepField:
		// For traits, fields would be on the parent object
		obj, err := e.getObjectByID(r.ParentObjectID)
		if err != nil {
			return nil, nil
		}
		if obj.Fields != nil {
			if val, ok := obj.Fields[step.Name]; ok {
				return val, nil
			}
		}
		return nil, nil

	case PathStepRefs:
		// Find referenced objects of the given type
		refs, err := e.findTraitRefs(r, step.Name)
		if err != nil {
			return nil, nil
		}
		if len(refs) > 0 {
			return refs[0], nil
		}
		return nil, nil
	}

	return nil, nil
}

// evaluateObjectPath evaluates a path expression against an object result.
func (e *Executor) evaluateObjectPath(r ObjectResult, path *PathExpr) (interface{}, error) {
	if len(path.Steps) == 0 {
		return nil, nil
	}

	step := path.Steps[0]
	switch step.Kind {
	case PathStepField:
		if r.Fields != nil {
			if val, ok := r.Fields[step.Name]; ok {
				// If more steps, need to navigate further
				if len(path.Steps) == 1 {
					return val, nil
				}
			}
		}
		return nil, nil

	case PathStepParent:
		if r.ParentID == nil {
			return nil, nil
		}
		if len(path.Steps) == 1 {
			return *r.ParentID, nil
		}
		// Need to look up the parent object for further navigation
		obj, err := e.getObjectByID(*r.ParentID)
		if err != nil {
			return nil, nil
		}
		return e.evaluateObjectPath(*obj, &PathExpr{Steps: path.Steps[1:]})

	case PathStepAncestor:
		// Find ancestor of the given type
		ancestor, err := e.findAncestorOfType(r.ID, step.Name)
		if err != nil || ancestor == nil {
			return nil, nil
		}
		if len(path.Steps) == 1 {
			return ancestor.ID, nil
		}
		return e.evaluateObjectPath(*ancestor, &PathExpr{Steps: path.Steps[1:]})

	case PathStepRefs:
		// Find referenced objects of the given type
		refs, err := e.findObjectRefs(r.ID, step.Name)
		if err != nil {
			return nil, nil
		}
		if len(refs) > 0 {
			return refs[0], nil
		}
		return nil, nil
	}

	return nil, nil
}

// evaluateTraitSortSubquery evaluates a sort subquery with _ bound to a trait result.
func (e *Executor) evaluateTraitSortSubquery(r TraitResult, spec *SortSpec) (interface{}, error) {
	subQ := spec.SubQuery

	// Build conditions for the subquery, substituting _ references
	if subQ.Type == QueryTypeTrait {
		// Find co-located or related traits
		values, err := e.findRelatedTraitValues(r, subQ)
		if err != nil {
			return nil, nil
		}
		return aggregate(values, spec.Aggregation), nil
	}

	// Object subquery - find related objects
	objects, err := e.findRelatedObjects(r.ParentObjectID, subQ)
	if err != nil {
		return nil, nil
	}
	if len(objects) == 0 {
		return nil, nil
	}

	// If there's a path attached, evaluate it on the found objects
	if spec.Path != nil {
		var values []interface{}
		for _, obj := range objects {
			val, _ := e.evaluateObjectPath(obj, spec.Path)
			if val != nil {
				values = append(values, val)
			}
		}
		return aggregate(values, spec.Aggregation), nil
	}

	return objects[0].ID, nil
}

// evaluateObjectSortSubquery evaluates a sort subquery with _ bound to an object result.
func (e *Executor) evaluateObjectSortSubquery(r ObjectResult, spec *SortSpec) (interface{}, error) {
	subQ := spec.SubQuery

	if subQ.Type == QueryTypeTrait {
		// Find traits on or within this object
		values, err := e.findObjectTraitValues(r.ID, subQ)
		if err != nil {
			return nil, nil
		}
		return aggregate(values, spec.Aggregation), nil
	}

	// Object subquery - find related objects
	objects, err := e.findRelatedObjects(r.ID, subQ)
	if err != nil {
		return nil, nil
	}
	if len(objects) == 0 {
		return nil, nil
	}

	// If there's a path attached, evaluate it on the found objects
	if spec.Path != nil {
		var values []interface{}
		for _, obj := range objects {
			val, _ := e.evaluateObjectPath(obj, spec.Path)
			if val != nil {
				values = append(values, val)
			}
		}
		return aggregate(values, spec.Aggregation), nil
	}

	return objects[0].ID, nil
}

// findRelatedTraitValues finds trait values matching a subquery relative to a trait.
// It searches for traits that are:
// 1. Co-located (same file and line) as the current trait
// 2. On the same parent object as the current trait
func (e *Executor) findRelatedTraitValues(r TraitResult, subQ *Query) ([]interface{}, error) {
	// Build conditions from the subquery predicates
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "t.trait_type = ?")
	args = append(args, subQ.TypeName)

	// Add value predicate if present
	for _, pred := range subQ.Predicates {
		if vp, ok := pred.(*ValuePredicate); ok {
			if vp.CompareOp == CompareEq {
				if vp.Negated() {
					conditions = append(conditions, "LOWER(t.value) != LOWER(?)")
				} else {
					conditions = append(conditions, "LOWER(t.value) = LOWER(?)")
				}
			} else {
				op := vp.CompareOp.String()
				if vp.Negated() {
					conditions = append(conditions, fmt.Sprintf("NOT (t.value %s ?)", op))
				} else {
					conditions = append(conditions, fmt.Sprintf("t.value %s ?", op))
				}
			}
			args = append(args, vp.Value)
		}
	}

	// Don't match self
	conditions = append(conditions, "t.id != ?")
	args = append(args, r.ID)

	// Look for co-located traits OR traits on the same parent object
	query := fmt.Sprintf(`
		SELECT t.value FROM traits t
		WHERE %s
		  AND (
		      (t.file_path = ? AND t.line_number = ?)
		      OR t.parent_object_id = ?
		  )
		ORDER BY t.line_number
	`, strings.Join(conditions, " AND "))
	args = append(args, r.FilePath, r.Line, r.ParentObjectID)

	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []interface{}
	for rows.Next() {
		var value sql.NullString
		if err := rows.Scan(&value); err != nil {
			continue
		}
		if value.Valid {
			values = append(values, value.String)
		}
	}
	return values, rows.Err()
}

// findObjectTraitValues finds trait values on or within an object.
func (e *Executor) findObjectTraitValues(objectID string, subQ *Query) ([]interface{}, error) {
	// Build conditions from the subquery predicates
	var conditions []string
	var args []interface{}

	// First arg is for the subtree CTE
	args = append(args, objectID)

	conditions = append(conditions, "t.trait_type = ?")
	args = append(args, subQ.TypeName)

	// Add value predicate if present
	for _, pred := range subQ.Predicates {
		if vp, ok := pred.(*ValuePredicate); ok {
			if vp.CompareOp == CompareEq {
				if vp.Negated() {
					conditions = append(conditions, "LOWER(t.value) != LOWER(?)")
				} else {
					conditions = append(conditions, "LOWER(t.value) = LOWER(?)")
				}
			} else {
				op := vp.CompareOp.String()
				if vp.Negated() {
					conditions = append(conditions, fmt.Sprintf("NOT (t.value %s ?)", op))
				} else {
					conditions = append(conditions, fmt.Sprintf("t.value %s ?", op))
				}
			}
			args = append(args, vp.Value)
		}
	}

	// Look for traits on this object or its descendants
	query := fmt.Sprintf(`
		WITH RECURSIVE subtree AS (
			SELECT id FROM objects WHERE id = ?
			UNION ALL
			SELECT o.id FROM objects o
			JOIN subtree s ON o.parent_id = s.id
		)
		SELECT t.value FROM traits t
		WHERE t.parent_object_id IN (SELECT id FROM subtree)
		  AND %s
		ORDER BY t.line_number
	`, strings.Join(conditions, " AND "))

	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []interface{}
	for rows.Next() {
		var value sql.NullString
		if err := rows.Scan(&value); err != nil {
			continue
		}
		if value.Valid {
			values = append(values, value.String)
		}
	}
	return values, rows.Err()
}

// findRelatedObjects finds objects matching a subquery that are related to sourceID.
// Related objects are those that:
// 1. Are referenced by the source
// 2. Reference the source
// 3. Are ancestors or descendants of the source
func (e *Executor) findRelatedObjects(sourceID string, subQ *Query) ([]ObjectResult, error) {
	// Build conditions from the subquery predicates
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "o.type = ?")
	args = append(args, subQ.TypeName)

	// Add field predicates
	for _, pred := range subQ.Predicates {
		if fp, ok := pred.(*FieldPredicate); ok {
			jsonPath := fmt.Sprintf("$.%s", fp.Field)
			if fp.IsExists {
				if fp.Negated() {
					conditions = append(conditions, "json_extract(o.fields, ?) IS NULL")
				} else {
					conditions = append(conditions, "json_extract(o.fields, ?) IS NOT NULL")
				}
				args = append(args, jsonPath)
			} else if fp.CompareOp != CompareEq {
				op := fp.CompareOp.String()
				if fp.Negated() {
					conditions = append(conditions, fmt.Sprintf("NOT (json_extract(o.fields, ?) %s ?)", op))
				} else {
					conditions = append(conditions, fmt.Sprintf("json_extract(o.fields, ?) %s ?", op))
				}
				args = append(args, jsonPath, fp.Value)
			} else {
				if fp.Negated() {
					conditions = append(conditions, "LOWER(json_extract(o.fields, ?)) != LOWER(?)")
				} else {
					conditions = append(conditions, "LOWER(json_extract(o.fields, ?)) = LOWER(?)")
				}
				args = append(args, jsonPath, fp.Value)
			}
		}
	}

	// Find objects that are related to sourceID:
	// - Referenced by sourceID
	// - Or that match the type (for simple subqueries like {object:project})
	query := fmt.Sprintf(`
		SELECT DISTINCT o.id, o.type, o.fields, o.file_path, o.line_start, o.parent_id
		FROM objects o
		WHERE %s
		  AND (
		      -- Objects referenced by source
		      o.id IN (
		          SELECT COALESCE(r.target_id, r.target_raw) 
		          FROM refs r WHERE r.source_id = ?
		      )
		      -- Or just match the type (fallback for simple queries)
		      OR 1=1
		  )
		ORDER BY o.file_path, o.line_start
	`, strings.Join(conditions, " AND "))
	args = append(args, sourceID)

	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ObjectResult
	for rows.Next() {
		var r ObjectResult
		var fieldsJSON string
		if err := rows.Scan(&r.ID, &r.Type, &fieldsJSON, &r.FilePath, &r.LineStart, &r.ParentID); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(fieldsJSON), &r.Fields); err != nil {
			r.Fields = make(map[string]interface{})
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// getObjectByID retrieves an object by its ID.
func (e *Executor) getObjectByID(id string) (*ObjectResult, error) {
	row := e.db.QueryRow(`
		SELECT id, type, fields, file_path, line_start, parent_id
		FROM objects WHERE id = ?
	`, id)

	var r ObjectResult
	var fieldsJSON string
	if err := row.Scan(&r.ID, &r.Type, &fieldsJSON, &r.FilePath, &r.LineStart, &r.ParentID); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(fieldsJSON), &r.Fields); err != nil {
		r.Fields = make(map[string]interface{})
	}
	return &r, nil
}

// findAncestorOfType finds an ancestor of the given type.
func (e *Executor) findAncestorOfType(objectID string, typeName string) (*ObjectResult, error) {
	row := e.db.QueryRow(`
		WITH RECURSIVE ancestors AS (
			SELECT id, parent_id, type, fields, file_path, line_start FROM objects WHERE id = ?
			UNION ALL
			SELECT o.id, o.parent_id, o.type, o.fields, o.file_path, o.line_start
			FROM objects o JOIN ancestors a ON o.id = a.parent_id
		)
		SELECT id, type, fields, file_path, line_start, parent_id FROM objects
		WHERE id IN (SELECT id FROM ancestors WHERE type = ?)
		LIMIT 1
	`, objectID, typeName)

	var r ObjectResult
	var fieldsJSON string
	var parentID sql.NullString
	if err := row.Scan(&r.ID, &r.Type, &fieldsJSON, &r.FilePath, &r.LineStart, &parentID); err != nil {
		return nil, err
	}
	if parentID.Valid {
		r.ParentID = &parentID.String
	}
	if err := json.Unmarshal([]byte(fieldsJSON), &r.Fields); err != nil {
		r.Fields = make(map[string]interface{})
	}
	return &r, nil
}

// findTraitRefs finds referenced object IDs from a trait's line.
func (e *Executor) findTraitRefs(r TraitResult, typeName string) ([]string, error) {
	rows, err := e.db.Query(`
		SELECT DISTINCT COALESCE(r.target_id, r.target_raw) as target
		FROM refs r
		JOIN objects o ON (r.target_id = o.id OR r.target_raw = o.id)
		WHERE r.file_path = ? AND r.line_number = ? AND o.type = ?
	`, r.FilePath, r.Line, typeName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			continue
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// findObjectRefs finds referenced object IDs from an object.
func (e *Executor) findObjectRefs(objectID string, typeName string) ([]string, error) {
	rows, err := e.db.Query(`
		SELECT DISTINCT COALESCE(r.target_id, r.target_raw) as target
		FROM refs r
		JOIN objects o ON (r.target_id = o.id OR r.target_raw = o.id)
		WHERE r.source_id = ? AND o.type = ?
	`, objectID, typeName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			continue
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// groupTraitResults groups trait results by the group specification.
func (e *Executor) groupTraitResults(results []TraitResult, spec *GroupSpec) (*SortedTraitResult, error) {
	groups := make(map[string][]TraitResult)
	groupOrder := []string{}

	for _, r := range results {
		key, err := e.computeTraitGroupKey(r, spec)
		if err != nil {
			key = ""
		}
		keyStr := fmt.Sprintf("%v", key)
		if _, exists := groups[keyStr]; !exists {
			groupOrder = append(groupOrder, keyStr)
		}
		groups[keyStr] = append(groups[keyStr], r)
	}

	// Sort groups alphabetically (empty key last)
	sort.Slice(groupOrder, func(i, j int) bool {
		if groupOrder[i] == "" {
			return false
		}
		if groupOrder[j] == "" {
			return true
		}
		return groupOrder[i] < groupOrder[j]
	})

	var traitGroups []TraitGroup
	for _, key := range groupOrder {
		label := key
		if key == "" {
			label = "(ungrouped)"
		}
		traitGroups = append(traitGroups, TraitGroup{
			Key:     key,
			Label:   label,
			Results: groups[key],
		})
	}

	return &SortedTraitResult{Groups: traitGroups}, nil
}

// groupObjectResults groups object results by the group specification.
func (e *Executor) groupObjectResults(results []ObjectResult, spec *GroupSpec) (*SortedObjectResult, error) {
	groups := make(map[string][]ObjectResult)
	groupOrder := []string{}

	for _, r := range results {
		key, err := e.computeObjectGroupKey(r, spec)
		if err != nil {
			key = ""
		}
		keyStr := fmt.Sprintf("%v", key)
		if _, exists := groups[keyStr]; !exists {
			groupOrder = append(groupOrder, keyStr)
		}
		groups[keyStr] = append(groups[keyStr], r)
	}

	// Sort groups alphabetically (empty key last)
	sort.Slice(groupOrder, func(i, j int) bool {
		if groupOrder[i] == "" {
			return false
		}
		if groupOrder[j] == "" {
			return true
		}
		return groupOrder[i] < groupOrder[j]
	})

	var objectGroups []ObjectGroup
	for _, key := range groupOrder {
		label := key
		if key == "" {
			label = "(ungrouped)"
		}
		objectGroups = append(objectGroups, ObjectGroup{
			Key:     key,
			Label:   label,
			Results: groups[key],
		})
	}

	return &SortedObjectResult{Groups: objectGroups}, nil
}

// computeTraitGroupKey computes the group key for a trait result.
func (e *Executor) computeTraitGroupKey(r TraitResult, spec *GroupSpec) (interface{}, error) {
	if spec.Path != nil && spec.SubQuery == nil {
		return e.evaluateTraitPath(r, spec.Path)
	}

	if spec.SubQuery != nil {
		return e.evaluateTraitGroupSubquery(r, spec)
	}

	return nil, fmt.Errorf("group spec has neither path nor subquery")
}

// computeObjectGroupKey computes the group key for an object result.
func (e *Executor) computeObjectGroupKey(r ObjectResult, spec *GroupSpec) (interface{}, error) {
	if spec.Path != nil && spec.SubQuery == nil {
		return e.evaluateObjectPath(r, spec.Path)
	}

	if spec.SubQuery != nil {
		return e.evaluateObjectGroupSubquery(r, spec)
	}

	return nil, fmt.Errorf("group spec has neither path nor subquery")
}

// evaluateTraitGroupSubquery evaluates a group subquery with _ bound to a trait result.
func (e *Executor) evaluateTraitGroupSubquery(r TraitResult, spec *GroupSpec) (interface{}, error) {
	// For grouping, we typically want a single key, so we just return the first match
	subQ := spec.SubQuery

	if subQ.Type == QueryTypeObject {
		// Find related object (e.g., referenced project)
		refs, err := e.findTraitRefs(r, subQ.TypeName)
		if err != nil || len(refs) == 0 {
			return nil, nil
		}
		return refs[0], nil
	}

	// Trait subquery - find co-located trait
	values, err := e.findRelatedTraitValues(r, subQ)
	if err != nil || len(values) == 0 {
		return nil, nil
	}
	return values[0], nil
}

// evaluateObjectGroupSubquery evaluates a group subquery with _ bound to an object result.
func (e *Executor) evaluateObjectGroupSubquery(r ObjectResult, spec *GroupSpec) (interface{}, error) {
	subQ := spec.SubQuery

	if subQ.Type == QueryTypeObject {
		// Find related objects
		if r.ParentID != nil {
			return *r.ParentID, nil
		}
		return nil, nil
	}

	// Trait subquery - not commonly used for grouping objects
	return nil, nil
}

// compareSortKeys compares two sort keys.
func compareSortKeys(a, b interface{}, descending bool) bool {
	// Handle nil values (NULLS LAST)
	if a == nil && b == nil {
		return false
	}
	if a == nil {
		return false // nil sorts last
	}
	if b == nil {
		return true // nil sorts last
	}

	var less bool

	// Compare based on type
	switch av := a.(type) {
	case string:
		if bv, ok := b.(string); ok {
			less = av < bv
		}
	case int:
		if bv, ok := b.(int); ok {
			less = av < bv
		}
	case int64:
		if bv, ok := b.(int64); ok {
			less = av < bv
		}
	case float64:
		if bv, ok := b.(float64); ok {
			less = av < bv
		}
	default:
		// Fall back to string comparison
		less = fmt.Sprintf("%v", a) < fmt.Sprintf("%v", b)
	}

	if descending {
		return !less
	}
	return less
}

// aggregate applies an aggregation function to a list of values.
func aggregate(values []interface{}, agg AggregationType) interface{} {
	if len(values) == 0 {
		return nil
	}

	switch agg {
	case AggCount:
		return len(values)
	case AggFirst:
		return values[0]
	case AggMin:
		min := values[0]
		for _, v := range values[1:] {
			if compareSortKeys(v, min, false) {
				min = v
			}
		}
		return min
	case AggMax:
		max := values[0]
		for _, v := range values[1:] {
			if compareSortKeys(max, v, false) {
				max = v
			}
		}
		return max
	}
	return values[0]
}
