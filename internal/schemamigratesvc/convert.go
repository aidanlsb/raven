package schemamigratesvc

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/frontmatter"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type ConvertTraitRequest struct {
	TraitName  string
	TargetType string
	Mapping    map[string]interface{}
	Confirm    bool
}

type ConvertFieldRequest struct {
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
	Changes        []schemasvc.SchemaChange
	ChangesApplied int
	Hint           string
}

type valueConvertPlan struct {
	SchemaPlan    *schemasvc.ValueConvertPlan
	MarkdownFiles map[string][]byte
	Changes       []schemasvc.SchemaChange
}

type valueConversion struct {
	kind            string
	name            string
	typeName        string
	isTrait         bool
	traitName       string
	fieldName       string
	sourceType      schema.FieldType
	targetType      schema.FieldType
	traitDef        *schema.TraitDefinition
	fieldDef        *schema.FieldDefinition
	order           []string
	validate        func(fieldvalue.FieldValue, bool, []string) error
	hasDefault      bool
	defaultRaw      interface{}
	buildSchemaPlan func(newValues []string, newDefault interface{}, hasDefault bool) (*schemasvc.ValueConvertPlan, error)
}

func ConvertTrait(rt *vaultruntime.Runtime, req ConvertTraitRequest) (*ConvertResult, error) {
	conv, err := prepareTraitConversion(rt, req)
	if err != nil {
		return nil, err
	}
	return runValueConversion(rt, conv, req.Mapping, req.Confirm)
}

func ConvertField(rt *vaultruntime.Runtime, req ConvertFieldRequest) (*ConvertResult, error) {
	conv, err := prepareFieldConversion(rt, req)
	if err != nil {
		return nil, err
	}
	return runValueConversion(rt, conv, req.Mapping, req.Confirm)
}

func prepareTraitConversion(rt *vaultruntime.Runtime, req ConvertTraitRequest) (*valueConversion, error) {
	traitName := strings.TrimSpace(req.TraitName)
	if traitName == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "trait name cannot be empty").WithSuggestion("Usage: rvn schema convert trait <name> --map-json '<json>'")
	}

	schemaDoc, err := loadSchemaDocument(rt.VaultPath)
	if err != nil {
		return nil, err
	}
	traitDef, ok := schemaDoc.Schema().Traits[traitName]
	if !ok || traitDef == nil {
		return nil, svcerr.New(codes.ErrTraitNotFound, fmt.Sprintf("trait '%s' not found", traitName))
	}

	sourceType := normalizedConversionType(traitDef.Type, true)
	targetType, setType, err := resolveConversionTarget(sourceType, req.TargetType, true)
	if err != nil {
		return nil, err
	}
	if err := validateCollectionConversion(sourceType, targetType); err != nil {
		return nil, err
	}

	return &valueConversion{
		kind:       "trait",
		name:       traitName,
		isTrait:    true,
		traitName:  traitName,
		sourceType: sourceType,
		targetType: targetType,
		traitDef:   traitDef,
		order:      traitMappingOrder(traitDef, sourceType),
		hasDefault: traitDef.Default != nil,
		defaultRaw: traitDef.Default,
		validate: func(value fieldvalue.FieldValue, element bool, enumValues []string) error {
			targetDef := *traitDef
			targetDef.Type = targetType
			targetDef.Values = enumValues
			if element {
				if elem, ok := targetType.ElementType(); ok {
					targetDef.Type = elem
				}
			}
			if err := validateTraitLiteralValue(value); err != nil {
				return err
			}
			return schema.ValidateTraitValue(&targetDef, value)
		},
		buildSchemaPlan: func(newValues []string, newDefault interface{}, hasDefault bool) (*schemasvc.ValueConvertPlan, error) {
			return schemasvc.BuildTraitConvertPlan(schemasvc.ConvertTraitPlanRequest{
				SchemaDoc:  schemaDoc,
				TraitName:  traitName,
				SourceType: sourceType,
				TargetType: targetType,
				SetType:    setType,
				NewValues:  newValues,
				NewDefault: newDefault,
				HasDefault: hasDefault,
			})
		},
	}, nil
}

