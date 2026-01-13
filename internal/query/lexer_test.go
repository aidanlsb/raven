package query

import (
	"testing"
)

func TestLexerNewTokens(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		tokens []Token
	}{
		{
			name:  "equals operator",
			input: "==",
			tokens: []Token{
				{Type: TokenEqEq, Value: "=="},
				{Type: TokenEOF},
			},
		},
		{
			name:  "not equals operator",
			input: "!=",
			tokens: []Token{
				{Type: TokenBangEq, Value: "!="},
				{Type: TokenEOF},
			},
		},
		{
			name:  "pipeline operator",
			input: "|>",
			tokens: []Token{
				{Type: TokenPipeline, Value: "|>"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "assignment operator",
			input: "=",
			tokens: []Token{
				{Type: TokenEq, Value: "="},
				{Type: TokenEOF},
			},
		},
		{
			name:  "comma",
			input: ",",
			tokens: []Token{
				{Type: TokenComma, Value: ","},
				{Type: TokenEOF},
			},
		},
		{
			name:  "regex literal",
			input: "/^web.*api$/",
			tokens: []Token{
				{Type: TokenRegex, Value: "^web.*api$"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "regex with escaped slash",
			input: `/foo\/bar/`,
			tokens: []Token{
				{Type: TokenRegex, Value: "foo/bar"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "bang still works alone",
			input: "!has",
			tokens: []Token{
				{Type: TokenBang, Value: "!"},
				{Type: TokenIdent, Value: "has"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "pipe still works alone",
			input: "a | b",
			tokens: []Token{
				{Type: TokenIdent, Value: "a"},
				{Type: TokenPipe, Value: "|"},
				{Type: TokenIdent, Value: "b"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "field equality v2 syntax",
			input: ".status==active",
			tokens: []Token{
				{Type: TokenDot, Value: "."},
				{Type: TokenIdent, Value: "status"},
				{Type: TokenEqEq, Value: "=="},
				{Type: TokenIdent, Value: "active"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "includes function syntax",
			input: `includes(.name, "website")`,
			tokens: []Token{
				{Type: TokenIdent, Value: "includes"},
				{Type: TokenLParen, Value: "("},
				{Type: TokenDot, Value: "."},
				{Type: TokenIdent, Value: "name"},
				{Type: TokenComma, Value: ","},
				{Type: TokenString, Value: "website"},
				{Type: TokenRParen, Value: ")"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "matches function syntax",
			input: `matches(.name, "^api")`,
			tokens: []Token{
				{Type: TokenIdent, Value: "matches"},
				{Type: TokenLParen, Value: "("},
				{Type: TokenDot, Value: "."},
				{Type: TokenIdent, Value: "name"},
				{Type: TokenComma, Value: ","},
				{Type: TokenString, Value: "^api"},
				{Type: TokenRParen, Value: ")"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "raw string",
			input: `r"foo\bar"`,
			tokens: []Token{
				{Type: TokenString, Value: `foo\bar`},
				{Type: TokenEOF},
			},
		},
		{
			name:  "pipeline with sort",
			input: "|> sort(.name, asc)",
			tokens: []Token{
				{Type: TokenPipeline, Value: "|>"},
				{Type: TokenIdent, Value: "sort"},
				{Type: TokenLParen, Value: "("},
				{Type: TokenDot, Value: "."},
				{Type: TokenIdent, Value: "name"},
				{Type: TokenComma, Value: ","},
				{Type: TokenIdent, Value: "asc"},
				{Type: TokenRParen, Value: ")"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "assignment with count",
			input: "todos = count",
			tokens: []Token{
				{Type: TokenIdent, Value: "todos"},
				{Type: TokenEq, Value: "="},
				{Type: TokenIdent, Value: "count"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "comparison operators still work",
			input: ".priority>5 .created>=2025",
			tokens: []Token{
				{Type: TokenDot, Value: "."},
				{Type: TokenIdent, Value: "priority"},
				{Type: TokenGt, Value: ">"},
				{Type: TokenIdent, Value: "5"},
				{Type: TokenDot, Value: "."},
				{Type: TokenIdent, Value: "created"},
				{Type: TokenGte, Value: ">="},
				{Type: TokenIdent, Value: "2025"},
				{Type: TokenEOF},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			for i, expected := range tt.tokens {
				tok := lexer.NextToken()
				if tok.Type != expected.Type {
					t.Errorf("token %d: expected type %v, got %v (value: %q)", i, expected.Type, tok.Type, tok.Value)
				}
				if expected.Value != "" && tok.Value != expected.Value {
					t.Errorf("token %d: expected value %q, got %q", i, expected.Value, tok.Value)
				}
			}
		})
	}
}
