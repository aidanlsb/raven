package check

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestValidatorBasic(t *testing.T) {
	// Create a simple schema
	s := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {
				Fields: map[string]*schema.FieldDefinition{
					"name": {Type: schema.FieldTypeString, Required: true},
				},
			},
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"title": {Type: schema.FieldTypeString},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{
			"due": {Type: schema.FieldTypeDate},
		},
	}

	objectIDs := []string{"people/freya", "projects/bifrost"}
	v := NewValidator(s, objectIDs)

	t.Run("valid document", func(t *testing.T) {
		doc := &parser.ParsedDocument{
			FilePath: "people/freya.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "people/freya",
					ObjectType: "person",
					Fields: map[string]schema.FieldValue{
						"name": schema.String("Freya"),
					},
				},
			},
		}

		issues := v.ValidateDocument(doc)
		if len(issues) != 0 {
			t.Errorf("Expected no issues, got %d: %v", len(issues), issues)
		}
	})

	t.Run("missing required field", func(t *testing.T) {
		doc := &parser.ParsedDocument{
			FilePath: "people/thor.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "people/thor",
					ObjectType: "person",
					Fields:     map[string]schema.FieldValue{}, // Missing 'name'
				},
			},
		}

		issues := v.ValidateDocument(doc)
		hasRequiredFieldError := false
		for _, issue := range issues {
			if issue.Level == LevelError && strings.Contains(strings.ToLower(issue.Message), "required") {
				hasRequiredFieldError = true
				break
			}
		}
		if !hasRequiredFieldError {
			t.Errorf("Expected required field error, got: %v", issues)
		}
	})

	t.Run("broken reference", func(t *testing.T) {
		doc := &parser.ParsedDocument{
			FilePath: "projects/website.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "projects/website",
					ObjectType: "project",
				},
			},
			Refs: []*parser.ParsedRef{
				{SourceID: "projects/website", TargetRaw: "people/nonexistent", Line: 10},
			},
		}

		issues := v.ValidateDocument(doc)
		hasBrokenRef := false
		for _, issue := range issues {
			if issue.Level == LevelError && strings.Contains(issue.Message, "not found") {
				hasBrokenRef = true
				break
			}
		}
		if !hasBrokenRef {
			t.Errorf("Expected broken reference error, got: %v", issues)
		}
	})

	t.Run("valid reference", func(t *testing.T) {
		doc := &parser.ParsedDocument{
			FilePath: "projects/website.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "projects/website",
					ObjectType: "project",
				},
			},
			Refs: []*parser.ParsedRef{
				{SourceID: "projects/bifrost", TargetRaw: "people/freya", Line: 10},
			},
		}

		issues := v.ValidateDocument(doc)
		for _, issue := range issues {
			if strings.Contains(issue.Message, "not found") && strings.Contains(issue.Message, "freya") {
				t.Errorf("Should not have error for valid reference: %v", issue)
			}
		}
	})
}

func TestValidatorUnknownFrontmatterKey(t *testing.T) {
	s := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {
				Fields: map[string]*schema.FieldDefinition{
					"name": {Type: schema.FieldTypeString},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{},
	}

	objectIDs := []string{"people/freya"}
	v := NewValidator(s, objectIDs)

	doc := &parser.ParsedDocument{
		FilePath: "people/freya.md",
		Objects: []*parser.ParsedObject{
			{
				ID:         "people/freya",
				ObjectType: "person",
				Fields: map[string]schema.FieldValue{
					"name":          schema.String("Freya"),
					"unknown_field": schema.String("should trigger error"),
				},
			},
		},
	}

	issues := v.ValidateDocument(doc)
	hasUnknownKeyError := false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "Unknown frontmatter key") {
			hasUnknownKeyError = true
			break
		}
	}
	if !hasUnknownKeyError {
		t.Errorf("Expected unknown frontmatter key error, got: %v", issues)
	}
}

