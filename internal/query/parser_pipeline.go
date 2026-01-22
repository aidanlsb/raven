package query

import (
	"fmt"
	"strconv"
	"strings"
)

// parsePipeline parses the stages after |>
// Stages can be: assignments, filter(), sort(), limit()
func (p *Parser) parsePipeline() (*Pipeline, error) {
	pipeline := &Pipeline{}

	for {
		// Stop at EOF, closing braces, or closing parens
		if p.curr.Type == TokenEOF || p.curr.Type == TokenRBrace || p.curr.Type == TokenRParen {
			break
		}

		stage, err := p.parsePipelineStage()
		if err != nil {
			return nil, err
		}
		if stage == nil {
			break
		}
		pipeline.Stages = append(pipeline.Stages, stage)
	}

	return pipeline, nil
}

// parsePipelineStage parses a single pipeline stage
func (p *Parser) parsePipelineStage() (PipelineStage, error) {
	if p.curr.Type != TokenIdent {
		return nil, nil
	}

	name := p.curr.Value

	// Check if this is an assignment (name = ...)
	if p.peek.Type == TokenEq {
		return p.parseAssignment()
	}

	// Otherwise it's a function call
	switch strings.ToLower(name) {
	case "filter":
		return p.parseFilterStage()
	case "sort":
		return p.parseSortStage()
	case "limit":
		return p.parseLimitStage()
	default:
		return nil, fmt.Errorf("unknown pipeline function: %s", name)
	}
}

// parseAssignment parses:
//   - name = count({subquery})
//   - name = count(navfunc(_))
//   - name = min(.value, {trait:...})   -- min of trait values (requires .value)
//   - name = min(.field, {object:...})  -- min of field values on objects
//   - name = max(.value, {trait:...})   -- max of trait values (requires .value)
//   - name = max(.field, {object:...})  -- max of field values on objects
//   - name = sum(.value, {trait:...})   -- sum of trait values (requires .value)
//   - name = sum(.field, {object:...})  -- sum of field values on objects
func (p *Parser) parseAssignment() (PipelineStage, error) {
	name := p.curr.Value
	p.advance() // consume name
	p.advance() // consume =

	// Expect aggregation function
	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected aggregation function (count, min, max, sum)")
	}

	var aggType AggregationType
	aggName := strings.ToLower(p.curr.Value)
	switch aggName {
	case "count":
		aggType = AggCount
	case "min":
		aggType = AggMin
	case "max":
		aggType = AggMax
	case "sum":
		aggType = AggSum
	default:
		return nil, fmt.Errorf("unknown aggregation function: %s", p.curr.Value)
	}
	p.advance()

	// Expect (
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}

	stage := &AssignmentStage{
		Name:        name,
		Aggregation: aggType,
	}

	// Check for field argument first: min(.field, {subquery})
	if p.curr.Type == TokenDot {
		p.advance()
		if p.curr.Type != TokenIdent {
			return nil, fmt.Errorf("expected field name after '.'")
		}
		stage.AggField = p.curr.Value
		p.advance()

		// Expect comma before subquery
		if err := p.expect(TokenComma); err != nil {
			return nil, fmt.Errorf("expected ',' after field in %s(.field, {subquery})", aggName)
		}
	}

	// Parse argument - either a subquery or a navigation function
	if p.curr.Type == TokenLBrace {
		// Subquery
		p.advance()
		subQuery, err := p.parseQuery()
		if err != nil {
			return nil, fmt.Errorf("in aggregation subquery: %w", err)
		}
		if err := p.expect(TokenRBrace); err != nil {
			return nil, err
		}
		stage.SubQuery = subQuery
	} else if p.curr.Type == TokenIdent {
		// Navigation function like refs(_), refd(_), ancestors(_), descendants(_)
		navName := p.curr.Value
		p.advance()

		// Expect (_)
		if err := p.expect(TokenLParen); err != nil {
			return nil, fmt.Errorf("expected '(' after navigation function %s", navName)
		}
		if err := p.expect(TokenUnderscore); err != nil {
			return nil, fmt.Errorf("expected '_' in navigation function")
		}
		if err := p.expect(TokenRParen); err != nil {
			return nil, fmt.Errorf("expected ')' after navigation function argument")
		}

		stage.NavFunc = &NavFunc{Name: navName}
	} else {
		return nil, fmt.Errorf("expected {subquery} or navigation function in aggregation")
	}

	// Expect closing )
	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	return stage, nil
}

