// Package parser handles parsing markdown files.
package parser

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/schema"
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

// ParseFrontmatter parses YAML frontmatter from markdown content.
// Returns nil if no frontmatter is found.
func ParseFrontmatter(content string) (*Frontmatter, error) {
	lines := strings.Split(content, "\n")

	// Check for opening ---
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, nil
	}

	// Find closing ---
	endLine := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endLine = i
			break
		}
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

	if yamlData == nil {
		return nil, nil
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
			fm.Fields[key] = yamlToFieldValue(value)
		}
	}

	return fm, nil
}

// yamlToFieldValue converts a YAML value to a FieldValue.
func yamlToFieldValue(value interface{}) schema.FieldValue {
	switch v := value.(type) {
	case string:
		// Check if it's a reference
		if strings.HasPrefix(v, "[[") && strings.HasSuffix(v, "]]") {
			return schema.Ref(v[2 : len(v)-2])
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
		// YAML parses dates as time.Time - convert to date string
		return schema.Date(v.Format("2006-01-02"))
	case []interface{}:
		items := make([]schema.FieldValue, 0, len(v))
		for _, item := range v {
			items = append(items, yamlToFieldValue(item))
		}
		return schema.Array(items)
	case nil:
		return schema.Null()
	default:
		return schema.Null()
	}
}
