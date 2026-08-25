package schemachange

import (
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestClassify_NilAfter(t *testing.T) {
	t.Parallel()
	result := Classify(Diff{
		Before: schema.New(),
		After:  nil,
	})
	if result.Policy != PolicyFullScan {
		t.Errorf("expected PolicyFullScan when After is nil, got %v", result.Policy)
	}
	if len(result.Reasons) == 0 {
		t.Error("expected reasons when After is nil")
	}
}

func TestClassify_EmptyBefore(t *testing.T) {
	t.Parallel()
	// Treat nil Before as initial schema creation
	result := Classify(Diff{
		Before: nil,
		After:  schema.New(),
	})
	if result.Policy != PolicyNone {
		t.Errorf("expected PolicyNone for initial built-in schema setup, got %v", result.Policy)
	}
}

func TestClassify_TypeAdded(t *testing.T) {
	t.Parallel()
	before := schema.New()
	after := schema.New()
	after.Types["meeting"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"title": {Type: schema.FieldTypeString},
		},
	}

	result := Classify(Diff{Before: before, After: after})
	if result.Policy != PolicyFullScan {
		t.Errorf("expected PolicyFullScan, got %v", result.Policy)
	}
	if !contains(result.Reasons, "type added: meeting") {
		t.Errorf("expected reason for type addition, got %v", result.Reasons)
	}
}

func TestClassify_TypeRemoved(t *testing.T) {
	t.Parallel()
	before := schema.New()
	before.Types["meeting"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{},
	}
	after := schema.New()

	result := Classify(Diff{Before: before, After: after})
	if result.Policy != PolicyFullScan {
		t.Errorf("expected PolicyFullScan, got %v", result.Policy)
	}
	if !contains(result.Reasons, "type removed: meeting") {
		t.Errorf("expected reason for type removal, got %v", result.Reasons)
	}
}

func TestClassify_FieldAdded(t *testing.T) {
	t.Parallel()
	before := schema.New()
	before.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name": {Type: schema.FieldTypeString},
		},
	}
	after := schema.New()
	after.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name":  {Type: schema.FieldTypeString},
			"email": {Type: schema.FieldTypeString},
		},
	}

	result := Classify(Diff{Before: before, After: after})
	if result.Policy != PolicyFullScan {
		t.Errorf("expected PolicyFullScan, got %v", result.Policy)
	}
	if !contains(result.Reasons, "field added: person.email") {
		t.Errorf("expected reason for field addition, got %v", result.Reasons)
	}
}

func TestClassify_FieldRemoved(t *testing.T) {
	t.Parallel()
	before := schema.New()
	before.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name":  {Type: schema.FieldTypeString},
			"email": {Type: schema.FieldTypeString},
		},
	}
	after := schema.New()
	after.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name": {Type: schema.FieldTypeString},
		},
	}

	result := Classify(Diff{Before: before, After: after})
	if result.Policy != PolicyFullScan {
		t.Errorf("expected PolicyFullScan, got %v", result.Policy)
	}
	if !contains(result.Reasons, "field removed: person.email") {
		t.Errorf("expected reason for field removal, got %v", result.Reasons)
	}
}

func TestClassify_FieldTypeChanged(t *testing.T) {
	t.Parallel()
	before := schema.New()
	before.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name": {Type: schema.FieldTypeString},
		},
	}
	after := schema.New()
	after.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name": {Type: schema.FieldTypeNumber},
		},
	}

	result := Classify(Diff{Before: before, After: after})
	if result.Policy != PolicyFullScan {
		t.Errorf("expected PolicyFullScan, got %v", result.Policy)
	}
	if !contains(result.Reasons, "field type changed: person.name") {
		t.Errorf("expected reason for field type change, got %v", result.Reasons)
	}
}

func TestClassify_FieldRequiredChanged(t *testing.T) {
	t.Parallel()
	before := schema.New()
	before.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name": {Type: schema.FieldTypeString, Required: false},
		},
	}
	after := schema.New()
	after.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name": {Type: schema.FieldTypeString, Required: true},
		},
	}

	result := Classify(Diff{Before: before, After: after})
	if result.Policy != PolicyFullScan {
		t.Errorf("expected PolicyFullScan, got %v", result.Policy)
	}
	if !contains(result.Reasons, "field required changed: person.name") {
		t.Errorf("expected reason for required change, got %v", result.Reasons)
	}
}

func TestClassify_TraitAdded(t *testing.T) {
	t.Parallel()
	before := schema.New()
	after := schema.New()
	after.Traits["priority"] = &schema.TraitDefinition{
		Type:   schema.FieldTypeEnum,
		Values: []string{"low", "high"},
	}

	result := Classify(Diff{Before: before, After: after})
	if result.Policy != PolicyFullScan {
		t.Errorf("expected PolicyFullScan, got %v", result.Policy)
	}
	if !contains(result.Reasons, "trait added: priority") {
		t.Errorf("expected reason for trait addition, got %v", result.Reasons)
	}
}

