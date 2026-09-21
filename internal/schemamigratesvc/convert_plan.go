package schemamigratesvc

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type conversionMapper struct {
	sourceType schema.FieldType
	targetType schema.FieldType
	values     map[string]fieldvalue.FieldValue
}

func buildConversionMapper(
	rawMapping map[string]interface{},
	sourceType, targetType schema.FieldType,
	order []string,
	validate func(fieldvalue.FieldValue, bool, []string) error,
) (*conversionMapper, []string, error) {
	if rawMapping == nil {
		return nil, nil, svcerr.New(codes.ErrInvalidInput, "--map-json must be a JSON object").WithSuggestion(`Provide an exhaustive map, for example --map-json '{"high":true,"low":false}'`)
	}
	if len(rawMapping) == 0 {
		return nil, nil, svcerr.New(codes.ErrInvalidInput, "--map-json must contain at least one mapping")
	}

	mapper := &conversionMapper{
		sourceType: sourceType,
		targetType: targetType,
		values:     make(map[string]fieldvalue.FieldValue, len(rawMapping)),
	}
	orderedKeys := orderedMappingKeys(rawMapping, order)
	elementMapping := sourceType.IsArray() && targetType.IsArray()
	for _, key := range orderedKeys {
		raw := rawMapping[key]
		if containsMappingObject(raw) {
			return nil, nil, invalidMappingValueError(key, targetType, fmt.Errorf("nested JSON objects are not supported"))
		}
		value, err := mappingValueForTarget(raw, targetType, elementMapping)
		if err != nil {
			return nil, nil, invalidMappingValueError(key, targetType, err)
		}
		mapper.values[key] = value
	}

	newValues := newEnumValues(mapper.values, orderedKeys, sourceType, targetType)
	if targetType.IsEnum() && len(newValues) == 0 {
		return nil, nil, svcerr.New(codes.ErrInvalidInput, "enum conversion must produce at least one string value")
	}
	for _, key := range orderedKeys {
		if err := validate(mapper.values[key], elementMapping, newValues); err != nil {
			return nil, nil, invalidMappingValueError(key, targetType, err)
		}
	}
	return mapper, newValues, nil
}

func invalidMappingValueError(key string, targetType schema.FieldType, cause error) error {
	return svcerr.Wrap(codes.ErrInvalidInput, fmt.Sprintf("mapping for %q is invalid for target type '%s': %v", key, targetType, cause), cause).WithSuggestion("Use JSON values in the target type's representation").WithDetails(map[string]interface{}{"map_key": key, "target_type": targetType})
}

func (m *conversionMapper) convert(value fieldvalue.FieldValue, missing map[string]struct{}) (fieldvalue.FieldValue, bool) {
	if m.sourceType.IsArray() && m.targetType.IsArray() {
		items, ok := value.AsArray()
		if !ok {
			items = []fieldvalue.FieldValue{value}
		}
		converted := make([]fieldvalue.FieldValue, 0, len(items))
		complete := true
		for _, item := range items {
			key := conversionMapKey(item)
			mapped, exists := m.values[key]
			if !exists {
				missing[key] = struct{}{}
				complete = false
				continue
			}
			converted = append(converted, mapped)
		}
		return fieldvalue.Array(converted), complete
	}

	key := conversionMapKey(value)
	mapped, ok := m.values[key]
	if !ok {
		missing[key] = struct{}{}
		return fieldvalue.Null(), false
	}
	return mapped, true
}

func resolveConversionTarget(sourceType schema.FieldType, requested string, trait bool) (schema.FieldType, bool, error) {
	requested = strings.TrimSpace(strings.ToLower(requested))
	if requested == "" {
		return sourceType, false, nil
	}
	if requested == "boolean" {
		requested = string(schema.FieldTypeBool)
	}
	targetType := schema.FieldType(requested)
	valid := schema.IsValidFieldType(targetType)
	if trait {
		valid = schema.IsValidTraitType(targetType)
	}
	if !valid {
		validTypes := schema.ValidFieldTypes()
		if trait {
			validTypes = schema.ValidTraitTypes()
		}
		return "", false, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("unsupported target type '%s'", requested)).WithSuggestion(fmt.Sprintf("Use one of: %s", validTypes)).WithDetails(map[string]interface{}{"target_type": requested, "valid_types": validTypes})
	}
	return targetType, true, nil
}

