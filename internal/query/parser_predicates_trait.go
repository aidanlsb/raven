package query

import (
	"fmt"
	"strings"
)

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

// parseOnPredicate parses on:{object:type ...}, on:[[target]], or on:_
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

// parseWithinPredicate parses within:{object:type ...}, within:[[target]], or within:_
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
