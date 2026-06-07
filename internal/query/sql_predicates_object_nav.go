package query

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/index"
)

const recursivePredicateMaxDepth = 100

// buildHasPredicateSQL builds SQL for has(section...) or has(trait:...) predicates.
func (e *Executor) buildHasPredicateSQL(p *HasPredicate, alias string) (string, []interface{}, error) {
	scopeID := fmt.Sprintf("%s.id", alias)
	switch p.SubQuery.Type {
	case QueryTypeTrait:
		cond, args, err := e.traitSubqueryCondition(p.SubQuery, "t")
		if err != nil {
			return "", nil, err
		}
		sql := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM traits t
			WHERE t.parent_object_id = %s AND %s
		)`, scopeID, cond)
		if p.Negated() {
			sql = "NOT " + sql
		}
		return sql, args, nil
	case QueryTypeSection:
		cond, args, err := e.sectionSubqueryCondition(p.SubQuery, "child_s")
		if err != nil {
			return "", nil, err
		}
		sql := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM sections child_s
			WHERE %s AND %s
		)`, directSectionParentCondition("child_s", scopeID), cond)
		if p.Negated() {
			sql = "NOT " + sql
		}
		return sql, args, nil
	default:
		return "", nil, fmt.Errorf("has() expects a section or trait query")
	}
}

// buildContainsPredicateSQL builds SQL for recursive contains(section...) or contains(trait:...) predicates.
func (e *Executor) buildContainsPredicateSQL(p *ContainsPredicate, alias string) (string, []interface{}, error) {
	scopeID := fmt.Sprintf("%s.id", alias)
	switch p.SubQuery.Type {
	case QueryTypeTrait:
		cond, args, err := e.traitSubqueryCondition(p.SubQuery, "t")
		if err != nil {
			return "", nil, err
		}
		sql := fmt.Sprintf(`EXISTS (
			WITH RECURSIVE subtree AS (
				SELECT %s AS id, 0 AS depth
				UNION ALL
				SELECT s.id, subtree.depth + 1
				FROM sections s
				JOIN subtree ON %s
				WHERE subtree.depth < ?
			)
			SELECT 1 FROM traits t
			WHERE t.parent_object_id IN (SELECT id FROM subtree) AND %s
		)`, scopeID, directSectionParentCondition("s", "subtree.id"), cond)
		args = append([]interface{}{recursivePredicateMaxDepth}, args...)
		if p.Negated() {
			sql = "NOT " + sql
		}
		return sql, args, nil
	case QueryTypeSection:
		cond, args, err := e.sectionSubqueryCondition(p.SubQuery, "desc_s")
		if err != nil {
			return "", nil, err
		}
		sql := fmt.Sprintf(`EXISTS (
			WITH RECURSIVE descendants AS (
				SELECT s.id, 1 AS depth
				FROM sections s
				WHERE %s
				UNION ALL
				SELECT s.id, descendants.depth + 1
				FROM sections s
				JOIN descendants ON s.parent_section_id = descendants.id
				WHERE descendants.depth < ?
			)
			SELECT 1 FROM sections desc_s
			WHERE desc_s.id IN (SELECT id FROM descendants) AND %s
		)`, directSectionParentCondition("s", scopeID), cond)
		args = append([]interface{}{recursivePredicateMaxDepth}, args...)
		if p.Negated() {
			sql = "NOT " + sql
		}
		return sql, args, nil
	default:
		return "", nil, fmt.Errorf("contains() expects a section or trait query")
	}
}

func (e *Executor) buildInPredicateSQL(p *InPredicate, alias string, kind predicateKind) (string, []interface{}, error) {
	parentExpr := scopeParentExpr(alias, kind)
	if parentExpr == "" {
		return "", nil, fmt.Errorf("in() is not valid for this query")
	}
	return e.buildScopeMatchPredicate(p.Target, p.SubQuery, parentExpr, p.Negated())
}

func (e *Executor) buildWithinPredicateSQL(p *WithinPredicate, alias string, kind predicateKind) (string, []interface{}, error) {
	scopeID := currentScopeExpr(alias, kind)
	if scopeID == "" {
		return "", nil, fmt.Errorf("within() is not valid for this query")
	}

	targetCond, targetArgs, err := e.scopeMatcherCondition(p.Target, p.SubQuery, "anc")
	if err != nil {
		return "", nil, err
	}
	sql := fmt.Sprintf(`EXISTS (
		WITH RECURSIVE subtree AS (
			SELECT %s AS id, 0 AS depth
			UNION ALL
			SELECT COALESCE(sec.parent_section_id, sec.file_object_id) AS id, subtree.depth + 1
			FROM subtree
			JOIN sections sec ON sec.id = subtree.id
			WHERE subtree.depth < ? AND COALESCE(sec.parent_section_id, sec.file_object_id) IS NOT NULL
		)
		SELECT 1 FROM subtree anc WHERE anc.depth > 0 AND %s
	)`, scopeID, targetCond)
	args := append([]interface{}{recursivePredicateMaxDepth}, targetArgs...)
	if p.Negated() {
		sql = "NOT " + sql
	}
	return sql, args, nil
}

func (e *Executor) buildScopeMatchPredicate(target string, subQuery *Query, candidateExpr string, negated bool) (string, []interface{}, error) {
	targetCond, args, err := e.scopeMatcherCondition(target, subQuery, "candidate")
	if err != nil {
		return "", nil, err
	}
	cond := fmt.Sprintf(`EXISTS (
		WITH candidate AS (SELECT %s AS id)
		SELECT 1 FROM candidate
		WHERE %s
	)`, candidateExpr, targetCond)
	if negated {
		cond = "NOT " + cond
	}
	return cond, args, nil
}

