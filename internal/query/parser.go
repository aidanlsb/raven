package query

import (
	"fmt"
	"strconv"
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

	// Keyword predicate
	if p.curr.Type == TokenIdent {
		keyword := strings.ToLower(p.curr.Value)

		// v2: 'value' uses operators directly (==, !=, <, >, etc.) instead of :
		if keyword == "value" {
			p.advance()
			return p.parseValuePredicate(negated)
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

// parseFieldPredicate parses .field==value, .field~="pattern", .field=~/regex/, etc.
// v2 syntax uses explicit operators instead of :
func (p *Parser) parseFieldPredicate(negated bool) (Predicate, error) {
	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected field name after '.'")
	}

	field := p.curr.Value
	p.advance()

	// Determine the operator
	var compareOp CompareOp
	var stringOp StringOp

	switch p.curr.Type {
	case TokenEqEq:
		compareOp = CompareEq
		p.advance()
	case TokenBangEq:
		compareOp = CompareNeq
		p.advance()
	case TokenLt:
		compareOp = CompareLt
		p.advance()
	case TokenGt:
		compareOp = CompareGt
		p.advance()
	case TokenLte:
		compareOp = CompareLte
		p.advance()
	case TokenGte:
		compareOp = CompareGte
		p.advance()
	case TokenTildeEq:
		stringOp = StringContains
		p.advance()
	case TokenCaretEq:
		stringOp = StringStartsWith
		p.advance()
	case TokenDollarEq:
		stringOp = StringEndsWith
		p.advance()
	case TokenEqTilde:
		stringOp = StringRegex
		p.advance()
	default:
		return nil, fmt.Errorf("expected operator (==, !=, <, >, <=, >=, ~=, ^=, $=, =~) after field name, got %v", p.curr.Type)
	}

	var value string
	isExists := false

	// Handle regex specially - expect TokenRegex
	if stringOp == StringRegex {
		if p.curr.Type != TokenRegex {
			return nil, fmt.Errorf("expected /regex/ after =~")
		}
		value = p.curr.Value
		p.advance()
	} else {
		switch p.curr.Type {
		case TokenStar:
			if stringOp != StringNone {
				return nil, fmt.Errorf("string operators cannot be used with '*' (exists check)")
			}
			if compareOp != CompareEq && compareOp != CompareNeq {
				return nil, fmt.Errorf("only == and != can be used with '*' (exists check)")
			}
			value = "*"
			isExists = true
			p.advance()
		case TokenIdent:
			value = p.curr.Value
			p.advance()
		case TokenRef:
			value = p.curr.Value
			p.advance()
		case TokenString:
			value = p.curr.Value
			p.advance()
		default:
			return nil, fmt.Errorf("expected field value, '*', or quoted string")
		}
	}

	return &FieldPredicate{
		basePredicate: basePredicate{negated: negated},
		Field:         field,
		Value:         value,
		IsExists:      isExists,
		CompareOp:     compareOp,
		StringOp:      stringOp,
	}, nil
}

// parseHasPredicate parses has:{trait:name ...} or has:_
func (p *Parser) parseHasPredicate(negated bool) (Predicate, error) {
	// Check for self-reference _
	if p.curr.Type == TokenUnderscore {
		p.advance()
		return &HasPredicate{
			basePredicate: basePredicate{negated: negated},
			IsSelfRef:     true,
		}, nil
	}

	subQuery, err := p.parseSubQuery(QueryTypeTrait, "trait")
	if err != nil {
		return nil, err
	}
	return &HasPredicate{
		basePredicate: basePredicate{negated: negated},
		SubQuery:      subQuery,
	}, nil
}

// parseParentPredicate parses parent:type, parent:{object:type ...}, parent:[[target]], or parent:_
func (p *Parser) parseParentPredicate(negated bool) (Predicate, error) {
	// Check for self-reference _
	if p.curr.Type == TokenUnderscore {
		p.advance()
		return &ParentPredicate{
			basePredicate: basePredicate{negated: negated},
			IsSelfRef:     true,
		}, nil
	}

	// Check for direct reference [[target]]
	if p.curr.Type == TokenRef {
		target := p.curr.Value
		p.advance()
		return &ParentPredicate{
			basePredicate: basePredicate{negated: negated},
			Target:        target,
		}, nil
	}

	subQuery, err := p.parseSubQuery(QueryTypeObject, "object")
	if err != nil {
		return nil, err
	}
	return &ParentPredicate{
		basePredicate: basePredicate{negated: negated},
		SubQuery:      subQuery,
	}, nil
}

// parseAncestorPredicate parses ancestor:type, ancestor:{object:type ...}, ancestor:[[target]], or ancestor:_
func (p *Parser) parseAncestorPredicate(negated bool) (Predicate, error) {
	// Check for self-reference _
	if p.curr.Type == TokenUnderscore {
		p.advance()
		return &AncestorPredicate{
			basePredicate: basePredicate{negated: negated},
			IsSelfRef:     true,
		}, nil
	}

	// Check for direct reference [[target]]
	if p.curr.Type == TokenRef {
		target := p.curr.Value
		p.advance()
		return &AncestorPredicate{
			basePredicate: basePredicate{negated: negated},
			Target:        target,
		}, nil
	}

	subQuery, err := p.parseSubQuery(QueryTypeObject, "object")
	if err != nil {
		return nil, err
	}
	return &AncestorPredicate{
		basePredicate: basePredicate{negated: negated},
		SubQuery:      subQuery,
	}, nil
}

// parseChildPredicate parses child:type, child:{object:type ...}, child:[[target]], or child:_
func (p *Parser) parseChildPredicate(negated bool) (Predicate, error) {
	// Check for self-reference _
	if p.curr.Type == TokenUnderscore {
		p.advance()
		return &ChildPredicate{
			basePredicate: basePredicate{negated: negated},
			IsSelfRef:     true,
		}, nil
	}

	// Check for direct reference [[target]]
	if p.curr.Type == TokenRef {
		target := p.curr.Value
		p.advance()
		return &ChildPredicate{
			basePredicate: basePredicate{negated: negated},
			Target:        target,
		}, nil
	}

	subQuery, err := p.parseSubQuery(QueryTypeObject, "object")
	if err != nil {
		return nil, err
	}
	return &ChildPredicate{
		basePredicate: basePredicate{negated: negated},
		SubQuery:      subQuery,
	}, nil
}

// parseDescendantPredicate parses descendant:type, descendant:{object:type ...}, descendant:[[target]], or descendant:_
func (p *Parser) parseDescendantPredicate(negated bool) (Predicate, error) {
	// Check for self-reference _
	if p.curr.Type == TokenUnderscore {
		p.advance()
		return &DescendantPredicate{
			basePredicate: basePredicate{negated: negated},
			IsSelfRef:     true,
		}, nil
	}

	// Check for direct reference [[target]]
	if p.curr.Type == TokenRef {
		target := p.curr.Value
		p.advance()
		return &DescendantPredicate{
			basePredicate: basePredicate{negated: negated},
			Target:        target,
		}, nil
	}

	subQuery, err := p.parseSubQuery(QueryTypeObject, "object")
	if err != nil {
		return nil, err
	}
	return &DescendantPredicate{
		basePredicate: basePredicate{negated: negated},
		SubQuery:      subQuery,
	}, nil
}

// parseContainsPredicate parses contains:{trait:name ...} or contains:_
func (p *Parser) parseContainsPredicate(negated bool) (Predicate, error) {
	// Check for self-reference _
	if p.curr.Type == TokenUnderscore {
		p.advance()
		return &ContainsPredicate{
			basePredicate: basePredicate{negated: negated},
			IsSelfRef:     true,
		}, nil
	}

	subQuery, err := p.parseSubQuery(QueryTypeTrait, "trait")
	if err != nil {
		return nil, err
	}
	return &ContainsPredicate{
		basePredicate: basePredicate{negated: negated},
		SubQuery:      subQuery,
	}, nil
}

// parseRefsPredicate parses refs:[[target]], refs:{object:type ...}, or refs:_
func (p *Parser) parseRefsPredicate(negated bool) (Predicate, error) {
	// Check for self-reference _
	if p.curr.Type == TokenUnderscore {
		p.advance()
		return &RefsPredicate{
			basePredicate: basePredicate{negated: negated},
			IsSelfRef:     true,
		}, nil
	}

	// Check for direct reference [[target]]
	if p.curr.Type == TokenRef {
		target := p.curr.Value
		p.advance()
		return &RefsPredicate{
			basePredicate: basePredicate{negated: negated},
			Target:        target,
		}, nil
	}

	// Otherwise expect a subquery
	if p.curr.Type == TokenLBrace {
		subQuery, err := p.parseSubQuery(QueryTypeObject, "object")
		if err != nil {
			return nil, err
		}
		return &RefsPredicate{
			basePredicate: basePredicate{negated: negated},
			SubQuery:      subQuery,
		}, nil
	}

	return nil, fmt.Errorf("expected [[reference]], {subquery}, or _ after refs:")
}

// parseContentPredicate parses content:"search terms"
func (p *Parser) parseContentPredicate(negated bool) (Predicate, error) {
	// Expect a quoted string
	if p.curr.Type != TokenString {
		return nil, fmt.Errorf("expected quoted string after content:, got %v", p.curr.Type)
	}

	searchTerm := p.curr.Value
	p.advance()

	return &ContentPredicate{
		basePredicate: basePredicate{negated: negated},
		SearchTerm:    searchTerm,
	}, nil
}

// parseValuePredicate parses value==val, value<val, value~="pattern", etc.
// v2 syntax: value is a keyword followed by an operator
func (p *Parser) parseValuePredicate(negated bool) (Predicate, error) {
	// Determine the operator
	var compareOp CompareOp
	var stringOp StringOp

	switch p.curr.Type {
	case TokenEqEq:
		compareOp = CompareEq
		p.advance()
	case TokenBangEq:
		compareOp = CompareNeq
		p.advance()
	case TokenLt:
		compareOp = CompareLt
		p.advance()
	case TokenGt:
		compareOp = CompareGt
		p.advance()
	case TokenLte:
		compareOp = CompareLte
		p.advance()
	case TokenGte:
		compareOp = CompareGte
		p.advance()
	case TokenTildeEq:
		stringOp = StringContains
		p.advance()
	case TokenCaretEq:
		stringOp = StringStartsWith
		p.advance()
	case TokenDollarEq:
		stringOp = StringEndsWith
		p.advance()
	case TokenEqTilde:
		stringOp = StringRegex
		p.advance()
	default:
		return nil, fmt.Errorf("expected operator (==, !=, <, >, <=, >=, ~=, ^=, $=, =~) after 'value', got %v", p.curr.Type)
	}

	var value string

	// Handle regex specially
	if stringOp == StringRegex {
		if p.curr.Type != TokenRegex {
			return nil, fmt.Errorf("expected /regex/ after =~")
		}
		value = p.curr.Value
		p.advance()
	} else {
		switch p.curr.Type {
		case TokenIdent:
			value = p.curr.Value
		case TokenRef:
			value = p.curr.Literal
		case TokenString:
			value = p.curr.Value
		default:
			return nil, fmt.Errorf("expected value or quoted string")
		}
		p.advance()
	}

	return &ValuePredicate{
		basePredicate: basePredicate{negated: negated},
		Value:         value,
		CompareOp:     compareOp,
		StringOp:      stringOp,
	}, nil
}

// parseSourcePredicate parses source:inline or source:frontmatter
func (p *Parser) parseSourcePredicate(negated bool) (Predicate, error) {
	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected 'inline' or 'frontmatter'")
	}
	source := strings.ToLower(p.curr.Value)
	if source != "inline" && source != "frontmatter" {
		return nil, fmt.Errorf("invalid source: %s (expected 'inline' or 'frontmatter')", source)
	}
	p.advance()
	return &SourcePredicate{
		basePredicate: basePredicate{negated: negated},
		Source:        source,
	}, nil
}

// parseOnPredicate parses on:type, on:{object:type ...}, on:[[target]], or on:_
func (p *Parser) parseOnPredicate(negated bool) (Predicate, error) {
	// Check for self-reference _
	if p.curr.Type == TokenUnderscore {
		p.advance()
		return &OnPredicate{
			basePredicate: basePredicate{negated: negated},
			IsSelfRef:     true,
		}, nil
	}

	// Check for direct reference [[target]]
	if p.curr.Type == TokenRef {
		target := p.curr.Value
		p.advance()
		return &OnPredicate{
			basePredicate: basePredicate{negated: negated},
			Target:        target,
		}, nil
	}

	subQuery, err := p.parseSubQuery(QueryTypeObject, "object")
	if err != nil {
		return nil, err
	}
	return &OnPredicate{
		basePredicate: basePredicate{negated: negated},
		SubQuery:      subQuery,
	}, nil
}

// parseWithinPredicate parses within:type, within:{object:type ...}, within:[[target]], or within:_
func (p *Parser) parseWithinPredicate(negated bool) (Predicate, error) {
	// Check for self-reference _
	if p.curr.Type == TokenUnderscore {
		p.advance()
		return &WithinPredicate{
			basePredicate: basePredicate{negated: negated},
			IsSelfRef:     true,
		}, nil
	}

	// Check for direct reference [[target]]
	if p.curr.Type == TokenRef {
		target := p.curr.Value
		p.advance()
		return &WithinPredicate{
			basePredicate: basePredicate{negated: negated},
			Target:        target,
		}, nil
	}

	subQuery, err := p.parseSubQuery(QueryTypeObject, "object")
	if err != nil {
		return nil, err
	}
	return &WithinPredicate{
		basePredicate: basePredicate{negated: negated},
		SubQuery:      subQuery,
	}, nil
}

// parseAtPredicate parses at:{trait:...}, at:[[target]], or at:_
func (p *Parser) parseAtPredicate(negated bool) (Predicate, error) {
	// Check for self-reference _
	if p.curr.Type == TokenUnderscore {
		p.advance()
		return &AtPredicate{
			basePredicate: basePredicate{negated: negated},
			IsSelfRef:     true,
		}, nil
	}

	// Check for direct reference [[target]]
	if p.curr.Type == TokenRef {
		target := p.curr.Value
		p.advance()
		return &AtPredicate{
			basePredicate: basePredicate{negated: negated},
			Target:        target,
		}, nil
	}

	// Otherwise expect a trait subquery
	subQuery, err := p.parseSubQuery(QueryTypeTrait, "trait")
	if err != nil {
		return nil, err
	}
	return &AtPredicate{
		basePredicate: basePredicate{negated: negated},
		SubQuery:      subQuery,
	}, nil
}

// parseRefdPredicate parses refd:{object:...}, refd:{trait:...}, refd:[[target]], or refd:_
// Note: Unlike most predicates, refd: accepts both object and trait subqueries because
// something can be referenced by either objects or traits.
// v2: No shorthand expansion - explicit subqueries required
func (p *Parser) parseRefdPredicate(negated bool) (Predicate, error) {
	// Check for self-reference _
	if p.curr.Type == TokenUnderscore {
		p.advance()
		return &RefdPredicate{
			basePredicate: basePredicate{negated: negated},
			IsSelfRef:     true,
		}, nil
	}

	// Check for direct reference [[target]]
	if p.curr.Type == TokenRef {
		target := p.curr.Value
		p.advance()
		return &RefdPredicate{
			basePredicate: basePredicate{negated: negated},
			Target:        target,
		}, nil
	}

	// Full subquery in braces required (accepts either object or trait)
	if p.curr.Type != TokenLBrace {
		return nil, fmt.Errorf("expected [[reference]], {subquery}, or _ after refd: (shorthand syntax removed in v2)")
	}

	p.advance()
	subQuery, err := p.parseQuery()
	if err != nil {
		return nil, fmt.Errorf("in refd subquery: %w", err)
	}
	if err := p.expect(TokenRBrace); err != nil {
		return nil, fmt.Errorf("unclosed refd subquery: %w", err)
	}
	return &RefdPredicate{
		basePredicate: basePredicate{negated: negated},
		SubQuery:      subQuery,
	}, nil
}

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
//   - name = min({trait:...})           -- min of trait values
//   - name = min(.field, {object:...})  -- min of field values on objects
//   - name = max(.field, {object:...})  -- max of field values on objects
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

// parseSortStage parses: sort(field, asc) or sort(variable, desc)
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

	// Comma and direction
	if err := p.expect(TokenComma); err != nil {
		return nil, err
	}

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
