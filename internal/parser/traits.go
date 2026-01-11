package parser

import (
	"regexp"
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
)

// TraitAnnotation represents a parsed @trait() annotation.
type TraitAnnotation struct {
	TraitName string
	// Value is the single trait value (nil for boolean traits like @highlight)
	Value       *schema.FieldValue
	Content     string // Content after the trait on the same line
	Line        int
	StartOffset int
	EndOffset   int
}

// HasValue returns true if this trait has a value.
func (t *TraitAnnotation) HasValue() bool {
	return t.Value != nil && !t.Value.IsNull()
}

// ValueString returns the value as a string, or empty string if no value.
func (t *TraitAnnotation) ValueString() string {
	if t.Value == nil {
		return ""
	}
	if s, ok := t.Value.AsString(); ok {
		return s
	}
	return ""
}

// traitRegex matches @trait_name or @trait_name(value)
// The (?:^|[\s\-\*]) ensures @ is at start of line or after whitespace/list markers
var traitRegex = regexp.MustCompile(`(?:^|[\s\-\*])@(\w+)(?:\s*\(([^)]*)\))?`)

// ParseTraitAnnotations parses all trait annotations from a line.
// It automatically filters out traits that appear inside inline code spans (backticks).
func ParseTraitAnnotations(line string, lineNumber int) []TraitAnnotation {
	var traits []TraitAnnotation

	// Remove inline code spans to avoid matching traits inside them
	// We use the sanitized line for matching but preserve positions
	sanitizedLine := RemoveInlineCode(line)
	matches := traitRegex.FindAllStringSubmatchIndex(sanitizedLine, -1)

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		// match[2:4] is the trait name capture group
		traitName := line[match[2]:match[3]]

		// match[4:6] is the value capture group (may be -1 if not present)
		var value *schema.FieldValue
		if match[4] >= 0 && match[5] >= 0 {
			valueStr := strings.TrimSpace(line[match[4]:match[5]])
			if valueStr != "" {
				fv := parseTraitValue(valueStr)
				value = &fv
			}
		}

		// Extract content (everything after the trait annotation on the same line)
		afterTrait := ""
		if match[1] < len(line) {
			afterTrait = strings.TrimSpace(line[match[1]:])
		}

		traits = append(traits, TraitAnnotation{
			TraitName:   traitName,
			Value:       value,
			Content:     afterTrait,
			Line:        lineNumber,
			StartOffset: match[0],
			EndOffset:   match[1],
		})
	}

	return traits
}

// ParseTrait parses a single trait from a line (returns first match).
func ParseTrait(line string, lineNumber int) *TraitAnnotation {
	traits := ParseTraitAnnotations(line, lineNumber)
	if len(traits) == 0 {
		return nil
	}
	return &traits[0]
}

// parseTraitValue parses a single trait value.
// Values can be: date, datetime, string, reference, or enum value.
func parseTraitValue(valueStr string) schema.FieldValue {
	valueStr = strings.TrimSpace(valueStr)

	// Check for reference syntax [[...]]
	if strings.HasPrefix(valueStr, "[[") && strings.HasSuffix(valueStr, "]]") {
		ref := valueStr[2 : len(valueStr)-2]
		return schema.Ref(ref)
	}

	// Check for datetime (contains 'T')
	if strings.Contains(valueStr, "T") && len(valueStr) >= 16 {
		return schema.Datetime(valueStr)
	}

	// Check for date (YYYY-MM-DD pattern)
	if len(valueStr) == 10 && valueStr[4] == '-' && valueStr[7] == '-' {
		return schema.Date(valueStr)
	}

	// Everything else is a string (enum values, plain strings, etc.)
	return schema.String(valueStr)
}

// ExtractTraitContent extracts the full content for a trait.
// Returns the content after removing all trait annotations from the line.
//
// CONTENT SCOPE RULE: A trait's content consists of all text on the same line
// as the trait annotation. This same rule applies to determining which references
// are associated with a trait - refs on the same line are considered part of the
// trait's content. See IsRefOnTraitLine for the matching implementation.
func ExtractTraitContent(lines []string, lineIdx int) string {
	if lineIdx >= len(lines) {
		return ""
	}

	line := lines[lineIdx]
	// Remove the trait annotation itself, return remaining content
	result := traitRegex.ReplaceAllString(line, "")
	return strings.TrimSpace(result)
}

// IsRefOnTraitLine returns true if a reference is on the same line as a trait.
// This implements the CONTENT SCOPE RULE: refs on the same line as a trait
// are considered associated with that trait's content.
//
// This function is the single source of truth for trait-to-reference association.
// The query executor uses this same logic (matching by file_path and line_number).
func IsRefOnTraitLine(traitLine, refLine int) bool {
	return traitLine == refLine
}
