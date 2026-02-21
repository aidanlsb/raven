package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

type fieldValidationError struct {
	ObjectType string
	Issues     []schema.ValidationError
}

type unknownFieldMutationError struct {
	ObjectType   string
	Unknown      []string
	Allowed      []string
	AllowedCount int
}

func (e *fieldValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "field validation failed"
	}
	if len(e.Issues) == 1 {
		issue := e.Issues[0]
		return fmt.Sprintf("invalid value for field '%s': %s", issue.Field, issue.Message)
	}

	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		parts = append(parts, fmt.Sprintf("'%s': %s", issue.Field, issue.Message))
	}
	sort.Strings(parts)
	return fmt.Sprintf("invalid field values: %s", strings.Join(parts, "; "))
}

func (e *fieldValidationError) Suggestion() string {
	if strings.TrimSpace(e.ObjectType) == "" {
		return "Ensure values match the schema field types"
	}
	return fmt.Sprintf("Ensure values match the schema field types for type '%s'", e.ObjectType)
}

func (e *unknownFieldMutationError) Error() string {
	if len(e.Unknown) == 1 {
		return fmt.Sprintf("unknown field '%s' for type '%s'", e.Unknown[0], e.ObjectType)
	}
	return fmt.Sprintf("unknown fields for type '%s': %s", e.ObjectType, strings.Join(e.Unknown, ", "))
}

func (e *unknownFieldMutationError) Suggestion() string {
	return fmt.Sprintf("Run 'rvn schema type %s' to view valid fields, or add missing fields with 'rvn schema add field %s <field_name> --type <field_type>'", e.ObjectType, e.ObjectType)
}

func (e *unknownFieldMutationError) Details() map[string]interface{} {
	details := map[string]interface{}{
		"object_type":    e.ObjectType,
		"unknown_fields": e.Unknown,
	}
	if len(e.Allowed) > 0 {
		details["allowed_fields"] = e.Allowed
		details["allowed_count"] = e.AllowedCount
	}
	return details
}

func normalizeMutationType(objectType string) string {
	if strings.TrimSpace(objectType) == "" {
		return "page"
	}
	return objectType
}

func prepareValidatedFieldMutation(objectType string, existingFields map[string]schema.FieldValue, updates map[string]string, sch *schema.Schema, allowedUnknown map[string]bool) (map[string]schema.FieldValue, map[string]string, []string, error) {
	normalizedType := normalizeMutationType(objectType)
	fieldDefs := fieldDefsForObjectType(sch, normalizedType)
	resolvedUpdates := resolveDateKeywordsForUpdates(updates, fieldDefs)

	parsedUpdates := make(map[string]schema.FieldValue, len(resolvedUpdates))
	for fieldName, value := range resolvedUpdates {
		parsedUpdates[fieldName] = parseFieldValueToSchema(value)
	}

	validatedUpdates, warnings, err := prepareValidatedFieldMutationValues(normalizedType, existingFields, parsedUpdates, sch, allowedUnknown)
	if err != nil {
		return nil, nil, warnings, err
	}

	return validatedUpdates, resolvedUpdates, warnings, nil
}

func prepareValidatedFrontmatterMutation(content string, fm *parser.Frontmatter, objectType string, updates map[string]string, sch *schema.Schema, allowedUnknown map[string]bool) (string, map[string]string, []string, error) {
	normalizedType := normalizeMutationType(objectType)
	fieldDefs := fieldDefsForObjectType(sch, normalizedType)
	resolvedUpdates := resolveDateKeywordsForUpdates(updates, fieldDefs)
	typedUpdates := make(map[string]schema.FieldValue, len(resolvedUpdates))
	for key, value := range resolvedUpdates {
		typedUpdates[key] = parseFieldValueToSchema(value)
	}

	newContent, warnings, err := prepareValidatedFrontmatterMutationValues(content, fm, normalizedType, typedUpdates, sch, allowedUnknown)
	if err != nil {
		return "", nil, warnings, err
	}

	return newContent, resolvedUpdates, warnings, nil
}

func prepareValidatedFieldMutationValues(objectType string, existingFields map[string]schema.FieldValue, updates map[string]schema.FieldValue, sch *schema.Schema, allowedUnknown map[string]bool) (map[string]schema.FieldValue, []string, error) {
	normalizedType := normalizeMutationType(objectType)
	fieldDefs := fieldDefsForObjectType(sch, normalizedType)
	coercedUpdates := coerceFieldMutationValues(updates, fieldDefs)
	if unknownErr := detectUnknownFieldMutationByNames(normalizedType, sch, fieldNamesFromValueUpdates(coercedUpdates), allowedUnknown); unknownErr != nil {
		return nil, nil, unknownErr
	}

	merged := make(map[string]schema.FieldValue, len(existingFields)+len(coercedUpdates))
	for key, value := range existingFields {
		merged[key] = value
	}
	for key, value := range coercedUpdates {
		merged[key] = value
	}

	if err := validateMergedFields(normalizedType, merged, sch); err != nil {
		return nil, nil, err
	}

	return coercedUpdates, nil, nil
}

