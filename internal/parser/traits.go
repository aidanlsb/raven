package parser

import (
	"regexp"
	"strings"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/wikilink"
)

// TraitAnnotation represents a parsed @trait() annotation.
type TraitAnnotation struct {
	TraitName string
	// Value is the single trait value (nil for boolean traits like @highlight)
	Value       *schema.FieldValue
	Content     string // Full line content with all trait annotations removed
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

// traitRegex matches @trait_name or @trait_name(value) for parsing.
// The (?:^|[\s\-\*]) ensures @ is at start of line or after whitespace/list markers.
// This strict pattern prevents matching things like email addresses.
var traitRegex = regexp.MustCompile(`(?:^|[\s\-\*])@(\w+)(?:\s*\(([^)]*)\))?`)

// TraitHighlightPattern is a regex for highlighting traits in already-parsed content.
// It's simpler than traitRegex because it doesn't need context validation - it's
// used for display purposes on content that has already been parsed.
// Capture groups: [1] = trait name, [2] = value (if present)
var TraitHighlightPattern = regexp.MustCompile(`@(\w+)(?:\(([^)]*)\))?`)

// StripTraitAnnotations removes all trait annotations from a line and returns
// the remaining content.
//
// CONTENT SCOPE RULE: A trait's content consists of all text on the same line
// as the trait annotation, with trait annotations removed.
func StripTraitAnnotations(line string) string {
	return strings.TrimSpace(traitRegex.ReplaceAllString(line, ""))
}

// ParseTraitAnnotations parses all trait annotations from a text segment.
//
// Note: This function does NOT filter out inline code. When used with the AST-based
// parser (ExtractFromAST), code filtering is handled by the AST walker which skips
// CodeSpan nodes entirely. The text passed to this function should already be from
// a non-code AST node.
func ParseTraitAnnotations(line string, lineNumber int) []TraitAnnotation {
	var traits []TraitAnnotation

	matches := traitRegex.FindAllStringSubmatchIndex(line, -1)

	// Compute the full line content once by removing ALL trait annotations.
	// This ensures traits at any position (start, middle, end) get the same content.
	lineContent := StripTraitAnnotations(line)

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

		traits = append(traits, TraitAnnotation{
			TraitName:   traitName,
			Value:       value,
			Content:     lineContent,
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

	// Check for reference syntax [[target]] or [[target|display]]
	// Use wikilink.ParseExact to correctly handle aliases
	if target, _, ok := wikilink.ParseExact(valueStr); ok {
		return schema.Ref(target)
	}

	// Check for datetime.
	// Use canonical validation rather than heuristics to avoid misclassifying random
	// strings that happen to contain 'T' (and invalid dates like 2025-13-45).
	if dates.IsValidDatetime(valueStr) {
		return schema.Datetime(valueStr)
	}

	// Check for date (YYYY-MM-DD).
	if dates.IsValidDate(valueStr) {
		return schema.Date(valueStr)
	}

	// Everything else is a string (enum values, plain strings, etc.)
	return schema.String(valueStr)
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