func prepareFieldConversion(rt *vaultruntime.Runtime, req ConvertFieldRequest) (*valueConversion, error) {
	typeName := strings.TrimSpace(req.TypeName)
	fieldName := strings.TrimSpace(req.FieldName)
	if typeName == "" || fieldName == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "type and field names cannot be empty").WithSuggestion("Usage: rvn schema convert field <type> <field> --map-json '<json>'")
	}
	if schema.IsBuiltinType(typeName) {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("cannot convert fields on built-in type '%s'", typeName))
	}

	schemaDoc, err := loadSchemaDocument(rt.VaultPath)
	if err != nil {
		return nil, err
	}
	typeDef, ok := schemaDoc.Schema().Types[typeName]
	if !ok || typeDef == nil {
		return nil, svcerr.New(codes.ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName))
	}
	fieldDef, ok := typeDef.Fields[fieldName]
	if !ok || fieldDef == nil {
		return nil, svcerr.New(codes.ErrFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName))
	}

	sourceType := normalizedConversionType(fieldDef.Type, false)
	targetType, setType, err := resolveConversionTarget(sourceType, req.TargetType, false)
	if err != nil {
		return nil, err
	}
	if typeDef.NameField == fieldName && targetType != schema.FieldTypeString {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("field '%s.%s' is the type's name_field and must remain string", typeName, fieldName)).WithSuggestion("Change name_field before converting this field to a non-string type")
	}
	if err := validateCollectionConversion(sourceType, targetType); err != nil {
		return nil, err
	}
	if targetType.IsRef() && !sourceType.IsRef() {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("cannot convert non-reference field '%s.%s' to '%s' without a reference target", typeName, fieldName, targetType)).WithSuggestion("The schema convert command does not infer ref targets; convert an existing ref/ref[] field so its target can be preserved")
	}

	return &valueConversion{
		kind:       "field",
		name:       fieldName,
		typeName:   typeName,
		fieldName:  fieldName,
		sourceType: sourceType,
		targetType: targetType,
		fieldDef:   fieldDef,
		order:      fieldMappingOrder(fieldDef, sourceType),
		hasDefault: fieldDef.Default != nil,
		defaultRaw: fieldDef.Default,
		validate: func(value fieldvalue.FieldValue, element bool, enumValues []string) error {
			targetDef := *fieldDef
			targetDef.Type = targetType
			targetDef.Values = enumValues
			if !targetType.IsRef() {
				targetDef.Target = ""
			}
			if element {
				if elem, ok := targetType.ElementType(); ok {
					targetDef.Type = elem
				}
			}
			errors := schema.ValidateFields(
				map[string]fieldvalue.FieldValue{fieldName: value},
				map[string]*schema.FieldDefinition{fieldName: &targetDef},
				schemaDoc.Schema(),
			)
			if len(errors) > 0 {
				return errors[0]
			}
			return nil
		},
		buildSchemaPlan: func(newValues []string, newDefault interface{}, hasDefault bool) (*schemasvc.ValueConvertPlan, error) {
			return schemasvc.BuildFieldConvertPlan(schemasvc.ConvertFieldPlanRequest{
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
		},
	}, nil
}

func runValueConversion(rt *vaultruntime.Runtime, conv *valueConversion, mapping map[string]interface{}, confirm bool) (*ConvertResult, error) {
	walkOptions, err := conversionWalkOptions(rt)
	if err != nil {
		return nil, err
	}

	mapper, newValues, err := buildConversionMapper(mapping, conv.sourceType, conv.targetType, conv.order, conv.validate)
	if err != nil {
		return nil, err
	}

	missing := make(map[string]struct{})
	enumValues := []string(nil)
	if conv.isTrait {
		enumValues = conv.traitDef.Values
	} else {
		enumValues = conv.fieldDef.Values
	}
	addFiniteRequiredValues(missing, mapper.values, conv.sourceType, enumValues)

	var newDefault interface{}
	if conv.hasDefault {
		defaultValue := parser.FieldValueFromYAML(conv.defaultRaw)
		mapped, ok := mapper.convert(defaultValue, missing)
		if ok {
			newDefault = frontmatter.FieldValueToYAMLValue(mapped)
		}
	}

	markdownFiles := make(map[string][]byte)
	changes := make([]schemasvc.SchemaChange, 0)
	err = vault.WalkMarkdownFilesWithOptions(rt.VaultPath, walkOptions, func(result vault.WalkResult) error {
		if result.Error != nil {
			return result.Error
		}
		if result.Document == nil {
			return nil
		}

		if conv.isTrait {
			staged, converted := stageTraitConversions(result.Document.RawContent, result.Document.Traits, conv.traitName, conv.traitDef, mapper, missing)
			if converted == 0 {
				return nil
			}
			markdownFiles[result.Path] = staged
			description := fmt.Sprintf("convert @%s value", conv.traitName)
			if converted > 1 {
				description = fmt.Sprintf("convert %d @%s values", converted, conv.traitName)
			}
			changes = append(changes, schemasvc.SchemaChange{
				FilePath:    result.RelativePath,
				ChangeType:  "trait_value",
				Description: description,
			})
			return nil
		}

		staged, changed, found := stageFieldConversion(result.Document.RawContent, conv.typeName, conv.fieldName, mapper, missing)
		if !found || !changed {
			return nil
		}
		markdownFiles[result.Path] = staged
		changes = append(changes, schemasvc.SchemaChange{
			FilePath:    result.RelativePath,
			ChangeType:  "frontmatter_value",
			Description: fmt.Sprintf("convert frontmatter field '%s.%s'", conv.typeName, conv.fieldName),
			Line:        1,
		})
		return nil
	})
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInternal, err.Error(), err)
	}
	if err := exhaustiveMappingError(missing); err != nil {
		return nil, err
	}

	schemaPlan, err := conv.buildSchemaPlan(newValues, newDefault, conv.hasDefault)
	if err != nil {
		return nil, err
	}

	plan := &valueConvertPlan{
		SchemaPlan:    schemaPlan,
		MarkdownFiles: markdownFiles,
		Changes:       append(append([]schemasvc.SchemaChange(nil), schemaPlan.Changes...), changes...),
	}
	return finishValueConversion(rt, confirm, conv.kind, conv.name, conv.typeName, conv.sourceType, conv.targetType, plan)
}
