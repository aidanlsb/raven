package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ravenscroftj/raven/internal/schema"
)

// TypeDeclaration represents a parsed ::type() declaration.
type TypeDeclaration struct {
	TypeName string
	ID       string
	Fields   map[string]schema.FieldValue
	Tags     []string
	Line     int
}

// EmbeddedTypeInfo contains simplified embedded type info for the document parser.
type EmbeddedTypeInfo struct {
	TypeName string
	ID       string
	Fields   map[string]schema.FieldValue
	Tags     []string
}

// typeDeclRegex matches ::typename(args...)
var typeDeclRegex = regexp.MustCompile(`^::(\w+)\s*\(([^)]*)\)\s*$`)

// ParseEmbeddedType parses an embedded type declaration from a line.
// Returns nil if the line is not a type declaration or has no ID.
func ParseEmbeddedType(line string, lineNumber int) *EmbeddedTypeInfo {
	decl, err := ParseTypeDeclaration(line, lineNumber)
	if err != nil || decl == nil {
		return nil
	}

	// Embedded types must have an ID
	if decl.ID == "" {
		return nil
	}

	return &EmbeddedTypeInfo{
		TypeName: decl.TypeName,
		ID:       decl.ID,
		Fields:   decl.Fields,
		Tags:     decl.Tags,
	}
}

// ParseTypeDeclaration parses a type declaration from a line.
func ParseTypeDeclaration(line string, lineNumber int) (*TypeDeclaration, error) {
	trimmed := strings.TrimSpace(line)

	if !strings.HasPrefix(trimmed, "::") {
		return nil, nil
	}

	matches := typeDeclRegex.FindStringSubmatch(trimmed)
	if matches == nil {
		return nil, fmt.Errorf("invalid type declaration syntax: %s", trimmed)
	}

	typeName := matches[1]
	argsStr := matches[2]

	fields, err := parseArguments(argsStr)
	if err != nil {
		return nil, err
	}

	// Extract ID from fields
	var id string
	if idVal, ok := fields["id"]; ok {
		if s, ok := idVal.AsString(); ok {
			id = s
		}
	}

	return &TypeDeclaration{
		TypeName: typeName,
		ID:       id,
		Fields:   fields,
		Tags:     []string{}, // TODO: extract inline tags
		Line:     lineNumber,
	}, nil
}

// parseArguments parses comma-separated key=value arguments.
func parseArguments(args string) (map[string]schema.FieldValue, error) {
	fields := make(map[string]schema.FieldValue)

	if strings.TrimSpace(args) == "" {
		return fields, nil
	}

	// Simple state machine to handle nested brackets and quotes
	var currentKey strings.Builder
	var currentValue strings.Builder
	inKey := true
	inQuotes := false
	bracketDepth := 0

	for _, c := range args {
		switch c {
		case '"':
			if bracketDepth == 0 {
				inQuotes = !inQuotes
			}
			currentValue.WriteRune(c)

		case '[':
			if !inQuotes {
				bracketDepth++
			}
			currentValue.WriteRune(c)

		case ']':
			if !inQuotes {
				bracketDepth--
			}
			currentValue.WriteRune(c)

		case '=':
			if !inQuotes && bracketDepth == 0 && inKey {
				inKey = false
			} else if inKey {
				currentKey.WriteRune(c)
			} else {
				currentValue.WriteRune(c)
			}

		case ',':
			if !inQuotes && bracketDepth == 0 {
				// End of argument
				key := strings.TrimSpace(currentKey.String())
				if key != "" {
					value := parseValue(strings.TrimSpace(currentValue.String()))
					fields[key] = value
				}
				currentKey.Reset()
				currentValue.Reset()
				inKey = true
			} else {
				currentValue.WriteRune(c)
			}

		default:
			if inKey {
				currentKey.WriteRune(c)
			} else {
				currentValue.WriteRune(c)
			}
		}
	}

	// Handle last argument
	key := strings.TrimSpace(currentKey.String())
	if key != "" {
		value := parseValue(strings.TrimSpace(currentValue.String()))
		fields[key] = value
	}

	return fields, nil
}

// parseValue parses a single value.
func parseValue(s string) schema.FieldValue {
	s = strings.TrimSpace(s)

	if s == "" {
		return schema.Null()
	}

	// Reference [[...]] - exactly 2 opening and 2 closing brackets
	if strings.HasPrefix(s, "[[") && !strings.HasPrefix(s, "[[[") && strings.HasSuffix(s, "]]") {
		return schema.Ref(s[2 : len(s)-2])
	}

	// Array (including array of refs like [[[a]], [[b]]])
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		inner := s[1 : len(s)-1]
		items := parseArrayItems(inner)
		return schema.Array(items)
	}

	// Quoted string
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && len(s) >= 2 {
		return schema.String(s[1 : len(s)-1])
	}

	// Boolean
	if s == "true" {
		return schema.Bool(true)
	}
	if s == "false" {
		return schema.Bool(false)
	}

	// Number
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return schema.Number(n)
	}

	// Date (YYYY-MM-DD) or datetime (YYYY-MM-DDTHH:MM)
	if len(s) >= 10 && s[0] >= '0' && s[0] <= '9' {
		if strings.Contains(s, "T") {
			return schema.Datetime(s)
		}
		if len(s) == 10 && s[4] == '-' && s[7] == '-' {
			return schema.Date(s)
		}
	}

	// Time only (HH:MM) - treat as string
	if len(s) == 5 && s[2] == ':' {
		return schema.String(s)
	}

	// Default to string
	return schema.String(s)
}

// parseArrayItems parses array items, handling nested references.
func parseArrayItems(s string) []schema.FieldValue {
	var items []schema.FieldValue
	var current strings.Builder
	bracketDepth := 0
	inQuotes := false

	for _, c := range s {
		switch c {
		case '"':
			inQuotes = !inQuotes
			current.WriteRune(c)

		case '[':
			if !inQuotes {
				bracketDepth++
			}
			current.WriteRune(c)

		case ']':
			if !inQuotes {
				bracketDepth--
			}
			current.WriteRune(c)

		case ',':
			if !inQuotes && bracketDepth == 0 {
				item := parseValue(strings.TrimSpace(current.String()))
				if !item.IsNull() {
					items = append(items, item)
				}
				current.Reset()
			} else {
				current.WriteRune(c)
			}

		default:
			current.WriteRune(c)
		}
	}

	// Handle last item
	if current.Len() > 0 {
		item := parseValue(strings.TrimSpace(current.String()))
		if !item.IsNull() {
			items = append(items, item)
		}
	}

	return items
}