func TestValidatorTraitValidation(t *testing.T) {
	s := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"page": {},
		},
		Traits: map[string]*schema.TraitDefinition{
			"due": {Type: schema.FieldTypeDate},
			"priority": {
				Type:   schema.FieldTypeEnum,
				Values: []string{"low", "medium", "high"},
			},
		},
	}

	objectIDs := []string{"notes/test"}
	v := NewValidator(s, objectIDs)

	t.Run("valid date trait", func(t *testing.T) {
		dueValue := schema.String("2025-02-01")
		doc := &parser.ParsedDocument{
			FilePath: "notes/test.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "notes/test",
					ObjectType: "page",
				},
			},
			Traits: []*parser.ParsedTrait{
				{
					TraitType:      "due",
					Value:          &dueValue,
					ParentObjectID: "notes/test",
					Line:           5,
				},
			},
		}

		issues := v.ValidateDocument(doc)
		for _, issue := range issues {
			if strings.Contains(issue.Message, "due") && issue.Level == LevelError {
				t.Errorf("Should not have error for valid date trait: %v", issue)
			}
		}
	})

	t.Run("invalid date trait", func(t *testing.T) {
		badValue := schema.String("not-a-date")
		doc := &parser.ParsedDocument{
			FilePath: "notes/test.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "notes/test",
					ObjectType: "page",
				},
			},
			Traits: []*parser.ParsedTrait{
				{
					TraitType:      "due",
					Value:          &badValue,
					ParentObjectID: "notes/test",
					Line:           5,
				},
			},
		}

		issues := v.ValidateDocument(doc)
		hasDateError := false
		for _, issue := range issues {
			if issue.Type == IssueInvalidDateFormat {
				hasDateError = true
				break
			}
		}
		if !hasDateError {
			t.Errorf("Expected invalid date format error, got: %v", issues)
		}
	})

	t.Run("invalid enum trait", func(t *testing.T) {
		badValue := schema.String("critical") // Not in enum
		doc := &parser.ParsedDocument{
			FilePath: "notes/test.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "notes/test",
					ObjectType: "page",
				},
			},
			Traits: []*parser.ParsedTrait{
				{
					TraitType:      "priority",
					Value:          &badValue,
					ParentObjectID: "notes/test",
					Line:           5,
				},
			},
		}

		issues := v.ValidateDocument(doc)
		hasEnumError := false
		for _, issue := range issues {
			if strings.Contains(issue.Message, "Invalid value") || strings.Contains(issue.Message, "allowed") {
				hasEnumError = true
				break
			}
		}
		if !hasEnumError {
			t.Errorf("Expected invalid enum error, got: %v", issues)
		}
	})
}

func TestIssueLevel(t *testing.T) {
	t.Run("error string", func(t *testing.T) {
		if LevelError.String() != "ERROR" {
			t.Errorf("LevelError.String() = %q, want %q", LevelError.String(), "ERROR")
		}
	})

	t.Run("warning string", func(t *testing.T) {
		if LevelWarning.String() != "WARN" {
			t.Errorf("LevelWarning.String() = %q, want %q", LevelWarning.String(), "WARN")
		}
	})
}

func TestInferConfidence(t *testing.T) {
	t.Run("certain string", func(t *testing.T) {
		if ConfidenceCertain.String() != "certain" {
			t.Errorf("ConfidenceCertain.String() = %q, want %q", ConfidenceCertain.String(), "certain")
		}
	})

	t.Run("inferred string", func(t *testing.T) {
		if ConfidenceInferred.String() != "inferred" {
			t.Errorf("ConfidenceInferred.String() = %q, want %q", ConfidenceInferred.String(), "inferred")
		}
	})

	t.Run("unknown string", func(t *testing.T) {
		if ConfidenceUnknown.String() != "unknown" {
			t.Errorf("ConfidenceUnknown.String() = %q, want %q", ConfidenceUnknown.String(), "unknown")
		}
	})
}

