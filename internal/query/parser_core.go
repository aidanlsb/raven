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

	// Pipeline (|>) was removed from the query language.
	if p.curr.Type == TokenPipeline {
		return nil, fmt.Errorf("pipeline operator '|>' is no longer supported")
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
		// Stop at EOF, closing parens, or OR operator
		if p.curr.Type == TokenEOF || p.curr.Type == TokenRParen || p.curr.Type == TokenPipe {
			break
		}

		pred, err := p.parseUnaryPredicate(qt)
		if err != nil {
			return nil, err
		}
		if pred == nil {
			if p.curr.Type == TokenEOF || p.curr.Type == TokenRParen || p.curr.Type == TokenPipe {
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
	// Brace subqueries were removed from the core syntax (v3).
	// Braces are not valid predicate tokens anymore.
	if p.curr.Type == TokenLBrace {
		return nil, fmt.Errorf("brace subqueries are no longer supported; write nested queries directly (e.g., has(trait:due .value==past))")
	}

	// Field predicate (starts with .)
	if p.curr.Type == TokenDot {
		p.advance()
		return p.parseFieldPredicate(negated)
	}

	// Keyword predicate or function call
	if p.curr.Type == TokenIdent {
		keyword := strings.ToLower(p.curr.Value)

		// Function-style predicates: func(...)
		// v3: all structural predicates are functions (no keyword: forms).
		if p.peek.Type == TokenLParen {
			switch keyword {
			// String functions
			case "contains":
				p.advance() // consume function name
				return p.parseStringFuncPredicate(negated, StringFuncIncludes)
			case "includes":
				return nil, fmt.Errorf("includes() is no longer supported; use contains()")
			case "startswith":
				p.advance()
				return p.parseStringFuncPredicate(negated, StringFuncStartsWith)
			case "endswith":
				p.advance()
				return p.parseStringFuncPredicate(negated, StringFuncEndsWith)
			case "matches":
				p.advance()
				return p.parseStringFuncPredicate(negated, StringFuncMatches)
			// Existence (v3: prefer exists() + !exists())
			case "exists":
				p.advance()
				return p.parseExistsPredicate(negated)
			case "notnull":
				return nil, fmt.Errorf("notnull() is no longer supported; use exists(.field)")
			case "isnull":
				return nil, fmt.Errorf("isnull() is no longer supported; use !exists(.field)")
			// Content search (v3: function form only)
			case "content":
				p.advance()
				return p.parseContentFuncPredicate(negated)
			// Scalar membership + array quantifiers
			case "in":
				p.advance()
				return p.parseInPredicate(negated)
			case "any":
				p.advance()
				return p.parseArrayQuantifierPredicate(negated, ArrayQuantifierAny)
			case "all":
				p.advance()
				return p.parseArrayQuantifierPredicate(negated, ArrayQuantifierAll)
			case "none":
				p.advance()
				return p.parseArrayQuantifierPredicate(negated, ArrayQuantifierNone)
			// Structural predicates (v3)
			case "has":
				p.advance()
				return p.parseHasFuncPredicate(negated)
			case "encloses":
				p.advance()
				return p.parseEnclosesFuncPredicate(negated)
			case "parent":
				p.advance()
				return p.parseObjectNavFuncPredicate(negated, "parent")
			case "ancestor":
				p.advance()
				return p.parseObjectNavFuncPredicate(negated, "ancestor")
			case "child":
				p.advance()
				return p.parseObjectNavFuncPredicate(negated, "child")
			case "descendant":
				p.advance()
				return p.parseObjectNavFuncPredicate(negated, "descendant")
			case "on":
				p.advance()
				return p.parseTraitNavFuncPredicate(negated, "on")
			case "within":
				p.advance()
				return p.parseTraitNavFuncPredicate(negated, "within")
			case "refs":
				p.advance()
				return p.parseRefsFuncPredicate(negated)
			case "refd":
				p.advance()
				return p.parseRefdFuncPredicate(negated)
			case "at":
				p.advance()
				return p.parseAtFuncPredicate(negated)
			}
		}

		// Common legacy error: keyword:predicate syntax
		if p.peek.Type == TokenColon {
			switch keyword {
			case "content":
				return nil, fmt.Errorf(`content:"..." is no longer supported; use content("...")`)
			case "has":
				return nil, fmt.Errorf("has:{...} is no longer supported; use has(trait:...)")
			case "contains":
				return nil, fmt.Errorf("contains:{...} is no longer supported; use encloses(trait:...)")
			case "refs":
				return nil, fmt.Errorf("refs:... is no longer supported; use refs([[target]]) or refs(object:...)")
			case "refd":
				return nil, fmt.Errorf("refd:... is no longer supported; use refd([[source]]) or refd(object:...)")
			case "on":
				return nil, fmt.Errorf("on:{...} is no longer supported; use on(object:...)")
			case "within":
				return nil, fmt.Errorf("within:{...} is no longer supported; use within(object:...)")
			case "at":
				return nil, fmt.Errorf("at:{...} is no longer supported; use at(trait:...)")
			default:
				return nil, fmt.Errorf("keyword-style predicates are no longer supported; use function-call predicates (e.g., has(...), refs(...), content(...))")
			}
		}

		return nil, fmt.Errorf("unexpected identifier '%s': expected a function call like has(...), refs(...), content(...), or a field predicate like .field==value", keyword)
	}

	return nil, nil
}

func (p *Parser) parseExistsPredicate(negated bool) (Predicate, error) {
	// exists(.field)
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	if p.curr.Type != TokenDot {
		return nil, fmt.Errorf("expected .field as argument to exists()")
	}
	p.advance()
	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected field name after '.'")
	}
	field := p.curr.Value
	p.advance()
	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}
	return &FieldPredicate{
		basePredicate: basePredicate{negated: negated},
		Field:         field,
		Value:         "*",
		IsExists:      true,
		CompareOp:     CompareEq,
	}, nil
}

