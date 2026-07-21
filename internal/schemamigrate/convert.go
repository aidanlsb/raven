package schemamigrate

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/frontmatter"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/vault"
)

type ConvertTraitRequest struct {
	VaultPath  string
	TraitName  string
	TargetType string
	Mapping    map[string]interface{}
	Confirm    bool
}

type ConvertFieldRequest struct {
	VaultPath  string
	TypeName   string
	FieldName  string
	TargetType string
	Mapping    map[string]interface{}
	Confirm    bool
}

type ConvertResult struct {
	Preview        bool
	Kind           string
	Name           string
	TypeName       string
	SourceType     string
	TargetType     string
	TotalChanges   int
	Changes        []schemasvc.ValueConvertChange
	ChangesApplied int
	Hint           string
}

type valueConvertPlan struct {
	SchemaPlan    *schemasvc.ValueConvertPlan
	MarkdownFiles map[string][]byte
	Changes       []schemasvc.ValueConvertChange
}

type conversionMapper struct {
	sourceType schema.FieldType
	targetType schema.FieldType
	values     map[string]schema.FieldValue
}

func ConvertTrait(req ConvertTraitRequest) (*ConvertResult, error) {
	traitName := strings.TrimSpace(req.TraitName)
	if traitName == "" {
		return nil, newError(schemasvc.ErrorInvalidInput, "trait name cannot be empty", "Usage: rvn schema convert trait <name> --map-json '<json>'", nil, nil)
	}

	schemaDoc, err := loadSchemaDocument(req.VaultPath)
	if err != nil {
		return nil, err
	}
	traitDef, ok := schemaDoc.Schema().Traits[traitName]
	if !ok || traitDef == nil {
		return nil, newError(schemasvc.ErrorTraitNotFound, fmt.Sprintf("trait '%s' not found", traitName), "", nil, nil)
	}

	sourceType := normalizedConversionType(traitDef.Type, true)
	targetType, setType, err := resolveConversionTarget(sourceType, req.TargetType, true)
	if err != nil {
		return nil, err
	}
	if err := validateCollectionConversion(sourceType, targetType); err != nil {
		return nil, err
	}
	walkOptions, err := conversionWalkOptions(req.VaultPath)
	if err != nil {
		return nil, err
	}

	mapper, newValues, err := buildConversionMapper(req.Mapping, sourceType, targetType, traitMappingOrder(traitDef, sourceType), func(value schema.FieldValue, element bool, enumValues []string) error {
		targetDef := *traitDef
		targetDef.Type = targetType
		targetDef.Values = enumValues
		if element {
			targetDef.Type = arrayElementType(targetType)
		}
		if err := validateTraitLiteralValue(value); err != nil {
			return err
		}
		return schema.ValidateTraitValue(&targetDef, value)
	})
	if err != nil {
		return nil, err
	}

	missing := make(map[string]struct{})
	addFiniteRequiredValues(missing, mapper.values, sourceType, traitDef.Values)

	var newDefault interface{}
	hasDefault := traitDef.Default != nil
	if hasDefault {
		defaultValue := parser.FieldValueFromYAML(traitDef.Default)
		mapped, ok := mapper.convert(defaultValue, missing)
		if ok {
			newDefault = frontmatter.FieldValueToYAMLValue(mapped)
		}
	}

	markdownFiles := make(map[string][]byte)
	changes := make([]schemasvc.ValueConvertChange, 0)
	err = vault.WalkMarkdownFilesWithOptions(req.VaultPath, walkOptions, func(result vault.WalkResult) error {
		if result.Error != nil {
			return result.Error
		}
		if result.Document == nil {
			return nil
		}

		staged, converted := stageTraitConversions(result.Document.RawContent, result.Document.Traits, traitName, traitDef, mapper, missing)
		if converted == 0 {
			return nil
		}
		markdownFiles[result.Path] = staged
		description := fmt.Sprintf("convert @%s value", traitName)
		if converted > 1 {
			description = fmt.Sprintf("convert %d @%s values", converted, traitName)
		}
		changes = append(changes, schemasvc.ValueConvertChange{
			FilePath:    result.RelativePath,
			ChangeType:  "trait_value",
			Description: description,
		})
		return nil
	})
	if err != nil {
		return nil, newError(schemasvc.ErrorInternal, err.Error(), "", nil, err)
	}
	if err := exhaustiveMappingError(missing); err != nil {
		return nil, err
	}

	schemaPlan, err := schemasvc.BuildTraitConvertPlan(schemasvc.ConvertTraitPlanRequest{
		SchemaDoc:  schemaDoc,
		TraitName:  traitName,
		SourceType: sourceType,
		TargetType: targetType,
		SetType:    setType,
		NewValues:  newValues,
		NewDefault: newDefault,
		HasDefault: hasDefault,
	})
	if err != nil {
		return nil, err
	}

	plan := &valueConvertPlan{
		SchemaPlan:    schemaPlan,
		MarkdownFiles: markdownFiles,
		Changes:       append(append([]schemasvc.ValueConvertChange(nil), schemaPlan.Changes...), changes...),
	}
	return finishValueConversion(req.VaultPath, req.Confirm, "trait", traitName, "", sourceType, targetType, plan)
}