func TestValidatorTargetTypeValidation(t *testing.T) {
	s := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"lead": {
						Type:   schema.FieldTypeRef,
						Target: "person",
					},
				},
			},
			"person": {
				DefaultPath: "people/",
				Fields: map[string]*schema.FieldDefinition{
					"name": {Type: schema.FieldTypeString},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{},
	}

	objectInfos := []ObjectInfo{
		{ID: "people/freya", Type: "person"},
		{ID: "projects/website", Type: "project"},
	}
	v := NewValidatorWithTypes(s, objectInfos)

	t.Run("correct target type", func(t *testing.T) {
		doc := &parser.ParsedDocument{
			FilePath: "projects/website.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "projects/website",
					ObjectType: "project",
					Fields: map[string]schema.FieldValue{
						"lead": schema.String("people/freya"),
					},
				},
			},
		}

		issues := v.ValidateDocument(doc)
		for _, issue := range issues {
			if issue.Type == IssueWrongTargetType {
				t.Errorf("Should not have wrong target type error for valid ref: %v", issue)
			}
		}
	})

	t.Run("wrong target type", func(t *testing.T) {
		doc := &parser.ParsedDocument{
			FilePath: "projects/mobile.md",
			Objects: []*parser.ParsedObject{
				{
					ID:         "projects/mobile",
					ObjectType: "project",
					Fields: map[string]schema.FieldValue{
						"lead": schema.String("projects/website"), // Wrong - should be person
					},
				},
			},
		}

		issues := v.ValidateDocument(doc)
		hasWrongTypeError := false
		for _, issue := range issues {
			if issue.Type == IssueWrongTargetType {
				hasWrongTypeError = true
				break
			}
		}
		if !hasWrongTypeError {
			t.Errorf("Expected wrong target type error, got: %v", issues)
		}
	})
}

func TestValidatorSchemaIntegrity(t *testing.T) {
	t.Run("unused type", func(t *testing.T) {
		s := &schema.Schema{
			Types: map[string]*schema.TypeDefinition{
				"person":  {Fields: map[string]*schema.FieldDefinition{}},
				"meeting": {Fields: map[string]*schema.FieldDefinition{}}, // Never used
			},
			Traits: map[string]*schema.TraitDefinition{},
		}

		objectInfos := []ObjectInfo{
			{ID: "people/freya", Type: "person"},
		}
		v := NewValidatorWithTypes(s, objectInfos)

		// Validate a document to populate usedTypes
		doc := &parser.ParsedDocument{
			FilePath: "people/freya.md",
			Objects: []*parser.ParsedObject{
				{ID: "people/freya", ObjectType: "person"},
			},
		}
		v.ValidateDocument(doc)

		schemaIssues := v.ValidateSchema()
		hasUnusedType := false
		for _, issue := range schemaIssues {
			if issue.Type == IssueUnusedType && issue.Value == "meeting" {
				hasUnusedType = true
				break
			}
		}
		if !hasUnusedType {
			t.Errorf("Expected unused type warning for 'meeting', got: %v", schemaIssues)
		}
	})

	t.Run("missing target type", func(t *testing.T) {
		s := &schema.Schema{
			Types: map[string]*schema.TypeDefinition{
				"project": {
					Fields: map[string]*schema.FieldDefinition{
						"owner": {
							Type:   schema.FieldTypeRef,
							Target: "nonexistent", // Type doesn't exist
						},
					},
				},
			},
			Traits: map[string]*schema.TraitDefinition{},
		}

		v := NewValidatorWithTypes(s, []ObjectInfo{})
		schemaIssues := v.ValidateSchema()

		hasMissingTarget := false
		for _, issue := range schemaIssues {
			if issue.Type == IssueMissingTargetType {
				hasMissingTarget = true
				break
			}
		}
		if !hasMissingTarget {
			t.Errorf("Expected missing target type error, got: %v", schemaIssues)
		}
	})

	t.Run("self-referential required field", func(t *testing.T) {
		s := &schema.Schema{
			Types: map[string]*schema.TypeDefinition{
				"person": {
					Fields: map[string]*schema.FieldDefinition{
						"manager": {
							Type:     schema.FieldTypeRef,
							Target:   "person",
							Required: true, // Can't create first person!
						},
					},
				},
			},
			Traits: map[string]*schema.TraitDefinition{},
		}

		v := NewValidatorWithTypes(s, []ObjectInfo{})
		schemaIssues := v.ValidateSchema()

		hasSelfRef := false
		for _, issue := range schemaIssues {
			if issue.Type == IssueSelfReferentialRequired {
				hasSelfRef = true
				break
			}
		}
		if !hasSelfRef {
			t.Errorf("Expected self-referential required field warning, got: %v", schemaIssues)
		}
	})
}