// parseFilterStage parses: filter(expr)
func (p *Parser) parseFilterStage() (PipelineStage, error) {
	p.advance() // consume 'filter'

	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}

	expr, err := p.parseFilterExpr()
	if err != nil {
		return nil, err
	}

	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	return &FilterStage{Expr: expr}, nil
}

// parseFilterExpr parses: variable > value, .field == value, etc.
func (p *Parser) parseFilterExpr() (*FilterExpr, error) {
	expr := &FilterExpr{}

	// Left side: variable name or .field
	if p.curr.Type == TokenDot {
		p.advance()
		if p.curr.Type != TokenIdent {
			return nil, fmt.Errorf("expected field name after '.'")
		}
		expr.Left = p.curr.Value
		expr.IsField = true
		p.advance()
	} else if p.curr.Type == TokenIdent {
		expr.Left = p.curr.Value
		p.advance()
	} else {
		return nil, fmt.Errorf("expected variable name or .field in filter expression")
	}

	// Operator
	switch p.curr.Type {
	case TokenEqEq:
		expr.Op = CompareEq
	case TokenBangEq:
		expr.Op = CompareNeq
	case TokenLt:
		expr.Op = CompareLt
	case TokenGt:
		expr.Op = CompareGt
	case TokenLte:
		expr.Op = CompareLte
	case TokenGte:
		expr.Op = CompareGte
	default:
		return nil, fmt.Errorf("expected comparison operator in filter expression")
	}
	p.advance()

	// Right side: value
	switch p.curr.Type {
	case TokenIdent:
		expr.Right = p.curr.Value
	case TokenString:
		expr.Right = p.curr.Value
	default:
		return nil, fmt.Errorf("expected value in filter expression")
	}
	p.advance()

	return expr, nil
}

// parseSortStage parses: sort(field), sort(field, asc), or sort(field, desc)
// Direction defaults to ascending if not specified.
func (p *Parser) parseSortStage() (PipelineStage, error) {
	p.advance() // consume 'sort'

	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}

	criterion := SortCriterion{}

	// Field or variable name
	if p.curr.Type == TokenDot {
		p.advance()
		if p.curr.Type != TokenIdent {
			return nil, fmt.Errorf("expected field name after '.'")
		}
		criterion.Field = p.curr.Value
		criterion.IsField = true
		p.advance()
	} else if p.curr.Type == TokenIdent {
		criterion.Field = p.curr.Value
		p.advance()
	} else {
		return nil, fmt.Errorf("expected field or variable name in sort")
	}

	// Optional: comma and direction (defaults to ascending)
	if p.curr.Type == TokenComma {
		p.advance()

		if p.curr.Type != TokenIdent {
			return nil, fmt.Errorf("expected 'asc' or 'desc'")
		}
		switch strings.ToLower(p.curr.Value) {
		case "asc":
			criterion.Descending = false
		case "desc":
			criterion.Descending = true
		default:
			return nil, fmt.Errorf("expected 'asc' or 'desc', got '%s'", p.curr.Value)
		}
		p.advance()
	}
	// If no comma, default is ascending (Descending = false, which is the zero value)

	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	return &SortStage{Criteria: []SortCriterion{criterion}}, nil
}

// parseLimitStage parses: limit(n)
func (p *Parser) parseLimitStage() (PipelineStage, error) {
	p.advance() // consume 'limit'

	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}

	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected number in limit()")
	}

	n, err := strconv.Atoi(p.curr.Value)
	if err != nil {
		return nil, fmt.Errorf("invalid limit value '%s': must be a positive integer", p.curr.Value)
	}
	if n <= 0 {
		return nil, fmt.Errorf("limit must be a positive integer, got %d", n)
	}
	p.advance()

	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	return &LimitStage{N: n}, nil
}
