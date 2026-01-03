package query

import (
	"strings"
	"unicode"
)

// TokenType represents the type of a lexer token.
type TokenType int

const (
	TokenEOF      TokenType = iota
	TokenIdent              // identifiers like "object", "trait", "project", "due"
	TokenColon              // :
	TokenDot                // .
	TokenBang               // !
	TokenPipe               // |
	TokenLParen             // (
	TokenRParen             // )
	TokenLBrace             // {
	TokenRBrace             // }
	TokenLBracket           // [
	TokenRBracket           // ]
	TokenStar               // *
	TokenRef                // [[...]] reference
	TokenError              // error token
)

// Token represents a lexer token.
type Token struct {
	Type    TokenType
	Value   string
	Pos     int
	Literal string // original text for references
}

// Lexer tokenizes a query string.
type Lexer struct {
	input string
	pos   int
	start int
}

// NewLexer creates a new lexer for the given input.
func NewLexer(input string) *Lexer {
	return &Lexer{input: input}
}

// NextToken returns the next token from the input.
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	if l.pos >= len(l.input) {
		return Token{Type: TokenEOF, Pos: l.pos}
	}

	l.start = l.pos
	ch := l.input[l.pos]

	switch ch {
	case ':':
		l.pos++
		return Token{Type: TokenColon, Value: ":", Pos: l.start}
	case '.':
		l.pos++
		return Token{Type: TokenDot, Value: ".", Pos: l.start}
	case '!':
		l.pos++
		return Token{Type: TokenBang, Value: "!", Pos: l.start}
	case '|':
		l.pos++
		return Token{Type: TokenPipe, Value: "|", Pos: l.start}
	case '(':
		l.pos++
		return Token{Type: TokenLParen, Value: "(", Pos: l.start}
	case ')':
		l.pos++
		return Token{Type: TokenRParen, Value: ")", Pos: l.start}
	case '{':
		l.pos++
		return Token{Type: TokenLBrace, Value: "{", Pos: l.start}
	case '}':
		l.pos++
		return Token{Type: TokenRBrace, Value: "}", Pos: l.start}
	case '[':
		// Check for [[ reference
		if l.pos+1 < len(l.input) && l.input[l.pos+1] == '[' {
			return l.scanReference()
		}
		l.pos++
		return Token{Type: TokenLBracket, Value: "[", Pos: l.start}
	case ']':
		l.pos++
		return Token{Type: TokenRBracket, Value: "]", Pos: l.start}
	case '*':
		l.pos++
		return Token{Type: TokenStar, Value: "*", Pos: l.start}
	default:
		if isIdentStart(ch) {
			return l.scanIdent()
		}
		l.pos++
		return Token{Type: TokenError, Value: string(ch), Pos: l.start}
	}
}

func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) && unicode.IsSpace(rune(l.input[l.pos])) {
		l.pos++
	}
}

func (l *Lexer) scanIdent() Token {
	start := l.pos
	for l.pos < len(l.input) && isIdentChar(l.input[l.pos]) {
		l.pos++
	}
	value := l.input[start:l.pos]
	return Token{Type: TokenIdent, Value: value, Pos: start}
}

func (l *Lexer) scanReference() Token {
	start := l.pos
	// Skip [[
	l.pos += 2

	// Find closing ]]
	depth := 1
	for l.pos < len(l.input) && depth > 0 {
		if l.pos+1 < len(l.input) && l.input[l.pos] == ']' && l.input[l.pos+1] == ']' {
			depth--
			if depth == 0 {
				l.pos += 2
				break
			}
		}
		l.pos++
	}

	literal := l.input[start:l.pos]
	// Extract the reference path (without [[ and ]])
	value := strings.TrimPrefix(strings.TrimSuffix(literal, "]]"), "[[")
	return Token{Type: TokenRef, Value: value, Literal: literal, Pos: start}
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		ch == '_' ||
		ch == '-' ||
		(ch >= '0' && ch <= '9')
}

func isIdentChar(ch byte) bool {
	return isIdentStart(ch) || ch == '/' || ch == '#'
}