func TestClassify_TraitRemoved(t *testing.T) {
	t.Parallel()
	before := schema.New()
	before.Traits["priority"] = &schema.TraitDefinition{
		Type: schema.FieldTypeEnum,
	}
	after := schema.New()

	result := Classify(Diff{Before: before, After: after})
	if result.Policy != PolicyFullScan {
		t.Errorf("expected PolicyFullScan, got %v", result.Policy)
	}
	if !contains(result.Reasons, "trait removed: priority") {
		t.Errorf("expected reason for trait removal, got %v", result.Reasons)
	}
}

func TestClassify_TraitTypeChanged(t *testing.T) {
	t.Parallel()
	before := schema.New()
	before.Traits["priority"] = &schema.TraitDefinition{
		Type: schema.FieldTypeBool,
	}
	after := schema.New()
	after.Traits["priority"] = &schema.TraitDefinition{
		Type: schema.FieldTypeEnum,
	}

	result := Classify(Diff{Before: before, After: after})
	if result.Policy != PolicyFullScan {
		t.Errorf("expected PolicyFullScan, got %v", result.Policy)
	}
	if !contains(result.Reasons, "trait type changed: priority") {
		t.Errorf("expected reason for trait type change, got %v", result.Reasons)
	}
}

func TestClassify_TypeDefaultPathChanged(t *testing.T) {
	t.Parallel()
	before := schema.New()
	before.Types["person"] = &schema.TypeDefinition{
		DefaultPath: "people/",
		Fields:      map[string]*schema.FieldDefinition{},
	}
	after := schema.New()
	after.Types["person"] = &schema.TypeDefinition{
		DefaultPath: "person/",
		Fields:      map[string]*schema.FieldDefinition{},
	}

	result := Classify(Diff{Before: before, After: after})
	if result.Policy != PolicyResolverRefresh {
		t.Errorf("expected PolicyResolverRefresh, got %v", result.Policy)
	}
	if !contains(result.Reasons, "type default_path changed: person") {
		t.Errorf("expected reason for default_path change, got %v", result.Reasons)
	}
}

func TestClassify_DescriptionChanged(t *testing.T) {
	t.Parallel()
	before := schema.New()
	before.Types["person"] = &schema.TypeDefinition{
		Description: "Old description",
		Fields:      map[string]*schema.FieldDefinition{},
	}
	after := schema.New()
	after.Types["person"] = &schema.TypeDefinition{
		Description: "New description",
		Fields:      map[string]*schema.FieldDefinition{},
	}

	result := Classify(Diff{Before: before, After: after})
	if result.Policy != PolicyNone {
		t.Errorf("expected PolicyNone for description change, got %v", result.Policy)
	}
	if !contains(result.Reasons, "metadata-only change") {
		t.Errorf("expected metadata-only reason, got %v", result.Reasons)
	}
}

func TestClassify_NameFieldChanged(t *testing.T) {
	t.Parallel()
	before := schema.New()
	before.Types["person"] = &schema.TypeDefinition{
		NameField: "name",
		Fields: map[string]*schema.FieldDefinition{
			"name":  {Type: schema.FieldTypeString},
			"title": {Type: schema.FieldTypeString},
		},
	}
	after := schema.New()
	after.Types["person"] = &schema.TypeDefinition{
		NameField: "title",
		Fields: map[string]*schema.FieldDefinition{
			"name":  {Type: schema.FieldTypeString},
			"title": {Type: schema.FieldTypeString},
		},
	}

	result := Classify(Diff{Before: before, After: after})
	if result.Policy != PolicyNone {
		t.Errorf("expected PolicyNone for name_field change (metadata), got %v", result.Policy)
	}
}

func TestClassify_MultipleChanges_FullScanWins(t *testing.T) {
	t.Parallel()
	// Both a field addition (full scan) and default_path change (resolver refresh)
	before := schema.New()
	before.Types["person"] = &schema.TypeDefinition{
		DefaultPath: "people/",
		Fields: map[string]*schema.FieldDefinition{
			"name": {Type: schema.FieldTypeString},
		},
	}
	after := schema.New()
	after.Types["person"] = &schema.TypeDefinition{
		DefaultPath: "person/",
		Fields: map[string]*schema.FieldDefinition{
			"name":  {Type: schema.FieldTypeString},
			"email": {Type: schema.FieldTypeString},
		},
	}

	result := Classify(Diff{Before: before, After: after})
	if result.Policy != PolicyFullScan {
		t.Errorf("expected PolicyFullScan when both full-scan and resolver changes present, got %v", result.Policy)
	}
	if !contains(result.Reasons, "field added: person.email") {
		t.Errorf("expected full-scan reason, got %v", result.Reasons)
	}
}

func TestClassify_BuiltinTypesIgnored(t *testing.T) {
	t.Parallel()
	// Changes to built-in types should not trigger invalidation
	before := schema.New()
	after := schema.New()
	after.Types["page"].Description = "Updated built-in description"

	result := Classify(Diff{Before: before, After: after})
	if result.Policy != PolicyNone {
		t.Errorf("expected PolicyNone for built-in type description change, got %v", result.Policy)
	}
}

func contains(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}
