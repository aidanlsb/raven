package query

import "testing"

func TestLexerDottedUnquotedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		tokens []Token
	}{
		{
			name:  "decimal number is one identifier",
			input: "4.5",
			tokens: []Token{
				{Type: TokenIdent, Value: "4.5"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "negative decimal is one identifier",
			input: "-0.5",
			tokens: []Token{
				{Type: TokenIdent, Value: "-0.5"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "dotted version is one identifier",
			input: "v0.0.40",
			tokens: []Token{
				{Type: TokenIdent, Value: "v0.0.40"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "field access still starts with a dot token",
			input: ".rating",
			tokens: []Token{
				{Type: TokenDot, Value: "."},
				{Type: TokenIdent, Value: "rating"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "comparison keeps field access separate from decimal value",
			input: ".rating > 4.5",
			tokens: []Token{
				{Type: TokenDot, Value: "."},
				{Type: TokenIdent, Value: "rating"},
				{Type: TokenGt, Value: ">"},
				{Type: TokenIdent, Value: "4.5"},
				{Type: TokenEOF},
			},
		},
		{
			name:  "value starting with a dot stays a value after the operator",
			input: ".ext == .pdf",
			tokens: []Token{
				{Type: TokenDot, Value: "."},
				{Type: TokenIdent, Value: "ext"},
				{Type: TokenEqEq, Value: "=="},
				{Type: TokenDot, Value: "."},
				{Type: TokenIdent, Value: "pdf"},
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
					t.Fatalf("token %d: type = %v, want %v (value %q)", i, tok.Type, expected.Type, tok.Value)
				}
				if expected.Value != "" && tok.Value != expected.Value {
					t.Fatalf("token %d: value = %q, want %q", i, tok.Value, expected.Value)
				}
			}
		})
	}
}
