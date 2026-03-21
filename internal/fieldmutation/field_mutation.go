package fieldmutation

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/schema"
)

type ValidationError struct {
	ObjectType string
	Issues     []schema.ValidationError
}

type UnknownFieldMutationError struct {
	ObjectType   string
	Unknown      []string
	Allowed      []string
	AllowedCount int
}

type RefValidationContext struct {
	VaultPath    string
	VaultConfig  *config.VaultConfig
	ParseOptions *parser.ParseOptions
}

func (e *ValidationError) Error() string {
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

func (e *ValidationError) Suggestion() string {
	if strings.TrimSpace(e.ObjectType) == "" {
		return "Ensure values match the schema field types"
	}
	return fmt.Sprintf("Ensure values match the schema field types for type '%s'", e.ObjectType)
}

func (e *UnknownFieldMutationError) Error() string {
	if len(e.Unknown) == 1 {
		return fmt.Sprintf("unknown field '%s' for type '%s'", e.Unknown[0], e.ObjectType)
	}
	return fmt.Sprintf("unknown fields for type '%s': %s", e.ObjectType, strings.Join(e.Unknown, ", "))
}

func (e *UnknownFieldMutationError) Suggestion() string {
	return fmt.Sprintf("Run 'rvn schema type %s' to view valid fields, or add missing fields with 'rvn schema add field %s <field_name> --type <field_type>'", e.ObjectType, e.ObjectType)
}

func (e *UnknownFieldMutationError) Details() map[string]interface{} {
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

func PrepareValidatedFieldMutation(
	objectType string,
	existingFields map[string]schema.FieldValue,
	updates map[string]string,
	sch *schema.Schema,
	allowedUnknown map[string]bool,
	refCtx *RefValidationContext,
) (map[string]schema.FieldValue, map[string]string, []string, error) {
	normalizedType := normalizeMutationType(objectType)
	fieldDefs := fieldDefsForObjectType(sch, normalizedType)
	resolvedUpdates := resolveDateKeywordsForUpdates(updates, fieldDefs)

	parsedUpdates := make(map[string]schema.FieldValue, len(resolvedUpdates))
	for fieldName, value := range resolvedUpdates {
		parsedUpdates[fieldName] = parseFieldValueToSchema(value)
	}

	validatedUpdates, warnings, err := PrepareValidatedFieldMutationValues(normalizedType, existingFields, parsedUpdates, sch, allowedUnknown, refCtx)
	if err != nil {
		return nil, nil, warnings, err
	}

	return validatedUpdates, resolvedUpdates, warnings, nil
}

func PrepareValidatedFrontmatterMutation(
	content string,
	fm *parser.Frontmatter,
	objectType string,
	updates map[string]string,
	sch *schema.Schema,
	allowedUnknown map[string]bool,
	refCtx *RefValidationContext,
) (string, map[string]string, []string, error) {
	normalizedType := normalizeMutationType(objectType)
	fieldDefs := fieldDefsForObjectType(sch, normalizedType)
	resolvedUpdates := resolveDateKeywordsForUpdates(updates, fieldDefs)
	typedUpdates := make(map[string]schema.FieldValue, len(resolvedUpdates))
	for key, value := range resolvedUpdates {
		typedUpdates[key] = parseFieldValueToSchema(value)
	}

	newContent, warnings, err := PrepareValidatedFrontmatterMutationValues(content, fm, normalizedType, typedUpdates, sch, allowedUnknown, refCtx)
	if err != nil {
		return "", nil, warnings, err
	}

	return newContent, resolvedUpdates, warnings, nil
}

func PrepareValidatedFieldMutationValues(
	objectType string,
	existingFields map[string]schema.FieldValue,
	updates map[string]schema.FieldValue,
	sch *schema.Schema,
	allowedUnknown map[string]bool,
	refCtx *RefValidationContext,
) (map[string]schema.FieldValue, []string, error) {
	normalizedType := normalizeMutationType(objectType)
	fieldDefs := fieldDefsForObjectType(sch, normalizedType)
	coercedUpdates := coerceFieldMutationValues(updates, fieldDefs)
	if unknownErr := DetectUnknownFieldMutationByNames(normalizedType, sch, fieldNamesFromValueUpdates(coercedUpdates), allowedUnknown); unknownErr != nil {
		return nil, nil, unknownErr
	}

	merged := make(map[string]schema.FieldValue, len(existingFields)+len(coercedUpdates))
	for key, value := range existingFields {
		merged[key] = value
	}
	for key, value := range coercedUpdates {
		merged[key] = value
	}

	if err := validateMergedFields(normalizedType, merged, sch, refCtx); err != nil {
		return nil, nil, err
	}

	return coercedUpdates, nil, nil
}

func PrepareValidatedFrontmatterMutationValues(
	content string,
	fm *parser.Frontmatter,
	objectType string,
	updates map[string]schema.FieldValue,
	sch *schema.Schema,
	allowedUnknown map[string]bool,
	refCtx *RefValidationContext,
) (string, []string, error) {
	existingFields := make(map[string]schema.FieldValue)
	if fm != nil && fm.Fields != nil {
		for key, value := range fm.Fields {
			existingFields[key] = value
		}
	}

	validatedUpdates, warnings, err := PrepareValidatedFieldMutationValues(objectType, existingFields, updates, sch, allowedUnknown, refCtx)
	if err != nil {
		return "", warnings, err
	}

	newContent, err := updateFrontmatterWithFieldValues(content, validatedUpdates)
	if err != nil {
		return "", warnings, err
	}

	updatedFM, err := parser.ParseFrontmatter(newContent)
	if err != nil {
		return "", warnings, err
	}
	if updatedFM == nil {
		return "", warnings, fmt.Errorf("file has no frontmatter after update")
	}
	if err := validateMergedFields(normalizeMutationType(objectType), updatedFM.Fields, sch, refCtx); err != nil {
		return "", warnings, err
	}

	return newContent, warnings, nil
}

func DetectUnknownFieldMutationByNames(
	objectType string,
	sch *schema.Schema,
	fieldNames []string,
	allowedUnknown map[string]bool,
) *UnknownFieldMutationError {
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

	return &UnknownFieldMutationError{
		ObjectType:   objectType,
		Unknown:      unknown,
		Allowed:      allowed,
		AllowedCount: len(allowed),
	}
}

func SerializeFieldValueLiteral(value schema.FieldValue) string {
	if value.IsNull() {
		return "null"
	}
	if ref, ok := value.AsRef(); ok {
		return "[[" + ref + "]]"
	}
	if arr, ok := value.AsArray(); ok {
		parts := make([]string, 0, len(arr))
		for _, item := range arr {
			parts = append(parts, SerializeFieldValueLiteral(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	if s, ok := value.AsString(); ok {
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

func ParseFieldValuesJSON(raw string) (map[string]schema.FieldValue, error) {
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

func normalizeMutationType(objectType string) string {
	if strings.TrimSpace(objectType) == "" {
		return "page"
	}
	return objectType
}

func fieldDefsForObjectType(sch *schema.Schema, objectType string) map[string]*schema.FieldDefinition {
	if sch == nil {
		return nil
	}
	if objectType == "" {
		objectType = "page"
	}
	typeDef, ok := sch.Types[objectType]
	if !ok || typeDef == nil {
		return nil
	}
	return typeDef.Fields
}

func resolveDateKeywordsForUpdates(updates map[string]string, fieldDefs map[string]*schema.FieldDefinition) map[string]string {
	if fieldDefs == nil {
		return updates
	}

	resolved := make(map[string]string, len(updates))
	for field, value := range updates {
		resolved[field] = resolveDateKeywordForFieldValue(value, fieldDefs[field])
	}
	return resolved
}

func resolveDateKeywordForFieldValue(value string, fieldDef *schema.FieldDefinition) string {
	if fieldDef == nil {
		return value
	}

	switch fieldDef.Type {
	case schema.FieldTypeDate:
		if resolved, ok := resolveRelativeDateKeyword(value); ok {
			return resolved
		}
	case schema.FieldTypeDateArray:
		if resolved, ok := resolveDateKeywordList(value); ok {
			return resolved
		}
	}

	return value
}

func resolveRelativeDateKeyword(value string) (string, bool) {
	resolved, ok := dates.ResolveRelativeDateKeyword(value, time.Now(), time.Monday)
	if !ok || resolved.Kind != dates.RelativeDateInstant {
		return "", false
	}
	return resolved.Date.Format(dates.DateLayout), true
}

func resolveDateKeywordList(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return "", false
	}

	inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if inner == "" {
		return "", false
	}

	parts := strings.Split(inner, ",")
	changed := false
	for i, part := range parts {
		part = strings.TrimSpace(part)
		unquoted := strings.Trim(part, `"'`)
		if resolved, ok := resolveRelativeDateKeyword(unquoted); ok {
			parts[i] = resolved
			changed = true
		} else {
			parts[i] = part
		}
	}

	if !changed {
		return "", false
	}

	return "[" + strings.Join(parts, ", ") + "]", true
}

func parseFieldValueToSchema(value string) schema.FieldValue {
	if strings.EqualFold(strings.TrimSpace(value), "null") {
		return schema.Null()
	}
	return parser.ParseFieldValue(value)
}

func updateFrontmatterWithFieldValues(content string, updates map[string]schema.FieldValue) (string, error) {
	lines := strings.Split(content, "\n")

	startLine, endLine, ok := parser.FrontmatterBounds(lines)
	if !ok {
		return "", fmt.Errorf("no frontmatter found")
	}
	if endLine == -1 {
		return "", fmt.Errorf("unclosed frontmatter")
	}

	frontmatterContent := strings.Join(lines[startLine+1:endLine], "\n")
	var yamlData map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterContent), &yamlData); err != nil {
		return "", fmt.Errorf("failed to parse frontmatter: %w", err)
	}
	if yamlData == nil {
		yamlData = make(map[string]interface{})
	}

	for key, value := range updates {
		yamlData[key] = fieldValueToYAMLValue(value)
	}

	newFrontmatter, err := yaml.Marshal(yamlData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	var result strings.Builder
	result.WriteString("---\n")
	result.Write(newFrontmatter)
	result.WriteString("---")

	if endLine+1 < len(lines) {
		result.WriteString("\n")
		result.WriteString(strings.Join(lines[endLine+1:], "\n"))
	}

	return result.String(), nil
}

func fieldValueToYAMLValue(value schema.FieldValue) interface{} {
	if value.IsNull() {
		return nil
	}
	if ref, ok := value.AsRef(); ok {
		return "[[" + ref + "]]"
	}
	if arr, ok := value.AsArray(); ok {
		items := make([]interface{}, 0, len(arr))
		for _, item := range arr {
			items = append(items, fieldValueToYAMLValue(item))
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

func fieldNamesFromValueUpdates(updates map[string]schema.FieldValue) []string {
	names := make([]string, 0, len(updates))
	for name := range updates {
		names = append(names, name)
	}
	return names
}

func validateMergedFields(objectType string, fields map[string]schema.FieldValue, sch *schema.Schema, refCtx *RefValidationContext) error {
	if sch == nil {
		return nil
	}

	typeDef, ok := sch.Types[objectType]
	if !ok || typeDef == nil {
		return nil
	}

	issues := schema.ValidateFields(fields, typeDef.Fields, sch)
	if len(issues) == 0 {
		issues = validateRefTargets(fields, typeDef.Fields, sch, refCtx)
	}
	if len(issues) == 0 {
		return nil
	}

	return &ValidationError{
		ObjectType: objectType,
		Issues:     issues,
	}
}

func validateRefTargets(
	fields map[string]schema.FieldValue,
	fieldDefs map[string]*schema.FieldDefinition,
	sch *schema.Schema,
	refCtx *RefValidationContext,
) []schema.ValidationError {
	if refCtx == nil || strings.TrimSpace(refCtx.VaultPath) == "" || refCtx.VaultConfig == nil {
		return nil
	}

	rt := &readsvc.Runtime{
		VaultPath: refCtx.VaultPath,
		VaultCfg:  refCtx.VaultConfig,
		Schema:    sch,
	}

	db, err := index.Open(refCtx.VaultPath)
	if err == nil {
		db.SetDailyDirectory(refCtx.VaultConfig.GetDailyDirectory())
		rt.DB = db
		defer db.Close()
	}

	parseOpts := refCtx.ParseOptions
	if parseOpts == nil {
		parseOpts = &parser.ParseOptions{
			ObjectsRoot: refCtx.VaultConfig.GetObjectsRoot(),
			PagesRoot:   refCtx.VaultConfig.GetPagesRoot(),
		}
	}

	var issues []schema.ValidationError
	for fieldName, fieldDef := range fieldDefs {
		if fieldDef == nil || fieldDef.Target == "" {
			continue
		}
		if fieldDef.Type != schema.FieldTypeRef && fieldDef.Type != schema.FieldTypeRefArray {
			continue
		}

		value, ok := fields[fieldName]
		if !ok || value.IsNull() {
			continue
		}

		refs := parser.ExtractRefsFromFieldValue(value, parser.RefExtractOptions{
			AllowBareStrings: true,
		})
		for _, ref := range refs {
			actualType, resolveErr := resolveReferenceType(rt, parseOpts, ref.TargetRaw)
			if resolveErr != nil || actualType == "" {
				continue
			}
			if actualType != fieldDef.Target {
				issues = append(issues, schema.ValidationError{
					Field:   fieldName,
					Message: fmt.Sprintf("reference [[%s]] resolves to type '%s', expected '%s'", ref.TargetRaw, actualType, fieldDef.Target),
				})
				break
			}
		}
	}

	return issues
}

func resolveReferenceType(rt *readsvc.Runtime, parseOpts *parser.ParseOptions, rawRef string) (string, error) {
	resolved, err := readsvc.ResolveReference(rawRef, rt, false)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(resolved.FilePath)
	if err != nil {
		return "", err
	}

	doc, err := parser.ParseDocumentWithOptions(string(content), resolved.FilePath, rt.VaultPath, parseOpts)
	if err != nil {
		return "", err
	}

	for _, obj := range doc.Objects {
		if obj.ID == resolved.ObjectID {
			return obj.ObjectType, nil
		}
	}

	return "", fmt.Errorf("resolved object %q not found in parsed document", resolved.ObjectID)
}

func parseJSONObject(raw string) (map[string]interface{}, error) {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return obj, nil
}
