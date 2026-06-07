package parser

import (
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/wikilink"
)

type valueParseOptions struct {
	strictDates   bool
	parseBooleans bool
	parseNumbers  bool
	parseArrays   bool
	stripQuotes   bool
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
			if dates.IsValidDatetime(s) {
				return schema.Datetime(s)
			}
		} else if len(s) == 10 && s[4] == '-' && s[7] == '-' {
			if dates.IsValidDate(s) {
				return schema.Date(s)
			}
		}
	}

	// Time only (HH:MM) - treat as string
	if len(s) == 5 && s[2] == ':' {
		return schema.String(s)
	}

	// Default to string
	return schema.String(s)
}

// ParseFieldValue parses a single Raven field literal.
func ParseFieldValue(s string) schema.FieldValue {
	return parseValueWithOptions(s, valueParseOptions{
		parseBooleans: true,
		parseNumbers:  true,
		parseArrays:   true,
		stripQuotes:   true,
	})
}

// ParseTraitValue parses a trait value using strict date/datetime validation.
// Traits have a single value slot, but that value can be an array.
func ParseTraitValue(s string) schema.FieldValue {
	return parseValueWithOptions(s, valueParseOptions{
		strictDates: true,
		parseArrays: true,
		stripQuotes: true,
	})
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
