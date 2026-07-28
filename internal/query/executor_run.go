package query

import "fmt"

// Execute parses and executes a query string, returning rows for its query root.
func (e *Executor) Execute(queryStr string) (interface{}, error) {
	scoped := e.withExecutionNow()

	q, err := Parse(queryStr)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	switch q.Type {
	case QueryTypeObject:
		return scoped.executeObjectQuery(q)
	case QueryTypeTrait:
		return scoped.executeTraitQuery(q)
	case QueryTypeSection:
		return scoped.executeSectionQuery(q)
	case QueryTypeLink:
		return scoped.executeLinkQuery(q)
	default:
		return nil, fmt.Errorf("unsupported query type: %d", q.Type)
	}
}
