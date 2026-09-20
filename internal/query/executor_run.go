package query

import "fmt"

// Execute parses a query string and returns the matching rows. It is a thin
// helper over Parse + Run for callers that already have a query string and
// want the typed row slice rather than a RunResult.
func (e *Executor) Execute(queryStr string) (interface{}, error) {
	q, err := Parse(queryStr)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	result, err := e.Run(q, RunRequest{})
	if err != nil {
		return nil, err
	}
	switch q.Type {
	case QueryTypeObject:
		return result.Objects, nil
	case QueryTypeTrait:
		return result.Traits, nil
	case QueryTypeSection:
		return result.Sections, nil
	case QueryTypeLink:
		return result.Links, nil
	default:
		return nil, fmt.Errorf("unsupported query type: %d", q.Type)
	}
}
