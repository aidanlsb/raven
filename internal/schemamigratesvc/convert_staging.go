package schemamigratesvc

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/frontmatter"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func stageFieldConversion(
	raw, typeName, fieldName string,
	mapper *conversionMapper,
	missing map[string]struct{},
) ([]byte, bool, bool) {
	lines := strings.Split(raw, "\n")
	start, end, ok := parser.FrontmatterBounds(lines)
	if !ok || end == -1 {
		return nil, false, false
	}
	values, ok := decodeYAMLMap([]byte(strings.Join(lines[start+1:end], "\n")))
	if !ok {
		return nil, false, false
	}
	if objectType, _ := values["type"].(string); objectType != typeName {
		return nil, false, false
	}
	rawValue, found := values[fieldName]
	if !found {
		return nil, false, false
	}

	mapped, complete := mapper.convert(parser.FieldValueFromYAML(rawValue), missing)
	if !complete {
		return nil, false, true
	}
	newRaw := frontmatter.FieldValueToYAMLValue(mapped)
	if reflect.DeepEqual(rawValue, newRaw) {
		return nil, false, true
	}
	values[fieldName] = newRaw
	newFrontmatter, ok := marshalYAMLMap(values)
	if !ok {
		return nil, false, true
	}

	var output strings.Builder
	output.WriteString("---\n")
	output.Write(newFrontmatter)
	output.WriteString("---")
	if end+1 < len(lines) {
		output.WriteString("\n")
		output.WriteString(strings.Join(lines[end+1:], "\n"))
	}
	return []byte(output.String()), true, true
}

func stageTraitConversions(
	raw string,
	traits []*model.Trait,
	traitName string,
	def *schema.TraitDefinition,
	mapper *conversionMapper,
	missing map[string]struct{},
) ([]byte, int) {
	lines := strings.Split(raw, "\n")
	targetLines := make(map[int]struct{})
	for _, trait := range traits {
		if trait == nil || trait.TraitType != traitName || trait.Line <= 0 || trait.Line > len(lines) {
			continue
		}
		targetLines[trait.Line] = struct{}{}
	}

	converted := 0
	for lineNumber := range targetLines {
		line := lines[lineNumber-1]
		annotations := parser.ParseTraitAnnotations(line, lineNumber)
		sort.SliceStable(annotations, func(i, j int) bool {
			return annotations[i].StartOffset > annotations[j].StartOffset
		})
		for _, annotation := range annotations {
			if annotation.TraitName != traitName {
				continue
			}
			value := effectiveTraitAnnotationValue(annotation.Value, def)
			mapped, complete := mapper.convert(value, missing)
			if !complete {
				continue
			}
			start, end := annotation.StartOffset, annotation.EndOffset
			if start < 0 || end > len(line) || start >= end {
				continue
			}
			segment := line[start:end]
			at := strings.Index(segment, "@"+traitName)
			if at < 0 {
				continue
			}
			replacement := segment[:at] + "@" + traitName
			if !mapped.IsNull() {
				replacement += "(" + serializeTraitConversionLiteral(mapped, false) + ")"
			}
			if replacement == segment {
				continue
			}
			line = line[:start] + replacement + line[end:]
			converted++
		}
		lines[lineNumber-1] = line
	}
	if converted == 0 {
		return nil, 0
	}
	return []byte(strings.Join(lines, "\n")), converted
}

func effectiveTraitAnnotationValue(value *fieldvalue.FieldValue, def *schema.TraitDefinition) fieldvalue.FieldValue {
	if value != nil {
		return *value
	}
	if def != nil && def.Default != nil {
		return parser.FieldValueFromYAML(def.Default)
	}
	if def != nil && normalizedConversionType(def.Type, true) == schema.FieldTypeBool {
		return fieldvalue.Bool(true)
	}
	return fieldvalue.Null()
}

func validateTraitLiteralValue(value fieldvalue.FieldValue) error {
	if array, ok := value.AsArray(); ok {
		for _, item := range array {
			if err := validateTraitLiteralValue(item); err != nil {
				return err
			}
		}
		return nil
	}
	if text, ok := value.AsString(); ok {
		switch {
		case strings.Contains(text, ")"):
			return fmt.Errorf("trait values containing ')' cannot be represented in @trait(...) syntax")
		case strings.ContainsAny(text, "\r\n"):
			return fmt.Errorf("trait values cannot contain newlines")
		case strings.Contains(text, `"`):
			return fmt.Errorf("trait values containing double quotes cannot be represented losslessly")
		case strings.Contains(text, "`"):
			return fmt.Errorf("trait values containing backticks cannot be represented losslessly")
		}
	}
	return nil
}

func serializeTraitConversionLiteral(value fieldvalue.FieldValue, inArray bool) string {
	if ref, ok := value.AsRef(); ok {
		return "[[" + ref + "]]"
	}
	if array, ok := value.AsArray(); ok {
		parts := make([]string, len(array))
		for i, item := range array {
			parts[i] = serializeTraitConversionLiteral(item, true)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	if text, ok := value.AsString(); ok {
		trimmed := strings.TrimSpace(text)
		quote := text == "" || text != trimmed || strings.Contains(text, `"`) ||
			(inArray && strings.Contains(text, ",")) ||
			(strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]"))
		if quote {
			return `"` + text + `"`
		}
		return text
	}
	if number, ok := value.AsNumber(); ok {
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
	if boolean, ok := value.AsBool(); ok {
		return strconv.FormatBool(boolean)
	}
	return fmt.Sprintf("%v", value.Raw())
}
