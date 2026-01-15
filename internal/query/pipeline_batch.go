package query

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/sqlutil"
)

// batchedSubqueryAggregationForObjects executes a single batched query for all objects.
// Returns an error if batching is not possible (caller should fall back to N+1).
func (e *Executor) batchedSubqueryAggregationForObjects(results []PipelineObjectResult, stage *AssignmentStage) error {
	subQuery := stage.SubQuery
	agg := stage.Aggregation

	// Only COUNT is supported for batched execution currently
	if agg != AggCount {
		return fmt.Errorf("batched aggregation only supports COUNT")
	}

	// Find the self-ref predicate and determine binding type
	bindingType, nonSelfRefPreds := e.extractSelfRefBinding(subQuery.Predicates)
	if bindingType == "" {
		return fmt.Errorf("no self-ref binding found")
	}

	// For now, only batch simple queries without extra predicates
	// Complex predicates (value filters, etc.) require N+1 fallback
	if len(nonSelfRefPreds) > 0 {
		return fmt.Errorf("batching not supported with extra predicates")
	}

	// Collect all object IDs
	objectIDs := make([]string, len(results))
	for i, r := range results {
		objectIDs[i] = r.ID
	}

	var countMap map[string]int
	var err error

	// Build and execute batched query based on subquery type and binding
	if subQuery.Type == QueryTypeTrait {
		countMap, err = e.batchedTraitCount(objectIDs, subQuery.TypeName, bindingType, nonSelfRefPreds)
	} else {
		countMap, err = e.batchedObjectCount(objectIDs, subQuery.TypeName, bindingType, nonSelfRefPreds)
	}

	if err != nil {
		return err
	}

	// Populate results from map
	for i := range results {
		count := countMap[results[i].ID]
		results[i].Computed[stage.Name] = count
	}
	return nil
}

// extractSelfRefBinding finds the _ binding predicate and returns the binding type and remaining predicates.
func (e *Executor) extractSelfRefBinding(predicates []Predicate) (string, []Predicate) {
	var bindingType string
	var remaining []Predicate

	for _, pred := range predicates {
		switch p := pred.(type) {
		case *WithinPredicate:
			if p.IsSelfRef {
				bindingType = "within"
				continue
			}
		case *OnPredicate:
			if p.IsSelfRef {
				bindingType = "on"
				continue
			}
		case *ParentPredicate:
			if p.IsSelfRef {
				bindingType = "parent"
				continue
			}
		case *AncestorPredicate:
			if p.IsSelfRef {
				bindingType = "ancestor"
				continue
			}
		case *ChildPredicate:
			if p.IsSelfRef {
				bindingType = "child"
				continue
			}
		case *DescendantPredicate:
			if p.IsSelfRef {
				bindingType = "descendant"
				continue
			}
		case *RefsPredicate:
			if p.IsSelfRef {
				bindingType = "refs"
				continue
			}
		case *RefdPredicate:
			if p.IsSelfRef {
				bindingType = "refd"
				continue
			}
		}
		remaining = append(remaining, pred)
	}

	return bindingType, remaining
}

// batchedTraitCount executes a single query to count traits for multiple objects.
func (e *Executor) batchedTraitCount(objectIDs []string, traitType string, bindingType string, extraPreds []Predicate) (map[string]int, error) {
	result := make(map[string]int)

	if len(objectIDs) == 0 {
		return result, nil
	}

	inClause, idArgs := sqlutil.InClauseArgs(objectIDs)

	var sql string
	var args []any
	switch bindingType {
	case "on":
		// Direct parent match - simple GROUP BY
		sql = fmt.Sprintf(`
			SELECT t.parent_object_id, COUNT(*)
			FROM traits t
			WHERE t.trait_type = ? AND t.parent_object_id IN (%s)
			GROUP BY t.parent_object_id
		`, inClause)
		args = append([]any{traitType}, idArgs...)

	case "within":
		// Need to find traits within the subtree of each object
		// Use a CTE to find all descendants of target objects, then count traits
		sql = fmt.Sprintf(`
			WITH RECURSIVE descendants AS (
				SELECT id, id as root_id FROM objects WHERE id IN (%s)
				UNION ALL
				SELECT o.id, d.root_id FROM objects o
				JOIN descendants d ON o.parent_id = d.id
			)
			SELECT d.root_id, COUNT(*)
			FROM traits t
			JOIN descendants d ON t.parent_object_id = d.id
			WHERE t.trait_type = ?
			GROUP BY d.root_id
		`, inClause)
		// Args: object IDs first for CTE, then trait type.
		args = append(append([]any{}, idArgs...), traitType)

	default:
		return nil, fmt.Errorf("unsupported binding type for batched trait count: %s", bindingType)
	}

	rows, err := e.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("batched trait count failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var objectID string
		var count int
		if err := rows.Scan(&objectID, &count); err != nil {
			return nil, err
		}
		result[objectID] = count
	}

	return result, rows.Err()
}

