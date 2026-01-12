package query

import (
	"strings"
	"unicode"

	"github.com/aidanlsb/raven/internal/wikilink"
)

// TokenType represents the type of a lexer token.
type TokenType int

const (
	TokenEOF        TokenType = iota
	TokenIdent                // identifiers like "object", "trait", "project", "due"
	TokenColon                // :
	TokenDot                  // .
	TokenBang                 // !
	TokenPipe                 // |
	TokenLParen               // (
	TokenRParen               // )
	TokenLBrace               // {
	TokenRBrace               // }
	TokenLBracket             // [
	TokenRBracket             // ]
	TokenStar                 // *
	TokenRef                  // [[...]] reference
	TokenString               // "quoted string" for content search
	TokenUnderscore           // _ (result reference)
	TokenLt                   // <
	TokenGt                   // >
	TokenLte                  // <=
	TokenGte                  // >=
	TokenError                // error token
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
	case '<':
		l.pos++
		if l.pos < len(l.input) && l.input[l.pos] == '=' {
			l.pos++
			return Token{Type: TokenLte, Value: "<=", Pos: l.start}
		}
		return Token{Type: TokenLt, Value: "<", Pos: l.start}
	case '>':
		l.pos++
		if l.pos < len(l.input) && l.input[l.pos] == '=' {
			l.pos++
			return Token{Type: TokenGte, Value: ">=", Pos: l.start}
		}
		return Token{Type: TokenGt, Value: ">", Pos: l.start}
	case '"':
		return l.scanString()
	case '_':
		// Check if it's a standalone _ or part of an identifier
		// Standalone _ is followed by nothing, whitespace, '.', ':', or EOF
		if l.pos+1 >= len(l.input) || !isIdentCharAfterUnderscore(l.input[l.pos+1]) {
			l.pos++
			return Token{Type: TokenUnderscore, Value: "_", Pos: l.start}
		}
		// Otherwise it's part of an identifier
		return l.scanIdent()
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
	end, target, literal, ok := wikilink.ScanAt(l.input, start)
	if !ok {
		// Consume one byte to avoid infinite loops on malformed input.
		l.pos++
		return Token{Type: TokenError, Value: l.input[start:l.pos], Pos: start}
	}
	l.pos = end
	return Token{Type: TokenRef, Value: target, Literal: literal, Pos: start}
}

func (l *Lexer) scanString() Token {
	start := l.pos
	// Skip opening quote
	l.pos++

	// Find closing quote
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == '"' {
			l.pos++
			break
		}
		// Handle escaped quotes
		if ch == '\\' && l.pos+1 < len(l.input) {
			l.pos += 2
			continue
		}
		l.pos++
	}

	literal := l.input[start:l.pos]
	// Extract the string content (without quotes)
	value := strings.TrimPrefix(strings.TrimSuffix(literal, "\""), "\"")
	// Unescape escaped quotes
	value = strings.ReplaceAll(value, "\\\"", "\"")
	return Token{Type: TokenString, Value: value, Literal: literal, Pos: start}
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

// isIdentCharAfterUnderscore returns true if ch would make a leading underscore
// part of an identifier. We want standalone _ to be TokenUnderscore when followed
// by '.', ':', whitespace, or EOF, but part of an identifier if followed by letters/digits.
func isIdentCharAfterUnderscore(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_' ||
		ch == '-'
}
