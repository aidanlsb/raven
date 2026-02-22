package schema

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/paths"
)

// SchemaWarning represents a non-fatal schema issue.
type SchemaWarning struct {
	Message string
}

// LoadResult contains the loaded schema and any warnings.
type LoadResult struct {
	Schema   *Schema
	Warnings []SchemaWarning
}

// Load loads the schema from a vault's schema.yaml file.
// Returns a default schema if the file doesn't exist.
func Load(vaultPath string) (*Schema, error) {
	result, err := LoadWithWarnings(vaultPath)
	if err != nil {
		return nil, err
	}
	return result.Schema, nil
}

// LoadWithWarnings loads the schema and returns any migration warnings.
func LoadWithWarnings(vaultPath string) (*LoadResult, error) {
	schemaPath := paths.SchemaPath(vaultPath)
	result := &LoadResult{Warnings: []SchemaWarning{}}

	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		result.Schema = NewSchema()
		return result, nil
	}

	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file %s: %w", schemaPath, err)
	}

	var schema Schema
	if err := yaml.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("failed to parse schema file %s: %w", schemaPath, err)
	}

	// Check schema version
	if schema.Version == 0 {
		// No version specified - assume v1 (old format)
		result.Warnings = append(result.Warnings, SchemaWarning{
			Message: "schema.yaml has no version field. Run 'rvn migrate --schema' to upgrade.",
		})
		schema.Version = 1
	} else if schema.Version < CurrentSchemaVersion {
		result.Warnings = append(result.Warnings, SchemaWarning{
			Message: fmt.Sprintf("schema.yaml is version %d, current is %d. Run 'rvn migrate --schema' to upgrade.", schema.Version, CurrentSchemaVersion),
		})
	}

	// Initialize maps if nil
	if schema.Types == nil {
		schema.Types = make(map[string]*TypeDefinition)
	}
	if schema.Core == nil {
		schema.Core = make(map[string]*CoreTypeDefinition)
	}
	if schema.Traits == nil {
		schema.Traits = make(map[string]*TraitDefinition)
	}
	if schema.Templates == nil {
		schema.Templates = make(map[string]*TemplateDefinition)
	}

	// Schema-level templates are file-backed only.
	for templateID, templateDef := range schema.Templates {
		if templateDef == nil {
			return nil, fmt.Errorf("template %q is null; expected an object with at least a file field", templateID)
		}
		spec := strings.TrimSpace(templateDef.File)
		if spec == "" {
			return nil, fmt.Errorf("template %q must define a non-empty file path", templateID)
		}
		if strings.Contains(spec, "\n") || strings.Contains(spec, "\r") {
			return nil, fmt.Errorf("template %q file must be a file path (inline templates are not supported)", templateID)
		}
	}

	// User type template settings must be valid.
	for typeName, typeDef := range schema.Types {
		if typeDef == nil {
			continue
		}
		if IsBuiltinType(typeName) {
			return nil, fmt.Errorf("type %q is a core type; configure it under 'core:' instead of 'types:'", typeName)
		}
		spec := strings.TrimSpace(typeDef.Template)
		if spec != "" {
			if strings.Contains(spec, "\n") || strings.Contains(spec, "\r") {
				return nil, fmt.Errorf("type %q template must be a file path (inline templates are not supported)", typeName)
			}
		}
		if err := validateTemplateBindings(typeName, typeDef.Templates, typeDef.DefaultTemplate, schema.Templates); err != nil {
			return nil, err
		}
	}

	// Core type template settings must be valid.
	for coreName, coreDef := range schema.Core {
		if !IsBuiltinType(coreName) {
			return nil, fmt.Errorf("unknown core type %q under 'core:'", coreName)
		}
		if coreDef == nil {
			return nil, fmt.Errorf("core type %q must be an object", coreName)
		}
		if coreName == "section" {
			if len(coreDef.Templates) > 0 || strings.TrimSpace(coreDef.DefaultTemplate) != "" {
				return nil, fmt.Errorf("core type %q does not support template configuration", coreName)
			}
			continue
		}
		if err := validateTemplateBindings("core."+coreName, coreDef.Templates, coreDef.DefaultTemplate, schema.Templates); err != nil {
			return nil, err
		}
	}

	// Ensure built-in types exist with their fixed definitions.
	// Built-in types are always overwritten to ensure consistency.
	schema.Types["page"] = &TypeDefinition{
		NameField: "title",
		Fields: map[string]*FieldDefinition{
			"title": {Type: FieldTypeString},
		},
	}
	schema.Types["section"] = &TypeDefinition{
		Fields: map[string]*FieldDefinition{
			"title": {Type: FieldTypeString},
			"level": {Type: FieldTypeNumber, Min: floatPtr(1), Max: floatPtr(6)},
		},
	}
	// Built-in 'date' type for daily notes
	dateType := &TypeDefinition{
		Fields: make(map[string]*FieldDefinition),
	}
	if coreDate := schema.Core["date"]; coreDate != nil {
		// Preserve template bindings for built-in date type from core config.
		dateType.Templates = append([]string(nil), coreDate.Templates...)
		dateType.DefaultTemplate = coreDate.DefaultTemplate
	}
	schema.Types["date"] = dateType
	if corePage := schema.Core["page"]; corePage != nil {
		schema.Types["page"].Templates = append([]string(nil), corePage.Templates...)
		schema.Types["page"].DefaultTemplate = corePage.DefaultTemplate
	}

	// Initialize nil field maps for types
	for _, typeDef := range schema.Types {
		if typeDef.Fields == nil {
			typeDef.Fields = make(map[string]*FieldDefinition)
		}
		for _, fieldDef := range typeDef.Fields {
			if fieldDef == nil {
				continue
			}
			fieldDef.Type = normalizeFieldType(fieldDef.Type)
		}
	}
	for _, traitDef := range schema.Traits {
		if traitDef == nil {
			continue
		}
		traitDef.Type = normalizeFieldType(traitDef.Type)
	}

	result.Schema = &schema
	return result, nil
}

