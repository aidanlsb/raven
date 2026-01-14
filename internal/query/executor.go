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

	// Query all aliases from the database
	aliasRows, err := e.db.Query("SELECT alias, id FROM objects WHERE alias IS NOT NULL AND alias != '' ORDER BY id")
	if err != nil {
		// Fall back to resolver without aliases
		e.resolver = resolver.New(objectIDs)
		return e.resolver, nil
	}
	defer aliasRows.Close()

	aliases := make(map[string]string)
	for aliasRows.Next() {
		var alias, id string
		if err := aliasRows.Scan(&alias, &id); err != nil {
			continue
		}
		if _, exists := aliases[alias]; !exists {
			aliases[alias] = id
		}
	}

	e.resolver = resolver.NewWithAliases(objectIDs, aliases, "daily")
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

// PipelineObjectResult represents an object with computed values from pipeline.
type PipelineObjectResult struct {
	ObjectResult
	Computed map[string]interface{} // Computed values from assignments
}

// PipelineTraitResult represents a trait with computed values from pipeline.
type PipelineTraitResult struct {
	TraitResult
	Computed map[string]interface{} // Computed values from assignments
}

// ExecuteObjectQueryWithPipeline executes an object query with pipeline processing.
func (e *Executor) ExecuteObjectQueryWithPipeline(q *Query) ([]PipelineObjectResult, error) {
	if q.Type != QueryTypeObject {
		return nil, fmt.Errorf("expected object query, got trait query")
	}

	// First, execute the base query
	baseResults, err := e.executeObjectQuery(q)
	if err != nil {
		return nil, err
	}

	// If no pipeline, return base results with empty computed maps
	if q.Pipeline == nil || len(q.Pipeline.Stages) == 0 {
		results := make([]PipelineObjectResult, len(baseResults))
		for i, r := range baseResults {
			results[i] = PipelineObjectResult{ObjectResult: r, Computed: make(map[string]interface{})}
		}
		return results, nil
	}

	// Execute pipeline stages
	return e.executePipelineForObjects(baseResults, q.Pipeline)
}

// ExecuteTraitQueryWithPipeline executes a trait query with pipeline processing.
func (e *Executor) ExecuteTraitQueryWithPipeline(q *Query) ([]PipelineTraitResult, error) {
	if q.Type != QueryTypeTrait {
		return nil, fmt.Errorf("expected trait query, got object query")
	}

	// First, execute the base query
	baseResults, err := e.executeTraitQuery(q)
	if err != nil {
		return nil, err
	}

	// If no pipeline, return base results with empty computed maps
	if q.Pipeline == nil || len(q.Pipeline.Stages) == 0 {
		results := make([]PipelineTraitResult, len(baseResults))
		for i, r := range baseResults {
			results[i] = PipelineTraitResult{TraitResult: r, Computed: make(map[string]interface{})}
		}
		return results, nil
	}

	// Execute pipeline stages
	return e.executePipelineForTraits(baseResults, q.Pipeline)
}

// mergeSortStages combines consecutive SortStages into a single stage with multiple criteria
func mergeSortStages(stages []PipelineStage) []PipelineStage {
	if len(stages) == 0 {
		return stages
	}

	merged := make([]PipelineStage, 0, len(stages))
	var currentSort *SortStage

	for _, stage := range stages {
		if s, ok := stage.(*SortStage); ok {
			if currentSort == nil {
				// Start a new sort stage
				currentSort = &SortStage{Criteria: make([]SortCriterion, 0, len(s.Criteria))}
			}
			// Append criteria to current sort
			currentSort.Criteria = append(currentSort.Criteria, s.Criteria...)
		} else {
			// Non-sort stage - flush current sort if any
			if currentSort != nil {
				merged = append(merged, currentSort)
				currentSort = nil
			}
			merged = append(merged, stage)
		}
	}

	// Flush remaining sort
	if currentSort != nil {
		merged = append(merged, currentSort)
	}

	return merged
}

// executePipelineForObjects executes pipeline stages on object results.
func (e *Executor) executePipelineForObjects(results []ObjectResult, pipeline *Pipeline) ([]PipelineObjectResult, error) {
	// Initialize results with computed maps
	pResults := make([]PipelineObjectResult, len(results))
	for i, r := range results {
		pResults[i] = PipelineObjectResult{ObjectResult: r, Computed: make(map[string]interface{})}
	}

	// Merge consecutive sort stages
	stages := mergeSortStages(pipeline.Stages)

	// Process each stage
	for _, stage := range stages {
		var err error
		switch s := stage.(type) {
		case *AssignmentStage:
			err = e.executeAssignmentForObjects(pResults, s)
		case *FilterStage:
			pResults, err = e.executeFilterForObjects(pResults, s)
		case *SortStage:
			err = e.executeSortForObjects(pResults, s)
		case *LimitStage:
			if len(pResults) > s.N {
				pResults = pResults[:s.N]
			}
		}
		if err != nil {
			return nil, err
		}
	}

	return pResults, nil
}

// executePipelineForTraits executes pipeline stages on trait results.
func (e *Executor) executePipelineForTraits(results []TraitResult, pipeline *Pipeline) ([]PipelineTraitResult, error) {
	// Initialize results with computed maps
	pResults := make([]PipelineTraitResult, len(results))
	for i, r := range results {
		pResults[i] = PipelineTraitResult{TraitResult: r, Computed: make(map[string]interface{})}
	}

	// Merge consecutive sort stages
	stages := mergeSortStages(pipeline.Stages)

	// Process each stage
	for _, stage := range stages {
		var err error
		switch s := stage.(type) {
		case *AssignmentStage:
			err = e.executeAssignmentForTraits(pResults, s)
		case *FilterStage:
			pResults, err = e.executeFilterForTraits(pResults, s)
		case *SortStage:
			err = e.executeSortForTraits(pResults, s)
		case *LimitStage:
			if len(pResults) > s.N {
				pResults = pResults[:s.N]
			}
		}
		if err != nil {
			return nil, err
		}
	}

	return pResults, nil
}