func validateCollectionConversion(sourceType, targetType schema.FieldType) error {
	if sourceType.IsArray() && !targetType.IsArray() {
		return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("cannot convert collection type '%s' to scalar type '%s'", sourceType, targetType)).WithSuggestion("Collection-to-scalar conversion has no unambiguous reduction rule; convert to another [] type")
	}
	return nil
}

func normalizedConversionType(fieldType schema.FieldType, trait bool) schema.FieldType {
	raw := strings.TrimSpace(strings.ToLower(string(fieldType)))
	if trait && (raw == "" || raw == "boolean") {
		return schema.FieldTypeBool
	}
	if raw == "reference" {
		return schema.FieldTypeRef
	}
	if raw == "reference[]" {
		return schema.FieldTypeRefArray
	}
	return schema.FieldType(raw)
}

func addFiniteRequiredValues(
	missing map[string]struct{},
	mapping map[string]fieldvalue.FieldValue,
	sourceType schema.FieldType,
	enumValues []string,
) {
	addIfMissing := func(value string) {
		if _, exists := mapping[value]; !exists {
			missing[value] = struct{}{}
		}
	}
	switch {
	case sourceType.IsEnum():
		for _, value := range enumValues {
			addIfMissing(value)
		}
	case sourceType.IsBool():
		addIfMissing("true")
		addIfMissing("false")
	}
}

func exhaustiveMappingError(required map[string]struct{}) error {
	if len(required) == 0 {
		return nil
	}
	missing := make([]string, 0, len(required))
	for value := range required {
		missing = append(missing, value)
	}
	sort.Strings(missing)
	return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("mapping is not exhaustive; missing %d value(s): %s", len(missing), strings.Join(quotedValues(missing), ", "))).WithSuggestion("Add every schema-allowed and observed live value to --map-json").WithDetails(map[string]interface{}{"missing_values": missing})
}

func quotedValues(values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strconv.Quote(value)
	}
	return out
}

