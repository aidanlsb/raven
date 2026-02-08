package query

import (
	"fmt"
	"strings"
)

// parseFieldPredicate parses .field==value, .field!=value, .field>value, etc.
// For string matching, use function-style predicates: contains(.field, "str"), startswith(...), etc.
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
		return nil, fmt.Errorf("expected comparison operator (==, !=, <, >, <=, >=) after field name; for string matching use contains(), startswith(), endswith(), or matches()")
	}

	var value string
	isRefValue := false

	switch p.curr.Type {
	case TokenStar:
		return nil, fmt.Errorf(".field==* is no longer supported; use exists(.field) or !exists(.field) instead")
	case TokenIdent:
		value = p.curr.Value
		p.advance()
	case TokenRef:
		value = p.curr.Value
		isRefValue = true
		p.advance()
	case TokenString:
		value = p.curr.Value
		p.advance()
	default:
		return nil, fmt.Errorf("expected field value or quoted string; for field presence use exists(.field)")
	}

	return &FieldPredicate{
		basePredicate: basePredicate{negated: negated},
		Field:         field,
		Value:         value,
		IsExists:      false,
		CompareOp:     compareOp,
		IsRefValue:    isRefValue,
	}, nil
}

type parsedValue struct {
	Value string
	IsRef bool
}

// parseValueList parses a bracketed list of scalar values: [a, "b", [[ref]]]
// Callers should ensure p.curr.Type == TokenLBracket.
func (p *Parser) parseValueList() ([]parsedValue, error) {
	if err := p.expect(TokenLBracket); err != nil {
		return nil, err
	}

	var values []parsedValue
	for {
		// End of list
		if p.curr.Type == TokenRBracket {
			p.advance()
			break
		}
		if p.curr.Type == TokenEOF {
			return nil, fmt.Errorf("unclosed value list: expected ']'")
		}

		var v parsedValue
		switch p.curr.Type {
		case TokenIdent:
			v.Value = p.curr.Value
			p.advance()
		case TokenString:
			v.Value = p.curr.Value
			p.advance()
		case TokenRef:
			v.Value = p.curr.Value
			v.IsRef = true
			p.advance()
		default:
			return nil, fmt.Errorf("expected list value (identifier, string, or reference), got %v", p.curr.Type)
		}
		values = append(values, v)

		// After a value, expect comma or closing bracket
		if p.curr.Type == TokenComma {
			p.advance()
			// Disallow trailing comma before ]
			if p.curr.Type == TokenRBracket {
				return nil, fmt.Errorf("trailing comma in value list is not allowed")
			}
			continue
		}
		if p.curr.Type == TokenRBracket {
			p.advance()
			break
		}
		return nil, fmt.Errorf("expected ',' or ']' in value list, got %v", p.curr.Type)
	}

	return values, nil
}

// parseInPredicate parses: in(.field, [a,b,"c"])
// This is useful for scalar membership checks like trait:due in(.value, [past,today]).
func (p *Parser) parseInPredicate(negated bool) (Predicate, error) {
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}

	// First arg: .field
	if p.curr.Type != TokenDot {
		return nil, fmt.Errorf("expected .field as first argument to in()")
	}
	p.advance()
	if p.curr.Type != TokenIdent {
		return nil, fmt.Errorf("expected field name after '.'")
	}
	field := p.curr.Value
	p.advance()

	if err := p.expect(TokenComma); err != nil {
		return nil, err
	}

	if p.curr.Type != TokenLBracket {
		return nil, fmt.Errorf("expected '[' to start value list as second argument to in()")
	}
	values, err := p.parseValueList()
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("value list cannot be empty")
	}

	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	// Single value: just a field predicate.
	if len(values) == 1 {
		return &FieldPredicate{
			basePredicate: basePredicate{negated: negated},
			Field:         field,
			Value:         values[0].Value,
			IsExists:      false,
			CompareOp:     CompareEq,
			IsRefValue:    values[0].IsRef,
		}, nil
	}

	// Multiple values: OR of == predicates.
	var preds []Predicate
	for _, v := range values {
		preds = append(preds, &FieldPredicate{
			Field:      field,
			Value:      v.Value,
			IsExists:   false,
			CompareOp:  CompareEq,
			IsRefValue: v.IsRef,
		})
	}
	var result Predicate = &OrPredicate{Predicates: preds}
	if negated {
		result = &NotPredicate{Inner: result}
	}
	return result, nil
}

