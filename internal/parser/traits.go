package parser

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ravenscroftj/raven/internal/schema"
)

// TraitAnnotation represents a parsed @trait() annotation.
type TraitAnnotation struct {
	TraitName   string
	Fields      map[string]schema.FieldValue
	Content     string // Content after the trait on the same line
	Line        int
	StartOffset int
	EndOffset   int
}

// traitRegex matches @trait_name or @trait_name(args...)
// The (?:^|[\s\-\*]) ensures @ is at start of line or after whitespace/list markers
var traitRegex = regexp.MustCompile(`(?:^|[\s\-\*])@(\w+)(?:\s*\(([^)]*)\))?`)

// ParseTraitAnnotations parses all trait annotations from a line.
func ParseTraitAnnotations(line string, lineNumber int) []TraitAnnotation {
	var traits []TraitAnnotation

	matches := traitRegex.FindAllStringSubmatchIndex(line, -1)

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		// match[2:4] is the trait name capture group
		traitName := line[match[2]:match[3]]

		// match[4:6] is the args capture group (may be -1 if not present)
		var argsStr string
		if match[4] >= 0 && match[5] >= 0 {
			argsStr = line[match[4]:match[5]]
		}

		// Parse arguments
		fields, _ := parseTraitArguments(argsStr)

		// Extract content (everything after the trait annotation on the same line)
		afterTrait := ""
		if match[1] < len(line) {
			afterTrait = strings.TrimSpace(line[match[1]:])
		}

		traits = append(traits, TraitAnnotation{
			TraitName:   traitName,
			Fields:      fields,
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

// parseTraitArguments parses trait arguments, reusing the type decl parser.
func parseTraitArguments(args string) (map[string]schema.FieldValue, error) {
	if strings.TrimSpace(args) == "" {
		return make(map[string]schema.FieldValue), nil
	}

	// Reuse the argument parsing logic from type declarations
	decl, err := ParseTypeDeclaration(fmt.Sprintf("::dummy(%s)", args), 0)
	if err != nil || decl == nil {
		return make(map[string]schema.FieldValue), nil
	}
	return decl.Fields, nil
}

// ExtractTraitContent extracts the full content for a trait.
// For now, returns the content after the trait on the same line.
func ExtractTraitContent(lines []string, lineIdx int) string {
	if lineIdx >= len(lines) {
		return ""
	}

	line := lines[lineIdx]
	// Remove the trait annotation itself, return remaining content
	result := traitRegex.ReplaceAllString(line, "")
	return strings.TrimSpace(result)
}
