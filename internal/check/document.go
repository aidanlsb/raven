package check

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

// ValidateDocument validates a parsed document.
func (v *Validator) ValidateDocument(doc *parser.ParsedDocument) []Issue {
	var issues []Issue

	seenIDs := make(map[string]struct{})
	for _, obj := range doc.Objects {
		if _, exists := seenIDs[obj.ID]; exists {
			issues = append(issues, Issue{
				Level:    LevelError,
				Type:     IssueDuplicateID,
				FilePath: doc.FilePath,
				Line:     obj.LineStart,
				Message:  fmt.Sprintf("Duplicate object ID '%s'", obj.ID),
				Value:    obj.ID,
				FixHint:  "Rename one of the duplicate objects",
			})
		}
		seenIDs[obj.ID] = struct{}{}
	}

	for _, obj := range doc.Objects {
		issues = append(issues, v.validateObject(doc.FilePath, obj)...)
	}

	for _, trait := range doc.Traits {
		issues = append(issues, v.validateTrait(doc.FilePath, trait)...)
	}

	for _, ref := range doc.Refs {
		issues = append(issues, v.validateRef(doc.FilePath, ref)...)
	}

	return issues
}

func (v *Validator) validateObject(filePath string, obj *model.Object) []Issue {
	var issues []Issue

	v.usedTypes[obj.Type] = struct{}{}

	typeDef, typeExists := v.schema.Types[obj.Type]
	if !typeExists && !schema.IsBuiltinType(obj.Type) {
		issues = append(issues, Issue{
			Level:      LevelError,
			Type:       IssueUnknownType,
			FilePath:   filePath,
			Line:       obj.LineStart,
			Message:    fmt.Sprintf("Unknown type '%s'", obj.Type),
			Value:      obj.Type,
			FixCommand: fmt.Sprintf("rvn schema add type %s", obj.Type),
			FixHint:    fmt.Sprintf("Add type '%s' to schema", obj.Type),
		})
		return issues
	}

	// Section IDs are derived from heading text, so there is no separate object ID check here.

	// Validate fields against schema (including unknown frontmatter keys).
	if typeDef != nil {
		fieldErrors := schema.ValidateFields(obj.Fields, typeDef.Fields, v.schema)
		for _, err := range fieldErrors {
			if err.Message == schema.MsgUnknownFrontmatterKey {
				issues = append(issues, Issue{
					Level:      LevelError,
					Type:       IssueUnknownFrontmatter,
					FilePath:   filePath,
					Line:       obj.LineStart,
					Message:    fmt.Sprintf("Unknown frontmatter key '%s' for type '%s'", err.Field, obj.Type),
					Value:      err.Field,
					FixCommand: fmt.Sprintf("rvn schema add field %s %s", obj.Type, err.Field),
					FixHint:    fmt.Sprintf("Add field '%s' to type '%s', or remove it from the file", err.Field, obj.Type),
				})
				continue
			}
			issueType := IssueInvalidFieldValue
			fixHint := "Fix or remove the invalid field value"
			if err.Message == "Required field is missing" {
				issueType = IssueMissingRequiredField
				fixHint = "Add the required field to the file's frontmatter"
			}
			issues = append(issues, Issue{
				Level:    LevelError,
				Type:     issueType,
				FilePath: filePath,
				Line:     obj.LineStart,
				Message:  err.Error(),
				FixHint:  fixHint,
			})
		}

		schemaRefs := parser.ExtractSchemaFieldRefs([]*model.Object{obj}, v.schema)
		for _, schemaRef := range schemaRefs {
			fieldDef := typeDef.Fields[schemaRef.FieldName]
			if fieldDef == nil {
				continue
			}
			syntheticRef := model.NewInlineReference(obj.ID, schemaRef.TargetRaw, nil, schemaRef.Line, 0, 0)
			refIssues := v.validateRefWithContext(filePath, obj.ID, syntheticRef, fieldDef.Target, schemaRef.FieldName)
			issues = append(issues, refIssues...)
		}
	}

	return issues
}

