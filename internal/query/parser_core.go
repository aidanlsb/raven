package query

import (
	"fmt"
	"strings"
)

// Parser parses query strings into Query ASTs.
type Parser struct {
	lexer *Lexer
	curr  Token
	peek  Token
}

// Parse parses a query string and returns a Query AST.
func Parse(input string) (*Query, error) {
	p := &Parser{lexer: NewLexer(input)}
	p.advance()
	p.advance()
	return p.parseQuery()
}

func (p *Parser) advance() {
	p.curr = p.peek
	p.peek = p.lexer.NextToken()
}

func (p *Parser) expect(t TokenType) error {
	if p.curr.Type != t {
		return fmt.Errorf("expected %v, got %v at pos %d", t, p.curr.Type, p.curr.Pos)
	}
	p.advance()
	return nil
}

// parseQuery parses a top-level query (object:type or trait:name).
func (p *Parser) parseQuery() (*Query, error) {
	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected 'object' or 'trait', got %v", p.curr.Value)
	}

	queryKind := strings.ToLower(p.curr.Value)
	p.advance()

	if err := p.expect(TokenColon); err != nil {
		return nil, err
	}

	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected type/trait name, got %v", p.curr.Value)
	}

	typeName := p.curr.Value
	p.advance()

	var query Query
	switch queryKind {
	case "object":
		query.Type = QueryTypeObject
	case "trait":
		query.Type = QueryTypeTrait
	default:
		return nil, fmt.Errorf("invalid query type: %s (expected 'object' or 'trait')", queryKind)
	}
	query.TypeName = typeName

	// Parse predicates
	predicates, err := p.parsePredicates(query.Type)
	if err != nil {
		return nil, err
	}
	query.Predicates = predicates

	// Check for pipeline operator |>
	if p.curr.Type == TokenPipeline {
		p.advance()
		pipeline, err := p.parsePipeline()
		if err != nil {
			return nil, err
		}
		query.Pipeline = pipeline
	}

	return &query, nil
}

// parsePredicates parses a boolean expression of predicates.
func (p *Parser) parsePredicates(qt QueryType) ([]Predicate, error) {
	pred, err := p.parseOrPredicate(qt)
	if err != nil {
		return nil, err
	}
	if pred == nil {
		return nil, nil
	}
	return []Predicate{pred}, nil
}

// parseOrPredicate parses OR expressions (lowest precedence).
func (p *Parser) parseOrPredicate(qt QueryType) (Predicate, error) {
	left, err := p.parseAndPredicate(qt)
	if err != nil {
		return nil, err
	}
	if left == nil {
		return nil, nil
	}

	for p.curr.Type == TokenPipe {
		p.advance()
		right, err := p.parseAndPredicate(qt)
		if err != nil {
			return nil, err
		}
		if right == nil {
			return nil, fmt.Errorf("expected predicate after '|'")
		}
		left = &OrPredicate{Left: left, Right: right}
	}

	return left, nil
}

// parseAndPredicate parses implicit AND expressions (middle precedence).
func (p *Parser) parseAndPredicate(qt QueryType) (Predicate, error) {
	var preds []Predicate

	for {
		// Stop at EOF, closing braces, closing parens, pipeline, or OR operator
		if p.curr.Type == TokenEOF || p.curr.Type == TokenRBrace || p.curr.Type == TokenRParen || p.curr.Type == TokenPipeline || p.curr.Type == TokenPipe {
			break
		}

		pred, err := p.parseUnaryPredicate(qt)
		if err != nil {
			return nil, err
		}
		if pred == nil {
			if p.curr.Type == TokenEOF || p.curr.Type == TokenRBrace || p.curr.Type == TokenRParen || p.curr.Type == TokenPipeline || p.curr.Type == TokenPipe {
				break
			}
			return nil, fmt.Errorf("unexpected token %v at pos %d", p.curr.Type, p.curr.Pos)
		}
		preds = append(preds, pred)
	}

	if len(preds) == 0 {
		return nil, nil
	}
	if len(preds) == 1 {
		return preds[0], nil
	}
	return &GroupPredicate{Predicates: preds}, nil
}

