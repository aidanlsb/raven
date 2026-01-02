package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Flags for schema add commands
var (
	schemaAddDefaultPath string
	schemaAddFieldType   string
	schemaAddRequired    bool
	schemaAddDefault     string
	schemaAddValues      string
	schemaAddTarget      string
)

var schemaAddCmd = &cobra.Command{
	Use:   "add <type|trait|field> <name> [parent-type]",
	Short: "Add a type, trait, or field to the schema",
	Long: `Add new definitions to schema.yaml.

Subcommands:
  type <name>              Add a new type
  trait <name>             Add a new trait
  field <type> <field>     Add a field to an existing type

Examples:
  rvn schema add type event --default-path events/
  rvn schema add trait priority --type enum --values high,medium,low
  rvn schema add field person email --type string --required`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		kind := args[0]

		switch kind {
		case "type":
			return addType(vaultPath, args[1], start)
		case "trait":
			return addTrait(vaultPath, args[1], start)
		case "field":
			if len(args) < 3 {
				return handleErrorMsg(ErrMissingArgument, "field requires type and field name", "Usage: rvn schema add field <type> <field>")
			}
			return addField(vaultPath, args[1], args[2], start)
		default:
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema kind: %s", kind), "Use: type, trait, or field")
		}
	},
}

func addType(vaultPath, typeName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if type already exists
	if _, exists := sch.Types[typeName]; exists {
		return handleErrorMsg(ErrObjectExists, fmt.Sprintf("type '%s' already exists", typeName), "")
	}

	// Check built-in types
	if typeName == "page" || typeName == "section" || typeName == "date" {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type", typeName), "Choose a different name")
	}

	// Read current schema file to preserve formatting
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	// Parse as YAML to modify
	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	// Ensure types map exists
	types, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		types = make(map[string]interface{})
		schemaDoc["types"] = types
	}

	// Build new type definition
	newType := make(map[string]interface{})
	if schemaAddDefaultPath != "" {
		newType["default_path"] = schemaAddDefaultPath
	}

	types[typeName] = newType

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"added":        "type",
			"name":         typeName,
			"default_path": schemaAddDefaultPath,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Added type '%s' to schema.yaml\n", typeName)
	if schemaAddDefaultPath != "" {
		fmt.Printf("  default_path: %s\n", schemaAddDefaultPath)
	}
	return nil
}

func addTrait(vaultPath, traitName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if trait already exists
	if _, exists := sch.Traits[traitName]; exists {
		return handleErrorMsg(ErrObjectExists, fmt.Sprintf("trait '%s' already exists", traitName), "")
	}

	// Read current schema file
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	// Ensure traits map exists
	traits, ok := schemaDoc["traits"].(map[string]interface{})
	if !ok {
		traits = make(map[string]interface{})
		schemaDoc["traits"] = traits
	}

	// Build new trait definition
	newTrait := make(map[string]interface{})

	// Default type is string
	traitType := schemaAddFieldType
	if traitType == "" {
		traitType = "string"
	}
	newTrait["type"] = traitType

	if schemaAddValues != "" {
		newTrait["values"] = strings.Split(schemaAddValues, ",")
	}
	if schemaAddDefault != "" {
		newTrait["default"] = schemaAddDefault
	}

	traits[traitName] = newTrait

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		result := map[string]interface{}{
			"added": "trait",
			"name":  traitName,
			"type":  traitType,
		}
		if schemaAddValues != "" {
			result["values"] = strings.Split(schemaAddValues, ",")
		}
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Added trait '%s' to schema.yaml\n", traitName)
	fmt.Printf("  type: %s\n", traitType)
	if schemaAddValues != "" {
		fmt.Printf("  values: %s\n", schemaAddValues)
	}
	return nil
}

