package query

import (
	"fmt"
	"strings"
)

// parseFieldPathFromIdent reads one or more dotted field names. The current
// token must already be the first identifier.
func (p *Parser) parseFieldPathFromIdent() (FieldPath, error) {
	if p.curr.Type != TokenIdent {
		return FieldPath{}, fmt.Errorf("expected field name after '.'")
	}
	fields, err := splitDottedIdent(p.curr.Value)
	if err != nil {
		return FieldPath{}, err
	}
	p.advance()
	for p.curr.Type == TokenDot {
		p.advance()
		if p.curr.Type != TokenIdent {
			return FieldPath{}, fmt.Errorf("expected field name after '.'")
		}
		more, err := splitDottedIdent(p.curr.Value)
		if err != nil {
			return FieldPath{}, err
		}
		fields = append(fields, more...)
		p.advance()
	}
	return FieldPath{Fields: fields}, nil
}

func (p *Parser) parseThisPath() (FieldPath, error) {
	if !isThisKeyword(p.curr) {
		return FieldPath{}, fmt.Errorf("expected 'this'")
	}
	raw := p.curr.Value
	p.advance()
	path := FieldPath{This: true}
	if strings.Contains(raw, ".") {
		_, rest, ok := strings.Cut(raw, ".")
		if !ok || rest == "" {
			return FieldPath{}, fmt.Errorf("expected field name after 'this.'")
		}
		fields, err := splitDottedIdent(rest)
		if err != nil {
			return FieldPath{}, err
		}
		path.Fields = fields
	}
	return path, nil
}

func (p *Parser) parseFieldOrThisPath() (FieldPath, error) {
	if isThisKeyword(p.curr) {
		return p.parseThisPath()
	}
	if p.curr.Type != TokenDot {
		return FieldPath{}, fmt.Errorf("expected .field or this")
	}
	p.advance()
	return p.parseFieldPathFromIdent()
}

func fieldPredicateFromPath(negated bool, path FieldPath, compareOp CompareOp, rhs CompareRHS) *FieldPredicate {
	pred := &FieldPredicate{
		basePredicate: basePredicate{negated: negated},
		Path:          path,
		Field:         path.first(),
		CompareOp:     compareOp,
		RHS:           rhs,
		Value:         rhs.Literal,
		IsRefValue:    rhs.IsRefValue,
	}
	return pred
}

// parseFieldPredicate parses .field==value, .project.status==active, etc.
// The leading '.' has already been consumed.
func (p *Parser) parseFieldPredicate(negated bool) (Predicate, error) {
	path, err := p.parseFieldPathFromIdent()
	if err != nil {
		return nil, err
	}
	return p.parseFieldPredicateAfterPath(negated, path)
}

func (p *Parser) parseThisPredicate(negated bool) (Predicate, error) {
	path, err := p.parseThisPath()
	if err != nil {
		return nil, err
	}
	return p.parseFieldPredicateAfterPath(negated, path)
}

func (p *Parser) parseFieldPredicateAfterPath(negated bool, path FieldPath) (Predicate, error) {
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
		return nil, fmt.Errorf("expected comparison operator (==, !=, <, >, <=, >=) after %s; for string matching use includes(), startswith(), endswith(), or matches()", path.String())
	}

	rhs, err := p.parseCompareRHS()
	if err != nil {
		return nil, err
	}
	return fieldPredicateFromPath(negated, path, compareOp, rhs), nil
}

