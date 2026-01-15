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

// parsePredicates parses a sequence of predicates.
func (p *Parser) parsePredicates(qt QueryType) ([]Predicate, error) {
	var predicates []Predicate

	for {
		// Stop at EOF, closing braces, closing parens, or pipeline
		if p.curr.Type == TokenEOF || p.curr.Type == TokenRBrace || p.curr.Type == TokenRParen || p.curr.Type == TokenPipeline {
			break
		}

		pred, err := p.parsePredicate(qt)
		if err != nil {
			return nil, err
		}
		if pred == nil {
			break
		}

		// Check for OR operator
		if p.curr.Type == TokenPipe {
			p.advance()
			right, err := p.parsePredicate(qt)
			if err != nil {
				return nil, err
			}
			pred = &OrPredicate{Left: pred, Right: right}
		}

		predicates = append(predicates, pred)
	}

	return predicates, nil
}

// parsePredicate parses a single predicate.
func (p *Parser) parsePredicate(qt QueryType) (Predicate, error) {
	// Check for negation
	negated := false
	if p.curr.Type == TokenBang {
		negated = true
		p.advance()
	}

	// Check for grouping parentheses
	if p.curr.Type == TokenLParen {
		p.advance()
		preds, err := p.parsePredicates(qt)
		if err != nil {
			return nil, err
		}
		if err := p.expect(TokenRParen); err != nil {
			return nil, fmt.Errorf("unclosed parenthesis: %w", err)
		}
		return &GroupPredicate{basePredicate: basePredicate{negated: negated}, Predicates: preds}, nil
	}

	// Field predicate (starts with .)
	if p.curr.Type == TokenDot {
		p.advance()
		return p.parseFieldPredicate(negated)
	}

	// Keyword predicate or function call
	if p.curr.Type == TokenIdent {
		keyword := strings.ToLower(p.curr.Value)

		// v2: 'value' uses operators directly (==, !=, <, >, etc.) instead of :
		if keyword == "value" {
			p.advance()
			return p.parseValuePredicate(negated)
		}

		// Check for function-style predicates: func(...)
		// String functions: includes, startswith, endswith, matches
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
		case "source":
			return p.parseSourcePredicate(negated)
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
