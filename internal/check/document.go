package check

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

// ValidateDocument validates a parsed document.
func (v *Validator) ValidateDocument(doc *parser.ParsedDocument) []Issue {
	var issues []Issue

	// Check for duplicate object IDs
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

	// Validate each object
	for _, obj := range doc.Objects {
		issues = append(issues, v.validateObject(doc.FilePath, obj)...)
	}

	// Validate traits
	for _, trait := range doc.Traits {
		issues = append(issues, v.validateTrait(doc.FilePath, trait)...)
	}

	// Validate references
	for _, ref := range doc.Refs {
		issues = append(issues, v.validateRef(doc.FilePath, ref)...)
	}

	return issues
}

func (v *Validator) validateObject(filePath string, obj *parser.ParsedObject) []Issue {
	var issues []Issue

	// Track type usage
	v.usedTypes[obj.ObjectType] = struct{}{}

	// Check if type is defined
	typeDef, typeExists := v.schema.Types[obj.ObjectType]
	if !typeExists && !schema.IsBuiltinType(obj.ObjectType) {
		issues = append(issues, Issue{
			Level:      LevelError,
			Type:       IssueUnknownType,
			FilePath:   filePath,
			Line:       obj.LineStart,
			Message:    fmt.Sprintf("Unknown type '%s'", obj.ObjectType),
			Value:      obj.ObjectType,
			FixCommand: fmt.Sprintf("rvn schema add type %s", obj.ObjectType),
			FixHint:    fmt.Sprintf("Add type '%s' to schema", obj.ObjectType),
		})
		return issues
	}

	// Section IDs are derived from heading text, so there is no separate object ID check here.

	// Validate fields against schema
	if typeDef != nil {
		fieldErrors := schema.ValidateFields(obj.Fields, typeDef.Fields, v.schema)
		for _, err := range fieldErrors {
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

		// Validate ref fields with type context for missing ref tracking
		for fieldName, fieldDef := range typeDef.Fields {
			if fieldDef == nil {
				continue
			}

			fieldValue, hasField := obj.Fields[fieldName]
			if !hasField {
				continue
			}

			// Handle ref fields
			if fieldDef.Type == schema.FieldTypeRef {
				if refStr, ok := fieldValue.AsString(); ok {
					// Create a synthetic ParsedRef to validate
					syntheticRef := &parser.ParsedRef{
						TargetRaw: refStr,
						Line:      obj.LineStart,
					}
					refIssues := v.validateRefWithContext(filePath, obj.ID, syntheticRef, fieldDef.Target, fieldName)
					issues = append(issues, refIssues...)
				}
			}

			// Handle ref[] (array) fields
			if fieldDef.Type == schema.FieldTypeRefArray {
				if arr, ok := fieldValue.AsArray(); ok {
					for _, item := range arr {
						if refStr, ok := item.AsString(); ok {
							syntheticRef := &parser.ParsedRef{
								TargetRaw: refStr,
								Line:      obj.LineStart,
							}
							refIssues := v.validateRefWithContext(filePath, obj.ID, syntheticRef, fieldDef.Target, fieldName)
							issues = append(issues, refIssues...)
						}
					}
				}
			}
		}

		// Check for unknown frontmatter keys (not a defined field)
		// Reserved keys that are always allowed
		reservedKeys := map[string]bool{
			"type":  true, // Object type declaration
			"id":    true, // Optional file object ID override
			"alias": true, // Alias for reference resolution
		}

		for fieldName := range obj.Fields {
			// Skip reserved keys
			if reservedKeys[fieldName] {
				continue
			}
			// Skip if it's a defined field
			if _, isField := typeDef.Fields[fieldName]; isField {
				continue
			}
			// Unknown key - error
			issues = append(issues, Issue{
				Level:      LevelError,
				Type:       IssueUnknownFrontmatter,
				FilePath:   filePath,
				Line:       obj.LineStart,
				Message:    fmt.Sprintf("Unknown frontmatter key '%s' for type '%s'", fieldName, obj.ObjectType),
				Value:      fieldName,
				FixCommand: fmt.Sprintf("rvn schema add field %s %s", obj.ObjectType, fieldName),
				FixHint:    fmt.Sprintf("Add field '%s' to type '%s', or remove it from the file", fieldName, obj.ObjectType),
			})
		}
	}

	return issues
}

func (v *Validator) validateTrait(filePath string, trait *parser.ParsedTrait) []Issue {
	var issues []Issue

	// Track trait usage
	v.usedTraits[trait.TraitType] = struct{}{}

	// Check if trait is defined
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
		// Track this undefined trait
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

	// Validate value based on trait type
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
