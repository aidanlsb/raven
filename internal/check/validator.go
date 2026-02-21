// Package check handles vault-wide validation.
package check

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
)

// IssueType categorizes validation issues for programmatic handling.
type IssueType string

const (
	IssueUnknownType             IssueType = "unknown_type"
	IssueMissingReference        IssueType = "missing_reference"
	IssueUndefinedTrait          IssueType = "undefined_trait"
	IssueUnknownFrontmatter      IssueType = "unknown_frontmatter_key"
	IssueDuplicateID             IssueType = "duplicate_object_id"
	IssueMissingRequiredField    IssueType = "missing_required_field"
	IssueInvalidFieldValue       IssueType = "invalid_field_value"
	IssueMissingRequiredTrait    IssueType = "missing_required_trait"
	IssueInvalidEnumValue        IssueType = "invalid_enum_value"
	IssueAmbiguousReference      IssueType = "ambiguous_reference"
	IssueInvalidTraitValue       IssueType = "invalid_trait_value"
	IssueParseError              IssueType = "parse_error"
	IssueWrongTargetType         IssueType = "wrong_target_type"
	IssueInvalidDateFormat       IssueType = "invalid_date_format"
	IssueShortRefCouldBeFullPath IssueType = "short_ref_could_be_full_path"
	IssueStaleIndex              IssueType = "stale_index"
	IssueUnusedType              IssueType = "unused_type"
	IssueUnusedTrait             IssueType = "unused_trait"
	IssueMissingTargetType       IssueType = "missing_target_type"
	IssueSelfReferentialRequired IssueType = "self_referential_required"
	IssueUnknownFieldType        IssueType = "unknown_field_type"
	IssueIDCollision             IssueType = "id_collision"
	IssueDuplicateAlias          IssueType = "duplicate_alias"
	IssueAliasCollision          IssueType = "alias_collision"
	IssueStaleFragment           IssueType = "stale_fragment"
)

// Issue represents a validation issue.
type Issue struct {
	Level      IssueLevel
	Type       IssueType
	FilePath   string
	Line       int
	Message    string
	Value      string // The problematic value (type name, trait name, ref, etc.)
	FixCommand string // Suggested command to fix the issue
	FixHint    string // Human-readable fix hint
}

// MissingRef represents a reference to a non-existent object.
type MissingRef struct {
	TargetPath     string          // The reference path (e.g., "people/baldur")
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
	objectTypes      map[string]string          // Object ID -> type name (for target type validation)
	aliases          map[string]string          // Alias -> object ID
	duplicateAliases []index.DuplicateAlias     // Aliases used by multiple objects
	missingRefs      map[string]*MissingRef     // Keyed by target path to dedupe
	undefinedTraits  map[string]*UndefinedTrait // Keyed by trait name to dedupe
	usedTypes        map[string]struct{}        // Types actually used in documents
	usedTraits       map[string]struct{}        // Traits actually used in documents
	shortRefs        map[string]string          // Short ref -> full path (for suggestions)
	usedShortNames   map[string]struct{}        // Short names actually used in references
	objectsRoot      string                     // Directory prefix for typed objects (e.g., "objects/")
	pagesRoot        string                     // Directory prefix for untyped pages (e.g., "pages/")
}

// ObjectInfo contains basic info about an object for validation.
type ObjectInfo struct {
	ID   string
	Type string
}

// NewValidator creates a new validator.
func NewValidator(s *schema.Schema, objectIDs []string) *Validator {
	return NewValidatorWithAliases(s, objectIDs, nil)
}

// NewValidatorWithAliases creates a new validator with alias support.
func NewValidatorWithAliases(s *schema.Schema, objectIDs []string, aliases map[string]string) *Validator {
	allIDs := make(map[string]struct{}, len(objectIDs))
	for _, id := range objectIDs {
		allIDs[id] = struct{}{}
	}

	return &Validator{
		schema:          s,
		resolver:        resolver.New(objectIDs, resolver.Options{Aliases: aliases}),
		allIDs:          allIDs,
		objectTypes:     make(map[string]string),
		aliases:         aliases,
		missingRefs:     make(map[string]*MissingRef),
		undefinedTraits: make(map[string]*UndefinedTrait),
		usedTypes:       make(map[string]struct{}),
		usedTraits:      make(map[string]struct{}),
		shortRefs:       make(map[string]string),
		usedShortNames:  make(map[string]struct{}),
	}
}