func prepareValidatedFrontmatterMutationValues(content string, fm *parser.Frontmatter, objectType string, updates map[string]schema.FieldValue, sch *schema.Schema, allowedUnknown map[string]bool) (string, []string, error) {
	existingFields := make(map[string]schema.FieldValue)
	if fm != nil && fm.Fields != nil {
		for key, value := range fm.Fields {
			existingFields[key] = value
		}
	}

	validatedUpdates, warnings, err := prepareValidatedFieldMutationValues(objectType, existingFields, updates, sch, allowedUnknown)
	if err != nil {
		return "", warnings, err
	}

	newContent, err := updateFrontmatterWithFieldValues(content, fm, validatedUpdates)
	if err != nil {
		return "", warnings, err
	}

	// Validate the exact serialized result to ensure write-time integrity.
	updatedFM, err := parser.ParseFrontmatter(newContent)
	if err != nil {
		return "", warnings, err
	}
	if updatedFM == nil {
		return "", warnings, fmt.Errorf("file has no frontmatter after update")
	}
	if err := validateMergedFields(normalizeMutationType(objectType), updatedFM.Fields, sch); err != nil {
		return "", warnings, err
	}

	return newContent, warnings, nil
}

func coerceFieldMutationValues(updates map[string]schema.FieldValue, fieldDefs map[string]*schema.FieldDefinition) map[string]schema.FieldValue {
	if len(updates) == 0 {
		return updates
	}

	coerced := make(map[string]schema.FieldValue, len(updates))
	for fieldName, value := range updates {
		coerced[fieldName] = coerceFieldValueForDefinition(value, fieldDefs[fieldName])
	}
	return coerced
}

func coerceFieldValueForDefinition(value schema.FieldValue, fieldDef *schema.FieldDefinition) schema.FieldValue {
	if fieldDef == nil {
		return value
	}

	switch fieldDef.Type {
	case schema.FieldTypeDate:
		if raw, ok := value.AsString(); ok {
			if resolved, resolvedOK := resolveRelativeDateKeyword(raw); resolvedOK {
				return schema.Date(resolved)
			}
		}
	case schema.FieldTypeDateArray:
		arr, ok := value.AsArray()
		if !ok {
			return value
		}
		changed := false
		coercedItems := make([]schema.FieldValue, len(arr))
		for i, item := range arr {
			coercedItems[i] = item
			raw, isString := item.AsString()
			if !isString {
				continue
			}
			resolved, resolvedOK := resolveRelativeDateKeyword(raw)
			if !resolvedOK {
				continue
			}
			coercedItems[i] = schema.Date(resolved)
			changed = true
		}
		if changed {
			return schema.Array(coercedItems)
		}
	}

	return value
}

func detectUnknownFieldMutationByNames(objectType string, sch *schema.Schema, fieldNames []string, allowedUnknown map[string]bool) *unknownFieldMutationError {
	if sch == nil {
		return nil
	}

	typeDef, ok := sch.Types[objectType]
	if !ok || typeDef == nil {
		return nil
	}

	sort.Strings(fieldNames)

	unknown := make([]string, 0)
	for _, fieldName := range fieldNames {
		if allowedUnknown != nil && allowedUnknown[fieldName] {
			continue
		}
		if _, exists := typeDef.Fields[fieldName]; exists {
			continue
		}
		unknown = append(unknown, fieldName)
	}
	if len(unknown) == 0 {
		return nil
	}

	allowed := make([]string, 0, len(typeDef.Fields))
	for fieldName := range typeDef.Fields {
		allowed = append(allowed, fieldName)
	}
	sort.Strings(allowed)

	return &unknownFieldMutationError{
		ObjectType:   objectType,
		Unknown:      unknown,
		Allowed:      allowed,
		AllowedCount: len(allowed),
	}
}

func fieldNamesFromValueUpdates(updates map[string]schema.FieldValue) []string {
	names := make([]string, 0, len(updates))
	for name := range updates {
		names = append(names, name)
	}
	return names
}

func validateMergedFields(objectType string, fields map[string]schema.FieldValue, sch *schema.Schema) error {
	if sch == nil {
		return nil
	}

	typeDef, ok := sch.Types[objectType]
	if !ok || typeDef == nil {
		return nil
	}

	issues := schema.ValidateFields(fields, typeDef.Fields, sch)
	if len(issues) == 0 {
		return nil
	}

	return &fieldValidationError{
		ObjectType: objectType,
		Issues:     issues,
	}
}

func warningMessagesToWarnings(messages []string) []Warning {
	warnings := make([]Warning, 0, len(messages))
	for _, message := range messages {
		warnings = append(warnings, Warning{
			Code:    WarnUnknownField,
			Message: message,
		})
	}
	return warnings
}

func serializeFieldValueLiteral(value schema.FieldValue) string {
	if value.IsNull() {
		return "null"
	}
	if ref, ok := value.AsRef(); ok {
		return "[[" + ref + "]]"
	}
	if arr, ok := value.AsArray(); ok {
		parts := make([]string, 0, len(arr))
		for _, item := range arr {
			parts = append(parts, serializeFieldValueLiteral(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	if s, ok := value.AsString(); ok {
		// Keep plain strings ergonomic unless they are ambiguous.
		lower := strings.ToLower(strings.TrimSpace(s))
		if lower == "true" || lower == "false" || lower == "null" {
			b, _ := json.Marshal(s)
			return string(b)
		}
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			b, _ := json.Marshal(s)
			return string(b)
		}
		return s
	}
	if n, ok := value.AsNumber(); ok {
		if n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	if b, ok := value.AsBool(); ok {
		return strconv.FormatBool(b)
	}
	b, err := json.Marshal(value.Raw())
	if err != nil {
		return fmt.Sprintf("%v", value.Raw())
	}
	return string(b)
}

func parseFieldValuesJSON(raw string) (map[string]schema.FieldValue, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	obj, err := parseJSONObject(raw)
	if err != nil {
		return nil, err
	}
	values := make(map[string]schema.FieldValue, len(obj))
	for key, value := range obj {
		values[key] = parser.FieldValueFromYAML(value)
	}
	return values, nil
}
