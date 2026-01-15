package query

import "fmt"

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