func (p *Parser) parseCompareRHS() (CompareRHS, error) {
	switch {
	case p.curr.Type == TokenStar:
		return CompareRHS{}, fmt.Errorf(".field==* is no longer supported; use exists(.field) or !exists(.field) instead")
	case isThisKeyword(p.curr):
		path, err := p.parseThisPath()
		if err != nil {
			return CompareRHS{}, err
		}
		return pathRHS(path), nil
	case p.curr.Type == TokenDot:
		p.advance()
		path, err := p.parseFieldPathFromIdent()
		if err != nil {
			return CompareRHS{}, err
		}
		return pathRHS(path), nil
	case p.curr.Type == TokenIdent:
		value := p.curr.Value
		p.advance()
		return literalRHS(value, false), nil
	case p.curr.Type == TokenRef:
		value := p.curr.Value
		p.advance()
		return literalRHS(value, true), nil
	case p.curr.Type == TokenString:
		value := p.curr.Value
		p.advance()
		return literalRHS(value, false), nil
	default:
		return CompareRHS{}, fmt.Errorf("expected field value, path, this, or quoted string; for field presence use exists(.field)")
	}
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
		case TokenDot:
			return nil, fmt.Errorf("oneof() values must be literals; quote dotted strings, for example \".pdf\"")
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

// parseInPredicate parses: oneof(.field, [a,b,"c"])
// This is useful for scalar membership checks like trait:due oneof(.value, [today,tomorrow]).
func (p *Parser) parseInPredicate(negated bool) (Predicate, error) {
	if err := p.expect(TokenLParen); err != nil {
		return nil, err
	}

	// First arg: .field, .field.hop, or this.field
	path, err := p.parseFieldOrThisPath()
	if err != nil {
		return nil, fmt.Errorf("expected .field as first argument to oneof()")
	}

	if err := p.expect(TokenComma); err != nil {
		return nil, err
	}

	if p.curr.Type != TokenLBracket {
		return nil, fmt.Errorf("expected '[' to start value list as second argument to oneof()")
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
		return fieldPredicateFromPath(negated, path, CompareEq, literalRHS(values[0].Value, values[0].IsRef)), nil
	}

	// Multiple values: OR of == predicates.
	var preds []Predicate
	for _, v := range values {
		preds = append(preds, fieldPredicateFromPath(false, path, CompareEq, literalRHS(v.Value, v.IsRef)))
	}
	var result Predicate = &OrPredicate{Predicates: preds}
	if negated {
		result = &NotPredicate{Inner: result}
	}
	return result, nil
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

	// Parse first argument: .field, .field.hop, this.field, or _
	switch {
	case isThisKeyword(p.curr) || p.curr.Type == TokenDot:
		path, err := p.parseFieldOrThisPath()
		if err != nil {
			return nil, err
		}
		pred.Path = path
		pred.Field = path.first()
	case p.curr.Type == TokenUnderscore:
		pred.Field = "_"
		pred.IsElementRef = true
		p.advance()
	default:
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

	path, err := p.parseFieldOrThisPath()
	if err != nil {
		return nil, fmt.Errorf("expected .field as first argument to %s()", quantifier)
	}
	pred.Path = path
	pred.Field = path.first()

	// Expect comma
	if err := p.expect(TokenComma); err != nil {
		return nil, err
	}

	// Parse second argument: element predicate (supports AND/OR)
	restore := p.disableSort()
	elementPred, err := p.parseElementOrPredicate()
	restore()
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
		pipePos := p.curr.Pos
		p.advance()
		if looksLikeShellPipeCommand(p.curr, p.peek) {
			return nil, shellPipeQueryError(pipePos)
		}
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
// Supports: _ == value, _ != value, includes(_, "str"), etc.
func (p *Parser) parseElementUnaryPredicate() (Predicate, error) {
	negated := false
	if p.curr.Type == TokenBang {
		negated = true
		p.advance()
	}

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

	if p.curr.Type == TokenUnderscore {
		return p.parseElementUnderscoreEquality(negated)
	}

	if p.curr.Type == TokenDot {
		p.advance()
		return p.parseFieldPredicate(negated)
	}

	if isThisKeyword(p.curr) {
		return p.parseThisPredicate(negated)
	}

	if p.curr.Type == TokenIdent {
		if p.isSortCall() {
			return nil, sortTopLevelError()
		}
		if pred, ok, err := p.tryParseElementFuncPredicate(negated); ok || err != nil {
			return pred, err
		}
	}

	return nil, fmt.Errorf("expected element predicate: _ == value, .field==value, this, or a function like includes(_, \"str\")")
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

	rhs, err := p.parseCompareRHS()
	if err != nil {
		return nil, err
	}

	return &ElementEqualityPredicate{
		basePredicate: basePredicate{negated: negated},
		Value:         rhs.Literal,
		RHS:           rhs,
		CompareOp:     compareOp,
		IsRefValue:    rhs.IsRefValue,
	}, nil
}

func (p *Parser) tryParseElementFuncPredicate(negated bool) (Predicate, bool, error) {
	if p.curr.Type != TokenIdent || p.peek.Type != TokenLParen {
		return nil, false, nil
	}
	funcName := strings.ToLower(p.curr.Value)
	switch funcName {
	case "includes":
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