// batchedObjectCount executes a single query to count objects for multiple parent objects.
func (e *Executor) batchedObjectCount(objectIDs []string, objectType string, bindingType string, extraPreds []Predicate) (map[string]int, error) {
	result := make(map[string]int)

	if len(objectIDs) == 0 {
		return result, nil
	}

	inClause, idArgs := sqlutil.InClauseArgs(objectIDs)

	var sql string
	var args []any
	switch bindingType {
	case "parent":
		// Direct children - simple GROUP BY
		sql = fmt.Sprintf(`
			SELECT o.parent_id, COUNT(*)
			FROM objects o
			WHERE o.type = ? AND o.parent_id IN (%s)
			GROUP BY o.parent_id
		`, inClause)
		args = append([]any{objectType}, idArgs...)

	case "ancestor":
		// Objects that have one of our objects as ancestor
		sql = fmt.Sprintf(`
			WITH RECURSIVE descendants AS (
				SELECT id, id as root_id FROM objects WHERE id IN (%s)
				UNION ALL
				SELECT o.id, d.root_id FROM objects o
				JOIN descendants d ON o.parent_id = d.id
			)
			SELECT d.root_id, COUNT(*)
			FROM objects o
			JOIN descendants d ON o.id = d.id
			WHERE o.type = ? AND o.id != d.root_id
			GROUP BY d.root_id
		`, inClause)
		// Args: object IDs first for CTE, then type.
		args = append(append([]any{}, idArgs...), objectType)

	case "refs":
		// Objects that reference our objects
		sql = fmt.Sprintf(`
			SELECT r.target_id, COUNT(DISTINCT r.source_id)
			FROM refs r
			JOIN objects o ON r.source_id = o.id
			WHERE o.type = ? AND (r.target_id IN (%s) OR r.target_raw IN (%s))
			GROUP BY r.target_id
		`, inClause, inClause)
		// Args: type, then IDs for target_id, then IDs for target_raw.
		args = append(append([]any{objectType}, idArgs...), idArgs...)

	case "refd":
		// Objects referenced by our objects
		sql = fmt.Sprintf(`
			SELECT r.source_id, COUNT(DISTINCT r.target_id)
			FROM refs r
			JOIN objects o ON r.target_id = o.id
			WHERE o.type = ? AND r.source_id IN (%s)
			GROUP BY r.source_id
		`, inClause)
		args = append([]any{objectType}, idArgs...)

	default:
		return nil, fmt.Errorf("unsupported binding type for batched object count: %s", bindingType)
	}

	rows, err := e.db.Query(sql, args...)
	if err != nil {
		return nil, fmt.Errorf("batched object count failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var objectID string
		var count int
		if err := rows.Scan(&objectID, &count); err != nil {
			return nil, err
		}
		result[objectID] = count
	}

	return result, rows.Err()
}

// batchedNavFuncForObjects executes batched navigation function aggregations.
func (e *Executor) batchedNavFuncForObjects(results []PipelineObjectResult, stage *AssignmentStage) error {
	if stage.Aggregation != AggCount {
		return fmt.Errorf("batched nav func only supports COUNT")
	}

	objectIDs := make([]string, len(results))
	for i, r := range results {
		objectIDs[i] = r.ID
	}

	var countMap map[string]int
	var err error

	switch stage.NavFunc.Name {
	case "refs":
		countMap, err = e.batchedCountRefsFrom(objectIDs)
	case "refd":
		countMap, err = e.batchedCountRefsTo(objectIDs)
	case "descendants":
		countMap, err = e.batchedCountDescendants(objectIDs)
	case "child":
		countMap, err = e.batchedCountChildren(objectIDs)
	default:
		return fmt.Errorf("unsupported nav func for batching: %s", stage.NavFunc.Name)
	}

	if err != nil {
		return err
	}

	for i := range results {
		count := countMap[results[i].ID]
		results[i].Computed[stage.Name] = count
	}
	return nil
}

// batchedCountRefsFrom counts outgoing references for multiple objects in one query.
func (e *Executor) batchedCountRefsFrom(objectIDs []string) (map[string]int, error) {
	result := make(map[string]int)
	if len(objectIDs) == 0 {
		return result, nil
	}

	inClause, args := sqlutil.InClauseArgs(objectIDs)

	sql := fmt.Sprintf(`
		SELECT source_id, COUNT(*)
		FROM refs
		WHERE source_id IN (%s)
		GROUP BY source_id
	`, inClause)

	rows, err := e.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		result[id] = count
	}
	return result, rows.Err()
}