func (p *Parser) parseContentFuncPredicate(negated bool) (Predicate, error) {
	// content("search terms")
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	if p.curr.Type != TokenString {
		return nil, fmt.Errorf(`content() requires a quoted string, e.g. content("search term")`)
	}
	term := p.curr.Value
	p.advance()
	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}
	return &ContentPredicate{
		basePredicate: basePredicate{negated: negated},
		SearchTerm:    term,
	}, nil
}

func (p *Parser) parseHasFuncPredicate(negated bool) (Predicate, error) {
	// has(trait:...)
	subq, err := p.parseQueryArg(QueryTypeTrait, "trait")
	if err != nil {
		return nil, err
	}
	return &HasPredicate{
		basePredicate: basePredicate{negated: negated},
		SubQuery:      subq,
	}, nil
}

func (p *Parser) parseEnclosesFuncPredicate(negated bool) (Predicate, error) {
	// encloses(trait:...) => subtree trait containment
	subq, err := p.parseQueryArg(QueryTypeTrait, "trait")
	if err != nil {
		return nil, err
	}
	return &ContainsPredicate{
		basePredicate: basePredicate{negated: negated},
		SubQuery:      subq,
	}, nil
}

func (p *Parser) parseObjectNavFuncPredicate(negated bool, kind string) (Predicate, error) {
	// parent(object:...), ancestor(object:...), child(object:...), descendant(object:...), or ...([[target]])
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	if p.curr.Type == TokenLBrace {
		return nil, fmt.Errorf("brace subqueries are no longer supported; use %s(object:...) or %s([[target]])", kind, kind)
	}

	// Direct reference target
	if p.curr.Type == TokenRef {
		target := p.curr.Value
		p.advance()
		if err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		switch kind {
		case "parent":
			return &ParentPredicate{basePredicate: basePredicate{negated: negated}, Target: target}, nil
		case "ancestor":
			return &AncestorPredicate{basePredicate: basePredicate{negated: negated}, Target: target}, nil
		case "child":
			return &ChildPredicate{basePredicate: basePredicate{negated: negated}, Target: target}, nil
		case "descendant":
			return &DescendantPredicate{basePredicate: basePredicate{negated: negated}, Target: target}, nil
		default:
			return nil, fmt.Errorf("unknown navigation predicate: %s()", kind)
		}
	}

	if p.curr.Type == TokenUnderscore {
		return nil, fmt.Errorf("self-reference '_' is no longer supported (pipeline removed)")
	}

	// Nested object query
	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected object query or [[target]] in %s()", kind)
	}
	subq, err := p.parseQuery()
	if err != nil {
		return nil, err
	}
	if subq.Type != QueryTypeObject {
		return nil, fmt.Errorf("expected object subquery in %s(), got trait subquery", kind)
	}
	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}
	switch kind {
	case "parent":
		return &ParentPredicate{basePredicate: basePredicate{negated: negated}, SubQuery: subq}, nil
	case "ancestor":
		return &AncestorPredicate{basePredicate: basePredicate{negated: negated}, SubQuery: subq}, nil
	case "child":
		return &ChildPredicate{basePredicate: basePredicate{negated: negated}, SubQuery: subq}, nil
	case "descendant":
		return &DescendantPredicate{basePredicate: basePredicate{negated: negated}, SubQuery: subq}, nil
	default:
		return nil, fmt.Errorf("unknown navigation predicate: %s()", kind)
	}
}

