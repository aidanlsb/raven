package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

// Flags for schema add commands
var (
	schemaAddDefaultPath string
	schemaAddNameField   string
	schemaAddFieldType   string
	schemaAddRequired    bool
	schemaAddDefault     string
	schemaAddValues      string
	schemaAddTarget      string
)

// Flags for schema update commands
var (
	schemaUpdateDefaultPath string
	schemaUpdateNameField   string
	schemaUpdateFieldType   string
	schemaUpdateRequired    string // "true", "false", or "" (no change)
	schemaUpdateDefault     string
	schemaUpdateValues      string
	schemaUpdateTarget      string
	schemaUpdateAddTrait    string
	schemaUpdateRemoveTrait string
)

// Flags for schema remove commands
var (
	schemaRemoveForce bool
)

// Flags for schema rename commands
var (
	schemaRenameConfirm bool
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
	if schema.IsBuiltinType(typeName) {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type", typeName), "Choose a different name")
	}

	// Interactive prompt for name_field if not provided and not in JSON mode
	nameField := schemaAddNameField
	if nameField == "" && !isJSONOutput() {
		fmt.Print("Which field should be the display name? (common: name, title; leave blank for none): ")
		var input string
		fmt.Scanln(&input)
		nameField = strings.TrimSpace(input)
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

	// Handle name_field - auto-create the field if it doesn't exist
	if nameField != "" {
		newType["name_field"] = nameField

		// Auto-create the field as required string
		fields := make(map[string]interface{})
		fields[nameField] = map[string]interface{}{
			"type":     "string",
			"required": true,
		}
		newType["fields"] = fields
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
		result := map[string]interface{}{
			"added":        "type",
			"name":         typeName,
			"default_path": schemaAddDefaultPath,
		}
		if nameField != "" {
			result["name_field"] = nameField
			result["auto_created_field"] = nameField
		}
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Added type '%s' to schema.yaml\n", typeName)
	if schemaAddDefaultPath != "" {
		fmt.Printf("  default_path: %s\n", schemaAddDefaultPath)
	}
	if nameField != "" {
		fmt.Printf("  name_field: %s (auto-created as required string)\n", nameField)
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

// validFieldTypes are the base field types supported by Raven
var validFieldTypes = map[string]bool{
	"string":   true,
	"number":   true,
	"date":     true,
	"datetime": true,
	"bool":     true,
	"boolean":  true,
	"enum":     true,
	"ref":      true,
}

// FieldTypeValidation holds the result of validating a field type specification
type FieldTypeValidation struct {
	Valid       bool
	BaseType    string
	IsArray     bool
	Error       string
	Suggestion  string
	Examples    []string
	ValidTypes  []string
	TargetHint  string
}

// validateFieldTypeSpec validates the --type and related flags for adding a field
func validateFieldTypeSpec(fieldType, target, values string, sch *schema.Schema) FieldTypeValidation {
	result := FieldTypeValidation{
		ValidTypes: []string{"string", "number", "date", "datetime", "bool", "enum", "ref"},
	}

	// Handle empty type (defaults to string)
	if fieldType == "" {
		fieldType = "string"
	}

	// Check for array suffix
	isArray := strings.HasSuffix(fieldType, "[]")
	baseType := strings.TrimSuffix(fieldType, "[]")

	result.BaseType = baseType
	result.IsArray = isArray

	// Check if this looks like a schema type name (common mistake)
	if sch != nil {
		if _, isSchemaType := sch.Types[baseType]; isSchemaType {
			result.Error = fmt.Sprintf("'%s' is a type name, not a field type", baseType)
			result.Suggestion = fmt.Sprintf("To reference objects of type '%s', use --type ref --target %s", baseType, baseType)
			if isArray {
				result.Examples = []string{
					fmt.Sprintf("--type ref[] --target %s  (array of %s references)", baseType, baseType),
				}
			} else {
				result.Examples = []string{
					fmt.Sprintf("--type ref --target %s  (single %s reference)", baseType, baseType),
					fmt.Sprintf("--type ref[] --target %s  (array of %s references)", baseType, baseType),
				}
			}
			return result
		}

		// Also check if adding [] to a type name
		if _, isSchemaType := sch.Types[strings.TrimSuffix(baseType, "[]")]; isSchemaType {
			cleanType := strings.TrimSuffix(baseType, "[]")
			result.Error = fmt.Sprintf("'%s' is a type name, not a field type", cleanType)
			result.Suggestion = fmt.Sprintf("To reference an array of '%s' objects, use --type ref[] --target %s", cleanType, cleanType)
			result.Examples = []string{
				fmt.Sprintf("--type ref[] --target %s", cleanType),
			}
			return result
		}
	}

	// Check if base type is valid
	if !validFieldTypes[baseType] {
		result.Error = fmt.Sprintf("'%s' is not a valid field type", fieldType)
		result.Suggestion = "Valid types: string, number, date, datetime, bool, enum, ref (add [] suffix for arrays)"
		result.Examples = []string{
			"--type string        (text)",
			"--type string[]      (array of text, e.g., tags)",
			"--type ref --target person   (reference to a person)",
			"--type ref[] --target person (array of person references)",
			"--type enum --values a,b,c   (single choice from list)",
		}
		return result
	}

	// Validate ref type requires target
	if (baseType == "ref") && target == "" {
		result.Error = "ref fields require --target to specify which type they reference"
		result.Suggestion = "Add --target <type_name> to specify the referenced type"
		if sch != nil && len(sch.Types) > 0 {
			var typeNames []string
			for name := range sch.Types {
				typeNames = append(typeNames, name)
			}
			// Sort for consistent output
			sort.Strings(typeNames)
			if len(typeNames) > 3 {
				typeNames = typeNames[:3]
			}
			result.Examples = []string{}
			for _, t := range typeNames {
				if isArray {
					result.Examples = append(result.Examples, fmt.Sprintf("--type ref[] --target %s", t))
				} else {
					result.Examples = append(result.Examples, fmt.Sprintf("--type ref --target %s", t))
				}
			}
		}
		result.TargetHint = "Available types can be listed with 'rvn schema types'"
		return result
	}

	// Validate enum type requires values
	if (baseType == "enum") && values == "" {
		result.Error = "enum fields require --values to specify allowed values"
		result.Suggestion = "Add --values with comma-separated allowed values"
		result.Examples = []string{
			"--type enum --values active,paused,done",
			"--type enum[] --values red,green,blue  (allows multiple selections)",
		}
		return result
	}

	// Warn if target is provided but type is not ref
	if target != "" && baseType != "ref" {
		result.Error = fmt.Sprintf("--target is only valid for ref fields, but type is '%s'", fieldType)
		result.Suggestion = fmt.Sprintf("Either change --type to ref (or ref[]) or remove --target")
		result.Examples = []string{
			fmt.Sprintf("--type ref --target %s  (single reference)", target),
			fmt.Sprintf("--type ref[] --target %s  (array of references)", target),
		}
		return result
	}

	// Validate target type exists if specified
	if target != "" && sch != nil {
		if _, exists := sch.Types[target]; !exists {
			// Check built-in types
			builtins := map[string]bool{"page": true, "section": true, "date": true}
			if !builtins[target] {
				result.Error = fmt.Sprintf("target type '%s' does not exist in schema", target)
				result.Suggestion = fmt.Sprintf("Either create the type first with 'rvn schema add type %s' or use an existing type", target)
				if len(sch.Types) > 0 {
					var typeNames []string
					for name := range sch.Types {
						typeNames = append(typeNames, name)
					}
					sort.Strings(typeNames)
					result.TargetHint = fmt.Sprintf("Existing types: %s", strings.Join(typeNames, ", "))
				}
				return result
			}
		}
	}

	result.Valid = true
	return result
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

	// Check if type is a built-in type (cannot be modified)
	if schema.IsBuiltinType(typeName) {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("cannot add fields to built-in type '%s'", typeName), "Built-in types (page, section, date) have fixed definitions. Use traits for additional metadata.")
	}

	// Check if field already exists
	if typeDef.Fields != nil {
		if _, exists := typeDef.Fields[fieldName]; exists {
			return handleErrorMsg(ErrObjectExists, fmt.Sprintf("field '%s' already exists on type '%s'", fieldName, typeName), "")
		}
	}

	// Validate field type specification
	validation := validateFieldTypeSpec(schemaAddFieldType, schemaAddTarget, schemaAddValues, sch)
	if !validation.Valid {
		details := map[string]interface{}{
			"field_type":  schemaAddFieldType,
			"valid_types": validation.ValidTypes,
		}
		if len(validation.Examples) > 0 {
			details["examples"] = validation.Examples
		}
		if validation.TargetHint != "" {
			details["target_hint"] = validation.TargetHint
		}
		hint := validation.Suggestion
		if len(validation.Examples) > 0 && !isJSONOutput() {
			hint += "\n\nExamples:\n"
			for _, ex := range validation.Examples {
				hint += "  " + ex + "\n"
			}
		}
		return handleErrorWithDetails(ErrInvalidInput, validation.Error, hint, details)
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
			"added":      "field",
			"type":       typeName,
			"field":      fieldName,
			"field_type": fieldType,
			"required":   schemaAddRequired,
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
  - name_field references valid string fields
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

		// Use the comprehensive validation function
		issues := schema.ValidateSchema(sch)

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

// =============================================================================
// UPDATE COMMANDS
// =============================================================================

var schemaUpdateCmd = &cobra.Command{
	Use:   "update <type|trait|field> <name> [parent-type]",
	Short: "Update a type, trait, or field in the schema",
	Long: `Update existing definitions in schema.yaml.

Subcommands:
  type <name>              Update an existing type
  trait <name>             Update an existing trait  
  field <type> <field>     Update a field on an existing type

Examples:
  rvn schema update type person --default-path people/
  rvn schema update trait priority --values critical,high,medium,low
  rvn schema update field person email --required=true
  rvn schema update type meeting --add-trait due`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		kind := args[0]

		switch kind {
		case "type":
			return updateType(vaultPath, args[1], start)
		case "trait":
			return updateTrait(vaultPath, args[1], start)
		case "field":
			if len(args) < 3 {
				return handleErrorMsg(ErrMissingArgument, "field requires type and field name", "Usage: rvn schema update field <type> <field>")
			}
			return updateField(vaultPath, args[1], args[2], start)
		default:
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema kind: %s", kind), "Use: type, trait, or field")
		}
	},
}

func updateType(vaultPath, typeName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Check built-in types
	if schema.IsBuiltinType(typeName) {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type and cannot be modified", typeName), "")
	}

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if type exists
	typeDef, exists := sch.Types[typeName]
	if !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "Use 'rvn schema add type' to create it")
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

	types := schemaDoc["types"].(map[string]interface{})
	typeNode, ok := types[typeName].(map[string]interface{})
	if !ok {
		typeNode = make(map[string]interface{})
		types[typeName] = typeNode
	}

	changes := []string{}

	// Apply updates
	if schemaUpdateDefaultPath != "" {
		typeNode["default_path"] = schemaUpdateDefaultPath
		changes = append(changes, fmt.Sprintf("default_path=%s", schemaUpdateDefaultPath))
	}

	// Handle name_field update
	if schemaUpdateNameField != "" {
		// Empty string means remove name_field
		if schemaUpdateNameField == "-" || schemaUpdateNameField == "none" || schemaUpdateNameField == "\"\"" {
			delete(typeNode, "name_field")
			changes = append(changes, "removed name_field")
		} else {
			// Check if field exists; if not, auto-create it
			fieldExists := false
			if typeDef.Fields != nil {
				if fieldDef, ok := typeDef.Fields[schemaUpdateNameField]; ok {
					fieldExists = true
					// Validate it's a string type
					if fieldDef.Type != schema.FieldTypeString {
						return handleErrorMsg(ErrInvalidInput,
							fmt.Sprintf("name_field must reference a string field, '%s' is type '%s'", schemaUpdateNameField, fieldDef.Type),
							"Choose a string field or create a new one")
					}
				}
			}

			typeNode["name_field"] = schemaUpdateNameField

			if !fieldExists {
				// Auto-create the field
				fields, ok := typeNode["fields"].(map[string]interface{})
				if !ok {
					fields = make(map[string]interface{})
					typeNode["fields"] = fields
				}
				fields[schemaUpdateNameField] = map[string]interface{}{
					"type":     "string",
					"required": true,
				}
				changes = append(changes, fmt.Sprintf("name_field=%s (auto-created as required string)", schemaUpdateNameField))
			} else {
				changes = append(changes, fmt.Sprintf("name_field=%s", schemaUpdateNameField))
			}
		}
	}

	// Handle trait additions/removals
	if schemaUpdateAddTrait != "" {
		// Check trait exists
		if _, exists := sch.Traits[schemaUpdateAddTrait]; !exists {
			return handleErrorMsg(ErrTraitNotFound, fmt.Sprintf("trait '%s' not found", schemaUpdateAddTrait), "Add it first with 'rvn schema add trait'")
		}

		traits, ok := typeNode["traits"].([]interface{})
		if !ok {
			traits = []interface{}{}
		}
		// Check if already present
		found := false
		for _, t := range traits {
			if t.(string) == schemaUpdateAddTrait {
				found = true
				break
			}
		}
		if !found {
			traits = append(traits, schemaUpdateAddTrait)
			typeNode["traits"] = traits
			changes = append(changes, fmt.Sprintf("added trait %s", schemaUpdateAddTrait))
		}
	}

	if schemaUpdateRemoveTrait != "" {
		traits, ok := typeNode["traits"].([]interface{})
		if ok {
			newTraits := []interface{}{}
			for _, t := range traits {
				if t.(string) != schemaUpdateRemoveTrait {
					newTraits = append(newTraits, t)
				}
			}
			typeNode["traits"] = newTraits
			changes = append(changes, fmt.Sprintf("removed trait %s", schemaUpdateRemoveTrait))
		}
	}

	if len(changes) == 0 {
		return handleErrorMsg(ErrInvalidInput, "no changes specified", "Use flags like --default-path, --name-field, --add-trait, --remove-trait")
	}

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
			"updated": "type",
			"name":    typeName,
			"changes": changes,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Updated type '%s'\n", typeName)
	for _, c := range changes {
		fmt.Printf("  %s\n", c)
	}
	return nil
}

func updateTrait(vaultPath, traitName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if trait exists
	if _, exists := sch.Traits[traitName]; !exists {
		return handleErrorMsg(ErrTraitNotFound, fmt.Sprintf("trait '%s' not found", traitName), "Use 'rvn schema add trait' to create it")
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

	traits := schemaDoc["traits"].(map[string]interface{})
	traitNode, ok := traits[traitName].(map[string]interface{})
	if !ok {
		traitNode = make(map[string]interface{})
		traits[traitName] = traitNode
	}

	changes := []string{}

	// Apply updates
	if schemaUpdateFieldType != "" {
		traitNode["type"] = schemaUpdateFieldType
		changes = append(changes, fmt.Sprintf("type=%s", schemaUpdateFieldType))
	}

	if schemaUpdateValues != "" {
		traitNode["values"] = strings.Split(schemaUpdateValues, ",")
		changes = append(changes, fmt.Sprintf("values=%s", schemaUpdateValues))
	}

	if schemaUpdateDefault != "" {
		traitNode["default"] = schemaUpdateDefault
		changes = append(changes, fmt.Sprintf("default=%s", schemaUpdateDefault))
	}

	if len(changes) == 0 {
		return handleErrorMsg(ErrInvalidInput, "no changes specified", "Use flags like --type, --values, --default")
	}

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
			"updated": "trait",
			"name":    traitName,
			"changes": changes,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Updated trait '%s'\n", traitName)
	for _, c := range changes {
		fmt.Printf("  %s\n", c)
	}
	return nil
}

func updateField(vaultPath, typeName, fieldName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if type exists
	typeDef, exists := sch.Types[typeName]
	if !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "")
	}

	// Check if type is a built-in type (cannot be modified)
	if schema.IsBuiltinType(typeName) {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("cannot modify fields on built-in type '%s'", typeName), "Built-in types (page, section, date) have fixed definitions.")
	}

	// Check if field exists
	if typeDef.Fields == nil {
		return handleErrorMsg(ErrFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName), "Use 'rvn schema add field' to create it")
	}
	if _, exists := typeDef.Fields[fieldName]; !exists {
		return handleErrorMsg(ErrFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName), "Use 'rvn schema add field' to create it")
	}

	// Check data integrity for required changes
	if schemaUpdateRequired == "true" {
		// This would make the field required - check all objects have it
		db, err := index.Open(vaultPath)
		if err == nil {
			defer db.Close()
			objects, err := db.QueryObjects(typeName)
			if err == nil && len(objects) > 0 {
				var missing []string
				for _, obj := range objects {
					var fields map[string]interface{}
					if obj.Fields != "" && obj.Fields != "{}" {
						json.Unmarshal([]byte(obj.Fields), &fields)
					}
					if _, hasField := fields[fieldName]; !hasField {
						missing = append(missing, obj.ID)
					}
				}
				if len(missing) > 0 {
					details := map[string]interface{}{
						"missing_field":    fieldName,
						"affected_count":   len(missing),
						"affected_objects": missing,
					}
					if len(missing) > 5 {
						details["affected_objects"] = append(missing[:5], "... and more")
					}
					return handleErrorWithDetails(ErrDataIntegrityBlock,
						fmt.Sprintf("%d objects of type '%s' lack field '%s'", len(missing), typeName, fieldName),
						"Add the field to these files, then retry",
						details)
				}
			}
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

	types := schemaDoc["types"].(map[string]interface{})
	typeNode := types[typeName].(map[string]interface{})
	fields := typeNode["fields"].(map[string]interface{})
	fieldNode, ok := fields[fieldName].(map[string]interface{})
	if !ok {
		fieldNode = make(map[string]interface{})
		fields[fieldName] = fieldNode
	}

	changes := []string{}

	// Apply updates
	if schemaUpdateFieldType != "" {
		fieldNode["type"] = schemaUpdateFieldType
		changes = append(changes, fmt.Sprintf("type=%s", schemaUpdateFieldType))
	}

	if schemaUpdateRequired != "" {
		required := schemaUpdateRequired == "true"
		fieldNode["required"] = required
		changes = append(changes, fmt.Sprintf("required=%v", required))
	}

	if schemaUpdateDefault != "" {
		fieldNode["default"] = schemaUpdateDefault
		changes = append(changes, fmt.Sprintf("default=%s", schemaUpdateDefault))
	}

	if schemaUpdateValues != "" {
		fieldNode["values"] = strings.Split(schemaUpdateValues, ",")
		changes = append(changes, fmt.Sprintf("values=%s", schemaUpdateValues))
	}

	if schemaUpdateTarget != "" {
		fieldNode["target"] = schemaUpdateTarget
		changes = append(changes, fmt.Sprintf("target=%s", schemaUpdateTarget))
	}

	if len(changes) == 0 {
		return handleErrorMsg(ErrInvalidInput, "no changes specified", "Use flags like --type, --required, --default")
	}

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
			"updated": "field",
			"type":    typeName,
			"field":   fieldName,
			"changes": changes,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Updated field '%s' on type '%s'\n", fieldName, typeName)
	for _, c := range changes {
		fmt.Printf("  %s\n", c)
	}
	return nil
}

// =============================================================================
// REMOVE COMMANDS
// =============================================================================

var schemaRemoveCmd = &cobra.Command{
	Use:   "remove <type|trait|field> <name> [parent-type]",
	Short: "Remove a type, trait, or field from the schema",
	Long: `Remove definitions from schema.yaml.

Subcommands:
  type <name>              Remove a type (objects become 'page' type)
  trait <name>             Remove a trait (existing instances remain in files)
  field <type> <field>     Remove a field from a type

By default, warns about affected files. Use --force to skip warnings.

Examples:
  rvn schema remove type event
  rvn schema remove trait priority --force
  rvn schema remove field person nickname`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		kind := args[0]

		switch kind {
		case "type":
			return removeType(vaultPath, args[1], start)
		case "trait":
			return removeTrait(vaultPath, args[1], start)
		case "field":
			if len(args) < 3 {
				return handleErrorMsg(ErrMissingArgument, "field requires type and field name", "Usage: rvn schema remove field <type> <field>")
			}
			return removeField(vaultPath, args[1], args[2], start)
		default:
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema kind: %s", kind), "Use: type, trait, or field")
		}
	},
}