// executeAssignmentForObjects computes an aggregation for each object result.
// Uses batched queries when possible to avoid N+1 query patterns.
func (e *Executor) executeAssignmentForObjects(results []PipelineObjectResult, stage *AssignmentStage) error {
	if len(results) == 0 {
		return nil
	}

	// Try batched execution for subqueries
	if stage.SubQuery != nil {
		if err := e.batchedSubqueryAggregationForObjects(results, stage); err == nil {
			return nil
		}
		// Fall back to N+1 if batching fails (complex predicates, etc.)
	}

	// Try batched execution for navigation functions
	if stage.NavFunc != nil {
		if err := e.batchedNavFuncForObjects(results, stage); err == nil {
			return nil
		}
		// Fall back to N+1 if batching fails
	}

	// Fallback: N+1 execution (for complex cases)
	for i := range results {
		value, err := e.computeAggregationForObject(&results[i].ObjectResult, stage)
		if err != nil {
			return err
		}
		results[i].Computed[stage.Name] = value
	}
	return nil
}

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

	// Build placeholders for IN clause
	placeholders := make([]string, len(objectIDs))
	args := make([]interface{}, 0, len(objectIDs)+1)
	args = append(args, traitType)
	for i, id := range objectIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	inClause := strings.Join(placeholders, ", ")

	var sql string
	switch bindingType {
	case "on":
		// Direct parent match - simple GROUP BY
		sql = fmt.Sprintf(`
			SELECT t.parent_object_id, COUNT(*)
			FROM traits t
			WHERE t.trait_type = ? AND t.parent_object_id IN (%s)
			GROUP BY t.parent_object_id
		`, inClause)

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
		// Reorder args: object IDs first for CTE, then trait type
		args = append([]interface{}{}, args[1:]...) // objectIDs
		args = append(args, traitType)

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

	// Build placeholders for IN clause
	placeholders := make([]string, len(objectIDs))
	args := make([]interface{}, 0, len(objectIDs)+1)
	args = append(args, objectType)
	for i, id := range objectIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	inClause := strings.Join(placeholders, ", ")

	var sql string
	switch bindingType {
	case "parent":
		// Direct children - simple GROUP BY
		sql = fmt.Sprintf(`
			SELECT o.parent_id, COUNT(*)
			FROM objects o
			WHERE o.type = ? AND o.parent_id IN (%s)
			GROUP BY o.parent_id
		`, inClause)

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
		// Reorder args: object IDs first for CTE, then type
		args = append([]interface{}{}, args[1:]...)
		args = append(args, objectType)

	case "refs":
		// Objects that reference our objects
		sql = fmt.Sprintf(`
			SELECT r.target_id, COUNT(DISTINCT r.source_id)
			FROM refs r
			JOIN objects o ON r.source_id = o.id
			WHERE o.type = ? AND (r.target_id IN (%s) OR r.target_raw IN (%s))
			GROUP BY r.target_id
		`, inClause, inClause)
		// Need to duplicate objectIDs for target_raw
		args = append(args, args[1:]...)

	case "refd":
		// Objects referenced by our objects
		sql = fmt.Sprintf(`
			SELECT r.source_id, COUNT(DISTINCT r.target_id)
			FROM refs r
			JOIN objects o ON r.target_id = o.id
			WHERE o.type = ? AND r.source_id IN (%s)
			GROUP BY r.source_id
		`, inClause)

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

	placeholders := make([]string, len(objectIDs))
	args := make([]interface{}, len(objectIDs))
	for i, id := range objectIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	sql := fmt.Sprintf(`
		SELECT source_id, COUNT(*)
		FROM refs
		WHERE source_id IN (%s)
		GROUP BY source_id
	`, strings.Join(placeholders, ", "))

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

	placeholders := make([]string, len(objectIDs))
	args := make([]interface{}, len(objectIDs)*2)
	for i, id := range objectIDs {
		placeholders[i] = "?"
		args[i] = id
		args[i+len(objectIDs)] = id
	}

	sql := fmt.Sprintf(`
		SELECT COALESCE(target_id, target_raw), COUNT(*)
		FROM refs
		WHERE target_id IN (%s) OR target_raw IN (%s)
		GROUP BY COALESCE(target_id, target_raw)
	`, strings.Join(placeholders, ", "), strings.Join(placeholders, ", "))

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

	placeholders := make([]string, len(objectIDs))
	args := make([]interface{}, len(objectIDs))
	for i, id := range objectIDs {
		placeholders[i] = "?"
		args[i] = id
	}

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
	`, strings.Join(placeholders, ", "))

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

	placeholders := make([]string, len(objectIDs))
	args := make([]interface{}, len(objectIDs))
	for i, id := range objectIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	sql := fmt.Sprintf(`
		SELECT parent_id, COUNT(*)
		FROM objects
		WHERE parent_id IN (%s)
		GROUP BY parent_id
	`, strings.Join(placeholders, ", "))

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

// executeAssignmentForTraits computes an aggregation for each trait result.
// Uses batched queries when possible to avoid N+1 query patterns.
func (e *Executor) executeAssignmentForTraits(results []PipelineTraitResult, stage *AssignmentStage) error {
	if len(results) == 0 {
		return nil
	}

	// Try batched execution for navigation functions
	if stage.NavFunc != nil {
		if err := e.batchedNavFuncForTraits(results, stage); err == nil {
			return nil
		}
		// Fall back to N+1 if batching fails
	}

	// Fallback: N+1 execution (for subqueries and complex cases)
	for i := range results {
		value, err := e.computeAggregationForTrait(&results[i].TraitResult, stage)
		if err != nil {
			return err
		}
		results[i].Computed[stage.Name] = value
	}
	return nil
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

// computeAggregationForObject computes an aggregation value for a single object.
func (e *Executor) computeAggregationForObject(obj *ObjectResult, stage *AssignmentStage) (interface{}, error) {
	// Handle navigation functions
	if stage.NavFunc != nil {
		return e.computeNavFuncForObject(obj, stage.NavFunc, stage.Aggregation)
	}

	// Handle subquery aggregation
	if stage.SubQuery != nil {
		return e.computeSubqueryAggregationForObject(obj, stage.SubQuery, stage.Aggregation, stage.AggField)
	}

	return 0, fmt.Errorf("assignment stage has neither nav function nor subquery")
}

// computeAggregationForTrait computes an aggregation value for a single trait.
func (e *Executor) computeAggregationForTrait(trait *TraitResult, stage *AssignmentStage) (interface{}, error) {
	// Handle navigation functions
	if stage.NavFunc != nil {
		return e.computeNavFuncForTrait(trait, stage.NavFunc, stage.Aggregation)
	}

	// Handle subquery aggregation
	if stage.SubQuery != nil {
		return e.computeSubqueryAggregationForTrait(trait, stage.SubQuery, stage.Aggregation, stage.AggField)
	}

	return 0, fmt.Errorf("assignment stage has neither nav function nor subquery")
}

// computeNavFuncForObject computes a navigation function result for an object.
func (e *Executor) computeNavFuncForObject(obj *ObjectResult, navFunc *NavFunc, agg AggregationType) (interface{}, error) {
	var count int
	var err error

	switch navFunc.Name {
	case "refs":
		count, err = e.countRefsFrom(obj.ID)
	case "refd":
		count, err = e.countRefsTo(obj.ID)
	case "ancestors":
		count, err = e.countAncestors(obj.ID)
	case "descendants":
		count, err = e.countDescendants(obj.ID)
	case "parent":
		if obj.ParentID != nil {
			count = 1
		}
	case "child":
		count, err = e.countChildren(obj.ID)
	default:
		return 0, fmt.Errorf("unknown navigation function: %s", navFunc.Name)
	}

	if err != nil {
		return 0, err
	}

	// For count aggregation, return the count
	if agg == AggCount {
		return count, nil
	}

	return count, nil
}

// computeNavFuncForTrait computes a navigation function result for a trait.
func (e *Executor) computeNavFuncForTrait(trait *TraitResult, navFunc *NavFunc, agg AggregationType) (interface{}, error) {
	// For traits, navigation functions typically operate on the parent object
	switch navFunc.Name {
	case "refs":
		// Count refs on the same line as the trait
		return e.countRefsOnLine(trait.FilePath, trait.Line)
	case "refd":
		// Traits aren't typically referenced directly, so this returns 0
		return 0, nil
	default:
		return 0, fmt.Errorf("navigation function %s not supported for traits", navFunc.Name)
	}
}

// computeSubqueryAggregationForObject executes a subquery with _ bound to the object.
func (e *Executor) computeSubqueryAggregationForObject(obj *ObjectResult, subQuery *Query, agg AggregationType, field string) (interface{}, error) {
	// Build the subquery SQL with _ bound to the current object
	// We need to replace ancestor:_ or within:_ predicates with the current object ID
	boundQuery := e.bindObjectToQuery(obj.ID, subQuery)

	switch subQuery.Type {
	case QueryTypeObject:
		results, err := e.executeObjectQuery(boundQuery)
		if err != nil {
			return 0, err
		}
		return e.aggregateObjectResults(results, agg, field)

	case QueryTypeTrait:
		results, err := e.executeTraitQuery(boundQuery)
		if err != nil {
			return 0, err
		}
		return e.aggregateTraitResults(results, agg)
	}

	return 0, nil
}

// computeSubqueryAggregationForTrait executes a subquery with _ bound to the trait.
func (e *Executor) computeSubqueryAggregationForTrait(trait *TraitResult, subQuery *Query, agg AggregationType, field string) (interface{}, error) {
	// Bind _ to the trait - this may error if predicates expect objects
	boundQuery, err := e.bindTraitToQuery(trait, subQuery)
	if err != nil {
		return 0, err
	}

	switch subQuery.Type {
	case QueryTypeObject:
		results, err := e.executeObjectQuery(boundQuery)
		if err != nil {
			return 0, err
		}
		return e.aggregateObjectResults(results, agg, field)

	case QueryTypeTrait:
		results, err := e.executeTraitQuery(boundQuery)
		if err != nil {
			return 0, err
		}
		return e.aggregateTraitResults(results, agg)
	}

	return 0, nil
}

// bindObjectToQuery creates a copy of the query with _ references resolved to the object ID.
func (e *Executor) bindObjectToQuery(objectID string, q *Query) *Query {
	// Deep copy the query and replace self-reference predicates
	bound := &Query{
		Type:     q.Type,
		TypeName: q.TypeName,
	}

	for _, pred := range q.Predicates {
		bound.Predicates = append(bound.Predicates, e.bindPredicateToObject(objectID, pred))
	}

	return bound
}

// bindPredicateToObject resolves _ references in a predicate to a specific object ID.
func (e *Executor) bindPredicateToObject(objectID string, pred Predicate) Predicate {
	switch p := pred.(type) {
	case *AncestorPredicate:
		if p.IsSelfRef {
			return &AncestorPredicate{
				basePredicate: p.basePredicate,
				Target:        objectID,
			}
		}
	case *WithinPredicate:
		if p.IsSelfRef {
			return &WithinPredicate{
				basePredicate: p.basePredicate,
				Target:        objectID,
			}
		}
	case *ParentPredicate:
		if p.IsSelfRef {
			return &ParentPredicate{
				basePredicate: p.basePredicate,
				Target:        objectID,
			}
		}
	case *DescendantPredicate:
		if p.IsSelfRef {
			return &DescendantPredicate{
				basePredicate: p.basePredicate,
				Target:        objectID,
			}
		}
	case *ChildPredicate:
		if p.IsSelfRef {
			return &ChildPredicate{
				basePredicate: p.basePredicate,
				Target:        objectID,
			}
		}
	case *RefsPredicate:
		if p.IsSelfRef {
			return &RefsPredicate{
				basePredicate: p.basePredicate,
				Target:        objectID,
			}
		}
	case *RefdPredicate:
		if p.IsSelfRef {
			return &RefdPredicate{
				basePredicate: p.basePredicate,
				Target:        objectID,
			}
		}
	case *OnPredicate:
		if p.IsSelfRef {
			return &OnPredicate{
				basePredicate: p.basePredicate,
				Target:        objectID,
			}
		}
	case *GroupPredicate:
		boundPreds := make([]Predicate, len(p.Predicates))
		for i, subPred := range p.Predicates {
			boundPreds[i] = e.bindPredicateToObject(objectID, subPred)
		}
		return &GroupPredicate{basePredicate: p.basePredicate, Predicates: boundPreds}
	case *OrPredicate:
		return &OrPredicate{
			basePredicate: p.basePredicate,
			Left:          e.bindPredicateToObject(objectID, p.Left),
			Right:         e.bindPredicateToObject(objectID, p.Right),
		}
	}
	return pred
}

// TraitBinding holds a trait's identity for binding _ references
type TraitBinding struct {
	ID       string // Trait ID
	FilePath string // File containing the trait
	Line     int    // Line number of the trait
}

// bindTraitToQuery creates a copy of the query with _ references resolved to the trait.
// IMPORTANT: _ ALWAYS represents the trait itself. Predicates that expect objects
// (like on:, within:) will error when given a trait reference.
func (e *Executor) bindTraitToQuery(trait *TraitResult, q *Query) (*Query, error) {
	bound := &Query{
		Type:     q.Type,
		TypeName: q.TypeName,
	}

	binding := &TraitBinding{
		ID:       trait.ID,
		FilePath: trait.FilePath,
		Line:     trait.Line,
	}

	for _, pred := range q.Predicates {
		boundPred, err := e.bindPredicateToTrait(binding, pred)
		if err != nil {
			return nil, err
		}
		bound.Predicates = append(bound.Predicates, boundPred)
	}

	return bound, nil
}

// bindPredicateToTrait resolves _ references in a predicate to a specific trait.
// Returns an error if the predicate expects an object but receives a trait reference.
func (e *Executor) bindPredicateToTrait(trait *TraitBinding, pred Predicate) (Predicate, error) {
	switch p := pred.(type) {
	case *OnPredicate:
		if p.IsSelfRef {
			return nil, fmt.Errorf("on:_ is invalid in trait context: 'on:' expects an object, but _ refers to a trait. Use a subquery to access related objects")
		}
	case *WithinPredicate:
		if p.IsSelfRef {
			return nil, fmt.Errorf("within:_ is invalid in trait context: 'within:' expects an object, but _ refers to a trait. Use a subquery to access related objects")
		}
	case *AncestorPredicate:
		if p.IsSelfRef {
			return nil, fmt.Errorf("ancestor:_ is invalid in trait context: 'ancestor:' expects an object, but _ refers to a trait")
		}
	case *DescendantPredicate:
		if p.IsSelfRef {
			return nil, fmt.Errorf("descendant:_ is invalid in trait context: 'descendant:' expects an object, but _ refers to a trait")
		}
	case *ParentPredicate:
		if p.IsSelfRef {
			return nil, fmt.Errorf("parent:_ is invalid in trait context: 'parent:' expects an object, but _ refers to a trait")
		}
	case *ChildPredicate:
		if p.IsSelfRef {
			return nil, fmt.Errorf("child:_ is invalid in trait context: 'child:' expects an object, but _ refers to a trait")
		}
	case *AtPredicate:
		if p.IsSelfRef {
			// at:_ for a trait means "at the same file+line as this trait"
			return &AtPredicate{
				basePredicate: p.basePredicate,
				Target:        trait.ID,
			}, nil
		}
	case *RefsPredicate:
		if p.IsSelfRef {
			// refs:_ means "has a reference to _" but traits can't be referenced via [[...]]
			return nil, fmt.Errorf("refs:_ is invalid in trait context: traits cannot be referenced via wikilinks")
		}
	case *HasPredicate:
		if p.IsSelfRef {
			// has:_ in trait context means "objects that have this trait"
			return &HasPredicate{
				basePredicate: p.basePredicate,
				TraitID:       trait.ID,
			}, nil
		}
	case *ContainsPredicate:
		if p.IsSelfRef {
			// contains:_ in trait context means "objects that contain this trait in subtree"
			return &ContainsPredicate{
				basePredicate: p.basePredicate,
				TraitID:       trait.ID,
			}, nil
		}
	case *RefdPredicate:
		if p.IsSelfRef {
			// refd:_ means "is referenced by _" - for traits, this means
			// "is referenced by the line containing this trait"
			// We encode the trait's file:line as a special marker
			return &RefdPredicate{
				basePredicate: p.basePredicate,
				Target:        fmt.Sprintf("__trait_line:%s:%d", trait.FilePath, trait.Line),
			}, nil
		}
	case *GroupPredicate:
		boundPreds := make([]Predicate, len(p.Predicates))
		for i, subPred := range p.Predicates {
			boundPred, err := e.bindPredicateToTrait(trait, subPred)
			if err != nil {
				return nil, err
			}
			boundPreds[i] = boundPred
		}
		return &GroupPredicate{basePredicate: p.basePredicate, Predicates: boundPreds}, nil
	case *OrPredicate:
		left, err := e.bindPredicateToTrait(trait, p.Left)
		if err != nil {
			return nil, err
		}
		right, err := e.bindPredicateToTrait(trait, p.Right)
		if err != nil {
			return nil, err
		}
		return &OrPredicate{
			basePredicate: p.basePredicate,
			Left:          left,
			Right:         right,
		}, nil
	}
	return pred, nil
}

// aggregateObjectResults computes an aggregate value from object results.
// For min/max/sum, a field name must be provided.
func (e *Executor) aggregateObjectResults(results []ObjectResult, agg AggregationType, field string) (interface{}, error) {
	switch agg {
	case AggCount:
		return len(results), nil
	case AggMin:
		if field == "" {
			return nil, fmt.Errorf("min() on object query requires a field: use min(.field, {object:...})")
		}
		if len(results) == 0 {
			return nil, nil
		}
		var minVal interface{}
		for _, r := range results {
			val := r.Fields[field]
			if val == nil {
				continue
			}
			if minVal == nil || compareValues(val, minVal) < 0 {
				minVal = val
			}
		}
		return minVal, nil
	case AggMax:
		if field == "" {
			return nil, fmt.Errorf("max() on object query requires a field: use max(.field, {object:...})")
		}
		if len(results) == 0 {
			return nil, nil
		}
		var maxVal interface{}
		for _, r := range results {
			val := r.Fields[field]
			if val == nil {
				continue
			}
			if maxVal == nil || compareValues(val, maxVal) > 0 {
				maxVal = val
			}
		}
		return maxVal, nil
	case AggSum:
		if field == "" {
			return nil, fmt.Errorf("sum() on object query requires a field: use sum(.field, {object:...})")
		}
		var sum float64
		for _, r := range results {
			val := r.Fields[field]
			if num, ok := toNumber(val); ok {
				sum += num
			}
		}
		return sum, nil
	default:
		return len(results), nil
	}
}

// aggregateTraitResults computes an aggregate value from trait results.
// For traits, min/max/sum operate on the trait's Value field.
func (e *Executor) aggregateTraitResults(results []TraitResult, agg AggregationType) (interface{}, error) {
	switch agg {
	case AggCount:
		return len(results), nil
	case AggMin:
		if len(results) == 0 {
			return nil, nil
		}
		var minVal *string
		for i := range results {
			if results[i].Value != nil {
				if minVal == nil || *results[i].Value < *minVal {
					minVal = results[i].Value
				}
			}
		}
		return minVal, nil
	case AggMax:
		if len(results) == 0 {
			return nil, nil
		}
		var maxVal *string
		for i := range results {
			if results[i].Value != nil {
				if maxVal == nil || *results[i].Value > *maxVal {
					maxVal = results[i].Value
				}
			}
		}
		return maxVal, nil
	case AggSum:
		var sum float64
		for _, r := range results {
			if r.Value != nil {
				if num, ok := toNumber(*r.Value); ok {
					sum += num
				}
			}
		}
		return sum, nil
	default:
		return len(results), nil
	}
}

// Helper functions for navigation

func (e *Executor) countRefsFrom(objectID string) (int, error) {
	var count int
	err := e.db.QueryRow("SELECT COUNT(*) FROM refs WHERE source_id = ?", objectID).Scan(&count)
	return count, err
}

func (e *Executor) countRefsTo(objectID string) (int, error) {
	var count int
	err := e.db.QueryRow("SELECT COUNT(*) FROM refs WHERE target_id = ? OR target_raw = ?", objectID, objectID).Scan(&count)
	return count, err
}

func (e *Executor) countAncestors(objectID string) (int, error) {
	var count int
	err := e.db.QueryRow(`
		WITH RECURSIVE ancestors AS (
			SELECT parent_id FROM objects WHERE id = ?
			UNION ALL
			SELECT o.parent_id FROM objects o
			JOIN ancestors a ON o.id = a.parent_id
			WHERE o.parent_id IS NOT NULL
		)
		SELECT COUNT(*) FROM ancestors WHERE parent_id IS NOT NULL
	`, objectID).Scan(&count)
	return count, err
}

func (e *Executor) countDescendants(objectID string) (int, error) {
	var count int
	err := e.db.QueryRow(`
		WITH RECURSIVE descendants AS (
			SELECT id FROM objects WHERE parent_id = ?
			UNION ALL
			SELECT o.id FROM objects o
			JOIN descendants d ON o.parent_id = d.id
		)
		SELECT COUNT(*) FROM descendants
	`, objectID).Scan(&count)
	return count, err
}

func (e *Executor) countChildren(objectID string) (int, error) {
	var count int
	err := e.db.QueryRow("SELECT COUNT(*) FROM objects WHERE parent_id = ?", objectID).Scan(&count)
	return count, err
}

func (e *Executor) countRefsOnLine(filePath string, line int) (int, error) {
	var count int
	err := e.db.QueryRow("SELECT COUNT(*) FROM refs WHERE file_path = ? AND line_number = ?", filePath, line).Scan(&count)
	return count, err
}

// Filter execution

func (e *Executor) executeFilterForObjects(results []PipelineObjectResult, stage *FilterStage) ([]PipelineObjectResult, error) {
	var filtered []PipelineObjectResult
	for _, r := range results {
		match, err := e.evaluateFilterExpr(r.Computed, r.Fields, stage.Expr)
		if err != nil {
			return nil, err
		}
		if match {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

func (e *Executor) executeFilterForTraits(results []PipelineTraitResult, stage *FilterStage) ([]PipelineTraitResult, error) {
	var filtered []PipelineTraitResult
	for _, r := range results {
		// For traits, create a pseudo-fields map with the value
		traitFields := map[string]interface{}{}
		if r.Value != nil {
			traitFields["value"] = *r.Value
		}
		match, err := e.evaluateFilterExpr(r.Computed, traitFields, stage.Expr)
		if err != nil {
			return nil, err
		}
		if match {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

func (e *Executor) evaluateFilterExpr(computed map[string]interface{}, fields map[string]interface{}, expr *FilterExpr) (bool, error) {
	// Get left value
	var leftVal interface{}
	if expr.IsField {
		if fields != nil {
			leftVal = fields[expr.Left]
		}
	} else {
		leftVal = computed[expr.Left]
	}

	// Convert to comparable types
	leftNum, leftIsNum := toNumber(leftVal)
	rightNum, rightIsNum := toNumber(expr.Right)

	// Compare
	switch expr.Op {
	case CompareEq:
		if leftIsNum && rightIsNum {
			return leftNum == rightNum, nil
		}
		return fmt.Sprint(leftVal) == expr.Right, nil
	case CompareNeq:
		if leftIsNum && rightIsNum {
			return leftNum != rightNum, nil
		}
		return fmt.Sprint(leftVal) != expr.Right, nil
	case CompareLt:
		if leftIsNum && rightIsNum {
			return leftNum < rightNum, nil
		}
		return fmt.Sprint(leftVal) < expr.Right, nil
	case CompareGt:
		if leftIsNum && rightIsNum {
			return leftNum > rightNum, nil
		}
		return fmt.Sprint(leftVal) > expr.Right, nil
	case CompareLte:
		if leftIsNum && rightIsNum {
			return leftNum <= rightNum, nil
		}
		return fmt.Sprint(leftVal) <= expr.Right, nil
	case CompareGte:
		if leftIsNum && rightIsNum {
			return leftNum >= rightNum, nil
		}
		return fmt.Sprint(leftVal) >= expr.Right, nil
	}

	return false, nil
}

func toNumber(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case string:
		var f float64
		_, err := fmt.Sscanf(n, "%f", &f)
		return f, err == nil
	}
	return 0, false
}

// Sort execution

func (e *Executor) executeSortForObjects(results []PipelineObjectResult, stage *SortStage) error {
	sort.SliceStable(results, func(i, j int) bool {
		// Compare by each criterion in order
		for _, c := range stage.Criteria {
			var iVal, jVal interface{}

			if c.IsField {
				iVal = results[i].Fields[c.Field]
				jVal = results[j].Fields[c.Field]
			} else {
				iVal = results[i].Computed[c.Field]
				jVal = results[j].Computed[c.Field]
			}

			cmp := compareValues(iVal, jVal)
			if cmp != 0 {
				if c.Descending {
					return cmp > 0
				}
				return cmp < 0
			}
			// Equal on this criterion, continue to next
		}
		return false // All criteria equal
	})
	return nil
}

func (e *Executor) executeSortForTraits(results []PipelineTraitResult, stage *SortStage) error {
	sort.SliceStable(results, func(i, j int) bool {
		// Compare by each criterion in order
		for _, c := range stage.Criteria {
			var iVal, jVal interface{}

			if c.IsField {
				// Traits don't have fields, use value
				if c.Field == "value" {
					iVal = results[i].Value
					jVal = results[j].Value
				}
			} else {
				iVal = results[i].Computed[c.Field]
				jVal = results[j].Computed[c.Field]
			}

			cmp := compareValues(iVal, jVal)
			if cmp != 0 {
				if c.Descending {
					return cmp > 0
				}
				return cmp < 0
			}
			// Equal on this criterion, continue to next
		}
		return false // All criteria equal
	})
	return nil
}

func compareValues(a, b interface{}) int {
	// Handle nil
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	// Try numeric comparison
	aNum, aIsNum := toNumber(a)
	bNum, bIsNum := toNumber(b)
	if aIsNum && bIsNum {
		if aNum < bNum {
			return -1
		}
		if aNum > bNum {
			return 1
		}
		return 0
	}

	// String comparison
	aStr := fmt.Sprint(a)
	bStr := fmt.Sprint(b)
	if aStr < bStr {
		return -1
	}
	if aStr > bStr {
		return 1
	}
	return 0
}

// executeObjectQuery executes an object query and returns matching objects.
// This is internal - external callers should use ExecuteObjectQueryWithPipeline.
func (e *Executor) executeObjectQuery(q *Query) ([]ObjectResult, error) {
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

// executeTraitQuery executes a trait query and returns matching traits.
// This is internal - external callers should use ExecuteTraitQueryWithPipeline.
func (e *Executor) executeTraitQuery(q *Query) ([]TraitResult, error) {
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

	return sqlStr, args, nil
}

// buildObjectPredicateSQL builds SQL for an object predicate.
func (e *Executor) buildObjectPredicateSQL(pred Predicate, alias string) (string, []interface{}, error) {
	switch p := pred.(type) {
	case *FieldPredicate:
		return e.buildFieldPredicateSQL(p, alias)
	case *StringFuncPredicate:
		return e.buildStringFuncPredicateSQL(p, alias)
	case *ArrayQuantifierPredicate:
		return e.buildArrayQuantifierPredicateSQL(p, alias)
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
	case *StringFuncPredicate:
		return e.buildTraitStringFuncPredicateSQL(p, alias)
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

// buildFieldPredicateSQL builds SQL for .field==value predicates.
// Comparisons are case-insensitive for equality, but case-sensitive for ordering comparisons.
func (e *Executor) buildFieldPredicateSQL(p *FieldPredicate, alias string) (string, []interface{}, error) {
	jsonPath := fmt.Sprintf("$.%s", p.Field)

	var cond string
	var args []interface{}

	if p.IsExists {
		// .field==* means field exists, .field!=* means field doesn't exist
		if p.CompareOp == CompareNeq {
			cond = fmt.Sprintf("json_extract(%s.fields, ?) IS NULL", alias)
		} else {
			cond = fmt.Sprintf("json_extract(%s.fields, ?) IS NOT NULL", alias)
		}
		args = append(args, jsonPath)
	} else if p.CompareOp == CompareNeq {
		// Not equals: check both scalar and array membership
		cond = fmt.Sprintf(`(
			LOWER(json_extract(%s.fields, ?)) != LOWER(?) AND
			NOT EXISTS (
				SELECT 1 FROM json_each(%s.fields, ?)
				WHERE LOWER(json_each.value) = LOWER(?)
			)
		)`, alias, alias)
		args = append(args, jsonPath, p.Value, jsonPath, p.Value)
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

// buildStringFuncPredicateSQL builds SQL for string function predicates.
// Handles: includes(.field, "str"), startswith(.field, "str"), endswith(.field, "str"), matches(.field, "pattern")
func (e *Executor) buildStringFuncPredicateSQL(p *StringFuncPredicate, alias string) (string, []interface{}, error) {
	jsonPath := fmt.Sprintf("$.%s", p.Field)
	fieldExpr := fmt.Sprintf("json_extract(%s.fields, ?)", alias)

	var cond string
	var args []interface{}
	args = append(args, jsonPath)

	// Determine case handling
	wrapLower := !p.CaseSensitive

	switch p.FuncType {
	case StringFuncIncludes:
		if wrapLower {
			cond = fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", fieldExpr)
		} else {
			cond = fmt.Sprintf("%s LIKE ?", fieldExpr)
		}
		args = append(args, "%"+p.Value+"%")

	case StringFuncStartsWith:
		if wrapLower {
			cond = fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", fieldExpr)
		} else {
			cond = fmt.Sprintf("%s LIKE ?", fieldExpr)
		}
		args = append(args, p.Value+"%")

	case StringFuncEndsWith:
		if wrapLower {
			cond = fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", fieldExpr)
		} else {
			cond = fmt.Sprintf("%s LIKE ?", fieldExpr)
		}
		args = append(args, "%"+p.Value)

	case StringFuncMatches:
		if wrapLower {
			// For case-insensitive regex, we use (?i) prefix in the pattern
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

// buildArrayQuantifierPredicateSQL builds SQL for array quantifier predicates.
// Handles: any(.tags, _ == "urgent"), all(.tags, startswith(_, "feature-")), none(.tags, _ == "deprecated")
func (e *Executor) buildArrayQuantifierPredicateSQL(p *ArrayQuantifierPredicate, alias string) (string, []interface{}, error) {
	jsonPath := fmt.Sprintf("$.%s", p.Field)

	var cond string
	var args []interface{}

	// Build the element condition
	elemCond, elemArgs, err := e.buildElementPredicateSQL(p.ElementPred)
	if err != nil {
		return "", nil, err
	}

	switch p.Quantifier {
	case ArrayQuantifierAny:
		// EXISTS (SELECT 1 FROM json_each(fields, '$.field') WHERE <elemCond>)
		cond = fmt.Sprintf(`EXISTS (
			SELECT 1 FROM json_each(%s.fields, ?)
			WHERE %s
		)`, alias, elemCond)
		args = append(args, jsonPath)
		args = append(args, elemArgs...)

	case ArrayQuantifierAll:
		// NOT EXISTS (SELECT 1 FROM json_each(fields, '$.field') WHERE NOT <elemCond>)
		// This means: there is no element that doesn't satisfy the condition
		cond = fmt.Sprintf(`NOT EXISTS (
			SELECT 1 FROM json_each(%s.fields, ?)
			WHERE NOT (%s)
		)`, alias, elemCond)
		args = append(args, jsonPath)
		args = append(args, elemArgs...)

	case ArrayQuantifierNone:
		// NOT EXISTS (SELECT 1 FROM json_each(fields, '$.field') WHERE <elemCond>)
		cond = fmt.Sprintf(`NOT EXISTS (
			SELECT 1 FROM json_each(%s.fields, ?)
			WHERE %s
		)`, alias, elemCond)
		args = append(args, jsonPath)
		args = append(args, elemArgs...)
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, args, nil
}

// buildElementPredicateSQL builds SQL for predicates used within array quantifiers.
// The context is json_each.value representing the current array element.
func (e *Executor) buildElementPredicateSQL(pred Predicate) (string, []interface{}, error) {
	switch p := pred.(type) {
	case *ElementEqualityPredicate:
		return e.buildElementEqualitySQL(p)

	case *StringFuncPredicate:
		if !p.IsElementRef {
			return "", nil, fmt.Errorf("string function in array context must use _ as first argument")
		}
		return e.buildElementStringFuncSQL(p)

	case *OrPredicate:
		leftCond, leftArgs, err := e.buildElementPredicateSQL(p.Left)
		if err != nil {
			return "", nil, err
		}
		rightCond, rightArgs, err := e.buildElementPredicateSQL(p.Right)
		if err != nil {
			return "", nil, err
		}
		cond := fmt.Sprintf("(%s OR %s)", leftCond, rightCond)
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, append(leftArgs, rightArgs...), nil

	case *GroupPredicate:
		var conditions []string
		var args []interface{}
		for _, subPred := range p.Predicates {
			cond, predArgs, err := e.buildElementPredicateSQL(subPred)
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

	default:
		return "", nil, fmt.Errorf("unsupported element predicate type: %T", pred)
	}
}

// buildElementEqualitySQL builds SQL for _ == value or _ != value.
func (e *Executor) buildElementEqualitySQL(p *ElementEqualityPredicate) (string, []interface{}, error) {
	var cond string
	var args []interface{}

	switch p.CompareOp {
	case CompareEq:
		// Case-insensitive equality
		cond = "LOWER(json_each.value) = LOWER(?)"
		args = append(args, p.Value)
	case CompareNeq:
		cond = "LOWER(json_each.value) != LOWER(?)"
		args = append(args, p.Value)
	case CompareLt:
		cond = "json_each.value < ?"
		args = append(args, p.Value)
	case CompareGt:
		cond = "json_each.value > ?"
		args = append(args, p.Value)
	case CompareLte:
		cond = "json_each.value <= ?"
		args = append(args, p.Value)
	case CompareGte:
		cond = "json_each.value >= ?"
		args = append(args, p.Value)
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, args, nil
}

// buildElementStringFuncSQL builds SQL for string functions on array elements.
func (e *Executor) buildElementStringFuncSQL(p *StringFuncPredicate) (string, []interface{}, error) {
	var cond string
	var args []interface{}

	wrapLower := !p.CaseSensitive

	switch p.FuncType {
	case StringFuncIncludes:
		if wrapLower {
			cond = "LOWER(json_each.value) LIKE LOWER(?)"
		} else {
			cond = "json_each.value LIKE ?"
		}
		args = append(args, "%"+p.Value+"%")

	case StringFuncStartsWith:
		if wrapLower {
			cond = "LOWER(json_each.value) LIKE LOWER(?)"
		} else {
			cond = "json_each.value LIKE ?"
		}
		args = append(args, p.Value+"%")

	case StringFuncEndsWith:
		if wrapLower {
			cond = "LOWER(json_each.value) LIKE LOWER(?)"
		} else {
			cond = "json_each.value LIKE ?"
		}
		args = append(args, "%"+p.Value)

	case StringFuncMatches:
		if wrapLower {
			cond = "json_each.value REGEXP ?"
			args = append(args, "(?i)"+p.Value)
		} else {
			cond = "json_each.value REGEXP ?"
			args = append(args, p.Value)
		}
	}

	if p.Negated() {
		cond = "NOT (" + cond + ")"
	}

	return cond, args, nil
}

// buildHasPredicateSQL builds SQL for has:{trait:...} predicates.
func (e *Executor) buildHasPredicateSQL(p *HasPredicate, alias string) (string, []interface{}, error) {
	// Handle bound trait ID (from has:_ in trait pipeline)
	if p.TraitID != "" {
		cond := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM traits
			WHERE parent_object_id = %s.id AND id = ?
		)`, alias)
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, []interface{}{p.TraitID}, nil
	}

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
	// Handle bound trait ID (from contains:_ in trait pipeline)
	if p.TraitID != "" {
		// Find objects that have this specific trait in their subtree
		cond := fmt.Sprintf(`EXISTS (
			WITH RECURSIVE subtree AS (
				SELECT id FROM objects WHERE id = %s.id
				UNION ALL
				SELECT o.id FROM objects o
				JOIN subtree s ON o.parent_id = s.id
			)
			SELECT 1 FROM traits t
			WHERE t.parent_object_id IN (SELECT id FROM subtree) AND t.id = ?
		)`, alias)
		if p.Negated() {
			cond = "NOT " + cond
		}
		return cond, []interface{}{p.TraitID}, nil
	}

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
		resolvedTarget := p.Target
		if resolved, err := e.resolveTarget(p.Target); err == nil && resolved != "" {
			resolvedTarget = resolved
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
		if wrapLower {
			cond = fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", fieldExpr)
		} else {
			cond = fmt.Sprintf("%s LIKE ?", fieldExpr)
		}
		args = append(args, "%"+p.Value+"%")

	case StringFuncStartsWith:
		if wrapLower {
			cond = fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", fieldExpr)
		} else {
			cond = fmt.Sprintf("%s LIKE ?", fieldExpr)
		}
		args = append(args, p.Value+"%")

	case StringFuncEndsWith:
		if wrapLower {
			cond = fmt.Sprintf("LOWER(%s) LIKE LOWER(?)", fieldExpr)
		} else {
			cond = fmt.Sprintf("%s LIKE ?", fieldExpr)
		}
		args = append(args, "%"+p.Value)

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
	case CompareNeq:
		cond = fmt.Sprintf("LOWER(%s.value) != LOWER(?)", alias)
		return cond, []interface{}{p.Value}, nil
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

// buildValueCondition builds a SQL condition for a ValuePredicate.
// This is a helper for use in subqueries where we don't have the full executor context.
func buildValueCondition(p *ValuePredicate, column string) (string, []interface{}) {
	var cond string

	switch p.CompareOp {
	case CompareLt:
		cond = fmt.Sprintf("%s < ?", column)
	case CompareGt:
		cond = fmt.Sprintf("%s > ?", column)
	case CompareLte:
		cond = fmt.Sprintf("%s <= ?", column)
	case CompareGte:
		cond = fmt.Sprintf("%s >= ?", column)
	case CompareNeq:
		cond = fmt.Sprintf("LOWER(%s) != LOWER(?)", column)
	default: // CompareEq
		if p.Negated() {
			cond = fmt.Sprintf("LOWER(%s) != LOWER(?)", column)
		} else {
			cond = fmt.Sprintf("LOWER(%s) = LOWER(?)", column)
		}
	}

	if p.CompareOp != CompareEq && p.Negated() {
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
		return e.executeObjectQuery(q)
	}
	return e.executeTraitQuery(q)
}

// bindSelfRefsForTrait creates a copy of the query with all IsSelfRef predicates
// bound to the given trait result.
//
// For a trait result, _ can be bound to:
// - at:_ → matches traits at the same file:line as this trait
// - on:_ → matches traits whose parent is this trait's parent (i.e., siblings)
// - within:_ → matches traits within this trait's parent object
// - refs:_ → matches things that reference this trait's parent object
// - refd:_ → matches things referenced by this trait's parent object
func (e *Executor) bindSelfRefsForTrait(q *Query, r TraitResult) *Query {
	boundQuery := &Query{
		Type:     q.Type,
		TypeName: q.TypeName,
	}

	for _, pred := range q.Predicates {
		boundQuery.Predicates = append(boundQuery.Predicates, e.bindPredicateForTrait(pred, r))
	}

	return boundQuery
}

// bindPredicateForTrait binds IsSelfRef in a predicate to a trait result.
func (e *Executor) bindPredicateForTrait(pred Predicate, r TraitResult) Predicate {
	switch p := pred.(type) {
	case *AtPredicate:
		if p.IsSelfRef {
			// at:_ for a trait means "same file and line as this trait"
			// We create a condition that matches the trait's location
			// Since we can't easily express file:line as a target, we use a special marker
			// that the SQL builder will handle
			return &AtPredicate{
				basePredicate: p.basePredicate,
				Target:        fmt.Sprintf("__selfref_trait:%s:%d", r.FilePath, r.Line),
			}
		}
		if p.SubQuery != nil {
			return &AtPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForTrait(p.SubQuery, r),
			}
		}
	case *OnPredicate:
		if p.IsSelfRef {
			// on:_ for a trait means "directly on this trait's parent object"
			return &OnPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ParentObjectID,
			}
		}
		if p.SubQuery != nil {
			return &OnPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForObject(p.SubQuery, ObjectResult{ID: r.ParentObjectID}),
			}
		}
	case *WithinPredicate:
		if p.IsSelfRef {
			// within:_ for a trait means "within this trait's parent object"
			return &WithinPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ParentObjectID,
			}
		}
		if p.SubQuery != nil {
			return &WithinPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForObject(p.SubQuery, ObjectResult{ID: r.ParentObjectID}),
			}
		}
	case *RefsPredicate:
		if p.IsSelfRef {
			// refs:_ for a trait means "references this trait's parent object"
			return &RefsPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ParentObjectID,
			}
		}
		if p.SubQuery != nil {
			return &RefsPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForObject(p.SubQuery, ObjectResult{ID: r.ParentObjectID}),
			}
		}
	case *RefdPredicate:
		if p.IsSelfRef {
			// refd:_ for a trait means "referenced by this trait's parent object"
			return &RefdPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ParentObjectID,
			}
		}
		if p.SubQuery != nil {
			boundSubQ := p.SubQuery
			if p.SubQuery.Type == QueryTypeObject {
				boundSubQ = e.bindSelfRefsForObject(p.SubQuery, ObjectResult{ID: r.ParentObjectID})
			} else {
				boundSubQ = e.bindSelfRefsForTrait(p.SubQuery, r)
			}
			return &RefdPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      boundSubQ,
			}
		}
	case *OrPredicate:
		return &OrPredicate{
			basePredicate: p.basePredicate,
			Left:          e.bindPredicateForTrait(p.Left, r),
			Right:         e.bindPredicateForTrait(p.Right, r),
		}
	case *GroupPredicate:
		boundPreds := make([]Predicate, len(p.Predicates))
		for i, subPred := range p.Predicates {
			boundPreds[i] = e.bindPredicateForTrait(subPred, r)
		}
		return &GroupPredicate{
			basePredicate: p.basePredicate,
			Predicates:    boundPreds,
		}
	}
	return pred
}

