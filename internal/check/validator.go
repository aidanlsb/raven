// Package check handles vault-wide validation.
package check

import (
	"fmt"
	"regexp"

	"github.com/ravenscroftj/raven/internal/parser"
	"github.com/ravenscroftj/raven/internal/resolver"
	"github.com/ravenscroftj/raven/internal/schema"
)

// Issue represents a validation issue.
type Issue struct {
	Level    IssueLevel
	FilePath string
	Line     int
	Message  string
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
	schema   *schema.Schema
	resolver *resolver.Resolver
	allIDs   map[string]struct{}
}

// NewValidator creates a new validator.
func NewValidator(s *schema.Schema, objectIDs []string) *Validator {
	allIDs := make(map[string]struct{}, len(objectIDs))
	for _, id := range objectIDs {
		allIDs[id] = struct{}{}
	}

	return &Validator{
		schema:   s,
		resolver: resolver.New(objectIDs),
		allIDs:   allIDs,
	}
}

// ValidateDocument validates a parsed document.
func (v *Validator) ValidateDocument(doc *parser.ParsedDocument) []Issue {
	var issues []Issue

	// Check for duplicate object IDs
	seenIDs := make(map[string]struct{})
	for _, obj := range doc.Objects {
		if _, exists := seenIDs[obj.ID]; exists {
			issues = append(issues, Issue{
				Level:    LevelError,
				FilePath: doc.FilePath,
				Line:     obj.LineStart,
				Message:  fmt.Sprintf("Duplicate object ID '%s'", obj.ID),
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
			Level:    LevelError,
			FilePath: filePath,
			Line:     obj.LineStart,
			Message:  fmt.Sprintf("Unknown type '%s'", obj.ObjectType),
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
				FilePath: filePath,
				Line:     obj.LineStart,
				Message:  "Embedded object missing 'id' field",
			})
		}
	}

	// Validate fields against schema
	if typeDef != nil {
		fieldErrors := schema.ValidateFields(obj.Fields, typeDef.Fields, v.schema)
		for _, err := range fieldErrors {
			issues = append(issues, Issue{
				Level:    LevelError,
				FilePath: filePath,
				Line:     obj.LineStart,
				Message:  err.Error(),
			})
		}

		// Check for type detection mismatch
		if obj.ObjectType != "" && obj.Heading == nil && typeDef.Detect != nil {
			detectedType := v.detectType(filePath, obj.Fields)
			if detectedType != "" && detectedType != obj.ObjectType {
				issues = append(issues, Issue{
					Level:    LevelWarning,
					FilePath: filePath,
					Line:     obj.LineStart,
					Message:  fmt.Sprintf("Detection rule would infer type '%s', but explicit type is '%s'", detectedType, obj.ObjectType),
				})
			}
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
			Level:    LevelWarning,
			FilePath: filePath,
			Line:     trait.Line,
			Message:  fmt.Sprintf("Undefined trait '@%s' will be skipped", trait.TraitType),
		})
		return issues
	}

	// Validate fields
	fieldErrors := schema.ValidateFields(trait.Fields, traitDef.Fields, v.schema)
	for _, err := range fieldErrors {
		issues = append(issues, Issue{
			Level:    LevelError,
			FilePath: filePath,
			Line:     trait.Line,
			Message:  err.Error(),
		})
	}

	return issues
}

func (v *Validator) validateRef(filePath string, ref *parser.ParsedRef) []Issue {
	var issues []Issue

	result := v.resolver.Resolve(ref.TargetRaw)

	if result.Ambiguous {
		issues = append(issues, Issue{
			Level:    LevelError,
			FilePath: filePath,
			Line:     ref.Line,
			Message:  fmt.Sprintf("Reference [[%s]] is ambiguous (matches: %v)", ref.TargetRaw, result.Matches),
		})
	} else if result.TargetID == "" {
		issues = append(issues, Issue{
			Level:    LevelError,
			FilePath: filePath,
			Line:     ref.Line,
			Message:  fmt.Sprintf("Reference [[%s]] not found", ref.TargetRaw),
		})
	}

	return issues
}

func (v *Validator) detectType(filePath string, fields map[string]schema.FieldValue) string {
	for typeName, typeDef := range v.schema.Types {
		if typeDef.Detect == nil {
			continue
		}

		// Check path pattern
		if typeDef.Detect.PathPattern != "" {
			re, err := regexp.Compile(typeDef.Detect.PathPattern)
			if err == nil && re.MatchString(filePath) {
				return typeName
			}
		}

		// Check attribute matches
		if typeDef.Detect.Attribute != nil {
			match := true
			for key, expected := range typeDef.Detect.Attribute {
				if val, ok := fields[key]; ok {
					if s, ok := val.AsString(); ok {
						if s != fmt.Sprintf("%v", expected) {
							match = false
							break
						}
					} else {
						match = false
						break
					}
				} else {
					match = false
					break
				}
			}
			if match {
				return typeName
			}
		}
	}

	return ""
}

func containsHash(s string) bool {
	for _, c := range s {
		if c == '#' {
			return true
		}
	}
	return false
}
