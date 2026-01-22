package query

import "fmt"

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
		return e.aggregateTraitResults(results, agg, field)
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
		return e.aggregateTraitResults(results, agg, field)
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
// For min/max/sum, the field must be specified (use ".value" for trait values).
func (e *Executor) aggregateTraitResults(results []TraitResult, agg AggregationType, field string) (interface{}, error) {
	switch agg {
	case AggCount:
		return len(results), nil
	case AggMin:
		if field == "" {
			return nil, fmt.Errorf("min() on trait query requires a field: use min(.value, {trait:...})")
		}
		if field != "value" {
			return nil, fmt.Errorf("min() on trait query only supports .value field, got .%s", field)
		}
		if len(results) == 0 {
			return nil, nil
		}
		var minVal *string
		for i := range results {
			if results[i].Value == nil {
				continue
			}
			if minVal == nil || compareValues(*results[i].Value, *minVal) < 0 {
				minVal = results[i].Value
			}
		}
		return minVal, nil
	case AggMax:
		if field == "" {
			return nil, fmt.Errorf("max() on trait query requires a field: use max(.value, {trait:...})")
		}
		if field != "value" {
			return nil, fmt.Errorf("max() on trait query only supports .value field, got .%s", field)
		}
		if len(results) == 0 {
			return nil, nil
		}
		var maxVal *string
		for i := range results {
			if results[i].Value == nil {
				continue
			}
			if maxVal == nil || compareValues(*results[i].Value, *maxVal) > 0 {
				maxVal = results[i].Value
			}
		}
		return maxVal, nil
	case AggSum:
		if field == "" {
			return nil, fmt.Errorf("sum() on trait query requires a field: use sum(.value, {trait:...})")
		}
		if field != "value" {
			return nil, fmt.Errorf("sum() on trait query only supports .value field, got .%s", field)
		}
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