func addField(vaultPath, typeName, fieldName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if type exists
	typeDef, exists := sch.Types[typeName]
	if !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "Add the type first with 'rvn schema add type'")
	}

	// Check if field already exists
	if typeDef.Fields != nil {
		if _, exists := typeDef.Fields[fieldName]; exists {
			return handleErrorMsg(ErrObjectExists, fmt.Sprintf("field '%s' already exists on type '%s'", fieldName, typeName), "")
		}
	}

	// Read current schema file
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	// Navigate to type definition
	types, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrSchemaInvalid, "types section not found", "")
	}

	typeNode, ok := types[typeName].(map[string]interface{})
	if !ok {
		typeNode = make(map[string]interface{})
		types[typeName] = typeNode
	}

	// Ensure fields map exists
	fields, ok := typeNode["fields"].(map[string]interface{})
	if !ok {
		fields = make(map[string]interface{})
		typeNode["fields"] = fields
	}

	// Build new field definition
	newField := make(map[string]interface{})

	fieldType := schemaAddFieldType
	if fieldType == "" {
		fieldType = "string"
	}
	newField["type"] = fieldType

	if schemaAddRequired {
		newField["required"] = true
	}
	if schemaAddDefault != "" {
		newField["default"] = schemaAddDefault
	}
	if schemaAddValues != "" {
		newField["values"] = strings.Split(schemaAddValues, ",")
	}
	if schemaAddTarget != "" {
		newField["target"] = schemaAddTarget
	}

	fields[fieldName] = newField

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		result := map[string]interface{}{
			"added":    "field",
			"type":     typeName,
			"field":    fieldName,
			"field_type": fieldType,
			"required": schemaAddRequired,
		}
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Added field '%s' to type '%s'\n", fieldName, typeName)
	fmt.Printf("  type: %s\n", fieldType)
	if schemaAddRequired {
		fmt.Println("  required: true")
	}
	return nil
}

var schemaValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the schema",
	Long: `Validate schema.yaml for correctness.

Checks:
  - Valid YAML syntax
  - Valid field types
  - Valid trait types
  - Referenced types exist
  - No circular references

Examples:
  rvn schema validate
  rvn schema validate --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()

		// Try to load the schema - this validates most things
		sch, err := schema.Load(vaultPath)
		if err != nil {
			return handleError(ErrSchemaInvalid, err, "Fix the errors and try again")
		}

		// Additional validation
		var issues []string

		// Check that ref targets exist
		for typeName, typeDef := range sch.Types {
			if typeDef.Fields != nil {
				for fieldName, fieldDef := range typeDef.Fields {
					if fieldDef != nil && fieldDef.Type == schema.FieldTypeRef && fieldDef.Target != "" {
						if _, exists := sch.Types[fieldDef.Target]; !exists {
							issues = append(issues, fmt.Sprintf("Type '%s' field '%s' references unknown type '%s'", typeName, fieldName, fieldDef.Target))
						}
					}
				}
			}

			// Check that declared traits exist
			for _, traitName := range typeDef.Traits.List() {
				if _, exists := sch.Traits[traitName]; !exists {
					issues = append(issues, fmt.Sprintf("Type '%s' declares unknown trait '%s'", typeName, traitName))
				}
			}
		}

		elapsed := time.Since(start).Milliseconds()

		if isJSONOutput() {
			result := map[string]interface{}{
				"valid":  len(issues) == 0,
				"issues": issues,
				"types":  len(sch.Types),
				"traits": len(sch.Traits),
			}
			outputSuccess(result, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		if len(issues) > 0 {
			fmt.Printf("Schema validation found %d issues:\n", len(issues))
			for _, issue := range issues {
				fmt.Printf("  ⚠ %s\n", issue)
			}
			return nil
		}

		fmt.Printf("✓ Schema is valid (%d types, %d traits)\n", len(sch.Types), len(sch.Traits))
		return nil
	},
}

func init() {
	// Add flags to schema add command
	schemaAddCmd.Flags().StringVar(&schemaAddDefaultPath, "default-path", "", "Default path for new type files")
	schemaAddCmd.Flags().StringVar(&schemaAddFieldType, "type", "", "Field/trait type (string, date, enum, ref, bool)")
	schemaAddCmd.Flags().BoolVar(&schemaAddRequired, "required", false, "Mark field as required")
	schemaAddCmd.Flags().StringVar(&schemaAddDefault, "default", "", "Default value")
	schemaAddCmd.Flags().StringVar(&schemaAddValues, "values", "", "Enum values (comma-separated)")
	schemaAddCmd.Flags().StringVar(&schemaAddTarget, "target", "", "Target type for ref fields")

	// Add subcommands to schema command
	schemaCmd.AddCommand(schemaAddCmd)
	schemaCmd.AddCommand(schemaValidateCmd)
}
