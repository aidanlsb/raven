package frontmatter

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/schema"
)

// Render serializes typed field values into a deterministic YAML frontmatter block.
func Render(typeName string, fields map[string]schema.FieldValue, blankFields map[string]bool) (string, error) {
	yamlData := make(map[string]interface{}, len(fields)+1)
	if strings.TrimSpace(typeName) != "" {
		yamlData["type"] = typeName
	}

	for key, value := range fields {
		if key == "type" {
			continue
		}
		yamlData[key] = FieldValueToYAMLValue(value)
	}

	return RenderData(yamlData, blankFields)
}

// RenderData serializes raw YAML-compatible frontmatter data into a deterministic block.
func RenderData(yamlData map[string]interface{}, blankFields map[string]bool) (string, error) {
	var builder strings.Builder
	builder.WriteString("---\n")

	writeField := func(key string, value interface{}) error {
		pair, err := yaml.Marshal(map[string]interface{}{key: value})
		if err != nil {
			return fmt.Errorf("failed to marshal frontmatter: %w", err)
		}
		builder.Write(pair)
		return nil
	}

	if typeValue, ok := yamlData["type"]; ok {
		if err := writeField("type", typeValue); err != nil {
			return "", err
		}
	}

	fieldNames := make(map[string]struct{}, len(yamlData)+len(blankFields))
	for key := range yamlData {
		if key == "type" {
			continue
		}
		fieldNames[key] = struct{}{}
	}
	for key := range blankFields {
		if key == "type" {
			continue
		}
		fieldNames[key] = struct{}{}
	}

	sortedFields := make([]string, 0, len(fieldNames))
	for key := range fieldNames {
		sortedFields = append(sortedFields, key)
	}
	sort.Strings(sortedFields)

	for _, key := range sortedFields {
		value, hasValue := yamlData[key]
		if !hasValue {
			builder.WriteString(key)
			builder.WriteString(": \n")
			continue
		}
		if err := writeField(key, value); err != nil {
			return "", err
		}
	}

	builder.WriteString("---\n")
	return builder.String(), nil
}

// FieldValueToYAMLValue converts a typed field value into its YAML representation.
func FieldValueToYAMLValue(value schema.FieldValue) interface{} {
	if value.IsNull() {
		return nil
	}
	if ref, ok := value.AsRef(); ok {
		return "[[" + ref + "]]"
	}
	if arr, ok := value.AsArray(); ok {
		items := make([]interface{}, 0, len(arr))
		for _, item := range arr {
			items = append(items, FieldValueToYAMLValue(item))
		}
		return items
	}
	if s, ok := value.AsString(); ok {
		return s
	}
	if n, ok := value.AsNumber(); ok {
		return n
	}
	if b, ok := value.AsBool(); ok {
		return b
	}
	return value.Raw()
}