func conversionMapKey(value fieldvalue.FieldValue) string {
	if value.IsNull() {
		return "null"
	}
	if ref, ok := value.AsRef(); ok {
		return "[[" + ref + "]]"
	}
	if array, ok := value.AsArray(); ok {
		parts := make([]string, len(array))
		for i, item := range array {
			parts[i] = conversionMapKey(item)
		}
		return "[" + strings.Join(parts, ",") + "]"
	}
	if value.IsDate() || value.IsDatetime() {
		text, _ := value.AsString()
		return text
	}
	if text, ok := value.AsString(); ok {
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

func fieldMappingOrder(def *schema.FieldDefinition, sourceType schema.FieldType) []string {
	if def == nil {
		return nil
	}
	return finiteMappingOrder(sourceType, def.Values)
}

func traitMappingOrder(def *schema.TraitDefinition, sourceType schema.FieldType) []string {
	if def == nil {
		return nil
	}
	return finiteMappingOrder(sourceType, def.Values)
}

func finiteMappingOrder(sourceType schema.FieldType, enumValues []string) []string {
	switch {
	case sourceType.IsEnum():
		return append([]string(nil), enumValues...)
	case sourceType.IsBool():
		return []string{"true", "false"}
	default:
		return nil
	}
}

func orderedMappingKeys(mapping map[string]interface{}, preferred []string) []string {
	seen := make(map[string]struct{}, len(mapping))
	keys := make([]string, 0, len(mapping))
	for _, key := range preferred {
		if _, exists := mapping[key]; !exists {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	extra := make([]string, 0, len(mapping)-len(keys))
	for key := range mapping {
		if _, exists := seen[key]; !exists {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	return append(keys, extra...)
}

func newEnumValues(
	mapping map[string]fieldvalue.FieldValue,
	order []string,
	sourceType, targetType schema.FieldType,
) []string {
	if !targetType.IsEnum() {
		return nil
	}
	seen := make(map[string]struct{})
	values := make([]string, 0)
	appendValue := func(value fieldvalue.FieldValue) {
		if value.IsNull() {
			return
		}
		text, ok := value.AsString()
		if !ok {
			return
		}
		if _, exists := seen[text]; exists {
			return
		}
		seen[text] = struct{}{}
		values = append(values, text)
	}
	for _, key := range order {
		value := mapping[key]
		if targetType.IsArray() && !sourceType.IsArray() {
			if items, ok := value.AsArray(); ok {
				for _, item := range items {
					appendValue(item)
				}
			}
			continue
		}
		appendValue(value)
	}
	return values
}

func mappingValueForTarget(raw interface{}, targetType schema.FieldType, elementMapping bool) (fieldvalue.FieldValue, error) {
	effectiveType := targetType
	if elementMapping {
		if elem, ok := targetType.ElementType(); ok {
			effectiveType = elem
		}
	}
	if elementType, ok := effectiveType.ElementType(); ok {
		switch values := raw.(type) {
		case []interface{}:
			items := make([]fieldvalue.FieldValue, 0, len(values))
			for _, item := range values {
				converted, err := mappingScalarForTarget(item, elementType)
				if err != nil {
					return fieldvalue.Null(), err
				}
				items = append(items, converted)
			}
			return fieldvalue.Array(items), nil
		case []string:
			items := make([]fieldvalue.FieldValue, 0, len(values))
			for _, item := range values {
				converted, err := mappingScalarForTarget(item, elementType)
				if err != nil {
					return fieldvalue.Null(), err
				}
				items = append(items, converted)
			}
			return fieldvalue.Array(items), nil
		default:
			return fieldvalue.Null(), fmt.Errorf("expected a JSON array")
		}
	}
	if _, isArray := raw.([]interface{}); isArray {
		return fieldvalue.Null(), fmt.Errorf("expected a scalar JSON value")
	}
	if _, isArray := raw.([]string); isArray {
		return fieldvalue.Null(), fmt.Errorf("expected a scalar JSON value")
	}
	return mappingScalarForTarget(raw, effectiveType)
}

func mappingScalarForTarget(raw interface{}, targetType schema.FieldType) (fieldvalue.FieldValue, error) {
	if raw == nil {
		return fieldvalue.Null(), fmt.Errorf("null is not a schema value type")
	}
	switch targetType {
	case schema.FieldTypeString, schema.FieldTypeURL, schema.FieldTypeDate, schema.FieldTypeDatetime:
		value, ok := raw.(string)
		if !ok {
			return fieldvalue.Null(), fmt.Errorf("expected a JSON string")
		}
		return fieldvalue.String(value), nil
	case schema.FieldTypeEnum:
		value, ok := raw.(string)
		if !ok {
			return fieldvalue.Null(), fmt.Errorf("expected a JSON string")
		}
		if parser.FieldValueFromYAML(value).IsRef() {
			return fieldvalue.Null(), fmt.Errorf("enum values cannot use wikilink syntax")
		}
		return fieldvalue.String(value), nil
	case schema.FieldTypeRef:
		value, ok := raw.(string)
		if !ok {
			return fieldvalue.Null(), fmt.Errorf("expected a JSON string containing a reference")
		}
		return parser.FieldValueFromYAML(value), nil
	case schema.FieldTypeBool:
		value, ok := raw.(bool)
		if !ok {
			return fieldvalue.Null(), fmt.Errorf("expected a JSON boolean")
		}
		return fieldvalue.Bool(value), nil
	case schema.FieldTypeNumber:
		number, ok := jsonNumber(raw)
		if !ok {
			return fieldvalue.Null(), fmt.Errorf("expected a JSON number")
		}
		return fieldvalue.Number(number), nil
	default:
		return fieldvalue.Null(), fmt.Errorf("unsupported target type '%s'", targetType)
	}
}

func jsonNumber(raw interface{}) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int8:
		return float64(value), true
	case int16:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint:
		return float64(value), true
	case uint8:
		return float64(value), true
	case uint16:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	case json.Number:
		number, err := value.Float64()
		return number, err == nil
	default:
		return 0, false
	}
}

func containsMappingObject(value interface{}) bool {
	switch typed := value.(type) {
	case map[string]interface{}, map[interface{}]interface{}, map[string]string:
		return true
	case []interface{}:
		for _, item := range typed {
			if containsMappingObject(item) {
				return true
			}
		}
	}
	return false
}

func conversionWalkOptions(rt *vaultruntime.Runtime) (*vault.WalkOptions, error) {
	if rt == nil || rt.VaultCfg == nil {
		return nil, svcerr.New(codes.ErrConfigInvalid, "failed to load raven.yaml").WithSuggestion("Fix raven.yaml and try again")
	}
	matcher, err := ravenignore.NewMatcher(rt.VaultCfg.GetExcludePatterns())
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrConfigInvalid, "invalid exclude configuration in raven.yaml", err).WithSuggestion("Fix raven.yaml and try again")
	}
	return &vault.WalkOptions{ExcludeMatcher: matcher}, nil
}