func TestValidatorShortRefSuggestion(t *testing.T) {
	s := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"page": {Fields: map[string]*schema.FieldDefinition{}},
		},
		Traits: map[string]*schema.TraitDefinition{},
	}

	objectInfos := []ObjectInfo{
		{ID: "people/freya", Type: "person"},
	}
	v := NewValidatorWithTypes(s, objectInfos)

	doc := &parser.ParsedDocument{
		FilePath: "notes/test.md",
		Objects: []*parser.ParsedObject{
			{ID: "notes/test", ObjectType: "page"},
		},
		Refs: []*parser.ParsedRef{
			{SourceID: "notes/test", TargetRaw: "freya", Line: 5}, // Short ref
		},
	}

	issues := v.ValidateDocument(doc)
	hasShortRefWarning := false
	for _, issue := range issues {
		if issue.Type == IssueShortRefCouldBeFullPath {
			hasShortRefWarning = true
			break
		}
	}
	if !hasShortRefWarning {
		t.Errorf("Expected short ref suggestion warning, got: %v", issues)
	}

	// Check that shortRefs map was populated
	if fullPath, ok := v.ShortRefs()["freya"]; !ok || fullPath != "people/freya" {
		t.Errorf("Expected shortRefs to contain 'freya' -> 'people/freya', got: %v", v.ShortRefs())
	}
}

func TestValidatorDatetimeValidation(t *testing.T) {
	s := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"page": {},
		},
		Traits: map[string]*schema.TraitDefinition{
			"remind": {Type: schema.FieldTypeDatetime},
		},
	}

	objectIDs := []string{"notes/test"}
	v := NewValidator(s, objectIDs)

	t.Run("valid datetime trait", func(t *testing.T) {
		validValue := schema.String("2025-02-01T09:00")
		doc := &parser.ParsedDocument{
			FilePath: "notes/test.md",
			Objects: []*parser.ParsedObject{
				{ID: "notes/test", ObjectType: "page"},
			},
			Traits: []*parser.ParsedTrait{
				{
					TraitType:      "remind",
					Value:          &validValue,
					ParentObjectID: "notes/test",
					Line:           5,
				},
			},
		}

		issues := v.ValidateDocument(doc)
		for _, issue := range issues {
			if issue.Type == IssueInvalidDateFormat {
				t.Errorf("Should not have datetime error for valid value: %v", issue)
			}
		}
	})

	t.Run("invalid datetime trait", func(t *testing.T) {
		badValue := schema.String("not-a-datetime")
		doc := &parser.ParsedDocument{
			FilePath: "notes/test.md",
			Objects: []*parser.ParsedObject{
				{ID: "notes/test", ObjectType: "page"},
			},
			Traits: []*parser.ParsedTrait{
				{
					TraitType:      "remind",
					Value:          &badValue,
					ParentObjectID: "notes/test",
					Line:           5,
				},
			},
		}

		issues := v.ValidateDocument(doc)
		hasDatetimeError := false
		for _, issue := range issues {
			if issue.Type == IssueInvalidDateFormat {
				hasDatetimeError = true
				break
			}
		}
		if !hasDatetimeError {
			t.Errorf("Expected invalid datetime format error, got: %v", issues)
		}
	})
}