func (v *Validator) validateTrait(filePath string, trait *model.Trait) []Issue {
	var issues []Issue

	v.usedTraits[trait.TraitType] = struct{}{}

	traitDef, exists := v.schema.Traits[trait.TraitType]
	if !exists {
		issues = append(issues, Issue{
			Level:      LevelWarning,
			Type:       IssueUndefinedTrait,
			FilePath:   filePath,
			Line:       trait.Line,
			Message:    fmt.Sprintf("Undefined trait '@%s'", trait.TraitType),
			Value:      trait.TraitType,
			FixCommand: fmt.Sprintf("rvn schema add trait %s", trait.TraitType),
			FixHint:    fmt.Sprintf("Add trait '%s' to schema", trait.TraitType),
		})
		v.trackUndefinedTrait(trait.TraitType, filePath, trait.Line, trait.HasValue())
		return issues
	}
	if traitDef == nil {
		issues = append(issues, Issue{
			Level:    LevelError,
			Type:     IssueInvalidTraitValue,
			FilePath: filePath,
			Line:     trait.Line,
			Message:  fmt.Sprintf("Trait '@%s' has invalid schema definition", trait.TraitType),
			Value:    trait.TraitType,
			FixHint:  fmt.Sprintf("Fix trait '@%s' in schema.yaml", trait.TraitType),
		})
		return issues
	}

	if !traitDef.IsBoolean() && !trait.HasValue() && traitDef.Default == nil {
		issues = append(issues, Issue{
			Level:    LevelWarning,
			Type:     IssueInvalidTraitValue,
			FilePath: filePath,
			Line:     trait.Line,
			Message:  fmt.Sprintf("Trait '@%s' expects a value", trait.TraitType),
			Value:    trait.TraitType,
			FixHint:  fmt.Sprintf("Add a value: @%s(<value>)", trait.TraitType),
		})
		return issues
	}

	if !trait.HasValue() {
		// Bare boolean trait usage is valid.
		return issues
	}

	if err := schema.ValidateTraitValue(traitDef, *trait.Value); err != nil {
		valueStr := trait.ValueString()
		if valueStr == "" {
			valueStr = fmt.Sprintf("%v", trait.Value.Raw())
		}

		issueType := IssueInvalidTraitValue
		fixHint := "Use a value that matches the trait schema"
		switch normalizedTraitFieldType(traitDef) {
		case schema.FieldTypeDate:
			issueType = IssueInvalidDateFormat
			fixHint = "Use date format YYYY-MM-DD (e.g., 2025-02-01)"
		case schema.FieldTypeDatetime:
			issueType = IssueInvalidDateFormat
			fixHint = "Use datetime format YYYY-MM-DDTHH:MM or YYYY-MM-DDTHH:MM:SS"
		case schema.FieldTypeEnum:
			issueType = IssueInvalidEnumValue
			fixHint = fmt.Sprintf("Change to one of: %v", traitDef.Values)
		case schema.FieldTypeBool:
			fixHint = fmt.Sprintf("Use @%s, @%s(true), or @%s(false)", trait.TraitType, trait.TraitType, trait.TraitType)
		case schema.FieldTypeNumber:
			fixHint = "Use a numeric value (e.g., @score(5) or @score(3.5))"
		case schema.FieldTypeRef:
			fixHint = fmt.Sprintf("Use @%s([[target]]) or @%s(target)", trait.TraitType, trait.TraitType)
		case schema.FieldTypeURL:
			fixHint = fmt.Sprintf("Use @%s(https://example.com)", trait.TraitType)
		}

		issues = append(issues, Issue{
			Level:    LevelError,
			Type:     issueType,
			FilePath: filePath,
			Line:     trait.Line,
			Message:  fmt.Sprintf("Invalid value '%s' for trait '@%s': %v", valueStr, trait.TraitType, err),
			Value:    valueStr,
			FixHint:  fixHint,
		})
	}

	return issues
}

func normalizedTraitFieldType(def *schema.TraitDefinition) schema.FieldType {
	if def == nil {
		return ""
	}
	if def.IsBoolean() {
		return schema.FieldTypeBool
	}
	return def.Type
}