// bindSelfRefsForObject creates a copy of the query with all IsSelfRef predicates
// bound to the given object result.
//
// For an object result, _ can be bound to:
// - on:_ → matches traits directly on this object
// - within:_ → matches traits within this object (self or descendants)
// - parent:_ → matches objects whose parent is this object
// - ancestor:_ → matches objects where this object is an ancestor
// - child:_ → matches objects that are children of this object
// - descendant:_ → matches objects that are descendants of this object
// - refs:_ → matches things that reference this object
// - refd:_ → matches things that this object references
func (e *Executor) bindSelfRefsForObject(q *Query, r ObjectResult) *Query {
	boundQuery := &Query{
		Type:     q.Type,
		TypeName: q.TypeName,
	}

	for _, pred := range q.Predicates {
		boundQuery.Predicates = append(boundQuery.Predicates, e.bindPredicateForObject(pred, r))
	}

	return boundQuery
}

// bindPredicateForObject binds IsSelfRef in a predicate to an object result.
func (e *Executor) bindPredicateForObject(pred Predicate, r ObjectResult) Predicate {
	switch p := pred.(type) {
	case *OnPredicate:
		if p.IsSelfRef {
			// on:_ for an object means "directly on this object"
			return &OnPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ID,
			}
		}
		if p.SubQuery != nil {
			return &OnPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForObject(p.SubQuery, r),
			}
		}
	case *WithinPredicate:
		if p.IsSelfRef {
			// within:_ for an object means "within this object"
			return &WithinPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ID,
			}
		}
		if p.SubQuery != nil {
			return &WithinPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForObject(p.SubQuery, r),
			}
		}
	case *ParentPredicate:
		if p.IsSelfRef {
			// parent:_ means "whose parent is this object"
			return &ParentPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ID,
			}
		}
		if p.SubQuery != nil {
			return &ParentPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForObject(p.SubQuery, r),
			}
		}
	case *AncestorPredicate:
		if p.IsSelfRef {
			// ancestor:_ means "this object is an ancestor"
			return &AncestorPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ID,
			}
		}
		if p.SubQuery != nil {
			return &AncestorPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForObject(p.SubQuery, r),
			}
		}
	case *ChildPredicate:
		if p.IsSelfRef {
			// child:_ means "this object is a child"
			return &ChildPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ID,
			}
		}
		if p.SubQuery != nil {
			return &ChildPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForObject(p.SubQuery, r),
			}
		}
	case *DescendantPredicate:
		if p.IsSelfRef {
			// descendant:_ means "this object is a descendant"
			return &DescendantPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ID,
			}
		}
		if p.SubQuery != nil {
			return &DescendantPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForObject(p.SubQuery, r),
			}
		}
	case *RefsPredicate:
		if p.IsSelfRef {
			// refs:_ means "references this object"
			return &RefsPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ID,
			}
		}
		if p.SubQuery != nil {
			return &RefsPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForObject(p.SubQuery, r),
			}
		}
	case *RefdPredicate:
		if p.IsSelfRef {
			// refd:_ means "referenced by this object"
			return &RefdPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ID,
			}
		}
		if p.SubQuery != nil {
			boundSubQ := p.SubQuery
			if p.SubQuery.Type == QueryTypeObject {
				boundSubQ = e.bindSelfRefsForObject(p.SubQuery, r)
			}
			return &RefdPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      boundSubQ,
			}
		}
	case *HasPredicate:
		if p.SubQuery != nil {
			// For has:, we need to pass through the binding to trait predicates
			return &HasPredicate{
				basePredicate: p.basePredicate,
				SubQuery:      e.bindSelfRefsForObjectInTraitQuery(p.SubQuery, r),
			}
		}
	case *ContainsPredicate:
		if p.SubQuery != nil {
			return &ContainsPredicate{
				basePredicate: p.basePredicate,
				SubQuery:      e.bindSelfRefsForObjectInTraitQuery(p.SubQuery, r),
			}
		}
	case *OrPredicate:
		return &OrPredicate{
			basePredicate: p.basePredicate,
			Left:          e.bindPredicateForObject(p.Left, r),
			Right:         e.bindPredicateForObject(p.Right, r),
		}
	case *GroupPredicate:
		boundPreds := make([]Predicate, len(p.Predicates))
		for i, subPred := range p.Predicates {
			boundPreds[i] = e.bindPredicateForObject(subPred, r)
		}
		return &GroupPredicate{
			basePredicate: p.basePredicate,
			Predicates:    boundPreds,
		}
	}
	return pred
}

