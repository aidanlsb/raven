package schema

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load loads the schema from a vault's schema.yaml file.
// Returns a default schema if the file doesn't exist.
func Load(vaultPath string) (*Schema, error) {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		return NewSchema(), nil
	}

	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file %s: %w", schemaPath, err)
	}

	var schema Schema
	if err := yaml.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("failed to parse schema file %s: %w", schemaPath, err)
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

	return &schema, nil
}

// CreateDefault creates a default schema.yaml file in the vault.
func CreateDefault(vaultPath string) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	defaultSchema := `# Raven Schema Configuration
# Define your types and traits here.
#
# Types: Define what objects ARE (frontmatter 'type:' field)
# Traits: Single-valued annotations on content (@name or @name(value))
#
# Built-in types (always available):
#   - page: fallback for files without explicit type
#   - section: auto-created for headings
#   - date: daily notes (files named YYYY-MM-DD.md in daily_directory)

types:
  person:
    default_path: people/
    fields:
      name:
        type: string
        required: true
      email:
        type: string

  project:
    default_path: projects/
    fields:
      status:
        type: enum
        values: [active, paused, completed]
        default: active

# Traits are single-valued annotations.
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
		return fmt.Errorf("failed to write schema file: %w", err)
	}

	return nil
}
