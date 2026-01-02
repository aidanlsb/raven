// Package check handles vault-wide validation.
package check

import (
	"fmt"

	"github.com/ravenscroftj/raven/internal/parser"
	"github.com/ravenscroftj/raven/internal/resolver"
	"github.com/ravenscroftj/raven/internal/schema"
)

// IssueType categorizes validation issues for programmatic handling.
type IssueType string

const (
	IssueUnknownType         IssueType = "unknown_type"
	IssueMissingReference    IssueType = "missing_reference"
	IssueUndefinedTrait      IssueType = "undefined_trait"
	IssueUnknownFrontmatter  IssueType = "unknown_frontmatter_key"
	IssueDuplicateID         IssueType = "duplicate_object_id"
	IssueMissingRequiredField IssueType = "missing_required_field"
	IssueMissingRequiredTrait IssueType = "missing_required_trait"
	IssueInvalidEnumValue    IssueType = "invalid_enum_value"
	IssueAmbiguousReference  IssueType = "ambiguous_reference"
	IssueInvalidTraitValue   IssueType = "invalid_trait_value"
	IssueParseError          IssueType = "parse_error"
	IssueMissingEmbeddedID   IssueType = "missing_embedded_id"
)

// Issue represents a validation issue.
type Issue struct {
	Level      IssueLevel
	Type       IssueType
	FilePath   string
	Line       int
	Message    string
	Value      string   // The problematic value (type name, trait name, ref, etc.)
	FixCommand string   // Suggested command to fix the issue
	FixHint    string   // Human-readable fix hint
}

// MissingRef represents a reference to a non-existent object.
type MissingRef struct {
	TargetPath     string          // The reference path (e.g., "people/carol")
	SourceFile     string          // File containing the reference
	SourceObjectID string          // Full object ID where ref was found (e.g., "daily/2026-01-01#team-sync")
	Line           int             // Line number
	InferredType   string          // Type inferred from context (empty if unknown)
	Confidence     InferConfidence // How confident we are about the type
	FieldSource    string          // If from a typed field, the field name (e.g., "attendees")
}

// UndefinedTrait represents a trait used but not defined in schema.
type UndefinedTrait struct {
	TraitName  string   // The trait name (without @)
	SourceFile string   // First file where it was found
	Line       int      // First line where it was found
	HasValue   bool     // Whether it was used with a value
	UsageCount int      // Number of times it appears
	Locations  []string // File:line locations (up to 5)
}

// InferConfidence indicates how confident we are about type inference.
type InferConfidence int

const (
	ConfidenceUnknown  InferConfidence = iota // No type inference possible
	ConfidenceInferred                        // Inferred from path matching default_path
	ConfidenceCertain                         // Certain from typed field
)

func (c InferConfidence) String() string {
	switch c {
	case ConfidenceCertain:
		return "certain"
	case ConfidenceInferred:
		return "inferred"
	default:
		return "unknown"
	}
}

// IssueLevel indicates the severity of an issue.
type IssueLevel int

const (
	LevelError IssueLevel = iota
	LevelWarning
)

func (l IssueLevel) String() string {
	switch l {
	case LevelError:
		return "ERROR"
	case LevelWarning:
		return "WARN"
	default:
		return "UNKNOWN"
	}
}

// Validator validates documents against a schema.
type Validator struct {
	schema           *schema.Schema
	resolver         *resolver.Resolver
	allIDs           map[string]struct{}
	missingRefs      map[string]*MissingRef      // Keyed by target path to dedupe
	undefinedTraits  map[string]*UndefinedTrait  // Keyed by trait name to dedupe
}

// NewValidator creates a new validator.
func NewValidator(s *schema.Schema, objectIDs []string) *Validator {
	allIDs := make(map[string]struct{}, len(objectIDs))
	for _, id := range objectIDs {
		allIDs[id] = struct{}{}
	}

	return &Validator{
		schema:          s,
		resolver:        resolver.New(objectIDs),
		allIDs:          allIDs,
		missingRefs:     make(map[string]*MissingRef),
		undefinedTraits: make(map[string]*UndefinedTrait),
	}
}

