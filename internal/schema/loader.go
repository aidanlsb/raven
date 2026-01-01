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

	// Initialize nil field maps
	for _, typeDef := range schema.Types {
		if typeDef.Fields == nil {
			typeDef.Fields = make(map[string]*FieldDefinition)
		}
	}
	for _, traitDef := range schema.Traits {
		if traitDef.Fields == nil {
			traitDef.Fields = make(map[string]*FieldDefinition)
		}
	}

	return &schema, nil
}

// CreateDefault creates a default schema.yaml file in the vault.
func CreateDefault(vaultPath string) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	defaultSchema := `# Raven Schema Configuration
# Define your types and traits here

types:
  # Example: person type
  # person:
  #   fields:
  #     name:
  #       type: string
  #       required: true
  #     email:
  #       type: string
  #   detect:
  #     path_pattern: "^people/"

  # Example: daily note type
  # daily:
  #   fields:
  #     date:
  #       type: date
  #       derived: from_filename
  #   detect:
  #     path_pattern: "^daily/\\d{4}-\\d{2}-\\d{2}\\.md$"

traits:
  task:
    fields:
      due:
        type: date
      priority:
        type: enum
        values: [low, medium, high]
        default: medium
      status:
        type: enum
        values: [todo, in_progress, done]
        default: todo
    cli:
      alias: tasks
      default_query: "status:todo OR status:in_progress"

  remind:
    fields:
      at:
        type: datetime
        positional: true

  highlight:
    fields:
      color:
        type: enum
        values: [yellow, red, green, blue]
        default: yellow
`

	if err := os.WriteFile(schemaPath, []byte(defaultSchema), 0644); err != nil {
		return fmt.Errorf("failed to write schema file: %w", err)
	}

	return nil
}