func normalizeFieldType(fieldType FieldType) FieldType {
	switch strings.ToLower(string(fieldType)) {
	case "reference":
		return FieldTypeRef
	case "reference[]":
		return FieldTypeRefArray
	case "url":
		return FieldTypeURL
	case "url[]":
		return FieldTypeURLArray
	default:
		return fieldType
	}
}

func validateTemplateBindings(owner string, templateIDs []string, defaultTemplate string, templates map[string]*TemplateDefinition) error {
	seenTemplateIDs := make(map[string]struct{})
	for _, templateID := range templateIDs {
		templateID = strings.TrimSpace(templateID)
		if templateID == "" {
			return fmt.Errorf("%s templates cannot contain empty template IDs", owner)
		}
		if _, seen := seenTemplateIDs[templateID]; seen {
			return fmt.Errorf("%s templates contains duplicate template ID %q", owner, templateID)
		}
		seenTemplateIDs[templateID] = struct{}{}
		if _, ok := templates[templateID]; !ok {
			return fmt.Errorf("%s references unknown template %q", owner, templateID)
		}
	}
	if strings.TrimSpace(defaultTemplate) != "" {
		trimmedDefault := strings.TrimSpace(defaultTemplate)
		if _, ok := seenTemplateIDs[trimmedDefault]; !ok {
			return fmt.Errorf("%s default_template %q is not included in templates", owner, defaultTemplate)
		}
	}
	return nil
}

// CreateDefault creates a default schema.yaml file in the vault.
// Returns true if a new file was created, false if one already existed.
func CreateDefault(vaultPath string) (bool, error) {
	schemaPath := paths.SchemaPath(vaultPath)

	// Skip if file already exists
	if _, err := os.Stat(schemaPath); err == nil {
		return false, nil
	}

	defaultSchema := `# Raven Schema Configuration
# Define your types and traits here.

version: 1  # Schema format version (do not change manually)

# Types: Define what objects ARE (frontmatter 'type:' field)
# Types have fields (structured data in frontmatter)
#
# Built-in types (always available):
#   - page: fallback for files without explicit type
#   - section: auto-created for headings
#   - date: daily notes (files named YYYY-MM-DD.md under directories.daily)
#
# Configure templates for core types under the 'core' block.
# Supported:
#   core.date.templates/default_template
#   core.page.templates/default_template
#   core.section: {}   (placeholder only; no configurable fields)
#
# name_field: When set, 'rvn new <type> <title>' auto-populates this field
# with the title argument. Makes object creation more intuitive.
# description: Optional context for humans/agents (types and fields).

types:
  person:
    description: People and contacts
    default_path: person/
    name_field: name
    fields:
      name:
        type: string
        required: true
      email:
        type: string
        description: Primary contact email

  project:
    default_path: project/
    name_field: name
    fields:
      name:
        type: string
        required: true

  # Example with file-based template:
  # meeting:
  #   default_path: meeting/
  #   templates: [meeting_standard]
  #   default_template: meeting_standard

templates:
  # meeting_standard:
  #   file: templates/meeting.md
  #   description: Standard meeting notes template

core:
  # date:
  #   templates: [daily_default]
  #   default_template: daily_default
  # page:
  #   templates: [note_default]
  #   default_template: note_default
  # section: {}

# Traits: Universal annotations in content (@name or @name(value))
# Traits can be used on any object - just add them to your content.
# Boolean traits: @todo (no value)
# Valued traits: @due(2025-02-01), @priority(high)
traits:
  # Date-related
  due:
    type: date

  # Task marker (boolean trait)
  todo:
    type: boolean

  # Priority
  priority:
    type: enum
    values: [low, medium, high]
    default: medium
`

	if err := atomicfile.WriteFile(schemaPath, []byte(defaultSchema), 0o644); err != nil {
		return false, fmt.Errorf("failed to write schema file: %w", err)
	}

	return true, nil
}