func removeType(vaultPath, typeName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Check built-in types
	if schema.IsBuiltinType(typeName) {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type and cannot be removed", typeName), "")
	}

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if type exists
	if _, exists := sch.Types[typeName]; !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "")
	}

	// Check for affected objects
	var warnings []Warning
	db, err := index.Open(vaultPath)
	if err == nil {
		defer db.Close()
		objects, err := db.QueryObjects(typeName)
		if err == nil && len(objects) > 0 {
			warning := Warning{
				Code:    "ORPHANED_FILES",
				Message: fmt.Sprintf("%d files of type '%s' will become 'page' type", len(objects), typeName),
			}
			warnings = append(warnings, warning)

			if !schemaRemoveForce && !isJSONOutput() {
				fmt.Printf("Warning: %d files of type '%s' will become 'page' type:\n", len(objects), typeName)
				for i, obj := range objects {
					if i >= 5 {
						fmt.Printf("  ... and %d more\n", len(objects)-5)
						break
					}
					fmt.Printf("  - %s\n", obj.FilePath)
				}
				fmt.Print("Continue? [y/N] ")
				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) != "y" {
					return handleErrorMsg(ErrConfirmationRequired, "operation cancelled", "Use --force to skip confirmation")
				}
			}
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

	types := schemaDoc["types"].(map[string]interface{})
	delete(types, typeName)

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
			"removed": "type",
			"name":    typeName,
		}
		outputSuccessWithWarnings(result, warnings, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Removed type '%s' from schema.yaml\n", typeName)
	return nil
}