// batchedCountRefsTo counts incoming references for multiple objects in one query.
func (e *Executor) batchedCountRefsTo(objectIDs []string) (map[string]int, error) {
	result := make(map[string]int)
	if len(objectIDs) == 0 {
		return result, nil
	}

	inClause, idArgs := sqlutil.InClauseArgs(objectIDs)
	args := append(append([]any{}, idArgs...), idArgs...)

	sql := fmt.Sprintf(`
		SELECT COALESCE(target_id, target_raw), COUNT(*)
		FROM refs
		WHERE target_id IN (%s) OR target_raw IN (%s)
		GROUP BY COALESCE(target_id, target_raw)
	`, inClause, inClause)

	rows, err := e.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		result[id] = count
	}
	return result, rows.Err()
}

// batchedCountDescendants counts descendants for multiple objects in one query.
func (e *Executor) batchedCountDescendants(objectIDs []string) (map[string]int, error) {
	result := make(map[string]int)
	if len(objectIDs) == 0 {
		return result, nil
	}

	inClause, args := sqlutil.InClauseArgs(objectIDs)

	sql := fmt.Sprintf(`
		WITH RECURSIVE descendants AS (
			SELECT id, id as root_id FROM objects WHERE id IN (%s)
			UNION ALL
			SELECT o.id, d.root_id FROM objects o
			JOIN descendants d ON o.parent_id = d.id
		)
		SELECT root_id, COUNT(*) - 1
		FROM descendants
		GROUP BY root_id
	`, inClause)

	rows, err := e.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		result[id] = count
	}
	return result, rows.Err()
}

// batchedCountChildren counts direct children for multiple objects in one query.
func (e *Executor) batchedCountChildren(objectIDs []string) (map[string]int, error) {
	result := make(map[string]int)
	if len(objectIDs) == 0 {
		return result, nil
	}

	inClause, args := sqlutil.InClauseArgs(objectIDs)

	sql := fmt.Sprintf(`
		SELECT parent_id, COUNT(*)
		FROM objects
		WHERE parent_id IN (%s)
		GROUP BY parent_id
	`, inClause)

	rows, err := e.db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		result[id] = count
	}
	return result, rows.Err()
}

// batchedNavFuncForTraits executes batched navigation function aggregations for traits.
func (e *Executor) batchedNavFuncForTraits(results []PipelineTraitResult, stage *AssignmentStage) error {
	if stage.Aggregation != AggCount {
		return fmt.Errorf("batched nav func only supports COUNT")
	}

	switch stage.NavFunc.Name {
	case "refs":
		// Count refs on each trait's line - batch by (file_path, line)
		return e.batchedCountRefsOnLines(results, stage.Name)
	default:
		return fmt.Errorf("unsupported nav func for trait batching: %s", stage.NavFunc.Name)
	}
}

// batchedCountRefsOnLines counts references on each trait's line in batched queries.
func (e *Executor) batchedCountRefsOnLines(results []PipelineTraitResult, assignName string) error {
	if len(results) == 0 {
		return nil
	}

	// Group traits by file for efficient querying
	type lineKey struct {
		filePath string
		line     int
	}
	lineToIndices := make(map[lineKey][]int)
	for i, r := range results {
		key := lineKey{r.FilePath, r.Line}
		lineToIndices[key] = append(lineToIndices[key], i)
	}

	// Get unique file paths
	files := make(map[string]bool)
	for key := range lineToIndices {
		files[key.filePath] = true
	}

	// Query ref counts grouped by file and line
	filePlaceholders := make([]string, 0, len(files))
	args := make([]interface{}, 0, len(files))
	for f := range files {
		filePlaceholders = append(filePlaceholders, "?")
		args = append(args, f)
	}

	sql := fmt.Sprintf(`
		SELECT file_path, line_number, COUNT(*)
		FROM refs
		WHERE file_path IN (%s)
		GROUP BY file_path, line_number
	`, strings.Join(filePlaceholders, ", "))

	rows, err := e.db.Query(sql, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Build count map
	countMap := make(map[lineKey]int)
	for rows.Next() {
		var filePath string
		var line, count int
		if err := rows.Scan(&filePath, &line, &count); err != nil {
			return err
		}
		countMap[lineKey{filePath, line}] = count
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Populate results
	for key, indices := range lineToIndices {
		count := countMap[key]
		for _, i := range indices {
			results[i].Computed[assignName] = count
		}
	}

	return nil
}