// NewValidatorWithTypes creates a new validator with object type information.
// objectInfos should contain ID and type for each object in the vault.
func NewValidatorWithTypes(s *schema.Schema, objectInfos []ObjectInfo) *Validator {
	return NewValidatorWithTypesAndAliases(s, objectInfos, nil)
}

// NewValidatorWithTypesAndAliases creates a new validator with type info and aliases.
func NewValidatorWithTypesAndAliases(s *schema.Schema, objectInfos []ObjectInfo, aliases map[string]string) *Validator {
	allIDs := make(map[string]struct{}, len(objectInfos))
	objectTypes := make(map[string]string, len(objectInfos))
	ids := make([]string, 0, len(objectInfos))

	for _, info := range objectInfos {
		allIDs[info.ID] = struct{}{}
		objectTypes[info.ID] = info.Type
		ids = append(ids, info.ID)
	}

	return &Validator{
		schema:          s,
		resolver:        resolver.New(ids, resolver.Options{Aliases: aliases}),
		allIDs:          allIDs,
		objectTypes:     objectTypes,
		aliases:         aliases,
		missingRefs:     make(map[string]*MissingRef),
		undefinedTraits: make(map[string]*UndefinedTrait),
		usedTypes:       make(map[string]struct{}),
		usedTraits:      make(map[string]struct{}),
		shortRefs:       make(map[string]string),
		usedShortNames:  make(map[string]struct{}),
	}
}

// SetDuplicateAliases sets duplicate alias information for validation.
// This should be called before ValidateSchema to report duplicate aliases.
func (v *Validator) SetDuplicateAliases(duplicates []index.DuplicateAlias) {
	v.duplicateAliases = duplicates
}

// SetDirectoryRoots sets the directory prefixes for typed objects and pages.
// When set, suggestions will strip these prefixes for cleaner display.
func (v *Validator) SetDirectoryRoots(objectsRoot, pagesRoot string) {
	v.objectsRoot = paths.NormalizeDirRoot(objectsRoot)
	v.pagesRoot = paths.NormalizeDirRoot(pagesRoot)
}

// SetDailyDirectory updates the resolver's daily directory.
func (v *Validator) SetDailyDirectory(dailyDir string) {
	v.resolver = resolver.New(v.allIDList(), resolver.Options{
		DailyDirectory: dailyDir,
		Aliases:        v.aliases,
	})
}

func (v *Validator) allIDList() []string {
	ids := make([]string, 0, len(v.allIDs))
	for id := range v.allIDs {
		ids = append(ids, id)
	}
	return ids
}

