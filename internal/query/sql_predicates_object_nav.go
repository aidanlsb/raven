package query

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/index"
)

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
			cond, condArgs := buildValueCondition(tp, "value")
			traitConditions = append(traitConditions, cond)
			args = append(args, condArgs...)
		case *FieldPredicate:
			// Handle .value predicate for traits
			if tp.Field == "value" {
				cond, condArgs := buildCompareCondition(tp.Value, tp.CompareOp, tp.Negated(), "value")
				traitConditions = append(traitConditions, cond)
				args = append(args, condArgs...)
			}
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
			cond, condArgs := buildValueCondition(tp, "t.value")
			traitConditions = append(traitConditions, cond)
			args = append(args, condArgs...)
		case *FieldPredicate:
			// Handle .value predicate for traits
			if tp.Field == "value" {
				cond, condArgs := buildCompareCondition(tp.Value, tp.CompareOp, tp.Negated(), "t.value")
				traitConditions = append(traitConditions, cond)
				args = append(args, condArgs...)
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
		// Resolve the target to its canonical object ID (like backlinks does)
		resolvedTarget, err := e.resolveTarget(p.Target)
		if err != nil {
			return "", nil, err
		}

		// Match against resolved target_id, OR fall back to target_raw for unresolved refs
		cond = fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs r
			WHERE r.source_id = %s.id AND (r.target_id = ? OR (r.target_id IS NULL AND r.target_raw = ?))
		)`, alias)
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

	return cond, []interface{}{index.BuildFTSContentQuery(p.SearchTerm)}, nil
}
