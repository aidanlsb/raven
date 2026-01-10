package schema

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
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
	schemaPath := filepath.Join(vaultPath, "schema.yaml")
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
	if schema.Traits == nil {
		schema.Traits = make(map[string]*TraitDefinition)
	}

	// Ensure built-in types exist
	if _, ok := schema.Types["page"]; !ok {
		schema.Types["page"] = &TypeDefinition{
			Fields: make(map[string]*FieldDefinition),
		}
	}
	if _, ok := schema.Types["section"]; !ok {
		schema.Types["section"] = &TypeDefinition{
			Fields: map[string]*FieldDefinition{
				"title": {Type: FieldTypeString},
				"level": {Type: FieldTypeNumber, Min: floatPtr(1), Max: floatPtr(6)},
			},
		}
	}
	// Built-in 'date' type for daily notes - locked, cannot be modified
	// Users should use traits for additional daily note metadata
	schema.Types["date"] = &TypeDefinition{
		Fields: make(map[string]*FieldDefinition),
	}

	// Initialize nil field maps for types
	for _, typeDef := range schema.Types {
		if typeDef.Fields == nil {
			typeDef.Fields = make(map[string]*FieldDefinition)
		}
	}

	result.Schema = &schema
	return result, nil
}

// CreateDefault creates a default schema.yaml file in the vault.
// Returns true if a new file was created, false if one already existed.
func CreateDefault(vaultPath string) (bool, error) {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Skip if file already exists
	if _, err := os.Stat(schemaPath); err == nil {
		return false, nil
	}

	defaultSchema := `# Raven Schema Configuration
# Define your types and traits here.

version: 2  # Schema format version (do not change manually)

# Types: Define what objects ARE (frontmatter 'type:' field)
# Types have fields (structured data in frontmatter)
#
# Built-in types (always available):
#   - page: fallback for files without explicit type
#   - section: auto-created for headings
#   - date: daily notes (files named YYYY-MM-DD.md in daily_directory)
#
# name_field: When set, 'rvn new <type> <title>' auto-populates this field
# with the title argument. Makes object creation more intuitive.

types:
  person:
    default_path: people/
    name_field: name
    fields:
      name:
        type: string
        required: true
      email:
        type: string

  project:
    default_path: projects/
    name_field: name
    fields:
      name:
        type: string
        required: true

  # Example with inline template:
  # meeting:
  #   default_path: meetings/
  #   template: |
  #     # {{title}}
  #     
  #     **Date:** {{date}}
  #     
  #     ## Attendees
  #     
  #     ## Notes
  #     
  #     ## Action Items
  #
  # Or use a file-based template:
  #   template: templates/meeting.md

# Traits: Universal annotations in content (@name or @name(value))
# Traits can be used on any object - just add them to your content.
# Boolean traits: @highlight (no value)
# Valued traits: @due(2025-02-01), @priority(high)
traits:
  # Date-related
  due:
    type: date

  remind:
    type: datetime

  # Priority/status
  priority:
    type: enum
    values: [low, medium, high]
    default: medium

  status:
    type: enum
    values: [todo, in_progress, done, blocked]
    default: todo

  # Markers (boolean traits)
  highlight:
    type: boolean

  pinned:
    type: boolean

  archived:
    type: boolean
`

	if err := os.WriteFile(schemaPath, []byte(defaultSchema), 0644); err != nil {
		return false, fmt.Errorf("failed to write schema file: %w", err)
	}

	return true, nil
}