func removeTrait(vaultPath, traitName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if trait exists
	if _, exists := sch.Traits[traitName]; !exists {
		return handleErrorMsg(ErrTraitNotFound, fmt.Sprintf("trait '%s' not found", traitName), "")
	}

	// Check for affected trait instances
	var warnings []Warning
	db, err := index.Open(vaultPath)
	if err == nil {
		defer db.Close()
		instances, err := db.QueryTraits(traitName, nil)
		if err == nil && len(instances) > 0 {
			warning := Warning{
				Code:    "ORPHANED_TRAITS",
				Message: fmt.Sprintf("%d instances of @%s will remain in files (no longer indexed)", len(instances), traitName),
			}
			warnings = append(warnings, warning)

			if !schemaRemoveForce && !isJSONOutput() {
				fmt.Printf("Warning: %d instances of @%s will remain in files (no longer indexed)\n", len(instances), traitName)
				fmt.Print("Continue? [y/N] ")
				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) != "y" {
					return handleErrorMsg(ErrConfirmationRequired, "operation cancelled", "Use --force to skip confirmation")
				}
			}
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

	traits := schemaDoc["traits"].(map[string]interface{})
	delete(traits, traitName)

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
			"removed": "trait",
			"name":    traitName,
		}
		outputSuccessWithWarnings(result, warnings, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Removed trait '%s' from schema.yaml\n", traitName)
	return nil
}

func removeField(vaultPath, typeName, fieldName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if type exists
	typeDef, exists := sch.Types[typeName]
	if !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "")
	}

	// Check if type is a built-in type (cannot be modified)
	if schema.IsBuiltinType(typeName) {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("cannot remove fields from built-in type '%s'", typeName), "Built-in types (page, section, date) have fixed definitions.")
	}

	// Check if field exists
	if typeDef.Fields == nil {
		return handleErrorMsg(ErrFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName), "")
	}
	fieldDef, exists := typeDef.Fields[fieldName]
	if !exists {
		return handleErrorMsg(ErrFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName), "")
	}

	// If field is required, block removal unless user fixes files first
	if fieldDef.Required {
		db, err := index.Open(vaultPath)
		if err == nil {
			defer db.Close()
			objects, err := db.QueryObjects(typeName)
			if err == nil && len(objects) > 0 {
				return handleErrorWithDetails(ErrDataIntegrityBlock,
					fmt.Sprintf("cannot remove required field '%s': %d objects have this field", fieldName, len(objects)),
					"First make the field optional with 'rvn schema update field', then remove it",
					map[string]interface{}{
						"field":          fieldName,
						"type":           typeName,
						"affected_count": len(objects),
					})
			}
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

	types := schemaDoc["types"].(map[string]interface{})
	typeNode := types[typeName].(map[string]interface{})
	fields := typeNode["fields"].(map[string]interface{})
	delete(fields, fieldName)

	// If fields is empty, remove the fields key entirely
	if len(fields) == 0 {
		delete(typeNode, "fields")
	}

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
			"removed": "field",
			"type":    typeName,
			"field":   fieldName,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Removed field '%s' from type '%s'\n", fieldName, typeName)
	return nil
}

// =============================================================================
// RENAME COMMANDS
// =============================================================================

var schemaRenameCmd = &cobra.Command{
	Use:   "rename <type> <old_name> <new_name>",
	Short: "Rename a type and update all references",
	Long: `Rename a type in schema.yaml and update all files that use it.

This command:
1. Renames the type in schema.yaml
2. Updates all 'type:' frontmatter fields
3. Updates all ::type() embedded declarations
4. Updates all ref field targets pointing to the old type

By default, previews changes. Use --confirm to apply.

Examples:
  rvn schema rename type event meeting
  rvn schema rename type event meeting --confirm`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		kind := args[0]

		switch kind {
		case "type":
			return renameType(vaultPath, args[1], args[2], start)
		default:
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema kind: %s", kind), "Currently supported: type")
		}
	},
}

