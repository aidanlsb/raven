// Package parser handles parsing markdown files.
package parser

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/wikilink"
)

// Frontmatter represents parsed frontmatter data.
type Frontmatter struct {
	// ObjectType is the type field (if present).
	ObjectType string

	// Fields are all other fields.
	Fields map[string]schema.FieldValue

	// Raw is the raw frontmatter content.
	Raw string

	// EndLine is the line where frontmatter ends (1-indexed).
	EndLine int
}

// FrontmatterBounds returns the opening and closing frontmatter line indices.
// It only detects frontmatter when the first line is '---'.
// If frontmatter is present but unclosed, endLine is -1.
func FrontmatterBounds(lines []string) (startLine int, endLine int, ok bool) {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return 0, -1, false
	}

	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return 0, i, true
		}
	}

	return 0, -1, true
}

// ParseFrontmatter parses YAML frontmatter from markdown content.
// Returns nil if no frontmatter is found.
func ParseFrontmatter(content string) (*Frontmatter, error) {
	lines := strings.Split(content, "\n")

	_, endLine, ok := FrontmatterBounds(lines)
	if !ok {
		return nil, nil
	}
	if endLine == -1 {
		return nil, nil // No closing ---
	}

	// Extract frontmatter content
	frontmatterContent := strings.Join(lines[1:endLine], "\n")

	// Parse as YAML
	var yamlData map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterContent), &yamlData); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter as YAML: %w", err)
	}

	// YAML can decode an empty document (or comments/whitespace only) into a nil map.
	// We still consider this "frontmatter present" because it affects body line offsets.
	if yamlData == nil {
		yamlData = map[string]interface{}{}
	}

	fm := &Frontmatter{
		Raw:     frontmatterContent,
		EndLine: endLine + 1, // +1 for 1-indexed lines
		Fields:  make(map[string]schema.FieldValue),
	}

	for key, value := range yamlData {
		switch key {
		case "type":
			if s, ok := value.(string); ok {
				fm.ObjectType = s
			}
		default:
			fm.Fields[key] = FieldValueFromYAML(value)
		}
	}

	return fm, nil
}

// FieldValueFromYAML converts a YAML value to a FieldValue.
func FieldValueFromYAML(value interface{}) schema.FieldValue {
	switch v := value.(type) {
	case string:
		// Check if it's a reference
		if target, _, ok := wikilink.ParseExact(v); ok {
			return schema.Ref(target)
		}
		return schema.String(v)
	case int:
		return schema.Number(float64(v))
	case int64:
		return schema.Number(float64(v))
	case float64:
		return schema.Number(v)
	case bool:
		return schema.Bool(v)
	case time.Time:
		// YAML parses dates/datetimes as time.Time - preserve time if present.
		if v.Hour() == 0 && v.Minute() == 0 && v.Second() == 0 && v.Nanosecond() == 0 {
			return schema.Date(v.Format(dates.DateLayout))
		}
		if v.Second() == 0 && v.Nanosecond() == 0 {
			return schema.Datetime(v.Format(dates.DatetimeLayout))
		}
		return schema.Datetime(v.Format(dates.DatetimeSecondsLayout))
	case []interface{}:
		items := make([]schema.FieldValue, 0, len(v))
		for _, item := range v {
			items = append(items, FieldValueFromYAML(item))
		}
		return schema.Array(items)
	case nil:
		return schema.Null()
	default:
		return schema.Null()
	}
}