// parseStringFuncPredicate parses: contains(.field, "value"), startswith(.field, "value"), etc.
// Also supports: contains(_, "value") for use within array quantifiers.
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

	// Parse second argument: string value (or /regex/ for matches)
	switch p.curr.Type {
	case TokenString:
		pred.Value = p.curr.Value
		p.advance()
	case TokenRegex:
		if funcType != StringFuncMatches {
			return nil, fmt.Errorf("regex literal is only supported for matches()")
		}
		pred.Value = p.curr.Value
		p.advance()
	default:
		if funcType == StringFuncMatches {
			return nil, fmt.Errorf("expected string or /regex/ as second argument to %s()", funcType)
		}
		return nil, fmt.Errorf("expected string value as second argument to %s()", funcType)
	}

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

	// Parse second argument: element predicate (supports AND/OR)
	elementPred, err := p.parseElementOrPredicate()
	if err != nil {
		return nil, err
	}
	pred.ElementPred = elementPred

	if err := p.expect(TokenRParen); err != nil {
		return nil, err
	}

	return pred, nil
}

// parseElementOrPredicate parses element predicates with OR (lowest precedence).
func (p *Parser) parseElementOrPredicate() (Predicate, error) {
	first, err := p.parseElementAndPredicate()
	if err != nil {
		return nil, err
	}
	if first == nil {
		return nil, fmt.Errorf("expected element predicate")
	}

	if p.curr.Type != TokenPipe {
		return first, nil
	}

	preds := []Predicate{first}
	for p.curr.Type == TokenPipe {
		p.advance()
		next, err := p.parseElementAndPredicate()
		if err != nil {
			return nil, err
		}
		if next == nil {
			return nil, fmt.Errorf("expected element predicate after '|'")
		}
		preds = append(preds, next)
	}

	return &OrPredicate{Predicates: preds}, nil
}

// parseElementAndPredicate parses element predicates with implicit AND.
func (p *Parser) parseElementAndPredicate() (Predicate, error) {
	var preds []Predicate

	for {
		if p.curr.Type == TokenRParen || p.curr.Type == TokenPipe {
			break
		}
		pred, err := p.parseElementUnaryPredicate()
		if err != nil {
			return nil, err
		}
		if pred == nil {
			return nil, fmt.Errorf("expected element predicate")
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

// parseElementUnaryPredicate parses element predicates used within array quantifiers.
// Supports: _ == value, _ != value, contains(_, "str"), etc.
func (p *Parser) parseElementUnaryPredicate() (Predicate, error) {
	// Check for negation
	negated := false
	if p.curr.Type == TokenBang {
		negated = true
		p.advance()
	}

	// Check for parenthesized group
	if p.curr.Type == TokenLParen {
		p.advance()
		pred, err := p.parseElementOrPredicate()
		if err != nil {
			return nil, err
		}
		if pred == nil {
			return nil, fmt.Errorf("expected element predicate inside parentheses")
		}
		if err := p.expect(TokenRParen); err != nil {
			return nil, fmt.Errorf("unclosed parenthesis in element predicate: %w", err)
		}
		if negated {
			return &NotPredicate{Inner: pred}, nil
		}
		return pred, nil
	}

	// Check for _ == value or _ != value
	if p.curr.Type == TokenUnderscore {
		return p.parseElementUnderscoreEquality(negated)
	}

	// Check for function-style predicates: contains(_, "str"), etc.
	if p.curr.Type == TokenIdent {
		if pred, ok, err := p.tryParseElementFuncPredicate(negated); ok || err != nil {
			return pred, err
		}
	}

	return nil, fmt.Errorf("expected element predicate: _ == value, _ != value, or a function like contains(_, \"str\")")
}

func (p *Parser) parseElementUnderscoreEquality(negated bool) (Predicate, error) {
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
	isRefValue := false
	switch p.curr.Type {
	case TokenIdent:
		value = p.curr.Value
	case TokenString:
		value = p.curr.Value
	case TokenRef:
		value = p.curr.Value
		isRefValue = true
	default:
		return nil, fmt.Errorf("expected value after comparison operator (identifier, string, or reference)")
	}
	p.advance()

	return &ElementEqualityPredicate{
		basePredicate: basePredicate{negated: negated},
		Value:         value,
		CompareOp:     compareOp,
		IsRefValue:    isRefValue,
	}, nil
}

func (p *Parser) tryParseElementFuncPredicate(negated bool) (Predicate, bool, error) {
	if p.curr.Type != TokenIdent || p.peek.Type != TokenLParen {
		return nil, false, nil
	}
	funcName := strings.ToLower(p.curr.Value)
	switch funcName {
	case "contains":
		p.advance()
		pred, err := p.parseStringFuncPredicate(negated, StringFuncIncludes)
		return pred, true, err
	case "startswith":
		p.advance()
		pred, err := p.parseStringFuncPredicate(negated, StringFuncStartsWith)
		return pred, true, err
	case "endswith":
		p.advance()
		pred, err := p.parseStringFuncPredicate(negated, StringFuncEndsWith)
		return pred, true, err
	case "matches":
		p.advance()
		pred, err := p.parseStringFuncPredicate(negated, StringFuncMatches)
		return pred, true, err
	default:
		return nil, false, nil
	}
}
