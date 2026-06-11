package check

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
)

// UndefinedTrait represents a trait used but not defined in schema.
type UndefinedTrait struct {
	TraitName  string   // The trait name (without @)
	SourceFile string   // First file where it was found
	Line       int      // First line where it was found
	HasValue   bool     // Whether it was used with a value
	UsageCount int      // Number of times it appears
	Locations  []string // File:line locations (up to 5)
}

// trackUndefinedTrait records an undefined trait for later reporting.
func (v *Validator) trackUndefinedTrait(traitName, sourceFile string, line int, hasValue bool) {
	location := fmt.Sprintf("%s:%d", sourceFile, line)

	if existing, ok := v.undefinedTraits[traitName]; ok {
		existing.UsageCount++
		// Track if any usage has a value
		if hasValue {
			existing.HasValue = true
		}
		// Keep up to 5 example locations
		if len(existing.Locations) < 5 {
			existing.Locations = append(existing.Locations, location)
		}
		return
	}

	v.undefinedTraits[traitName] = &UndefinedTrait{
		TraitName:  traitName,
		SourceFile: sourceFile,
		Line:       line,
		HasValue:   hasValue,
		UsageCount: 1,
		Locations:  []string{location},
	}
}

// ValidateSchema checks the schema for integrity issues.
// This should be called after all documents have been validated.
func (v *Validator) ValidateSchema() []SchemaIssue {
	var issues []SchemaIssue

	// Check for unused types (defined in schema but never used)
	for typeName := range v.schema.Types {
		// Skip built-in types
		if schema.IsBuiltinType(typeName) {
			continue
		}
		if _, used := v.usedTypes[typeName]; !used {
			issues = append(issues, SchemaIssue{
				Level:   LevelWarning,
				Type:    IssueUnusedType,
				Message: fmt.Sprintf("Type '%s' is defined in schema but never used", typeName),
				Value:   typeName,
				FixHint: fmt.Sprintf("Create a file with 'type: %s' or remove the type from schema", typeName),
			})
		}
	}

	// Check for unused traits (defined in schema but never used)
	for traitName := range v.schema.Traits {
		if _, used := v.usedTraits[traitName]; !used {
			issues = append(issues, SchemaIssue{
				Level:   LevelWarning,
				Type:    IssueUnusedTrait,
				Message: fmt.Sprintf("Trait '@%s' is defined in schema but never used", traitName),
				Value:   traitName,
				FixHint: fmt.Sprintf("Use @%s in a file or remove the trait from schema", traitName),
			})
		}
	}

	// Check for missing target types in ref fields
	for typeName, typeDef := range v.schema.Types {
		if typeDef == nil || typeDef.Fields == nil {
			continue
		}
		for fieldName, fieldDef := range typeDef.Fields {
			if fieldDef == nil {
				continue
			}
			if !schema.IsValidFieldType(fieldDef.Type) {
				issues = append(issues, SchemaIssue{
					Level:   LevelWarning,
					Type:    IssueUnknownFieldType,
					Message: fmt.Sprintf("Field '%s.%s' has unknown field type '%s'", typeName, fieldName, fieldDef.Type),
					Value:   string(fieldDef.Type),
					FixHint: fmt.Sprintf("Use one of: %s", schema.ValidFieldTypes()),
				})
			}
			// Check ref and ref[] fields with target constraints
			if (fieldDef.Type == schema.FieldTypeRef || fieldDef.Type == schema.FieldTypeRefArray) && fieldDef.Target != "" {
				// Check if target type exists
				if _, exists := v.schema.Types[fieldDef.Target]; !exists {
					// Also check built-in types
					if !schema.IsBuiltinType(fieldDef.Target) {
						issues = append(issues, SchemaIssue{
							Level:      LevelError,
							Type:       IssueMissingTargetType,
							Message:    fmt.Sprintf("Field '%s.%s' references non-existent type '%s'", typeName, fieldName, fieldDef.Target),
							Value:      fieldDef.Target,
							FixCommand: fmt.Sprintf("rvn schema add type %s", fieldDef.Target),
							FixHint:    fmt.Sprintf("Add type '%s' to schema or change the target", fieldDef.Target),
						})
					}
				}
			}
		}
	}

	// Check for self-referential required fields (impossible to create first instance)
	for typeName, typeDef := range v.schema.Types {
		if typeDef == nil || typeDef.Fields == nil {
			continue
		}
		for fieldName, fieldDef := range typeDef.Fields {
			if fieldDef == nil {
				continue
			}
			// Check if a required ref field points to the same type
			if fieldDef.Required && fieldDef.Default == nil {
				if (fieldDef.Type == schema.FieldTypeRef || fieldDef.Type == schema.FieldTypeRefArray) && fieldDef.Target == typeName {
					issues = append(issues, SchemaIssue{
						Level:   LevelWarning,
						Type:    IssueSelfReferentialRequired,
						Message: fmt.Sprintf("Type '%s' has required field '%s' that references itself - impossible to create first instance", typeName, fieldName),
						Value:   typeName + "." + fieldName,
						FixHint: "Make the field optional (required: false) or add a default value",
					})
				}
			}
		}
	}

	// Check for object ID collisions (same short name, different full paths)
	// Only warn if the short name is actually used in a reference somewhere
	collisions := v.resolver.FindCollisions()
	for _, collision := range collisions {
		if len(collision.ObjectIDs) >= 2 {
			// Only warn if this short name is actually used in a reference
			if _, used := v.usedShortNames[collision.ShortName]; !used {
				continue // Skip - this collision is hypothetical, not actually used
			}
			issues = append(issues, SchemaIssue{
				Level:   LevelWarning,
				Type:    IssueIDCollision,
				Message: fmt.Sprintf("Short name '%s' matches multiple objects: %s", collision.ShortName, strings.Join(collision.ObjectIDs, ", ")),
				Value:   collision.ShortName,
				FixHint: "Use full paths in references to avoid ambiguity (e.g., [[people/freya]] instead of [[freya]])",
			})
		}
	}

	// Check for alias collisions (alias conflicts with short name or object ID)
	aliasCollisions := v.resolver.FindAliasCollisions()
	for _, collision := range aliasCollisions {
		var msg string
		switch collision.ConflictsWith {
		case "short_name":
			msg = fmt.Sprintf("Alias '%s' conflicts with short name of object(s): %s", collision.Alias, strings.Join(collision.ObjectIDs, ", "))
		case "object_id":
			msg = fmt.Sprintf("Alias '%s' conflicts with existing object ID: %s", collision.Alias, strings.Join(collision.ObjectIDs, ", "))
		default:
			msg = fmt.Sprintf("Alias '%s' has a conflict: %s", collision.Alias, strings.Join(collision.ObjectIDs, ", "))
		}
		issues = append(issues, SchemaIssue{
			Level:   LevelError,
			Type:    IssueAliasCollision,
			Message: msg,
			Value:   collision.Alias,
			FixHint: "Rename the alias to something unique, or use full paths in references",
		})
	}

	// Check for duplicate aliases (multiple objects using the same alias)
	for _, dup := range v.duplicateAliases {
		issues = append(issues, SchemaIssue{
			Level:   LevelError,
			Type:    IssueDuplicateAlias,
			Message: fmt.Sprintf("Alias '%s' is used by multiple objects: %s", dup.Alias, strings.Join(dup.ObjectIDs, ", ")),
			Value:   dup.Alias,
			FixHint: "Each alias must be unique - rename one of the conflicting aliases",
		})
	}

	return issues
}