// TypeRenameChange represents a single change to be made
type TypeRenameChange struct {
	FilePath    string `json:"file_path"`
	ChangeType  string `json:"change_type"` // "frontmatter", "embedded", "schema_ref_target"
	Description string `json:"description"`
	Line        int    `json:"line,omitempty"`
}

func renameType(vaultPath, oldName, newName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Validate names
	if oldName == "" || newName == "" {
		return handleErrorMsg(ErrInvalidInput, "type names cannot be empty", "")
	}

	if oldName == newName {
		return handleErrorMsg(ErrInvalidInput, "old and new names are the same", "")
	}

	// Check built-in types
	if schema.IsBuiltinType(oldName) {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type and cannot be renamed", oldName), "")
	}
	if schema.IsBuiltinType(newName) {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("cannot rename to '%s' - it's a built-in type", newName), "")
	}

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if old type exists
	if _, exists := sch.Types[oldName]; !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", oldName), "")
	}

	// Check if new type already exists
	if _, exists := sch.Types[newName]; exists {
		return handleErrorMsg(ErrObjectExists, fmt.Sprintf("type '%s' already exists", newName), "Choose a different name")
	}

	// Collect all changes needed
	var changes []TypeRenameChange

	// 1. Schema changes - the type itself
	changes = append(changes, TypeRenameChange{
		FilePath:    "schema.yaml",
		ChangeType:  "schema_type",
		Description: fmt.Sprintf("rename type '%s' to '%s'", oldName, newName),
	})

	// 2. Schema changes - ref field targets
	for typeName, typeDef := range sch.Types {
		if typeDef == nil || typeDef.Fields == nil {
			continue
		}
		for fieldName, fieldDef := range typeDef.Fields {
			if fieldDef == nil {
				continue
			}
			if fieldDef.Target == oldName {
				changes = append(changes, TypeRenameChange{
					FilePath:    "schema.yaml",
					ChangeType:  "schema_ref_target",
					Description: fmt.Sprintf("update field '%s.%s' target from '%s' to '%s'", typeName, fieldName, oldName, newName),
				})
			}
		}
	}

	// 3. File changes - walk all markdown files
	err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Error != nil {
			return nil // Skip errors
		}

		content, err := os.ReadFile(result.Path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(content), "\n")

		// Check frontmatter for type: old_name
		for _, obj := range result.Document.Objects {
			if obj.ObjectType == oldName && !strings.Contains(obj.ID, "#") {
				// This is a file-level object with the old type
				changes = append(changes, TypeRenameChange{
					FilePath:    result.RelativePath,
					ChangeType:  "frontmatter",
					Description: fmt.Sprintf("change type: %s → type: %s", oldName, newName),
					Line:        1, // Frontmatter is at the start
				})
			}
		}

		// Check for embedded type declarations ::old_name(...)
		embeddedPattern := fmt.Sprintf("::%s(", oldName)
		for lineNum, line := range lines {
			if strings.Contains(line, embeddedPattern) {
				changes = append(changes, TypeRenameChange{
					FilePath:    result.RelativePath,
					ChangeType:  "embedded",
					Description: fmt.Sprintf("change ::%s(...) → ::%s(...)", oldName, newName),
					Line:        lineNum + 1,
				})
			}
		}

		return nil
	})

	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	elapsed := time.Since(start).Milliseconds()

	// If not confirming, show preview
	if !schemaRenameConfirm {
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"preview":       true,
				"old_name":      oldName,
				"new_name":      newName,
				"total_changes": len(changes),
				"changes":       changes,
				"hint":          "Run with --confirm to apply changes",
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		fmt.Printf("Preview: Rename type '%s' to '%s'\n\n", oldName, newName)
		fmt.Printf("Changes to be made (%d total):\n", len(changes))

		// Group by file
		byFile := make(map[string][]TypeRenameChange)
		for _, c := range changes {
			byFile[c.FilePath] = append(byFile[c.FilePath], c)
		}

		for file, fileChanges := range byFile {
			fmt.Printf("\n  %s:\n", file)
			for _, c := range fileChanges {
				if c.Line > 0 {
					fmt.Printf("    Line %d: %s\n", c.Line, c.Description)
				} else {
					fmt.Printf("    %s\n", c.Description)
				}
			}
		}

		fmt.Printf("\nRun with --confirm to apply these changes.\n")
		return nil
	}

	// Apply changes
	appliedChanges := 0

	// 1. Update schema.yaml
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	types, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrSchemaInvalid, "types section not found", "")
	}

	// Rename the type
	if typeDef, exists := types[oldName]; exists {
		types[newName] = typeDef
		delete(types, oldName)
		appliedChanges++
	}

	// Update ref field targets
	for _, typeDef := range types {
		typeMap, ok := typeDef.(map[string]interface{})
		if !ok {
			continue
		}
		fields, ok := typeMap["fields"].(map[string]interface{})
		if !ok {
			continue
		}
		for _, fieldDef := range fields {
			fieldMap, ok := fieldDef.(map[string]interface{})
			if !ok {
				continue
			}
			if target, ok := fieldMap["target"].(string); ok && target == oldName {
				fieldMap["target"] = newName
				appliedChanges++
			}
		}
	}

	// Write schema back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}
	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	// 2. Update markdown files
	err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Error != nil {
			return nil
		}

		content, err := os.ReadFile(result.Path)
		if err != nil {
			return nil
		}

		originalContent := string(content)
		newContent := originalContent
		modified := false

		// Update frontmatter type: old_name → type: new_name
		// Pattern matches "type: old_name" at start of line in frontmatter
		frontmatterPattern := regexp.MustCompile(`(?m)^type:\s*` + regexp.QuoteMeta(oldName) + `\s*$`)
		if frontmatterPattern.MatchString(newContent) {
			newContent = frontmatterPattern.ReplaceAllString(newContent, "type: "+newName)
			modified = true
			appliedChanges++
		}

		// Update embedded ::old_name(...) → ::new_name(...)
		embeddedPattern := regexp.MustCompile(`::` + regexp.QuoteMeta(oldName) + `\(`)
		if embeddedPattern.MatchString(newContent) {
			newContent = embeddedPattern.ReplaceAllString(newContent, "::"+newName+"(")
			modified = true
			appliedChanges++
		}

		if modified {
			if err := os.WriteFile(result.Path, []byte(newContent), 0644); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	elapsed = time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"renamed":         true,
			"old_name":        oldName,
			"new_name":        newName,
			"changes_applied": appliedChanges,
			"hint":            "Run 'rvn reindex --full' to update the index",
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Renamed type '%s' to '%s'\n", oldName, newName)
	fmt.Printf("  Applied %d changes\n", appliedChanges)
	fmt.Printf("\nRun 'rvn reindex --full' to update the index.\n")
	return nil
}

