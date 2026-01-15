package query

import "fmt"

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
		return nil, fmt.Errorf("content: requires a quoted string, e.g., content:\"search term\"")
	}

	searchTerm := p.curr.Value
	p.advance()

	return &ContentPredicate{
		basePredicate: basePredicate{negated: negated},
		SearchTerm:    searchTerm,
	}, nil
}