// bindSelfRefsForObjectInTraitQuery binds _ refs in a trait query to an object.
// This is used when we have a trait subquery inside an object query (e.g., has:{trait:due within:_}).
func (e *Executor) bindSelfRefsForObjectInTraitQuery(q *Query, r ObjectResult) *Query {
	boundQuery := &Query{
		Type:     q.Type,
		TypeName: q.TypeName,
	}

	for _, pred := range q.Predicates {
		boundQuery.Predicates = append(boundQuery.Predicates, e.bindTraitPredicateForObject(pred, r))
	}

	return boundQuery
}

// bindTraitPredicateForObject binds IsSelfRef in trait predicates to an object result.
func (e *Executor) bindTraitPredicateForObject(pred Predicate, r ObjectResult) Predicate {
	switch p := pred.(type) {
	case *OnPredicate:
		if p.IsSelfRef {
			return &OnPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ID,
			}
		}
		if p.SubQuery != nil {
			return &OnPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForObject(p.SubQuery, r),
			}
		}
	case *WithinPredicate:
		if p.IsSelfRef {
			return &WithinPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ID,
			}
		}
		if p.SubQuery != nil {
			return &WithinPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForObject(p.SubQuery, r),
			}
		}
	case *RefsPredicate:
		if p.IsSelfRef {
			return &RefsPredicate{
				basePredicate: p.basePredicate,
				Target:        r.ID,
			}
		}
		if p.SubQuery != nil {
			return &RefsPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForObject(p.SubQuery, r),
			}
		}
	case *AtPredicate:
		if p.SubQuery != nil {
			return &AtPredicate{
				basePredicate: p.basePredicate,
				Target:        p.Target,
				SubQuery:      e.bindSelfRefsForObjectInTraitQuery(p.SubQuery, r),
			}
		}
	case *OrPredicate:
		return &OrPredicate{
			basePredicate: p.basePredicate,
			Left:          e.bindTraitPredicateForObject(p.Left, r),
			Right:         e.bindTraitPredicateForObject(p.Right, r),
		}
	case *GroupPredicate:
		boundPreds := make([]Predicate, len(p.Predicates))
		for i, subPred := range p.Predicates {
			boundPreds[i] = e.bindTraitPredicateForObject(subPred, r)
		}
		return &GroupPredicate{
			basePredicate: p.basePredicate,
			Predicates:    boundPreds,
		}
	}
	return pred
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