// parseUnaryPredicate parses NOT and grouped predicates (highest precedence).
func (p *Parser) parseUnaryPredicate(qt QueryType) (Predicate, error) {
	// Check for negation
	negated := false
	if p.curr.Type == TokenBang {
		negated = true
		p.advance()
	}

	// Check for grouping parentheses
	if p.curr.Type == TokenLParen {
		p.advance()
		pred, err := p.parseOrPredicate(qt)
		if err != nil {
			return nil, err
		}
		if pred == nil {
			return nil, fmt.Errorf("expected predicate inside parentheses")
		}
		if err := p.expect(TokenRParen); err != nil {
			return nil, fmt.Errorf("unclosed parenthesis: %w", err)
		}
		if !negated {
			return pred, nil
		}
		return &GroupPredicate{basePredicate: basePredicate{negated: true}, Predicates: []Predicate{pred}}, nil
	}

	return p.parseAtomicPredicate(qt, negated)
}

// parseAtomicPredicate parses a single predicate without boolean composition.
func (p *Parser) parseAtomicPredicate(qt QueryType, negated bool) (Predicate, error) {
	// Field predicate (starts with .)
	if p.curr.Type == TokenDot {
		p.advance()
		return p.parseFieldPredicate(negated)
	}

	// Keyword predicate or function call
	if p.curr.Type == TokenIdent {
		keyword := strings.ToLower(p.curr.Value)

		// Check for function-style predicates: func(...)
		// String functions: includes, startswith, endswith, matches
		// Null checks: isnull, notnull
		// Array quantifiers: any, all, none
		if p.peek.Type == TokenLParen {
			switch keyword {
			case "includes":
				p.advance() // consume function name
				return p.parseStringFuncPredicate(negated, StringFuncIncludes)
			case "startswith":
				p.advance()
				return p.parseStringFuncPredicate(negated, StringFuncStartsWith)
			case "endswith":
				p.advance()
				return p.parseStringFuncPredicate(negated, StringFuncEndsWith)
			case "matches":
				p.advance()
				return p.parseStringFuncPredicate(negated, StringFuncMatches)
			case "isnull":
				p.advance()
				return p.parseNullCheckPredicate(negated, true) // isnull = field IS null
			case "notnull":
				p.advance()
				return p.parseNullCheckPredicate(negated, false) // notnull = field IS NOT null
			case "any":
				p.advance()
				return p.parseArrayQuantifierPredicate(negated, ArrayQuantifierAny)
			case "all":
				p.advance()
				return p.parseArrayQuantifierPredicate(negated, ArrayQuantifierAll)
			case "none":
				p.advance()
				return p.parseArrayQuantifierPredicate(negated, ArrayQuantifierNone)
			}
		}

		p.advance()

		if p.curr.Type != TokenColon {
			return nil, fmt.Errorf("expected ':' after %s", keyword)
		}
		p.advance()

		switch keyword {
		// Object predicates
		case "has":
			return p.parseHasPredicate(negated)
		case "parent":
			return p.parseParentPredicate(negated)
		case "ancestor":
			return p.parseAncestorPredicate(negated)
		case "child":
			return p.parseChildPredicate(negated)
		case "descendant":
			return p.parseDescendantPredicate(negated)
		case "contains":
			return p.parseContainsPredicate(negated)
		case "refs":
			return p.parseRefsPredicate(negated)
		case "content":
			return p.parseContentPredicate(negated)
		// Trait predicates
		case "on":
			return p.parseOnPredicate(negated)
		case "within":
			return p.parseWithinPredicate(negated)
		case "at":
			return p.parseAtPredicate(negated)
		case "refd":
			return p.parseRefdPredicate(negated)
		default:
			return nil, fmt.Errorf("unknown predicate: %s", keyword)
		}
	}

	return nil, nil
}
