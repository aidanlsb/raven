package parser

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/wikilink"
)

// TypeDeclaration represents a parsed ::type() declaration.
type TypeDeclaration struct {
	TypeName string
	ID       string
	Fields   map[string]schema.FieldValue
	Line     int
}

// EmbeddedTypeInfo contains simplified embedded type info for the document parser.
type EmbeddedTypeInfo struct {
	TypeName string
	ID       string
	Fields   map[string]schema.FieldValue
	// Line is the 1-indexed line number of the ::type(...) declaration in the file.
	// This is the declaration line (not the heading line).
	Line int
}

// typeDeclWithArgsRegex matches ::type-name(args...)
var typeDeclWithArgsRegex = regexp.MustCompile(`^::([\w-]+)\s*\(([^)]*)\)\s*$`)

// typeDeclNoArgsRegex matches ::type-name without parentheses (shorthand for ::typename())
var typeDeclNoArgsRegex = regexp.MustCompile(`^::([\w-]+)\s*$`)

// ParseEmbeddedType parses an embedded type declaration from a line.
// Returns nil if the line is not a type declaration.
// The ID field may be empty - the caller should derive it from the heading if so.
//
// Supports both forms:
//   - ::typename(field=value, ...) - with parentheses and optional fields
//   - ::typename - shorthand for ::typename() with no fields
func ParseEmbeddedType(line string, lineNumber int) *EmbeddedTypeInfo {
	decl, err := ParseTypeDeclaration(line, lineNumber)
	if err != nil || decl == nil {
		return nil
	}

	// ID is optional - caller will derive from heading if empty
	return &EmbeddedTypeInfo{
		TypeName: decl.TypeName,
		ID:       decl.ID,
		Fields:   decl.Fields,
		Line:     decl.Line,
	}
}

// ParseTypeDeclaration parses a type declaration from a line.
// Supports both ::typename(args...) and ::typename (without parentheses).
func ParseTypeDeclaration(line string, lineNumber int) (*TypeDeclaration, error) {
	trimmed := strings.TrimSpace(line)

	if !strings.HasPrefix(trimmed, "::") {
		return nil, nil
	}

	// Try matching with parentheses first
	if matches := typeDeclWithArgsRegex.FindStringSubmatch(trimmed); matches != nil {
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
			Line:     lineNumber,
		}, nil
	}

	// Try matching without parentheses (shorthand for ::typename())
	if matches := typeDeclNoArgsRegex.FindStringSubmatch(trimmed); matches != nil {
		typeName := matches[1]

		return &TypeDeclaration{
			TypeName: typeName,
			ID:       "",
			Fields:   make(map[string]schema.FieldValue),
			Line:     lineNumber,
		}, nil
	}

	return nil, fmt.Errorf("invalid type declaration syntax: %s", trimmed)
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

type valueParseOptions struct {
	strictDates   bool
	parseBooleans bool
	parseNumbers  bool
	parseArrays   bool
	stripQuotes   bool
}

// parseValue parses a single value.
func parseValue(s string) schema.FieldValue {
	return parseValueWithOptions(s, valueParseOptions{
		parseBooleans: true,
		parseNumbers:  true,
		parseArrays:   true,
		stripQuotes:   true,
	})
}

func parseValueWithOptions(s string, opts valueParseOptions) schema.FieldValue {
	s = strings.TrimSpace(s)

	if s == "" {
		return schema.Null()
	}

	// Reference [[target]] or [[target|display]] - exactly 2 opening and 2 closing brackets
	// Use wikilink.ParseExact to correctly handle aliases
	if !strings.HasPrefix(s, "[[[") {
		if target, _, ok := wikilink.ParseExact(s); ok {
			return schema.Ref(target)
		}
	}

	// Array (including array of refs like [[[a]], [[b]]])
	if opts.parseArrays && strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		inner := s[1 : len(s)-1]
		items := parseArrayItems(inner, opts)
		return schema.Array(items)
	}

	// Quoted string
	if opts.stripQuotes && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && len(s) >= 2 {
		return schema.String(s[1 : len(s)-1])
	}

	// Boolean
	if opts.parseBooleans {
		if s == "true" {
			return schema.Bool(true)
		}
		if s == "false" {
			return schema.Bool(false)
		}
	}

	// Number
	if opts.parseNumbers {
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			return schema.Number(n)
		}
	}

	// Date (YYYY-MM-DD) or datetime (YYYY-MM-DDTHH:MM)
	if opts.strictDates {
		if dates.IsValidDatetime(s) {
			return schema.Datetime(s)
		}
		if dates.IsValidDate(s) {
			return schema.Date(s)
		}
	} else if len(s) >= 10 && s[0] >= '0' && s[0] <= '9' {
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

// ParseFieldValue parses a single field value using the same rules as ::type() declarations.
func ParseFieldValue(s string) schema.FieldValue {
	return parseValueWithOptions(s, valueParseOptions{
		parseBooleans: true,
		parseNumbers:  true,
		parseArrays:   true,
		stripQuotes:   true,
	})
}

// ParseTraitValue parses a trait value using strict date/datetime validation.
func ParseTraitValue(s string) schema.FieldValue {
	return parseValueWithOptions(s, valueParseOptions{strictDates: true})
}

// parseArrayItems parses array items, handling nested references.
func parseArrayItems(s string, opts valueParseOptions) []schema.FieldValue {
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
				item := parseValueWithOptions(strings.TrimSpace(current.String()), opts)
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
		item := parseValueWithOptions(strings.TrimSpace(current.String()), opts)
		if !item.IsNull() {
			items = append(items, item)
		}
	}

	return items
}

// SerializeTypeDeclaration serializes a type declaration back to ::typename(fields) format.
func SerializeTypeDeclaration(typeName string, fields map[string]schema.FieldValue) string {
	if len(fields) == 0 {
		return "::" + typeName + "()"
	}

	// Collect and sort field names for consistent output
	var keys []string
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		v := fields[k]
		if !v.IsNull() {
			parts = append(parts, k+"="+serializeFieldValue(v))
		}
	}

	return "::" + typeName + "(" + strings.Join(parts, ", ") + ")"
}

// serializeFieldValue converts a FieldValue back to string format for embedded type declarations.
func serializeFieldValue(fv schema.FieldValue) string {
	if fv.IsNull() {
		return ""
	}

	// Check for reference first
	if ref, ok := fv.AsRef(); ok {
		return "[[" + ref + "]]"
	}

	// Check for array
	if arr, ok := fv.AsArray(); ok {
		var items []string
		for _, item := range arr {
			items = append(items, serializeFieldValue(item))
		}
		return "[" + strings.Join(items, ", ") + "]"
	}

	// Check for boolean
	if b, ok := fv.AsBool(); ok {
		if b {
			return "true"
		}
		return "false"
	}

	// Check for number
	if n, ok := fv.AsNumber(); ok {
		// Format without trailing zeros for integers
		if n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	}

	// String (may need quoting if it contains special chars)
	if s, ok := fv.AsString(); ok {
		return serializeString(s)
	}

	return ""
}

// serializeString returns a string value, quoting if necessary.
func serializeString(s string) string {
	// Quote if contains special characters that would break parsing
	needsQuote := strings.ContainsAny(s, ",()[]=\"")
	if needsQuote {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}