func (p *Parser) parseTraitNavFuncPredicate(negated bool, kind string) (Predicate, error) {
	// on(object:...), within(object:...), or ...([[target]])
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	if p.curr.Type == TokenLBrace {
		return nil, fmt.Errorf("brace subqueries are no longer supported; use %s(object:...) or %s([[target]])", kind, kind)
	}

	if p.curr.Type == TokenRef {
		target := p.curr.Value
		p.advance()
		if err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		switch kind {
		case "on":
			return &OnPredicate{basePredicate: basePredicate{negated: negated}, Target: target}, nil
		case "within":
			return &WithinPredicate{basePredicate: basePredicate{negated: negated}, Target: target}, nil
		default:
			return nil, fmt.Errorf("unknown trait navigation predicate: %s()", kind)
		}
	}

	if p.curr.Type == TokenUnderscore {
		return nil, fmt.Errorf("self-reference '_' is no longer supported (pipeline removed)")
	}

	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected object query or [[target]] in %s()", kind)
	}
	subq, err := p.parseQuery()
	if err != nil {
		return nil, err
	}
	if subq.Type != QueryTypeObject {
		return nil, fmt.Errorf("expected object subquery in %s(), got trait subquery", kind)
	}
	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}
	switch kind {
	case "on":
		return &OnPredicate{basePredicate: basePredicate{negated: negated}, SubQuery: subq}, nil
	case "within":
		return &WithinPredicate{basePredicate: basePredicate{negated: negated}, SubQuery: subq}, nil
	default:
		return nil, fmt.Errorf("unknown trait navigation predicate: %s()", kind)
	}
}

func (p *Parser) parseRefsFuncPredicate(negated bool) (Predicate, error) {
	// refs([[target]]) or refs(object:...)
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	if p.curr.Type == TokenLBrace {
		return nil, fmt.Errorf("brace subqueries are no longer supported; use refs(object:...)")
	}
	if p.curr.Type == TokenRef {
		target := p.curr.Value
		p.advance()
		if err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return &RefsPredicate{basePredicate: basePredicate{negated: negated}, Target: target}, nil
	}
	if p.curr.Type == TokenUnderscore {
		return nil, fmt.Errorf("self-reference '_' is no longer supported (pipeline removed)")
	}
	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected [[target]] or object subquery in refs()")
	}
	subq, err := p.parseQuery()
	if err != nil {
		return nil, err
	}
	if subq.Type != QueryTypeObject {
		return nil, fmt.Errorf("refs() subquery must be an object query")
	}
	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}
	return &RefsPredicate{basePredicate: basePredicate{negated: negated}, SubQuery: subq}, nil
}

func (p *Parser) parseRefdFuncPredicate(negated bool) (Predicate, error) {
	// refd([[source]]) or refd(object:...) or refd(trait:...)
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	if p.curr.Type == TokenLBrace {
		return nil, fmt.Errorf("brace subqueries are no longer supported; use refd(object:...) or refd(trait:...)")
	}
	if p.curr.Type == TokenRef {
		target := p.curr.Value
		p.advance()
		if err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
		return &RefdPredicate{basePredicate: basePredicate{negated: negated}, Target: target}, nil
	}
	if p.curr.Type == TokenUnderscore {
		return nil, fmt.Errorf("self-reference '_' is no longer supported (pipeline removed)")
	}
	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected [[source]] or subquery in refd()")
	}
	subq, err := p.parseQuery()
	if err != nil {
		return nil, err
	}
	// Unlike most predicates, refd accepts both object and trait subqueries.
	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}
	return &RefdPredicate{basePredicate: basePredicate{negated: negated}, SubQuery: subq}, nil
}

func (p *Parser) parseAtFuncPredicate(negated bool) (Predicate, error) {
	// at(trait:...)
	subq, err := p.parseQueryArg(QueryTypeTrait, "trait")
	if err != nil {
		return nil, err
	}
	return &AtPredicate{basePredicate: basePredicate{negated: negated}, SubQuery: subq}, nil
}

func (p *Parser) parseQueryArg(expected QueryType, expectedKind string) (*Query, error) {
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}
	if p.curr.Type == TokenLBrace {
		return nil, fmt.Errorf("brace subqueries are no longer supported; drop braces and write %s:... directly", expectedKind)
	}
	if p.curr.Type == TokenUnderscore {
		return nil, fmt.Errorf("self-reference '_' is no longer supported (pipeline removed)")
	}
	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected %s query in argument", expectedKind)
	}
	subq, err := p.parseQuery()
	if err != nil {
		return nil, err
	}
	if subq.Type != expected {
		return nil, fmt.Errorf("expected %s query in argument", expectedKind)
	}
	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}
	return subq, nil
}
