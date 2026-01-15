package query

import "fmt"

// parseSubQuery parses a full subquery ({object:type ...} or {trait:name ...})
// v2: No shorthand expansion - explicit subqueries required
func (p *Parser) parseSubQuery(expectedType QueryType, expectedKind string) (*Query, error) {
	// Full subquery in braces required
	if p.curr.Type != TokenLBrace {
		return nil, fmt.Errorf("expected '{' for %s subquery (shorthand syntax removed in v2)", expectedKind)
	}

	p.advance()

	// Parse the inner query
	subQuery, err := p.parseQuery()
	if err != nil {
		return nil, fmt.Errorf("in subquery: %w", err)
	}

	if err := p.expect(TokenRBrace); err != nil {
		return nil, fmt.Errorf("unclosed subquery brace: %w", err)
	}

	// Validate query type matches expectation
	if subQuery.Type != expectedType {
		return nil, fmt.Errorf("expected %s subquery, got %s", expectedKind,
			map[QueryType]string{QueryTypeObject: "object", QueryTypeTrait: "trait"}[subQuery.Type])
	}

	return subQuery, nil
}