func init() {
	// Add flags to schema add command
	schemaAddCmd.Flags().StringVar(&schemaAddDefaultPath, "default-path", "", "Default path for new type files")
	schemaAddCmd.Flags().StringVar(&schemaAddNameField, "name-field", "", "Field to use as display name (auto-created if doesn't exist)")
	schemaAddCmd.Flags().StringVar(&schemaAddFieldType, "type", "", "Field/trait type (string, date, enum, ref, bool)")
	schemaAddCmd.Flags().BoolVar(&schemaAddRequired, "required", false, "Mark field as required")
	schemaAddCmd.Flags().StringVar(&schemaAddDefault, "default", "", "Default value")
	schemaAddCmd.Flags().StringVar(&schemaAddValues, "values", "", "Enum values (comma-separated)")
	schemaAddCmd.Flags().StringVar(&schemaAddTarget, "target", "", "Target type for ref fields")

	// Add flags to schema update command
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateDefaultPath, "default-path", "", "Update default path for type")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateNameField, "name-field", "", "Set/update display name field (use '-' to remove)")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateFieldType, "type", "", "Update field/trait type")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateRequired, "required", "", "Update required status (true/false)")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateDefault, "default", "", "Update default value")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateValues, "values", "", "Update enum values (comma-separated)")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateTarget, "target", "", "Update target type for ref fields")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateAddTrait, "add-trait", "", "Add a trait to the type")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateRemoveTrait, "remove-trait", "", "Remove a trait from the type")

	// Add flags to schema remove command
	schemaRemoveCmd.Flags().BoolVar(&schemaRemoveForce, "force", false, "Skip confirmation prompts")

	// Add flags to schema rename command
	schemaRenameCmd.Flags().BoolVar(&schemaRenameConfirm, "confirm", false, "Apply the rename (default: preview only)")

	// Add subcommands to schema command
	schemaCmd.AddCommand(schemaAddCmd)
	schemaCmd.AddCommand(schemaUpdateCmd)
	schemaCmd.AddCommand(schemaRemoveCmd)
	schemaCmd.AddCommand(schemaRenameCmd)
	schemaCmd.AddCommand(schemaValidateCmd)
}
