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

	// Parse optional sort/group/limit clauses
	for p.curr.Type == TokenIdent {
		keyword := strings.ToLower(p.curr.Value)

		switch keyword {
		case "sort":
			if query.Sort != nil {
				return nil, fmt.Errorf("duplicate sort clause")
			}
			p.advance()
			if err := p.expect(TokenColon); err != nil {
				return nil, err
			}
			sort, err := p.parseSortSpec()
			if err != nil {
				return nil, err
			}
			query.Sort = sort

		case "group":
			if query.Group != nil {
				return nil, fmt.Errorf("duplicate group clause")
			}
			p.advance()
			if err := p.expect(TokenColon); err != nil {
				return nil, err
			}
			group, err := p.parseGroupSpec()
			if err != nil {
				return nil, err
			}
			query.Group = group

		case "limit":
			if query.Limit != 0 {
				return nil, fmt.Errorf("duplicate limit clause")
			}
			p.advance()
			if err := p.expect(TokenColon); err != nil {
				return nil, err
			}
			limit, err := p.parseLimit()
			if err != nil {
				return nil, err
			}
			query.Limit = limit

		default:
			// Not a clause keyword, stop parsing clauses
			goto done
		}
	}
done:

	return &query, nil
}

// parsePredicates parses a sequence of predicates.
func (p *Parser) parsePredicates(qt QueryType) ([]Predicate, error) {
	var predicates []Predicate

	for {
		// Stop at EOF, closing braces, or closing parens
		if p.curr.Type == TokenEOF || p.curr.Type == TokenRBrace || p.curr.Type == TokenRParen {
			break
		}

		// Stop at sort/group/limit keywords (these are clauses, not predicates)
		if p.curr.Type == TokenIdent {
			keyword := strings.ToLower(p.curr.Value)
			if keyword == "sort" || keyword == "group" || keyword == "limit" {
				break
			}
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
		case "value":
			return p.parseValuePredicate(negated)
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

// parseFieldPredicate parses .field:value, .field:<value, or .field:"quoted value"
func (p *Parser) parseFieldPredicate(negated bool) (Predicate, error) {
	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected field name after '.'")
	}

	field := p.curr.Value
	p.advance()

	if err := p.expect(TokenColon); err != nil {
		return nil, err
	}

	// Check for comparison operator prefix
	compareOp := CompareEq
	switch p.curr.Type {
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
	}

	var value string
	isExists := false

	switch p.curr.Type {
	case TokenStar:
		if compareOp != CompareEq {
			return nil, fmt.Errorf("comparison operators cannot be used with '*' (exists check)")
		}
		value = "*"
		isExists = true
		p.advance()
	case TokenIdent:
		value = p.curr.Value
		p.advance()
	case TokenRef:
		// Store the resolved reference target (without [[...]]), matching how refs
		// are stored in indexed JSON fields and how other predicates treat TokenRef.
		value = p.curr.Value
		p.advance()
	case TokenString:
		// Support quoted strings for values with spaces
		value = p.curr.Value
		p.advance()
	default:
		return nil, fmt.Errorf("expected field value, '*', or quoted string")
	}

	return &FieldPredicate{
		basePredicate: basePredicate{negated: negated},
		Field:         field,
		Value:         value,
		IsExists:      isExists,
		CompareOp:     compareOp,
	}, nil
}

// parseHasPredicate parses has:trait or has:{trait:name ...}
func (p *Parser) parseHasPredicate(negated bool) (Predicate, error) {
	subQuery, err := p.parseSubQuery(QueryTypeTrait, "trait")
	if err != nil {
		return nil, err
	}
	return &HasPredicate{
		basePredicate: basePredicate{negated: negated},
		SubQuery:      subQuery,
	}, nil
}

// parseParentPredicate parses parent:type, parent:{object:type ...}, or parent:[[target]]
func (p *Parser) parseParentPredicate(negated bool) (Predicate, error) {
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

// parseAncestorPredicate parses ancestor:type, ancestor:{object:type ...}, or ancestor:[[target]]
func (p *Parser) parseAncestorPredicate(negated bool) (Predicate, error) {
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

// parseChildPredicate parses child:type, child:{object:type ...}, or child:[[target]]
func (p *Parser) parseChildPredicate(negated bool) (Predicate, error) {
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

// parseDescendantPredicate parses descendant:type, descendant:{object:type ...}, or descendant:[[target]]
func (p *Parser) parseDescendantPredicate(negated bool) (Predicate, error) {
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

// parseContainsPredicate parses contains:{trait:name ...}
func (p *Parser) parseContainsPredicate(negated bool) (Predicate, error) {
	subQuery, err := p.parseSubQuery(QueryTypeTrait, "trait")
	if err != nil {
		return nil, err
	}
	return &ContainsPredicate{
		basePredicate: basePredicate{negated: negated},
		SubQuery:      subQuery,
	}, nil
}

// parseRefsPredicate parses refs:[[target]] or refs:{object:type ...}
func (p *Parser) parseRefsPredicate(negated bool) (Predicate, error) {
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

	return nil, fmt.Errorf("expected [[reference]] or {subquery} after refs:")
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

// parseValuePredicate parses value:val, value:<val, value:>val, or value:"quoted value"
func (p *Parser) parseValuePredicate(negated bool) (Predicate, error) {
	// Check for comparison operator prefix
	compareOp := CompareEq
	switch p.curr.Type {
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
	}

	var value string
	switch p.curr.Type {
	case TokenIdent:
		value = p.curr.Value
	case TokenRef:
		value = p.curr.Literal
	case TokenString:
		// Support quoted strings for values with spaces
		value = p.curr.Value
	default:
		return nil, fmt.Errorf("expected value or quoted string")
	}
	p.advance()
	return &ValuePredicate{
		basePredicate: basePredicate{negated: negated},
		Value:         value,
		CompareOp:     compareOp,
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

// parseOnPredicate parses on:type, on:{object:type ...}, or on:[[target]]
func (p *Parser) parseOnPredicate(negated bool) (Predicate, error) {
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

// parseWithinPredicate parses within:type, within:{object:type ...}, or within:[[target]]
func (p *Parser) parseWithinPredicate(negated bool) (Predicate, error) {
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

// parseAtPredicate parses at:{trait:...} or at:[[target]]
func (p *Parser) parseAtPredicate(negated bool) (Predicate, error) {
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

// parseRefdPredicate parses refd:{object:...}, refd:{trait:...}, refd:type, or refd:[[target]]
// Note: Unlike most predicates, refd: accepts both object and trait subqueries because
// something can be referenced by either objects or traits.
func (p *Parser) parseRefdPredicate(negated bool) (Predicate, error) {
	// Check for direct reference [[target]]
	if p.curr.Type == TokenRef {
		target := p.curr.Value
		p.advance()
		return &RefdPredicate{
			basePredicate: basePredicate{negated: negated},
			Target:        target,
		}, nil
	}

	// Full subquery in braces (accepts either object or trait)
	if p.curr.Type == TokenLBrace {
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

	// Shorthand: refd:type expands to refd:{object:type}
	if p.curr.Type == TokenIdent {
		typeName := p.curr.Value
		p.advance()
		return &RefdPredicate{
			basePredicate: basePredicate{negated: negated},
			SubQuery: &Query{
				Type:     QueryTypeObject,
				TypeName: typeName,
			},
		}, nil
	}

	return nil, fmt.Errorf("expected [[reference]], {subquery}, or type name after refd:")
}

// parseSubQuery parses either a shorthand (type name) or full subquery ({object:type ...})
func (p *Parser) parseSubQuery(expectedType QueryType, expectedKind string) (*Query, error) {
	// Full subquery in braces
	if p.curr.Type == TokenLBrace {
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

	// Shorthand: just a type/trait name
	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected type name or '{' for subquery")
	}

	typeName := p.curr.Value
	p.advance()

	return &Query{
		Type:     expectedType,
		TypeName: typeName,
	}, nil
}

// parseSortSpec parses sort:[agg:]{subquery} or sort:[agg:]_.path
func (p *Parser) parseSortSpec() (*SortSpec, error) {
	spec := &SortSpec{Aggregation: AggFirst}

	// Check for aggregation prefix
	if p.curr.Type == TokenIdent {
		switch strings.ToLower(p.curr.Value) {
		case "min":
			spec.Aggregation = AggMin
			p.advance()
			if err := p.expect(TokenColon); err != nil {
				return nil, err
			}
		case "max":
			spec.Aggregation = AggMax
			p.advance()
			if err := p.expect(TokenColon); err != nil {
				return nil, err
			}
		case "first":
			spec.Aggregation = AggFirst
			p.advance()
			if err := p.expect(TokenColon); err != nil {
				return nil, err
			}
		case "count":
			spec.Aggregation = AggCount
			p.advance()
			if err := p.expect(TokenColon); err != nil {
				return nil, err
			}
		}
	}

	// Check for subquery or path
	if p.curr.Type == TokenLBrace {
		// Subquery: {trait:due at:_}
		p.advance()
		subQuery, err := p.parseQuery()
		if err != nil {
			return nil, fmt.Errorf("in sort subquery: %w", err)
		}
		if err := p.expect(TokenRBrace); err != nil {
			return nil, err
		}
		spec.SubQuery = subQuery

		// Check for optional field extraction: {object:...}.field
		if p.curr.Type == TokenDot {
			p.advance()
			if p.curr.Type != TokenIdent {
				return nil, fmt.Errorf("expected field name after '.'")
			}
			// Append field extraction to path
			spec.Path = &PathExpr{Steps: []PathStep{{Kind: PathStepField, Name: p.curr.Value}}}
			p.advance()
		}
	} else if p.curr.Type == TokenUnderscore {
		// Path: _.parent.status
		path, err := p.parsePathExpr()
		if err != nil {
			return nil, err
		}
		spec.Path = path
	} else {
		return nil, fmt.Errorf("expected '{' or '_' in sort specification")
	}

	// Check for :asc or :desc
	if p.curr.Type == TokenColon {
		p.advance()
		if p.curr.Type != TokenIdent {
			return nil, fmt.Errorf("expected 'asc' or 'desc'")
		}
		switch strings.ToLower(p.curr.Value) {
		case "asc":
			spec.Descending = false
		case "desc":
			spec.Descending = true
		default:
			return nil, fmt.Errorf("expected 'asc' or 'desc', got '%s'", p.curr.Value)
		}
		p.advance()
	}

	return spec, nil
}

// parseGroupSpec parses group:{subquery} or group:_.path
func (p *Parser) parseGroupSpec() (*GroupSpec, error) {
	spec := &GroupSpec{}

	// Check for aggregation prefix (mainly count: for grouping)
	if p.curr.Type == TokenIdent {
		if strings.ToLower(p.curr.Value) == "count" {
			spec.Aggregation = AggCount
			p.advance()
			if err := p.expect(TokenColon); err != nil {
				return nil, err
			}
		}
	}

	// Check for subquery or path
	if p.curr.Type == TokenLBrace {
		p.advance()
		subQuery, err := p.parseQuery()
		if err != nil {
			return nil, fmt.Errorf("in group subquery: %w", err)
		}
		if err := p.expect(TokenRBrace); err != nil {
			return nil, err
		}
		spec.SubQuery = subQuery

		// Check for optional field extraction
		if p.curr.Type == TokenDot {
			p.advance()
			if p.curr.Type != TokenIdent {
				return nil, fmt.Errorf("expected field name after '.'")
			}
			spec.Path = &PathExpr{Steps: []PathStep{{Kind: PathStepField, Name: p.curr.Value}}}
			p.advance()
		}
	} else if p.curr.Type == TokenUnderscore {
		path, err := p.parsePathExpr()
		if err != nil {
			return nil, err
		}
		spec.Path = path
	} else {
		return nil, fmt.Errorf("expected '{' or '_' in group specification")
	}

	return spec, nil
}

// parseLimit parses a limit value (positive integer).
func (p *Parser) parseLimit() (int, error) {
	if p.curr.Type != TokenIdent {
		return 0, fmt.Errorf("expected number after limit:")
	}

	n, err := strconv.Atoi(p.curr.Value)
	if err != nil {
		return 0, fmt.Errorf("invalid limit value '%s': must be a positive integer", p.curr.Value)
	}
	if n <= 0 {
		return 0, fmt.Errorf("limit must be a positive integer, got %d", n)
	}
	p.advance()
	return n, nil
}

// parsePathExpr parses _.parent.status, _.refs:project, _.value, etc.
func (p *Parser) parsePathExpr() (*PathExpr, error) {
	if p.curr.Type != TokenUnderscore {
		return nil, fmt.Errorf("expected '_'")
	}
	p.advance()

	path := &PathExpr{}

	for p.curr.Type == TokenDot {
		p.advance()

		if p.curr.Type != TokenIdent {
			return nil, fmt.Errorf("expected path step after '.'")
		}

		stepName := p.curr.Value
		p.advance()

		switch stepName {
		case "parent":
			path.Steps = append(path.Steps, PathStep{Kind: PathStepParent})
		case "value":
			path.Steps = append(path.Steps, PathStep{Kind: PathStepValue})
		case "ancestor":
			// Expect :type
			if err := p.expect(TokenColon); err != nil {
				return nil, fmt.Errorf("expected ':type' after 'ancestor'")
			}
			if p.curr.Type != TokenIdent {
				return nil, fmt.Errorf("expected type name after 'ancestor:'")
			}
			path.Steps = append(path.Steps, PathStep{Kind: PathStepAncestor, Name: p.curr.Value})
			p.advance()
		case "refs":
			// Expect :type
			if err := p.expect(TokenColon); err != nil {
				return nil, fmt.Errorf("expected ':type' after 'refs'")
			}
			if p.curr.Type != TokenIdent {
				return nil, fmt.Errorf("expected type name after 'refs:'")
			}
			path.Steps = append(path.Steps, PathStep{Kind: PathStepRefs, Name: p.curr.Value})
			p.advance()
		default:
			// Regular field access
			path.Steps = append(path.Steps, PathStep{Kind: PathStepField, Name: stepName})
		}
	}

	if len(path.Steps) == 0 {
		return nil, fmt.Errorf("expected path after '_'")
	}

	return path, nil
}
