package query

import (
	"testing"
)

func TestLexerErrorTokens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantType  TokenType
		wantValue string
	}{
		{
			name:      "unterminated string",
			input:     `"hello world`,
			wantType:  TokenError,
			wantValue: "unterminated string literal",
		},
		{
			name:      "unterminated regex",
			input:     `/^foo`,
			wantType:  TokenError,
			wantValue: "unterminated regex literal",
		},
		{
			name:      "unterminated raw string",
			input:     `r"hello`,
			wantType:  TokenError,
			wantValue: "unterminated raw string literal",
		},
		{
			name:      "malformed wikilink unclosed",
			input:     `[[foo`,
			wantType:  TokenError,
			wantValue: "[",
		},
		{
			name:      "unrecognized character",
			input:     `~`,
			wantType:  TokenError,
			wantValue: "~",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != tt.wantType {
				t.Errorf("expected type %v, got %v (value: %q)", tt.wantType, tok.Type, tok.Value)
			}
			if tok.Value != tt.wantValue {
				t.Errorf("expected value %q, got %q", tt.wantValue, tok.Value)
			}
		})
	}
}

func TestLexerWikilinks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantType  TokenType
		wantValue string
	}{
		{
			name:      "simple wikilink",
			input:     `[[project/website]]`,
			wantType:  TokenRef,
			wantValue: "project/website",
		},
		{
			name:      "wikilink with display text",
			input:     `[[person/freya|Freya]]`,
			wantType:  TokenRef,
			wantValue: "person/freya",
		},
		{
			name:      "wikilink with fragment",
			input:     `[[daily/2025-01-01#standup]]`,
			wantType:  TokenRef,
			wantValue: "daily/2025-01-01#standup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != tt.wantType {
				t.Errorf("expected type %v, got %v (value: %q)", tt.wantType, tok.Type, tok.Value)
			}
			if tok.Value != tt.wantValue {
				t.Errorf("expected value %q, got %q", tt.wantValue, tok.Value)
			}
		})
	}
}

func TestLexerComparisonOperators(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantType  TokenType
		wantValue string
	}{
		{"less than", "<", TokenLt, "<"},
		{"less than or equal", "<=", TokenLte, "<="},
		{"greater than", ">", TokenGt, ">"},
		{"greater than or equal", ">=", TokenGte, ">="},
		{"equals", "==", TokenEqEq, "=="},
		{"not equals", "!=", TokenBangEq, "!="},
		{"single equals", "=", TokenEq, "="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != tt.wantType {
				t.Errorf("expected type %v, got %v", tt.wantType, tok.Type)
			}
			if tok.Value != tt.wantValue {
				t.Errorf("expected value %q, got %q", tt.wantValue, tok.Value)
			}
		})
	}
}

func TestLexerRawStrings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantValue string
	}{
		{
			name:      "raw string with backslash",
			input:     `r"foo\bar"`,
			wantValue: `foo\bar`,
		},
		{
			name:      "raw string empty",
			input:     `r""`,
			wantValue: "",
		},
		{
			name:      "raw string with special chars",
			input:     `r"^[a-z]+\d{3}$"`,
			wantValue: `^[a-z]+\d{3}$`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != TokenString {
				t.Errorf("expected TokenString, got %v (value: %q)", tok.Type, tok.Value)
			}
			if tok.Value != tt.wantValue {
				t.Errorf("expected value %q, got %q", tt.wantValue, tok.Value)
			}
		})
	}
}

func TestLexerStringEscaping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantValue string
	}{
		{
			name:      "escaped quote inside string",
			input:     `"say \"hello\""`,
			wantValue: `say "hello"`,
		},
		{
			name:      "simple string",
			input:     `"hello world"`,
			wantValue: "hello world",
		},
		{
			name:      "empty string",
			input:     `""`,
			wantValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != TokenString {
				t.Errorf("expected TokenString, got %v (value: %q)", tok.Type, tok.Value)
			}
			if tok.Value != tt.wantValue {
				t.Errorf("expected value %q, got %q", tt.wantValue, tok.Value)
			}
		})
	}
}

func TestLexerWhitespaceHandling(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		input      string
		wantTokens []Token
	}{
		{
			name:  "leading whitespace skipped",
			input: "   object",
			wantTokens: []Token{
				{Type: TokenIdent, Value: "object", Pos: 3},
				{Type: TokenEOF},
			},
		},
		{
			name:  "multiple spaces between tokens",
			input: "a    b",
			wantTokens: []Token{
				{Type: TokenIdent, Value: "a", Pos: 0},
				{Type: TokenIdent, Value: "b", Pos: 5},
				{Type: TokenEOF},
			},
		},
		{
			name:  "tabs and newlines treated as whitespace",
			input: "a\t\nb",
			wantTokens: []Token{
				{Type: TokenIdent, Value: "a", Pos: 0},
				{Type: TokenIdent, Value: "b", Pos: 3},
				{Type: TokenEOF},
			},
		},
		{
			name:  "empty input",
			input: "",
			wantTokens: []Token{
				{Type: TokenEOF},
			},
		},
		{
			name:  "whitespace only",
			input: "   \t  ",
			wantTokens: []Token{
				{Type: TokenEOF},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			for i, expected := range tt.wantTokens {
				tok := lexer.NextToken()
				if tok.Type != expected.Type {
					t.Errorf("token %d: expected type %v, got %v (value: %q)", i, expected.Type, tok.Type, tok.Value)
				}
				if expected.Value != "" && tok.Value != expected.Value {
					t.Errorf("token %d: expected value %q, got %q", i, expected.Value, tok.Value)
				}
				if expected.Pos != 0 && tok.Pos != expected.Pos {
					t.Errorf("token %d: expected pos %d, got %d", i, expected.Pos, tok.Pos)
				}
			}
		})
	}
}

func TestLexerUnderscoreToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantType  TokenType
		wantValue string
	}{
		{
			name:      "standalone underscore",
			input:     "_",
			wantType:  TokenUnderscore,
			wantValue: "_",
		},
		{
			name:      "underscore before dot",
			input:     "_.",
			wantType:  TokenUnderscore,
			wantValue: "_",
		},
		{
			name:      "underscore as identifier prefix",
			input:     "_foo",
			wantType:  TokenIdent,
			wantValue: "_foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tok := lexer.NextToken()
			if tok.Type != tt.wantType {
				t.Errorf("expected type %v, got %v (value: %q)", tt.wantType, tok.Type, tok.Value)
			}
			if tok.Value != tt.wantValue {
				t.Errorf("expected value %q, got %q", tt.wantValue, tok.Value)
			}
		})
	}
}

func TestLexerNewTokens(t *testing.T) {
	t.Parallel()
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
			name:  "contains function syntax",
			input: `contains(.name, "website")`,
			tokens: []Token{
				{Type: TokenIdent, Value: "contains"},
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
