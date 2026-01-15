package query

import (
	"sort"
	"strings"
)

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

	// Compare using the shared comparison engine so filter() matches sort/min/max behavior.
	// Right side is always a literal string from the query language.
	cmp := compareValues(leftVal, strings.TrimSpace(expr.Right))

	switch expr.Op {
	case CompareEq:
		return cmp == 0, nil
	case CompareNeq:
		return cmp != 0, nil
	case CompareLt:
		return cmp < 0, nil
	case CompareGt:
		return cmp > 0, nil
	case CompareLte:
		return cmp <= 0, nil
	case CompareGte:
		return cmp >= 0, nil
	default:
		return false, nil
	}
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
					if results[i].Value != nil {
						iVal = *results[i].Value
					}
					if results[j].Value != nil {
						jVal = *results[j].Value
					}
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
