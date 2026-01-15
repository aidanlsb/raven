package query

import (
	"fmt"
	"strings"
)

// parseFieldPredicate parses .field==value, .field!=value, .field>value, etc.
// For string matching, use function-style predicates: includes(.field, "str"), startswith(...), etc.
func (p *Parser) parseFieldPredicate(negated bool) (Predicate, error) {
	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected field name after '.'")
	}

	field := p.curr.Value
	p.advance()

	// Determine the operator
	var compareOp CompareOp

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
	default:
		return nil, fmt.Errorf("expected comparison operator (==, !=, <, >, <=, >=) after field name; for string matching use includes(), startswith(), endswith(), or matches()")
	}

	var value string
	isExists := false

	switch p.curr.Type {
	case TokenStar:
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

	return &FieldPredicate{
		basePredicate: basePredicate{negated: negated},
		Field:         field,
		Value:         value,
		IsExists:      isExists,
		CompareOp:     compareOp,
	}, nil
}

// parseValuePredicate parses value==val, value<val, value>val, etc.
// For string matching, use function-style predicates: includes(), startswith(), etc.
func (p *Parser) parseValuePredicate(negated bool) (Predicate, error) {
	// Determine the operator
	var compareOp CompareOp

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
	default:
		return nil, fmt.Errorf("expected comparison operator (==, !=, <, >, <=, >=) after 'value'; for string matching use includes(), startswith(), endswith(), or matches()")
	}

	var value string

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

	return &ValuePredicate{
		basePredicate: basePredicate{negated: negated},
		Value:         value,
		CompareOp:     compareOp,
	}, nil
}

// parseStringFuncPredicate parses: includes(.field, "value"), startswith(.field, "value"), etc.
// Also supports: includes(_, "value") for use within array quantifiers.
func (p *Parser) parseStringFuncPredicate(negated bool, funcType StringFuncType) (Predicate, error) {
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}

	pred := &StringFuncPredicate{
		basePredicate: basePredicate{negated: negated},
		FuncType:      funcType,
	}

	// Parse first argument: .field or _
	if p.curr.Type == TokenDot {
		p.advance()
		if p.curr.Type != TokenIdent {
			return nil, fmt.Errorf("expected field name after '.'")
		}
		pred.Field = p.curr.Value
		p.advance()
	} else if p.curr.Type == TokenUnderscore {
		pred.Field = "_"
		pred.IsElementRef = true
		p.advance()
	} else {
		return nil, fmt.Errorf("expected .field or _ as first argument to %s()", funcType)
	}

	// Expect comma
	if err := p.expect(TokenComma); err != nil {
		return nil, err
	}

	// Parse second argument: string value
	if p.curr.Type != TokenString {
		return nil, fmt.Errorf("expected string value as second argument to %s()", funcType)
	}
	pred.Value = p.curr.Value
	p.advance()

	// Check for optional third argument: case sensitivity (true = case-sensitive)
	if p.curr.Type == TokenComma {
		p.advance()
		if p.curr.Type != TokenIdent {
			return nil, fmt.Errorf("expected 'true' or 'false' for case sensitivity argument")
		}
		switch strings.ToLower(p.curr.Value) {
		case "true":
			pred.CaseSensitive = true
		case "false":
			pred.CaseSensitive = false
		default:
			return nil, fmt.Errorf("expected 'true' or 'false' for case sensitivity, got '%s'", p.curr.Value)
		}
		p.advance()
	}

	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	return pred, nil
}

// parseArrayQuantifierPredicate parses: any(.field, predicate), all(.field, predicate), none(.field, predicate)
func (p *Parser) parseArrayQuantifierPredicate(negated bool, quantifier ArrayQuantifierType) (Predicate, error) {
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}

	pred := &ArrayQuantifierPredicate{
		basePredicate: basePredicate{negated: negated},
		Quantifier:    quantifier,
	}

	// Parse first argument: .field
	if p.curr.Type != TokenDot {
		return nil, fmt.Errorf("expected .field as first argument to %s()", quantifier)
	}
	p.advance()
	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected field name after '.'")
	}
	pred.Field = p.curr.Value
	p.advance()

	// Expect comma
	if err := p.expect(TokenComma); err != nil {
		return nil, err
	}

	// Parse second argument: element predicate (with possible OR)
	elementPred, err := p.parseElementPredicateWithOr()
	if err != nil {
		return nil, err
	}
	pred.ElementPred = elementPred

	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	return pred, nil
}

// parseElementPredicateWithOr parses an element predicate that may contain OR.
func (p *Parser) parseElementPredicateWithOr() (Predicate, error) {
	left, err := p.parseElementPredicate()
	if err != nil {
		return nil, err
	}

	// Check for OR
	if p.curr.Type == TokenPipe {
		p.advance()
		right, err := p.parseElementPredicateWithOr()
		if err != nil {
			return nil, err
		}
		return &OrPredicate{Left: left, Right: right}, nil
	}

	return left, nil
}

// parseElementPredicate parses predicates used within array quantifiers.
// Supports: _ == value, _ != value, includes(_, "str"), etc.
func (p *Parser) parseElementPredicate() (Predicate, error) {
	// Check for negation
	negated := false
	if p.curr.Type == TokenBang {
		negated = true
		p.advance()
	}

	// Check for _ == value or _ != value
	if p.curr.Type == TokenUnderscore {
		p.advance()

		var compareOp CompareOp
		switch p.curr.Type {
		case TokenEqEq:
			compareOp = CompareEq
		case TokenBangEq:
			compareOp = CompareNeq
		case TokenLt:
			compareOp = CompareLt
		case TokenGt:
			compareOp = CompareGt
		case TokenLte:
			compareOp = CompareLte
		case TokenGte:
			compareOp = CompareGte
		default:
			return nil, fmt.Errorf("expected comparison operator (==, !=, <, >, <=, >=) after '_'")
		}
		p.advance()

		// Get the value
		var value string
		switch p.curr.Type {
		case TokenIdent:
			value = p.curr.Value
		case TokenString:
			value = p.curr.Value
		default:
			return nil, fmt.Errorf("expected value after comparison operator")
		}
		p.advance()

		return &ElementEqualityPredicate{
			basePredicate: basePredicate{negated: negated},
			Value:         value,
			CompareOp:     compareOp,
		}, nil
	}

	// Check for parenthesized group
	if p.curr.Type == TokenLParen {
		p.advance()
		pred, err := p.parseElementPredicate()
		if err != nil {
			return nil, err
		}

		// Check for OR within the group
		if p.curr.Type == TokenPipe {
			p.advance()
			right, err := p.parseElementPredicate()
			if err != nil {
				return nil, err
			}
			pred = &OrPredicate{Left: pred, Right: right}
		}

		if err := p.expect(TokenRParen); err != nil {
			return nil, fmt.Errorf("unclosed parenthesis in element predicate: %w", err)
		}

		if negated {
			return &GroupPredicate{
				basePredicate: basePredicate{negated: true},
				Predicates:    []Predicate{pred},
			}, nil
		}
		return pred, nil
	}

	// Check for function-style predicates: includes(_, "str"), etc.
	if p.curr.Type == TokenIdent {
		funcName := strings.ToLower(p.curr.Value)
		if p.peek.Type == TokenLParen {
			switch funcName {
			case "includes":
				p.advance()
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
			}
		}
	}

	return nil, fmt.Errorf("expected element predicate: _ == value, _ != value, or a function like includes(_, \"str\")")
}