func ConvertField(req ConvertFieldRequest) (*ConvertResult, error) {
	typeName := strings.TrimSpace(req.TypeName)
	fieldName := strings.TrimSpace(req.FieldName)
	if typeName == "" || fieldName == "" {
		return nil, newError(schemasvc.ErrorInvalidInput, "type and field names cannot be empty", "Usage: rvn schema convert field <type> <field> --map-json '<json>'", nil, nil)
	}
	if schema.IsBuiltinType(typeName) {
		return nil, newError(schemasvc.ErrorInvalidInput, fmt.Sprintf("cannot convert fields on built-in type '%s'", typeName), "", nil, nil)
	}

	schemaDoc, err := loadSchemaDocument(req.VaultPath)
	if err != nil {
		return nil, err
	}
	typeDef, ok := schemaDoc.Schema().Types[typeName]
	if !ok || typeDef == nil {
		return nil, newError(schemasvc.ErrorTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "", nil, nil)
	}
	fieldDef, ok := typeDef.Fields[fieldName]
	if !ok || fieldDef == nil {
		return nil, newError(schemasvc.ErrorFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName), "", nil, nil)
	}

	sourceType := normalizedConversionType(fieldDef.Type, false)
	targetType, setType, err := resolveConversionTarget(sourceType, req.TargetType, false)
	if err != nil {
		return nil, err
	}
	if typeDef.NameField == fieldName && targetType != schema.FieldTypeString {
		return nil, newError(
			schemasvc.ErrorInvalidInput,
			fmt.Sprintf("field '%s.%s' is the type's name_field and must remain string", typeName, fieldName),
			"Change name_field before converting this field to a non-string type",
			nil,
			nil,
		)
	}
	if err := validateCollectionConversion(sourceType, targetType); err != nil {
		return nil, err
	}
	if isRefConversionType(targetType) && !isRefConversionType(sourceType) {
		return nil, newError(
			schemasvc.ErrorInvalidInput,
			fmt.Sprintf("cannot convert non-reference field '%s.%s' to '%s' without a reference target", typeName, fieldName, targetType),
			"The schema convert command does not infer ref targets; convert an existing ref/ref[] field so its target can be preserved",
			nil,
			nil,
		)
	}
	walkOptions, err := conversionWalkOptions(req.VaultPath)
	if err != nil {
		return nil, err
	}

	order := fieldMappingOrder(fieldDef, sourceType)
	mapper, newValues, err := buildConversionMapper(req.Mapping, sourceType, targetType, order, func(value schema.FieldValue, element bool, enumValues []string) error {
		targetDef := *fieldDef
		targetDef.Type = targetType
		targetDef.Values = enumValues
		if !isRefConversionType(targetType) {
			targetDef.Target = ""
		}
		if element {
			targetDef.Type = arrayElementType(targetType)
		}
		errors := schema.ValidateFields(
			map[string]schema.FieldValue{fieldName: value},
			map[string]*schema.FieldDefinition{fieldName: &targetDef},
			schemaDoc.Schema(),
		)
		if len(errors) > 0 {
			return errors[0]
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	missing := make(map[string]struct{})
	addFiniteRequiredValues(missing, mapper.values, sourceType, fieldDef.Values)

	var newDefault interface{}
	hasDefault := fieldDef.Default != nil
	if hasDefault {
		defaultValue := parser.FieldValueFromYAML(fieldDef.Default)
		mapped, ok := mapper.convert(defaultValue, missing)
		if ok {
			newDefault = frontmatter.FieldValueToYAMLValue(mapped)
		}
	}

	markdownFiles := make(map[string][]byte)
	changes := make([]schemasvc.ValueConvertChange, 0)
	err = vault.WalkMarkdownFilesWithOptions(req.VaultPath, walkOptions, func(result vault.WalkResult) error {
		if result.Error != nil {
			return result.Error
		}
		if result.Document == nil {
			return nil
		}

		staged, changed, found := stageFieldConversion(result.Document.RawContent, typeName, fieldName, mapper, missing)
		if !found || !changed {
			return nil
		}
		markdownFiles[result.Path] = staged
		changes = append(changes, schemasvc.ValueConvertChange{
			FilePath:    result.RelativePath,
			ChangeType:  "frontmatter_value",
			Description: fmt.Sprintf("convert frontmatter field '%s.%s'", typeName, fieldName),
			Line:        1,
		})
		return nil
	})
	if err != nil {
		return nil, newError(schemasvc.ErrorInternal, err.Error(), "", nil, err)
	}
	if err := exhaustiveMappingError(missing); err != nil {
		return nil, err
	}

	schemaPlan, err := schemasvc.BuildFieldConvertPlan(schemasvc.ConvertFieldPlanRequest{
		SchemaDoc:  schemaDoc,
		TypeName:   typeName,
		FieldName:  fieldName,
		SourceType: sourceType,
		TargetType: targetType,
		SetType:    setType,
		NewValues:  newValues,
		NewDefault: newDefault,
		HasDefault: hasDefault,
	})
	if err != nil {
		return nil, err
	}

	plan := &valueConvertPlan{
		SchemaPlan:    schemaPlan,
		MarkdownFiles: markdownFiles,
		Changes:       append(append([]schemasvc.ValueConvertChange(nil), schemaPlan.Changes...), changes...),
	}
	return finishValueConversion(req.VaultPath, req.Confirm, "field", fieldName, typeName, sourceType, targetType, plan)
}

func finishValueConversion(
	vaultPath string,
	confirm bool,
	kind, name, typeName string,
	sourceType, targetType schema.FieldType,
	plan *valueConvertPlan,
) (*ConvertResult, error) {
	result := &ConvertResult{
		Preview:      !confirm,
		Kind:         kind,
		Name:         name,
		TypeName:     typeName,
		SourceType:   string(sourceType),
		TargetType:   string(targetType),
		TotalChanges: len(plan.Changes),
		Changes:      plan.Changes,
		Hint:         "Run with --confirm to apply changes",
	}
	if !confirm {
		return result, nil
	}

	applied, err := applyValueConvertPlan(vaultPath, plan)
	if err != nil {
		return nil, err
	}
	result.Preview = false
	result.Changes = nil
	result.TotalChanges = 0
	result.ChangesApplied = applied
	result.Hint = "Run 'rvn reindex --full' to update the index, then run 'rvn check'"
	return result, nil
}

func applyValueConvertPlan(vaultPath string, plan *valueConvertPlan) (int, error) {
	applied := 0
	if plan.SchemaPlan.SchemaMutations > 0 {
		if err := schemadoc.Write(vaultPath, plan.SchemaPlan.SchemaYAML); err != nil {
			return 0, schemasvc.MapSchemaDocError(err, "", schemasvc.ErrorSchemaInvalid)
		}
		applied += plan.SchemaPlan.SchemaMutations
	}
	for _, path := range sortedStringKeys(plan.MarkdownFiles) {
		if err := atomicfile.WriteFile(path, plan.MarkdownFiles[path], 0o644); err != nil {
			return 0, newError(schemasvc.ErrorFileWrite, err.Error(), "Some files may already be converted; review the vault and run 'rvn reindex --full'", nil, err)
		}
		applied++
	}
	return applied, nil
}

func buildConversionMapper(
	rawMapping map[string]interface{},
	sourceType, targetType schema.FieldType,
	order []string,
	validate func(schema.FieldValue, bool, []string) error,
) (*conversionMapper, []string, error) {
	if rawMapping == nil {
		return nil, nil, newError(
			schemasvc.ErrorInvalidInput,
			"--map-json must be a JSON object",
			`Provide an exhaustive map, for example --map-json '{"high":true,"low":false}'`,
			nil,
			nil,
		)
	}
	if len(rawMapping) == 0 {
		return nil, nil, newError(schemasvc.ErrorInvalidInput, "--map-json must contain at least one mapping", "", nil, nil)
	}

	mapper := &conversionMapper{
		sourceType: sourceType,
		targetType: targetType,
		values:     make(map[string]schema.FieldValue, len(rawMapping)),
	}
	orderedKeys := orderedMappingKeys(rawMapping, order)
	elementMapping := isArrayConversionType(sourceType) && isArrayConversionType(targetType)
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
	if isEnumConversionType(targetType) && len(newValues) == 0 {
		return nil, nil, newError(schemasvc.ErrorInvalidInput, "enum conversion must produce at least one string value", "", nil, nil)
	}
	for _, key := range orderedKeys {
		if err := validate(mapper.values[key], elementMapping, newValues); err != nil {
			return nil, nil, invalidMappingValueError(key, targetType, err)
		}
	}
	return mapper, newValues, nil
}

func invalidMappingValueError(key string, targetType schema.FieldType, cause error) error {
	return newError(
		schemasvc.ErrorInvalidInput,
		fmt.Sprintf("mapping for %q is invalid for target type '%s': %v", key, targetType, cause),
		"Use JSON values in the target type's representation",
		map[string]interface{}{"map_key": key, "target_type": targetType},
		cause,
	)
}

func (m *conversionMapper) convert(value schema.FieldValue, missing map[string]struct{}) (schema.FieldValue, bool) {
	if isArrayConversionType(m.sourceType) && isArrayConversionType(m.targetType) {
		items, ok := value.AsArray()
		if !ok {
			items = []schema.FieldValue{value}
		}
		converted := make([]schema.FieldValue, 0, len(items))
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
		return schema.Array(converted), complete
	}

	key := conversionMapKey(value)
	mapped, ok := m.values[key]
	if !ok {
		missing[key] = struct{}{}
		return schema.Null(), false
	}
	return mapped, true
}

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

func effectiveTraitAnnotationValue(value *schema.FieldValue, def *schema.TraitDefinition) schema.FieldValue {
	if value != nil {
		return *value
	}
	if def != nil && def.Default != nil {
		return parser.FieldValueFromYAML(def.Default)
	}
	if def != nil && normalizedConversionType(def.Type, true) == schema.FieldTypeBool {
		return schema.Bool(true)
	}
	return schema.Null()
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
		return "", false, newError(
			schemasvc.ErrorInvalidInput,
			fmt.Sprintf("unsupported target type '%s'", requested),
			fmt.Sprintf("Use one of: %s", validTypes),
			map[string]interface{}{"target_type": requested, "valid_types": validTypes},
			nil,
		)
	}
	return targetType, true, nil
}

func validateCollectionConversion(sourceType, targetType schema.FieldType) error {
	if isArrayConversionType(sourceType) && !isArrayConversionType(targetType) {
		return newError(
			schemasvc.ErrorInvalidInput,
			fmt.Sprintf("cannot convert collection type '%s' to scalar type '%s'", sourceType, targetType),
			"Collection-to-scalar conversion has no unambiguous reduction rule; convert to another [] type",
			nil,
			nil,
		)
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
	mapping map[string]schema.FieldValue,
	sourceType schema.FieldType,
	enumValues []string,
) {
	addIfMissing := func(value string) {
		if _, exists := mapping[value]; !exists {
			missing[value] = struct{}{}
		}
	}
	switch sourceType {
	case schema.FieldTypeEnum, schema.FieldTypeEnumArray:
		for _, value := range enumValues {
			addIfMissing(value)
		}
	case schema.FieldTypeBool, schema.FieldTypeBoolArray:
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
	return newError(
		schemasvc.ErrorInvalidInput,
		fmt.Sprintf("mapping is not exhaustive; missing %d value(s): %s", len(missing), strings.Join(quotedValues(missing), ", ")),
		"Add every schema-allowed and observed live value to --map-json",
		map[string]interface{}{"missing_values": missing},
		nil,
	)
}

func quotedValues(values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strconv.Quote(value)
	}
	return out
}

func conversionMapKey(value schema.FieldValue) string {
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

func validateTraitLiteralValue(value schema.FieldValue) error {
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

func serializeTraitConversionLiteral(value schema.FieldValue, inArray bool) string {
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
	switch sourceType {
	case schema.FieldTypeEnum, schema.FieldTypeEnumArray:
		return append([]string(nil), enumValues...)
	case schema.FieldTypeBool, schema.FieldTypeBoolArray:
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
	mapping map[string]schema.FieldValue,
	order []string,
	sourceType, targetType schema.FieldType,
) []string {
	if !isEnumConversionType(targetType) {
		return nil
	}
	seen := make(map[string]struct{})
	values := make([]string, 0)
	appendValue := func(value schema.FieldValue) {
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
		if isArrayConversionType(targetType) && !isArrayConversionType(sourceType) {
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

func mappingValueForTarget(raw interface{}, targetType schema.FieldType, elementMapping bool) (schema.FieldValue, error) {
	effectiveType := targetType
	if elementMapping {
		effectiveType = arrayElementType(targetType)
	}
	if isArrayConversionType(effectiveType) {
		elementType := arrayElementType(effectiveType)
		switch values := raw.(type) {
		case []interface{}:
			items := make([]schema.FieldValue, 0, len(values))
			for _, item := range values {
				converted, err := mappingScalarForTarget(item, elementType)
				if err != nil {
					return schema.Null(), err
				}
				items = append(items, converted)
			}
			return schema.Array(items), nil
		case []string:
			items := make([]schema.FieldValue, 0, len(values))
			for _, item := range values {
				converted, err := mappingScalarForTarget(item, elementType)
				if err != nil {
					return schema.Null(), err
				}
				items = append(items, converted)
			}
			return schema.Array(items), nil
		default:
			return schema.Null(), fmt.Errorf("expected a JSON array")
		}
	}
	if _, isArray := raw.([]interface{}); isArray {
		return schema.Null(), fmt.Errorf("expected a scalar JSON value")
	}
	if _, isArray := raw.([]string); isArray {
		return schema.Null(), fmt.Errorf("expected a scalar JSON value")
	}
	return mappingScalarForTarget(raw, effectiveType)
}

func mappingScalarForTarget(raw interface{}, targetType schema.FieldType) (schema.FieldValue, error) {
	if raw == nil {
		return schema.Null(), fmt.Errorf("null is not a schema value type")
	}
	switch targetType {
	case schema.FieldTypeString, schema.FieldTypeURL, schema.FieldTypeDate, schema.FieldTypeDatetime:
		value, ok := raw.(string)
		if !ok {
			return schema.Null(), fmt.Errorf("expected a JSON string")
		}
		return schema.String(value), nil
	case schema.FieldTypeEnum:
		value, ok := raw.(string)
		if !ok {
			return schema.Null(), fmt.Errorf("expected a JSON string")
		}
		if parser.FieldValueFromYAML(value).IsRef() {
			return schema.Null(), fmt.Errorf("enum values cannot use wikilink syntax")
		}
		return schema.String(value), nil
	case schema.FieldTypeRef:
		value, ok := raw.(string)
		if !ok {
			return schema.Null(), fmt.Errorf("expected a JSON string containing a reference")
		}
		return parser.FieldValueFromYAML(value), nil
	case schema.FieldTypeBool:
		value, ok := raw.(bool)
		if !ok {
			return schema.Null(), fmt.Errorf("expected a JSON boolean")
		}
		return schema.Bool(value), nil
	case schema.FieldTypeNumber:
		number, ok := jsonNumber(raw)
		if !ok {
			return schema.Null(), fmt.Errorf("expected a JSON number")
		}
		return schema.Number(number), nil
	default:
		return schema.Null(), fmt.Errorf("unsupported target type '%s'", targetType)
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

func isArrayConversionType(fieldType schema.FieldType) bool {
	return strings.HasSuffix(string(fieldType), "[]")
}

func isEnumConversionType(fieldType schema.FieldType) bool {
	return fieldType == schema.FieldTypeEnum || fieldType == schema.FieldTypeEnumArray
}

func isRefConversionType(fieldType schema.FieldType) bool {
	return fieldType == schema.FieldTypeRef || fieldType == schema.FieldTypeRefArray
}

func arrayElementType(fieldType schema.FieldType) schema.FieldType {
	return schema.FieldType(strings.TrimSuffix(string(fieldType), "[]"))
}

func conversionWalkOptions(vaultPath string) (*vault.WalkOptions, error) {
	vaultConfig, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return nil, newError(schemasvc.ErrorConfigInvalid, "failed to load raven.yaml", "Fix raven.yaml and try again", nil, err)
	}
	matcher, err := ravenignore.NewMatcher(vaultConfig.GetExcludePatterns())
	if err != nil {
		return nil, newError(schemasvc.ErrorConfigInvalid, "invalid exclude configuration in raven.yaml", "Fix raven.yaml and try again", nil, err)
	}
	return &vault.WalkOptions{ExcludeMatcher: matcher}, nil
}
