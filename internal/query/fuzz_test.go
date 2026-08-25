package query

import (
	"strings"
	"testing"
)

// FuzzLexer tests that the RQL lexer never panics on arbitrary input.
func FuzzLexer(f *testing.F) {
	// Seed corpus with valid and edge-case RQL queries
	f.Add("object:task")
	f.Add("trait:due")
	f.Add("[[books/dune]]")
	f.Add("object:task | trait:done")
	f.Add(`content:"search text"`)
	f.Add("object:task.status = done")
	f.Add("trait:due < 2026-08-25")
	f.Add("object:task |> count")
	f.Add("")
	f.Add("   ")
	f.Add("[[unclosed")
	f.Add(`"unclosed string`)
	f.Add("/unclosed regex")
	f.Add(strings.Repeat("a", 10000))
	f.Add(strings.Repeat("[[", 1000))
	f.Add(strings.Repeat(`"`, 100))
	f.Add("!!!!!!!")
	f.Add("<<<>>>")
	f.Add("(((())))")
	f.Add("{{{}}}[[[]]]")

	f.Fuzz(func(t *testing.T, input string) {
		// The lexer must not panic on any input
		lexer := NewLexer(input)

		// Tokenize the entire input
		tokenCount := 0
		maxTokens := 100000 // Prevent infinite loops
		for tokenCount < maxTokens {
			tok := lexer.NextToken()
			if tok.Type == TokenEOF {
				break
			}
			if tok.Type == TokenError {
				// Errors are fine, we just want no panics
				break
			}
			tokenCount++
		}

		// Ensure we eventually reach EOF or error
		if tokenCount >= maxTokens {
			t.Errorf("Lexer produced too many tokens without EOF")
		}
	})
}

// FuzzLexerTokenTypes tests that specific token types are lexed correctly without panicking.
func FuzzLexerTokenTypes(f *testing.F) {
	f.Add("identifier123")
	f.Add("[[reference]]")
	f.Add(`"string literal"`)
	f.Add("/regex pattern/")
	f.Add("<=>=<>===!=")
	f.Add("(){}[]")
	f.Add(":|!*.,")
	f.Add("|>")
	f.Add("_")

	f.Fuzz(func(t *testing.T, input string) {
		lexer := NewLexer(input)

		// Lex all tokens
		var tokens []Token
		for {
			tok := lexer.NextToken()
			tokens = append(tokens, tok)
			if tok.Type == TokenEOF || tok.Type == TokenError {
				break
			}
			if len(tokens) > 10000 {
				break
			}
		}

		// Basic invariants
		if len(tokens) == 0 {
			t.Error("Lexer returned no tokens")
		}

		// Last token should be EOF or Error
		lastToken := tokens[len(tokens)-1]
		if lastToken.Type != TokenEOF && lastToken.Type != TokenError {
			t.Errorf("Last token type = %v, want EOF or Error", lastToken.Type)
		}
	})
}

// FuzzLexerReferences tests that wikilink-style references are handled safely.
func FuzzLexerReferences(f *testing.F) {
	f.Add("[[simple]]")
	f.Add("[[with/path]]")
	f.Add("[[with#section]]")
	f.Add("[[display|target]]")
	f.Add("[[")
	f.Add("]]")
	f.Add("[[]]")
	f.Add(strings.Repeat("[", 100))
	f.Add("[[" + strings.Repeat("a", 10000) + "]]")

	f.Fuzz(func(t *testing.T, input string) {
		lexer := NewLexer(input)

		for i := 0; i < 1000; i++ {
			tok := lexer.NextToken()

			// Verify token positions are sane
			if tok.Pos < 0 {
				t.Errorf("Token pos = %d, want >= 0", tok.Pos)
			}
			// Token.Value contains the matched text
			if tok.Pos+len(tok.Value) > len(input) {
				t.Errorf("Token pos+value length = %d > input length = %d", tok.Pos+len(tok.Value), len(input))
			}

			if tok.Type == TokenEOF || tok.Type == TokenError {
				break
			}
		}
	})
}

// FuzzLexerStrings tests string literal lexing.
func FuzzLexerStrings(f *testing.F) {
	f.Add(`"simple string"`)
	f.Add(`"string with \"escapes\""`)
	f.Add(`"unclosed string`)
	f.Add(`""`)
	f.Add(`"`)
	f.Add(strings.Repeat(`"`, 100))
	f.Add(`"` + strings.Repeat("a", 10000) + `"`)

	f.Fuzz(func(t *testing.T, input string) {
		lexer := NewLexer(input)
		tok := lexer.NextToken()

		// Should always return some token (string, error, or EOF)
		if tok.Type != TokenString && tok.Type != TokenError && tok.Type != TokenEOF {
			// Could be other tokens if input doesn't start with quote
			return
		}

		// Positions should be valid
		if tok.Pos < 0 || tok.Pos > len(input) {
			t.Errorf("Invalid token position: pos=%d input_len=%d", tok.Pos, len(input))
		}
	})
}

// FuzzLexerOperators tests operator lexing.
func FuzzLexerOperators(f *testing.F) {
	f.Add("=")
	f.Add("==")
	f.Add("!=")
	f.Add("<")
	f.Add(">")
	f.Add("<=")
	f.Add(">=")
	f.Add("|>")
	f.Add("=====")
	f.Add("<><><>")

	f.Fuzz(func(t *testing.T, input string) {
		lexer := NewLexer(input)

		// Lex all tokens
		prevPos := 0
		for i := 0; i < 100; i++ {
			tok := lexer.NextToken()

			// Tokens should advance
			if tok.Pos < prevPos {
				t.Errorf("Token pos = %d < previous pos = %d", tok.Pos, prevPos)
			}
			prevPos = tok.Pos + len(tok.Value)

			if tok.Type == TokenEOF {
				break
			}
		}
	})
}