// MissingRefs returns all missing references collected during validation.
func (v *Validator) MissingRefs() []*MissingRef {
	refs := make([]*MissingRef, 0, len(v.missingRefs))
	for _, ref := range v.missingRefs {
		refs = append(refs, ref)
	}
	return refs
}

// UndefinedTraits returns all undefined traits collected during validation.
func (v *Validator) UndefinedTraits() []*UndefinedTrait {
	traits := make([]*UndefinedTrait, 0, len(v.undefinedTraits))
	for _, trait := range v.undefinedTraits {
		traits = append(traits, trait)
	}
	return traits
}

// inferTypeFromPath tries to match a path to a type's default_path.
func (v *Validator) inferTypeFromPath(targetPath string) (typeName string, confidence InferConfidence) {
	for name, typeDef := range v.schema.Types {
		if typeDef.DefaultPath != "" {
			// Check if path starts with default_path
			if len(targetPath) > len(typeDef.DefaultPath) &&
				targetPath[:len(typeDef.DefaultPath)] == typeDef.DefaultPath {
				return name, ConfidenceInferred
			}
		}
	}
	return "", ConfidenceUnknown
}

// ValidateDocument validates a parsed document.
func (v *Validator) ValidateDocument(doc *parser.ParsedDocument) []Issue {
	var issues []Issue

	// Check for duplicate object IDs
	seenIDs := make(map[string]struct{})
	for _, obj := range doc.Objects {
		if _, exists := seenIDs[obj.ID]; exists {
			issues = append(issues, Issue{
				Level:      LevelError,
				Type:       IssueDuplicateID,
				FilePath:   doc.FilePath,
				Line:       obj.LineStart,
				Message:    fmt.Sprintf("Duplicate object ID '%s'", obj.ID),
				Value:      obj.ID,
				FixHint:    "Rename one of the duplicate objects",
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

	// Check if type is defined
	typeDef, typeExists := v.schema.Types[obj.ObjectType]
	if !typeExists && obj.ObjectType != "page" && obj.ObjectType != "section" {
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

	// Check embedded objects have IDs (if not a section)
	if obj.Heading != nil && obj.ObjectType != "section" && obj.ParentID != nil {
		// This is an embedded typed object - it should have an ID in its ID field
		// The ID is part of the full object ID after #
		if !containsHash(obj.ID) {
			issues = append(issues, Issue{
				Level:    LevelError,
				Type:     IssueMissingEmbeddedID,
				FilePath: filePath,
				Line:     obj.LineStart,
				Message:  "Embedded object missing 'id' field",
				FixHint:  "Add 'id' parameter to the embedded object declaration",
			})
		}
	}

	// Validate fields against schema
	if typeDef != nil {
		fieldErrors := schema.ValidateFields(obj.Fields, typeDef.Fields, v.schema)
		for _, err := range fieldErrors {
			issues = append(issues, Issue{
				Level:    LevelError,
				Type:     IssueMissingRequiredField,
				FilePath: filePath,
				Line:     obj.LineStart,
				Message:  err.Error(),
				FixHint:  "Add the required field to the file's frontmatter",
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

		// Validate required traits
		for _, traitName := range typeDef.Traits.List() {
			if typeDef.Traits.IsRequired(traitName) {
				// Check if this trait is present in frontmatter
				if _, hasField := obj.Fields[traitName]; !hasField {
					issues = append(issues, Issue{
						Level:      LevelError,
						Type:       IssueMissingRequiredTrait,
						FilePath:   filePath,
						Line:       obj.LineStart,
						Message:    fmt.Sprintf("Required trait '%s' missing for type '%s'", traitName, obj.ObjectType),
						Value:      traitName,
						FixCommand: fmt.Sprintf("rvn set %s %s=<value>", obj.ID, traitName),
						FixHint:    fmt.Sprintf("Add '%s' to the file's frontmatter", traitName),
					})
				}
			}
		}

		// Validate trait values against trait definitions
		for _, traitName := range typeDef.Traits.List() {
			if fieldValue, hasField := obj.Fields[traitName]; hasField {
				traitDef, traitExists := v.schema.Traits[traitName]
				if !traitExists {
					issues = append(issues, Issue{
						Level:      LevelWarning,
						Type:       IssueUndefinedTrait,
						FilePath:   filePath,
						Line:       obj.LineStart,
						Message:    fmt.Sprintf("Trait '%s' used in type but not defined in schema", traitName),
						Value:      traitName,
						FixCommand: fmt.Sprintf("rvn schema add trait %s", traitName),
						FixHint:    fmt.Sprintf("Add trait '%s' to schema", traitName),
					})
					continue
				}

				// Validate enum values
				if traitDef.Type == schema.FieldTypeEnum {
					valueStr, ok := fieldValue.AsString()
					if ok {
						validValue := false
						for _, allowed := range traitDef.Values {
							if allowed == valueStr {
								validValue = true
								break
							}
						}
						if !validValue {
							issues = append(issues, Issue{
								Level:      LevelError,
								Type:       IssueInvalidEnumValue,
								FilePath:   filePath,
								Line:       obj.LineStart,
								Message:    fmt.Sprintf("Invalid value '%s' for trait '%s' (allowed: %v)", valueStr, traitName, traitDef.Values),
								Value:      valueStr,
								FixCommand: fmt.Sprintf("rvn set %s %s=<valid_value>", obj.ID, traitName),
								FixHint:    fmt.Sprintf("Change to one of: %v", traitDef.Values),
							})
						}
					}
				}
			}
		}

		// Check for unknown frontmatter keys (not a field, not a trait)
		// Reserved keys that are always allowed
		reservedKeys := map[string]bool{
			"type": true, // Object type declaration
			"tags": true, // Tags are always allowed
			"id":   true, // ID for embedded objects
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
			// Skip if it's a declared trait for this type
			if typeDef.Traits.HasTrait(fieldName) {
				continue
			}
			// Unknown key - error
			issues = append(issues, Issue{
				Level:      LevelError,
				Type:       IssueUnknownFrontmatter,
				FilePath:   filePath,
				Line:       obj.LineStart,
				Message:    fmt.Sprintf("Unknown frontmatter key '%s' for type '%s' (not a field or declared trait)", fieldName, obj.ObjectType),
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

	// Validate value based on trait type
	if traitDef.IsBoolean() {
		// Boolean traits should have no value
		if trait.HasValue() {
			issues = append(issues, Issue{
				Level:    LevelWarning,
				Type:     IssueInvalidTraitValue,
				FilePath: filePath,
				Line:     trait.Line,
				Message:  fmt.Sprintf("Trait '@%s' is a marker trait and should not have a value", trait.TraitType),
				Value:    trait.TraitType,
				FixHint:  fmt.Sprintf("Remove the value: use @%s instead of @%s(...)", trait.TraitType, trait.TraitType),
			})
		}
	} else {
		// Non-boolean traits should have a value
		if !trait.HasValue() {
			if traitDef.Default == nil {
				issues = append(issues, Issue{
					Level:    LevelWarning,
					Type:     IssueInvalidTraitValue,
					FilePath: filePath,
					Line:     trait.Line,
					Message:  fmt.Sprintf("Trait '@%s' expects a value", trait.TraitType),
					Value:    trait.TraitType,
					FixHint:  fmt.Sprintf("Add a value: @%s(<value>)", trait.TraitType),
				})
			}
		} else if traitDef.Type == schema.FieldTypeEnum {
			// Validate enum value
			valueStr := trait.ValueString()
			validValue := false
			for _, allowed := range traitDef.Values {
				if allowed == valueStr {
					validValue = true
					break
				}
			}
			if !validValue {
				issues = append(issues, Issue{
					Level:    LevelError,
					Type:     IssueInvalidEnumValue,
					FilePath: filePath,
					Line:     trait.Line,
					Message:  fmt.Sprintf("Invalid value '%s' for trait '@%s' (allowed: %v)", valueStr, trait.TraitType, traitDef.Values),
					Value:    valueStr,
					FixHint:  fmt.Sprintf("Change to one of: %v", traitDef.Values),
				})
			}
		}
	}

	return issues
}

func (v *Validator) validateRef(filePath string, ref *parser.ParsedRef) []Issue {
	return v.validateRefWithContext(filePath, "", ref, "", "")
}

// validateRefWithContext validates a reference with optional type context.
// If targetType is provided (from a typed field), we have certain confidence about the type.
func (v *Validator) validateRefWithContext(filePath, sourceObjectID string, ref *parser.ParsedRef, targetType, fieldName string) []Issue {
	var issues []Issue

	result := v.resolver.Resolve(ref.TargetRaw)

	if result.Ambiguous {
		issues = append(issues, Issue{
			Level:    LevelError,
			Type:     IssueAmbiguousReference,
			FilePath: filePath,
			Line:     ref.Line,
			Message:  fmt.Sprintf("Reference [[%s]] is ambiguous (matches: %v)", ref.TargetRaw, result.Matches),
			Value:    ref.TargetRaw,
			FixHint:  "Use a more specific path to disambiguate",
		})
	} else if result.TargetID == "" {
		// Determine the fix command based on type inference
		fixCmd := ""
		fixHint := ""
		if targetType != "" {
			fixCmd = fmt.Sprintf("rvn new %s \"%s\"", targetType, ref.TargetRaw)
			fixHint = fmt.Sprintf("Create the missing %s", targetType)
		} else {
			// Try to infer from path
			inferredType, conf := v.inferTypeFromPath(ref.TargetRaw)
			if conf == ConfidenceInferred {
				fixCmd = fmt.Sprintf("rvn new %s \"%s\"", inferredType, ref.TargetRaw)
				fixHint = fmt.Sprintf("Create the missing %s (inferred from path)", inferredType)
			} else {
				fixHint = "Create the missing page with 'rvn new <type> <title>'"
			}
		}

		issues = append(issues, Issue{
			Level:      LevelError,
			Type:       IssueMissingReference,
			FilePath:   filePath,
			Line:       ref.Line,
			Message:    fmt.Sprintf("Reference [[%s]] not found", ref.TargetRaw),
			Value:      ref.TargetRaw,
			FixCommand: fixCmd,
			FixHint:    fixHint,
		})

		// Track this missing reference with type inference
		v.trackMissingRef(ref.TargetRaw, filePath, sourceObjectID, ref.Line, targetType, fieldName)
	}

	return issues
}

// trackMissingRef records a missing reference with type inference.
func (v *Validator) trackMissingRef(targetPath, sourceFile, sourceObjectID string, line int, targetType, fieldName string) {
	// Normalize path (remove .md extension if present, treat as file path)
	normalizedPath := targetPath

	// If we already have this ref with higher confidence, don't downgrade
	if existing, ok := v.missingRefs[normalizedPath]; ok {
		if existing.Confidence >= ConfidenceCertain {
			return // Already have certain confidence
		}
		if targetType != "" {
			// Upgrade to certain confidence
			existing.InferredType = targetType
			existing.Confidence = ConfidenceCertain
			existing.FieldSource = fieldName
			existing.SourceObjectID = sourceObjectID
			return
		}
	}

	missing := &MissingRef{
		TargetPath:     normalizedPath,
		SourceFile:     sourceFile,
		SourceObjectID: sourceObjectID,
		Line:           line,
	}

	// Determine confidence and type
	if targetType != "" {
		// From a typed field - certain
		missing.InferredType = targetType
		missing.Confidence = ConfidenceCertain
		missing.FieldSource = fieldName
	} else {
		// Try to infer from path
		inferredType, confidence := v.inferTypeFromPath(normalizedPath)
		missing.InferredType = inferredType
		missing.Confidence = confidence
	}

	v.missingRefs[normalizedPath] = missing
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

func containsHash(s string) bool {
	for _, c := range s {
		if c == '#' {
			return true
		}
	}
	return false
}
