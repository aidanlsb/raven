package query

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/model"
)

// PipelineObjectResult represents an object with computed values from pipeline.
type PipelineObjectResult struct {
	model.Object
	Computed map[string]interface{} // Computed values from assignments
}

// PipelineTraitResult represents a trait with computed values from pipeline.
type PipelineTraitResult struct {
	model.Trait
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
			results[i] = PipelineObjectResult{Object: r, Computed: make(map[string]interface{})}
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
			results[i] = PipelineTraitResult{Trait: r, Computed: make(map[string]interface{})}
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
func (e *Executor) executePipelineForObjects(results []model.Object, pipeline *Pipeline) ([]PipelineObjectResult, error) {
	// Initialize results with computed maps
	pResults := make([]PipelineObjectResult, len(results))
	for i, r := range results {
		pResults[i] = PipelineObjectResult{Object: r, Computed: make(map[string]interface{})}
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
func (e *Executor) executePipelineForTraits(results []model.Trait, pipeline *Pipeline) ([]PipelineTraitResult, error) {
	// Initialize results with computed maps
	pResults := make([]PipelineTraitResult, len(results))
	for i, r := range results {
		pResults[i] = PipelineTraitResult{Trait: r, Computed: make(map[string]interface{})}
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