// displayID returns an object ID suitable for display (with directory prefix stripped).
func (v *Validator) displayID(id string) string {
	if v.objectsRoot != "" && strings.HasPrefix(id, v.objectsRoot) {
		return strings.TrimPrefix(id, v.objectsRoot)
	}
	if v.pagesRoot != "" && strings.HasPrefix(id, v.pagesRoot) {
		return strings.TrimPrefix(id, v.pagesRoot)
	}
	return id
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

	// Note: Embedded object IDs are now auto-generated from heading text if not explicitly provided,
	// so we no longer need to check for missing IDs.

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
			"id":    true, // ID for embedded objects
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

func (v *Validator) validateRef(filePath string, ref *parser.ParsedRef) []Issue {
	return v.validateRefWithContext(filePath, "", ref, "", "")
}

// validateRefWithContext validates a reference with optional type context.
// If targetType is provided (from a typed field), we have certain confidence about the type.
func (v *Validator) validateRefWithContext(filePath, sourceObjectID string, ref *parser.ParsedRef, targetType, fieldName string) []Issue {
	var issues []Issue

	// Track short name usage (references without path separators)
	// This is used to only warn about collisions for short names that are actually used
	if !strings.Contains(ref.TargetRaw, "/") && !strings.HasPrefix(ref.TargetRaw, "#") {
		v.usedShortNames[ref.TargetRaw] = struct{}{}
	}

	result := v.resolver.Resolve(ref.TargetRaw)

	if result.Ambiguous {
		message := formatAmbiguousRefMessage(ref.TargetRaw, result, v.displayID)
		fixHint := formatAmbiguousRefFixHint(result, v.displayID)
		issues = append(issues, Issue{
			Level:    LevelError,
			Type:     IssueAmbiguousReference,
			FilePath: filePath,
			Line:     ref.Line,
			Message:  message,
			Value:    ref.TargetRaw,
			FixHint:  fixHint,
		})
	} else if result.TargetID == "" {
		// Check if this is a stale fragment reference (file exists but section doesn't)
		if baseID, fragment, isEmbedded := paths.ParseEmbeddedID(ref.TargetRaw); isEmbedded && fragment != "" {
			baseResult := v.resolver.Resolve(baseID)
			if baseResult.TargetID != "" {
				issues = append(issues, Issue{
					Level:    LevelWarning,
					Type:     IssueStaleFragment,
					FilePath: filePath,
					Line:     ref.Line,
					Message:  fmt.Sprintf("Fragment reference [[%s]] not found — '%s' exists but has no section '#%s'", ref.TargetRaw, v.displayID(baseResult.TargetID), fragment),
					Value:    ref.TargetRaw,
					FixHint:  "The heading may have been renamed. Update the fragment or use ::type(id=...) for a stable ID",
				})
				return issues
			}
		}

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
	} else {
		// Reference resolved successfully - perform additional checks

		// Check if short ref could be a full path (for better clarity)
		if !strings.Contains(ref.TargetRaw, "/") && strings.Contains(result.TargetID, "/") {
			// Short ref that resolved to a full path - suggest using full path
			// Use displayID to strip directory prefix (e.g., "objects/") for cleaner suggestions
			suggestedID := v.displayID(result.TargetID)
			v.shortRefs[ref.TargetRaw] = suggestedID
			issues = append(issues, Issue{
				Level:    LevelWarning,
				Type:     IssueShortRefCouldBeFullPath,
				FilePath: filePath,
				Line:     ref.Line,
				Message:  fmt.Sprintf("Short reference [[%s]] could be written as [[%s]] for clarity", ref.TargetRaw, suggestedID),
				Value:    ref.TargetRaw,
				FixHint:  fmt.Sprintf("Consider using full path: [[%s]]", suggestedID),
			})
		}

		// Validate target type if specified (e.g., for ref fields with target constraint)
		if targetType != "" && len(v.objectTypes) > 0 {
			actualType, exists := v.objectTypes[result.TargetID]
			if exists && actualType != targetType {
				issues = append(issues, Issue{
					Level:    LevelError,
					Type:     IssueWrongTargetType,
					FilePath: filePath,
					Line:     ref.Line,
					Message:  fmt.Sprintf("Field '%s' expects type '%s', but [[%s]] is type '%s'", fieldName, targetType, ref.TargetRaw, actualType),
					Value:    ref.TargetRaw,
					FixHint:  fmt.Sprintf("Reference a '%s' object instead, or change the field's target type", targetType),
				})
			}
		}
	}

	return issues
}

func formatAmbiguousRefMessage(raw string, result resolver.ResolveResult, displayID func(string) string) string {
	if len(result.Matches) == 0 {
		return fmt.Sprintf("Reference [[%s]] is ambiguous", raw)
	}

	matchDetails := make([]string, 0, len(result.Matches))
	for _, match := range result.Matches {
		source := result.MatchSources[match]
		label := labelForMatchSource(source)
		matchDetails = append(matchDetails, fmt.Sprintf("%s → %s", label, displayID(match)))
	}

	return fmt.Sprintf("Reference [[%s]] is ambiguous: %s", raw, strings.Join(matchDetails, "; "))
}

func formatAmbiguousRefFixHint(result resolver.ResolveResult, displayID func(string) string) string {
	if len(result.Matches) == 0 {
		return "Use a more specific path to disambiguate"
	}

	example := displayID(result.Matches[0])
	hint := fmt.Sprintf("Use a more specific path (e.g., [[%s]])", example)
	if hasSource(result.MatchSources, "alias") || hasSource(result.MatchSources, "short_name") || hasSource(result.MatchSources, "name_field") {
		hint += " or rename the conflicting alias/short name"
	}
	return hint
}

func labelForMatchSource(source string) string {
	switch source {
	case "alias":
		return "alias"
	case "name_field":
		return "name field"
	case "object_id":
		return "object id"
	case "short_name":
		return "short name"
	case "suffix_match":
		return "suffix match"
	case "date":
		return "date"
	default:
		return "match"
	}
}

func hasSource(matchSources map[string]string, source string) bool {
	for _, s := range matchSources {
		if s == source {
			return true
		}
	}
	return false
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

// SchemaIssue represents a schema-level validation issue (not file-specific).
type SchemaIssue struct {
	Level      IssueLevel
	Type       IssueType
	Message    string
	Value      string // The type/trait/field name
	FixCommand string
	FixHint    string
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

// ShortRefs returns the short refs that could be full paths.
func (v *Validator) ShortRefs() map[string]string {
	return v.shortRefs
}