func (e *Executor) scopeMatcherCondition(target string, subQuery *Query, alias string) (string, []interface{}, error) {
	if target != "" {
		resolvedTarget, err := e.resolveTarget(target)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("%s.id = ?", alias), []interface{}{resolvedTarget}, nil
	}
	if subQuery == nil {
		return "", nil, fmt.Errorf("scope predicate requires a target or subquery")
	}
	switch subQuery.Type {
	case QueryTypeObject:
		objAlias := alias + "_obj"
		cond, args, err := e.buildObjectWhereForAlias(subQuery, objAlias)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf(`EXISTS (
			SELECT 1 FROM objects %s_obj
			WHERE %s_obj.id = %s.id AND %s
		)`, alias, alias, alias, cond), args, nil
	case QueryTypeSection:
		cond, args, err := e.sectionSubqueryCondition(subQuery, alias+"_sec")
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf(`EXISTS (
			SELECT 1 FROM sections %s_sec
			WHERE %s_sec.id = %s.id AND %s
		)`, alias, alias, alias, cond), args, nil
	default:
		return "", nil, fmt.Errorf("scope predicate expects a type or section query")
	}
}

func (e *Executor) buildObjectWhereForAlias(q *Query, alias string) (string, []interface{}, error) {
	conditions := []string{fmt.Sprintf("%s.type = ?", alias)}
	args := []interface{}{q.TypeName}
	if q.Predicate != nil {
		cond, predArgs, err := e.buildObjectPredicateSQL(q.Predicate, alias, q.TypeName)
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, cond)
		args = append(args, predArgs...)
	}
	return strings.Join(conditions, " AND "), args, nil
}

func (e *Executor) sectionSubqueryCondition(q *Query, alias string) (string, []interface{}, error) {
	if q.Type != QueryTypeSection {
		return "", nil, fmt.Errorf("expected section subquery")
	}
	if q.Predicate == nil {
		return "1=1", nil, nil
	}
	return e.buildSectionPredicateSQL(q.Predicate, alias)
}

func (e *Executor) traitSubqueryCondition(q *Query, alias string) (string, []interface{}, error) {
	if q.Type != QueryTypeTrait {
		return "", nil, fmt.Errorf("expected trait subquery")
	}
	conditions := []string{fmt.Sprintf("%s.trait_type = ?", alias)}
	args := []interface{}{q.TypeName}
	if q.Predicate != nil {
		cond, predArgs, err := e.buildTraitPredicateSQL(q.Predicate, alias)
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, cond)
		args = append(args, predArgs...)
	}
	return strings.Join(conditions, " AND "), args, nil
}

func directSectionParentCondition(sectionAlias, parentExpr string) string {
	return fmt.Sprintf("COALESCE(%s.parent_section_id, %s.file_object_id) = %s", sectionAlias, sectionAlias, parentExpr)
}

func currentScopeExpr(alias string, kind predicateKind) string {
	switch kind {
	case predicateKindTrait:
		return fmt.Sprintf("%s.parent_object_id", alias)
	case predicateKindSection, predicateKindObject:
		return fmt.Sprintf("%s.id", alias)
	default:
		return ""
	}
}

func scopeParentExpr(alias string, kind predicateKind) string {
	switch kind {
	case predicateKindTrait:
		return fmt.Sprintf("%s.parent_object_id", alias)
	case predicateKindSection:
		return fmt.Sprintf("COALESCE(%s.parent_section_id, %s.file_object_id)", alias, alias)
	default:
		return ""
	}
}

// buildRefsPredicateSQL builds SQL for refs([[target]]) or refs(type:...) predicates.
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
		targetCond, targetArgs := buildRefTargetVariantsCondition("r", resolvedTarget, p.Target)

		cond = fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs r
			WHERE (r.source_id = %s.id OR r.source_id LIKE %s.id || '#%%') AND %s
		)`, alias, alias, targetCond)
		args = append(args, targetArgs...)
	} else if p.SubQuery != nil {
		var targetTable string
		var targetAlias string
		var targetCondition string
		var err error
		switch p.SubQuery.Type {
		case QueryTypeObject:
			targetTable = "objects"
			targetAlias = "target_obj"
			targetCondition, args, err = e.buildObjectWhereForAlias(p.SubQuery, targetAlias)
		case QueryTypeSection:
			targetTable = "sections"
			targetAlias = "target_section"
			targetCondition, args, err = e.sectionSubqueryCondition(p.SubQuery, targetAlias)
		default:
			return "", nil, fmt.Errorf("refs() subquery must be a type or section query")
		}
		if err != nil {
			return "", nil, err
		}

		// Prefer target_id (resolved at index time), fall back to target_raw for unresolved refs
		cond = fmt.Sprintf(`EXISTS (
			SELECT 1 FROM refs r
			JOIN %s %s ON (
				r.target_id = %s.id OR 
				(r.target_id IS NULL AND r.target_raw = %s.id)
			)
			WHERE (r.source_id = %s.id OR r.source_id LIKE %s.id || '#%%') AND %s
		)`, targetTable, targetAlias, targetAlias, targetAlias, alias, alias, targetCondition)
	} else {
		return "", nil, fmt.Errorf("refs predicate must have target or subquery")
	}

	if p.Negated() {
		cond = "NOT " + cond
	}

	return cond, args, nil
}

// buildContentPredicateSQL builds SQL for content("search terms") predicates.
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
