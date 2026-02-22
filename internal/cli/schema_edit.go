package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

// Flags for schema add commands
var (
	schemaAddDefaultPath string
	schemaAddNameField   string
	schemaAddDescription string
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
	schemaUpdateDescription string
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
	schemaRenameConfirm           bool
	schemaRenameDefaultPathRename bool
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
  rvn schema add type event --default-path event/
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
	schemaPath := paths.SchemaPath(vaultPath)

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

	// Default to a path that matches the type name exactly unless explicitly overridden.
	defaultPath := schemaAddDefaultPath
	if strings.TrimSpace(defaultPath) == "" {
		defaultPath = paths.NormalizeDirRoot(typeName)
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
	newType["default_path"] = defaultPath
	if schemaAddDescription != "" {
		newType["description"] = schemaAddDescription
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

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		result := map[string]interface{}{
			"added":        "type",
			"name":         typeName,
			"default_path": defaultPath,
		}
		if schemaAddDescription != "" {
			result["description"] = schemaAddDescription
		}
		if nameField != "" {
			result["name_field"] = nameField
			result["auto_created_field"] = nameField
		}
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Added type '%s' to schema.yaml\n", typeName)
	fmt.Printf("  default_path: %s\n", defaultPath)
	if schemaAddDescription != "" {
		fmt.Printf("  description: %s\n", schemaAddDescription)
	}
	if nameField != "" {
		fmt.Printf("  name_field: %s (auto-created as required string)\n", nameField)
	}
	return nil
}

func addTrait(vaultPath, traitName string, start time.Time) error {
	schemaPath := paths.SchemaPath(vaultPath)

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

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
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
	"url":      true,
	"date":     true,
	"datetime": true,
	"bool":     true,
	"enum":     true,
	"ref":      true,
}

func normalizeFieldTypeAlias(baseType string) string {
	switch strings.ToLower(baseType) {
	case "boolean":
		return "bool"
	default:
		return strings.ToLower(baseType)
	}
}

// FieldTypeValidation holds the result of validating a field type specification
type FieldTypeValidation struct {
	Valid      bool
	BaseType   string
	IsArray    bool
	Error      string
	Suggestion string
	Examples   []string
	ValidTypes []string
	TargetHint string
}

// validateFieldTypeSpec validates the --type and related flags for adding a field
func validateFieldTypeSpec(fieldType, target, values string, sch *schema.Schema) FieldTypeValidation {
	result := FieldTypeValidation{
		ValidTypes: []string{"string", "number", "url", "date", "datetime", "bool", "enum", "ref"},
	}

	// Handle empty type (defaults to string)
	if fieldType == "" {
		fieldType = "string"
	}

	// Check for array suffix
	isArray := strings.HasSuffix(fieldType, "[]")
	baseType := normalizeFieldTypeAlias(strings.TrimSuffix(fieldType, "[]"))

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
		result.Suggestion = "Valid types: string, number, url, date, datetime, bool, enum, ref (add [] suffix for arrays)"
		result.Examples = []string{
			"--type string        (text)",
			"--type string[]      (array of text, e.g., tags)",
			"--type url           (web link)",
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
		result.Suggestion = "Either change --type to ref (or ref[]) or remove --target"
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
			if !schema.IsBuiltinType(target) {
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
	schemaPath := paths.SchemaPath(vaultPath)

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

	fieldType := validation.BaseType
	if fieldType == "" {
		fieldType = "string"
	}
	if validation.IsArray {
		fieldType += "[]"
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
	if schemaAddDescription != "" {
		newField["description"] = schemaAddDescription
	}

	fields[fieldName] = newField

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
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
		if schemaAddDescription != "" {
			result["description"] = schemaAddDescription
		}
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Added field '%s' to type '%s'\n", fieldName, typeName)
	fmt.Printf("  type: %s\n", fieldType)
	if schemaAddRequired {
		fmt.Println("  required: true")
	}
	if schemaAddDescription != "" {
		fmt.Printf("  description: %s\n", schemaAddDescription)
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
  rvn schema update type person --default-path person/
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
	schemaPath := paths.SchemaPath(vaultPath)

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
	if schemaUpdateDescription != "" {
		if schemaUpdateDescription == "-" || schemaUpdateDescription == "none" || schemaUpdateDescription == "\"\"" {
			delete(typeNode, "description")
			changes = append(changes, "removed description")
		} else {
			typeNode["description"] = schemaUpdateDescription
			changes = append(changes, fmt.Sprintf("description=%s", schemaUpdateDescription))
		}
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
		return handleErrorMsg(ErrInvalidInput, "no changes specified", "Use flags like --default-path, --description, --name-field, --add-trait, --remove-trait")
	}

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
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
	schemaPath := paths.SchemaPath(vaultPath)

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

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
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
	schemaPath := paths.SchemaPath(vaultPath)

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
					fields := obj.Fields
					if fields == nil {
						fields = map[string]interface{}{}
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
	if schemaUpdateDescription != "" {
		if schemaUpdateDescription == "-" || schemaUpdateDescription == "none" || schemaUpdateDescription == "\"\"" {
			delete(fieldNode, "description")
			changes = append(changes, "removed description")
		} else {
			fieldNode["description"] = schemaUpdateDescription
			changes = append(changes, fmt.Sprintf("description=%s", schemaUpdateDescription))
		}
	}

	if len(changes) == 0 {
		return handleErrorMsg(ErrInvalidInput, "no changes specified", "Use flags like --type, --required, --default, --description")
	}

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
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
	schemaPath := paths.SchemaPath(vaultPath)

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

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
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
	schemaPath := paths.SchemaPath(vaultPath)

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

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
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
	schemaPath := paths.SchemaPath(vaultPath)

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

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
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
	Use:   "rename <type|field> ...",
	Short: "Rename a type or field and update references",
	Long: `Rename a type or a field in the schema and update downstream usages.

Subcommands:
  type  <old_name> <new_name>
  field <type> <old_field> <new_field>

Rename type updates:
1. Type definition key in schema.yaml
2. All 'type:' frontmatter fields
3. All ::type() embedded declarations
4. All ref field targets pointing to the old type

Rename field updates:
1. Field key in schema.yaml for the target type
2. If name_field == old_field, updates it to new_field
3. Type templates referencing {{field.old_field}} (template files)
4. Object frontmatter keys for files with type:<type>
5. Field keys inside ::type(...) declarations (only for that type)
6. Saved queries in raven.yaml (best-effort for object:<type> queries)

By default, previews changes. Use --confirm to apply.
When type default_path clearly matches the type name, you can also rename
that directory with --rename-default-path.

Examples:
  rvn schema rename type event meeting
  rvn schema rename type event meeting --confirm

  rvn schema rename field person email email_address
  rvn schema rename field person email email_address --confirm`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		kind := args[0]

		switch kind {
		case "type":
			if len(args) != 3 {
				return handleErrorMsg(ErrMissingArgument, "type rename requires old and new names", "Usage: rvn schema rename type <old_name> <new_name>")
			}
			return renameType(vaultPath, args[1], args[2], start)
		case "field":
			if len(args) != 4 {
				return handleErrorMsg(ErrMissingArgument, "field rename requires type, old field, and new field", "Usage: rvn schema rename field <type> <old_field> <new_field>")
			}
			return renameField(vaultPath, args[1], args[2], args[3], start)
		default:
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema kind: %s", kind), "Currently supported: type, field")
		}
	},
}

// TypeRenameChange represents a single change to be made
type TypeRenameChange struct {
	FilePath    string `json:"file_path"`
	ChangeType  string `json:"change_type"` // "frontmatter", "embedded", "schema_ref_target", "schema_default_path", "directory_move"
	Description string `json:"description"`
	Line        int    `json:"line,omitempty"`
}

type typeDirectoryMove struct {
	SourceRelPath      string `json:"source_rel_path"`
	DestinationRelPath string `json:"destination_rel_path"`
	SourceID           string `json:"source_id"`
	DestinationID      string `json:"destination_id"`
}

type typeDefaultPathRenamePlan struct {
	OldDefaultPath string              `json:"old_default_path"`
	NewDefaultPath string              `json:"new_default_path"`
	Moves          []typeDirectoryMove `json:"moves,omitempty"`
}

// FieldRenameChange represents a single change to be made when renaming a field.
type FieldRenameChange struct {
	FilePath    string `json:"file_path"`
	ChangeType  string `json:"change_type"` // "schema_field", "schema_name_field", "template_inline", "template_file", "frontmatter", "embedded", "saved_query"
	Description string `json:"description"`
	Line        int    `json:"line,omitempty"`
}

// FieldRenameConflict represents a conflict that blocks renaming a field.
// Conflicts are hard errors (even with --confirm).
type FieldRenameConflict struct {
	FilePath      string `json:"file_path"`
	ConflictType  string `json:"conflict_type"` // "schema", "frontmatter", "embedded"
	Message       string `json:"message"`
	Line          int    `json:"line,omitempty"`
	OldFieldFound bool   `json:"old_field_found,omitempty"`
	NewFieldFound bool   `json:"new_field_found,omitempty"`
}

type fieldRenamePlan struct {
	Changes []FieldRenameChange

	SchemaYAML []byte

	TemplateFiles map[string][]byte // absolute path -> new content
	// For human preview grouping.
	TemplateFileRelPaths map[string]string // absolute path -> relative path

	RavenYAML []byte

	MarkdownFiles map[string][]byte // absolute path -> new content
	// For human preview grouping.
	MarkdownFileRelPaths map[string]string // absolute path -> relative path

	Conflicts []FieldRenameConflict
}

func renameField(vaultPath, typeName, oldField, newField string, start time.Time) error {
	if typeName == "" || oldField == "" || newField == "" {
		return handleErrorMsg(ErrInvalidInput, "type and field names cannot be empty", "Usage: rvn schema rename field <type> <old_field> <new_field>")
	}
	if oldField == newField {
		return handleErrorMsg(ErrInvalidInput, "old and new field names are the same", "")
	}

	// Check built-in types
	if schema.IsBuiltinType(typeName) {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("cannot rename fields on built-in type '%s'", typeName), "")
	}

	// Load existing schema for validation
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	typeDef, exists := sch.Types[typeName]
	if !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "")
	}
	if typeDef == nil || typeDef.Fields == nil {
		return handleErrorMsg(ErrFieldNotFound, fmt.Sprintf("type '%s' has no fields", typeName), "")
	}
	if _, ok := typeDef.Fields[oldField]; !ok {
		return handleErrorMsg(ErrFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", oldField, typeName), "")
	}
	if _, ok := typeDef.Fields[newField]; ok {
		return handleErrorMsg(ErrObjectExists, fmt.Sprintf("field '%s' already exists on type '%s'", newField, typeName), "")
	}

	plan, err := buildFieldRenamePlan(vaultPath, typeName, oldField, newField)
	if err != nil {
		return err
	}

	// Hard error on conflicts (even in preview)
	if len(plan.Conflicts) > 0 {
		return handleErrorWithDetails(
			ErrDataIntegrityBlock,
			fmt.Sprintf("field rename blocked by %d conflicts", len(plan.Conflicts)),
			"Resolve conflicts (remove one of the duplicate keys) and retry",
			map[string]interface{}{
				"type":       typeName,
				"old_field":  oldField,
				"new_field":  newField,
				"conflicts":  plan.Conflicts,
				"hint":       "Conflicts occur when both old and new field keys are present in the same object/declaration.",
				"next_steps": "Fix conflicts, then re-run the command (preview first).",
			},
		)
	}

	elapsed := time.Since(start).Milliseconds()

	// Preview
	if !schemaRenameConfirm {
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"preview":       true,
				"type":          typeName,
				"old_field":     oldField,
				"new_field":     newField,
				"total_changes": len(plan.Changes),
				"changes":       plan.Changes,
				"hint":          "Run with --confirm to apply changes",
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		fmt.Printf("Preview: Rename field '%s.%s' to '%s.%s'\n\n", typeName, oldField, typeName, newField)
		fmt.Printf("Changes to be made (%d total):\n", len(plan.Changes))

		byFile := make(map[string][]FieldRenameChange)
		for _, c := range plan.Changes {
			byFile[c.FilePath] = append(byFile[c.FilePath], c)
		}

		// Stable order for output
		var files []string
		for f := range byFile {
			files = append(files, f)
		}
		sort.Strings(files)

		for _, file := range files {
			fileChanges := byFile[file]
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

	// Apply (deterministic order, atomic writes)
	appliedChanges := 0

	// 1) schema.yaml
	if len(plan.SchemaYAML) > 0 {
		schemaPath := paths.SchemaPath(vaultPath)
		if err := atomicfile.WriteFile(schemaPath, plan.SchemaYAML, 0o644); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}
		appliedChanges++
	}

	// 2) Template files
	if len(plan.TemplateFiles) > 0 {
		var pathsSorted []string
		for p := range plan.TemplateFiles {
			pathsSorted = append(pathsSorted, p)
		}
		sort.Strings(pathsSorted)
		for _, p := range pathsSorted {
			if err := atomicfile.WriteFile(p, plan.TemplateFiles[p], 0o644); err != nil {
				return handleError(ErrFileWriteError, err, "")
			}
			appliedChanges++
		}
	}

	// 3) raven.yaml
	if len(plan.RavenYAML) > 0 {
		cfgPath := filepath.Join(vaultPath, "raven.yaml")
		if err := atomicfile.WriteFile(cfgPath, plan.RavenYAML, 0o644); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}
		appliedChanges++
	}

	// 4) Markdown files
	if len(plan.MarkdownFiles) > 0 {
		var pathsSorted []string
		for p := range plan.MarkdownFiles {
			pathsSorted = append(pathsSorted, p)
		}
		sort.Strings(pathsSorted)
		for _, p := range pathsSorted {
			if err := atomicfile.WriteFile(p, plan.MarkdownFiles[p], 0o644); err != nil {
				return handleError(ErrFileWriteError, err, "")
			}
			appliedChanges++
		}
	}

	elapsed = time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"renamed":         true,
			"type":            typeName,
			"old_field":       oldField,
			"new_field":       newField,
			"changes_applied": appliedChanges,
			"hint":            "Run 'rvn reindex --full' to update the index",
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Renamed field '%s.%s' to '%s.%s'\n", typeName, oldField, typeName, newField)
	fmt.Printf("  Applied %d changes\n", appliedChanges)
	fmt.Printf("\nRun 'rvn reindex --full' to update the index.\n")
	return nil
}

func buildFieldRenamePlan(vaultPath, typeName, oldField, newField string) (*fieldRenamePlan, error) {
	tokenOld := "{{field." + oldField + "}}"
	tokenNew := "{{field." + newField + "}}"

	plan := &fieldRenamePlan{
		TemplateFiles:        make(map[string][]byte),
		TemplateFileRelPaths: make(map[string]string),
		MarkdownFiles:        make(map[string][]byte),
		MarkdownFileRelPaths: make(map[string]string),
		Changes:              []FieldRenameChange{},
		Conflicts:            []FieldRenameConflict{},
	}

	// -------------------------------------------------------------------------
	// 1) schema.yaml updates (field key, name_field, template references)
	// -------------------------------------------------------------------------
	schemaPath := paths.SchemaPath(vaultPath)
	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(schemaData, &schemaDoc); err != nil {
		return nil, handleError(ErrSchemaInvalid, err, "")
	}

	types, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		return nil, handleErrorMsg(ErrSchemaInvalid, "types section not found", "")
	}
	typeNodeAny, ok := types[typeName]
	if !ok {
		return nil, handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "")
	}
	typeNode, ok := typeNodeAny.(map[string]interface{})
	if !ok {
		return nil, handleErrorMsg(ErrSchemaInvalid, fmt.Sprintf("type '%s' has invalid definition", typeName), "")
	}
	fieldsAny, ok := typeNode["fields"]
	if !ok {
		return nil, handleErrorMsg(ErrFieldNotFound, fmt.Sprintf("type '%s' has no fields", typeName), "")
	}
	fields, ok := fieldsAny.(map[string]interface{})
	if !ok {
		return nil, handleErrorMsg(ErrSchemaInvalid, fmt.Sprintf("type '%s' fields are invalid", typeName), "")
	}

	// Schema-level conflicts
	_, hasOld := fields[oldField]
	_, hasNew := fields[newField]
	if hasOld && hasNew {
		return nil, handleErrorMsg(ErrObjectExists, fmt.Sprintf("type '%s' already has both '%s' and '%s' fields", typeName, oldField, newField), "Choose a different new field name or remove one field first")
	}
	if hasNew {
		return nil, handleErrorMsg(ErrObjectExists, fmt.Sprintf("field '%s' already exists on type '%s'", newField, typeName), "Choose a different new field name")
	}
	if !hasOld {
		return nil, handleErrorMsg(ErrFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", oldField, typeName), "")
	}

	// Rename schema field key
	fields[newField] = fields[oldField]
	delete(fields, oldField)
	plan.Changes = append(plan.Changes, FieldRenameChange{
		FilePath:    "schema.yaml",
		ChangeType:  "schema_field",
		Description: fmt.Sprintf("rename field '%s' → '%s' on type '%s'", oldField, newField, typeName),
	})

	// Update name_field if needed
	if nf, ok := typeNode["name_field"].(string); ok && nf == oldField {
		typeNode["name_field"] = newField
		plan.Changes = append(plan.Changes, FieldRenameChange{
			FilePath:    "schema.yaml",
			ChangeType:  "schema_name_field",
			Description: fmt.Sprintf("update name_field: %s → %s", oldField, newField),
		})
	}

	// Update template file references
	if tmplSpec, ok := typeNode["template"].(string); ok && tmplSpec != "" {
		if looksLikeTemplatePath(tmplSpec) {
			absTmpl := filepath.Join(vaultPath, tmplSpec)
			if err := paths.ValidateWithinVault(vaultPath, absTmpl); err != nil {
				if errors.Is(err, paths.ErrPathOutsideVault) {
					// Silent no-op for security
				} else {
					return nil, handleError(ErrFileOutsideVault, err, "")
				}
			} else {
				tmplContent, err := os.ReadFile(absTmpl)
				if err == nil {
					newContent := strings.ReplaceAll(string(tmplContent), tokenOld, tokenNew)
					if newContent != string(tmplContent) {
						plan.TemplateFiles[absTmpl] = []byte(newContent)
						rel, _ := filepath.Rel(vaultPath, absTmpl)
						plan.TemplateFileRelPaths[absTmpl] = rel
						plan.Changes = append(plan.Changes, FieldRenameChange{
							FilePath:    rel,
							ChangeType:  "template_file",
							Description: fmt.Sprintf("update template variable %s → %s", tokenOld, tokenNew),
						})
					}
				}
			}
		}
	}

	schemaOut, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return nil, handleError(ErrInternal, err, "")
	}
	plan.SchemaYAML = schemaOut

	// -------------------------------------------------------------------------
	// 2) raven.yaml saved query rewrites (best-effort)
	// -------------------------------------------------------------------------
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return nil, handleError(ErrFileReadError, err, "")
	}

	changedQueries := false
	fieldRefPattern := regexp.MustCompile(`\.` + regexp.QuoteMeta(oldField) + `\b`)

	if vaultCfg != nil && vaultCfg.Queries != nil {
		// Stable order for deterministic output/change list
		var qNames []string
		for name := range vaultCfg.Queries {
			qNames = append(qNames, name)
		}
		sort.Strings(qNames)
		for _, name := range qNames {
			q := vaultCfg.Queries[name]
			if q == nil || q.Query == "" {
				continue
			}
			parsed, err := query.Parse(q.Query)
			if err != nil || parsed == nil {
				continue // best-effort: skip unparseable queries
			}
			if parsed.Type != query.QueryTypeObject || parsed.TypeName != typeName {
				continue
			}
			newQuery := fieldRefPattern.ReplaceAllString(q.Query, "."+newField)
			if newQuery != q.Query {
				q.Query = newQuery
				changedQueries = true
				plan.Changes = append(plan.Changes, FieldRenameChange{
					FilePath:    "raven.yaml",
					ChangeType:  "saved_query",
					Description: fmt.Sprintf("update saved query '%s': .%s → .%s", name, oldField, newField),
				})
			}
		}
	}

	if changedQueries {
		cfgOut, err := yaml.Marshal(vaultCfg)
		if err != nil {
			return nil, handleError(ErrInternal, err, "")
		}
		plan.RavenYAML = cfgOut
	}

	// -------------------------------------------------------------------------
	// 3) Markdown frontmatter + embedded ::type(...) rewrites
	// -------------------------------------------------------------------------
	err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Error != nil || result.Document == nil {
			return nil //nolint:nilerr
		}

		relPath := result.RelativePath
		original := result.Document.RawContent
		lines := strings.Split(original, "\n")

		// Determine frontmatter needs/conflicts.
		needsFrontmatterRename := false
		var frontmatterYAML map[string]interface{}
		startLine, endLine, fmOK := parser.FrontmatterBounds(lines)
		if fmOK && endLine != -1 {
			fmContent := strings.Join(lines[startLine+1:endLine], "\n")
			if err := yaml.Unmarshal([]byte(fmContent), &frontmatterYAML); err == nil {
				if frontmatterYAML == nil {
					frontmatterYAML = map[string]interface{}{}
				}
				if t, ok := frontmatterYAML["type"].(string); ok && t == typeName {
					_, oldPresent := frontmatterYAML[oldField]
					_, newPresent := frontmatterYAML[newField]
					if oldPresent && newPresent {
						plan.Conflicts = append(plan.Conflicts, FieldRenameConflict{
							FilePath:      relPath,
							ConflictType:  "frontmatter",
							Message:       fmt.Sprintf("frontmatter contains both '%s' and '%s'", oldField, newField),
							Line:          1,
							OldFieldFound: true,
							NewFieldFound: true,
						})
						// Don't attempt any edits for this file.
						return nil
					}
					if oldPresent {
						needsFrontmatterRename = true
					}
				}
			}
		}

		// Determine embedded needs/conflicts.
		typeDeclsToEdit := make([]*parser.EmbeddedTypeInfo, 0)
		contentStartLine := 1
		bodyContent := original
		if fmOK && endLine != -1 {
			// parser.ParseFrontmatter uses EndLine 1-indexed; we can derive from bounds here.
			// endLine is 0-indexed line index of closing '---', so body starts at endLine+1 (0-index),
			// which is (endLine+1)+1 in 1-indexed line numbers.
			contentStartLine = (endLine + 1) + 1
			if endLine+1 < len(lines) {
				bodyContent = strings.Join(lines[endLine+1:], "\n")
			} else {
				bodyContent = ""
			}
		}

		astContent, err := parser.ExtractFromAST([]byte(bodyContent), contentStartLine)
		if err == nil && astContent != nil {
			for _, decl := range astContent.TypeDecls {
				if decl == nil || decl.TypeName != typeName {
					continue
				}
				_, oldPresent := decl.Fields[oldField]
				_, newPresent := decl.Fields[newField]
				if oldPresent && newPresent {
					plan.Conflicts = append(plan.Conflicts, FieldRenameConflict{
						FilePath:      relPath,
						ConflictType:  "embedded",
						Message:       fmt.Sprintf("embedded ::%s(...) contains both '%s' and '%s'", typeName, oldField, newField),
						Line:          decl.Line,
						OldFieldFound: true,
						NewFieldFound: true,
					})
					return nil
				}
				if oldPresent {
					typeDeclsToEdit = append(typeDeclsToEdit, decl)
				}
			}
		}

		// If no changes needed, skip.
		if !needsFrontmatterRename && len(typeDeclsToEdit) == 0 {
			return nil
		}

		// Apply embedded line edits first (on the original lines slice).
		modified := false
		if len(typeDeclsToEdit) > 0 {
			// Stable order by line number
			sort.Slice(typeDeclsToEdit, func(i, j int) bool {
				return typeDeclsToEdit[i].Line < typeDeclsToEdit[j].Line
			})
			for _, decl := range typeDeclsToEdit {
				if decl.Line <= 0 || decl.Line-1 >= len(lines) {
					return nil //nolint:nilerr
				}
				declLine := lines[decl.Line-1]

				// Preserve leading whitespace
				leadingSpace := ""
				for _, c := range declLine {
					if c == ' ' || c == '\t' {
						leadingSpace += string(c)
					} else {
						break
					}
				}

				newFields := make(map[string]schema.FieldValue, len(decl.Fields))
				for k, v := range decl.Fields {
					newFields[k] = v
				}
				newFields[newField] = newFields[oldField]
				delete(newFields, oldField)

				newDecl := leadingSpace + parser.SerializeTypeDeclaration(typeName, newFields)
				if newDecl != declLine {
					lines[decl.Line-1] = newDecl
					modified = true
					plan.Changes = append(plan.Changes, FieldRenameChange{
						FilePath:    relPath,
						ChangeType:  "embedded",
						Description: fmt.Sprintf("rename field '%s' → '%s' inside ::%s(...)", oldField, newField, typeName),
						Line:        decl.Line,
					})
				}
			}
		}

		updatedLines := lines

		// Apply frontmatter rename by reconstructing the file (preserves updated body lines).
		if needsFrontmatterRename && fmOK && endLine != -1 {
			// Re-read frontmatter from the original (not from updated lines) to avoid accidental drift.
			// This ensures we only rename the key and preserve all other frontmatter keys/values.
			fmContent := strings.Join(strings.Split(original, "\n")[startLine+1:endLine], "\n")
			var fmMap map[string]interface{}
			if err := yaml.Unmarshal([]byte(fmContent), &fmMap); err == nil {
				if fmMap == nil {
					fmMap = map[string]interface{}{}
				}
				if _, ok := fmMap[oldField]; ok {
					fmMap[newField] = fmMap[oldField]
					delete(fmMap, oldField)

					newFM, err := yaml.Marshal(fmMap)
					if err == nil {
						var b strings.Builder
						b.WriteString("---\n")
						b.Write(newFM)
						b.WriteString("---")

						if endLine+1 < len(updatedLines) {
							b.WriteString("\n")
							b.WriteString(strings.Join(updatedLines[endLine+1:], "\n"))
						}

						updated := b.String()
						plan.MarkdownFiles[result.Path] = []byte(updated)
						plan.MarkdownFileRelPaths[result.Path] = relPath
						plan.Changes = append(plan.Changes, FieldRenameChange{
							FilePath:    relPath,
							ChangeType:  "frontmatter",
							Description: fmt.Sprintf("rename frontmatter key '%s:' → '%s:' for type '%s'", oldField, newField, typeName),
							Line:        1,
						})
						return nil
					}
				}
			}
		}

		// If we only had embedded changes (or frontmatter rewrite failed), write joined lines.
		if modified {
			updated := strings.Join(updatedLines, "\n")
			plan.MarkdownFiles[result.Path] = []byte(updated)
			plan.MarkdownFileRelPaths[result.Path] = relPath
		}

		return nil
	})
	if err != nil {
		return nil, handleError(ErrInternal, err, "")
	}

	return plan, nil
}

// looksLikeTemplatePath checks if a template spec looks like a file path.
func looksLikeTemplatePath(s string) bool {
	if s == "" {
		return false
	}
	// If it contains a slash, it's a path
	if strings.Contains(s, "/") {
		return true
	}
	// If it ends with .md, it's a path
	if strings.HasSuffix(s, ".md") {
		return true
	}
	// If it starts with "templates" or similar directory patterns
	if strings.HasPrefix(s, "templates") {
		return true
	}
	// Multi-line values are not valid template file paths.
	if strings.Contains(s, "\n") {
		return false
	}
	// Single line without slashes - could be a simple filename
	matched, _ := regexp.MatchString(`^[\w.-]+$`, s)
	return matched && len(s) < 100
}

func renameType(vaultPath, oldName, newName string, start time.Time) error {
	schemaPath := paths.SchemaPath(vaultPath)

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

	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
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
	var optionalChanges []TypeRenameChange

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

	// Optional: if default_path appears to be derived from the old type name,
	// offer renaming the directory as part of the operation.
	var defaultPathPlan *typeDefaultPathRenamePlan
	if oldTypeDef := sch.Types[oldName]; oldTypeDef != nil {
		if suggestedPath, ok := suggestRenamedDefaultPath(oldTypeDef.DefaultPath, oldName, newName); ok {
			defaultPathPlan = &typeDefaultPathRenamePlan{
				OldDefaultPath: paths.NormalizeDirRoot(oldTypeDef.DefaultPath),
				NewDefaultPath: suggestedPath,
			}
			optionalChanges = append(optionalChanges, TypeRenameChange{
				FilePath:    "schema.yaml",
				ChangeType:  "schema_default_path",
				Description: fmt.Sprintf("update default_path '%s' → '%s' for type '%s'", defaultPathPlan.OldDefaultPath, defaultPathPlan.NewDefaultPath, newName),
			})
		}
	}

	movesBySource := make(map[string]typeDirectoryMove)

	// 3. File changes - walk all markdown files
	err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Error != nil {
			return nil //nolint:nilerr // skip errors
		}

		content, err := os.ReadFile(result.Path)
		if err != nil {
			return nil //nolint:nilerr
		}
		lines := strings.Split(string(content), "\n")

		// Check frontmatter for type: old_name
		hasFileLevelOldType := false
		for _, obj := range result.Document.Objects {
			if obj.ObjectType == oldName && !strings.Contains(obj.ID, "#") {
				// This is a file-level object with the old type
				hasFileLevelOldType = true
				changes = append(changes, TypeRenameChange{
					FilePath:    result.RelativePath,
					ChangeType:  "frontmatter",
					Description: fmt.Sprintf("change type: %s → type: %s", oldName, newName),
					Line:        1, // Frontmatter is at the start
				})
			}
		}
		if hasFileLevelOldType && defaultPathPlan != nil {
			if move, ok := planTypeDirectoryMove(result.RelativePath, newName, defaultPathPlan, vaultCfg); ok {
				if _, exists := movesBySource[move.SourceRelPath]; !exists {
					movesBySource[move.SourceRelPath] = move
					defaultPathPlan.Moves = append(defaultPathPlan.Moves, move)
					optionalChanges = append(optionalChanges, TypeRenameChange{
						FilePath:    move.SourceRelPath,
						ChangeType:  "directory_move",
						Description: fmt.Sprintf("move file '%s' → '%s'", move.SourceRelPath, move.DestinationRelPath),
					})
				}
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
		hint := "Run with --confirm to apply changes"
		if defaultPathPlan != nil {
			hint = "Run with --confirm to apply changes. Add --rename-default-path to also rename the default directory and move matching files."
		}

		if isJSONOutput() {
			result := map[string]interface{}{
				"preview":       true,
				"old_name":      oldName,
				"new_name":      newName,
				"total_changes": len(changes),
				"changes":       changes,
				"hint":          hint,
			}
			if defaultPathPlan != nil {
				result["default_path_rename_available"] = true
				result["default_path_old"] = defaultPathPlan.OldDefaultPath
				result["default_path_new"] = defaultPathPlan.NewDefaultPath
				result["optional_total_changes"] = len(optionalChanges)
				result["optional_changes"] = optionalChanges
				result["files_to_move"] = len(defaultPathPlan.Moves)
			}
			outputSuccess(result, &Meta{QueryTimeMs: elapsed})
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

		if defaultPathPlan != nil {
			fmt.Printf("\nOptional default directory rename (%d changes):\n", len(optionalChanges))
			fmt.Printf("  default_path: %s → %s\n", defaultPathPlan.OldDefaultPath, defaultPathPlan.NewDefaultPath)
			if len(defaultPathPlan.Moves) > 0 {
				fmt.Printf("  files to move: %d\n", len(defaultPathPlan.Moves))
			}
			fmt.Printf("  (add --rename-default-path to apply these optional changes)\n")
		}

		fmt.Printf("\nRun with --confirm to apply these changes.\n")
		return nil
	}

	applyDefaultPathRename := false
	if defaultPathPlan != nil {
		applyDefaultPathRename = schemaRenameDefaultPathRename
		if !applyDefaultPathRename && shouldPromptForConfirm() {
			prompt := fmt.Sprintf("Also rename default_path '%s' -> '%s'?", defaultPathPlan.OldDefaultPath, defaultPathPlan.NewDefaultPath)
			if len(defaultPathPlan.Moves) > 0 {
				prompt = fmt.Sprintf("Also rename default_path '%s' -> '%s' and move %d files with reference updates?",
					defaultPathPlan.OldDefaultPath, defaultPathPlan.NewDefaultPath, len(defaultPathPlan.Moves))
			}
			applyDefaultPathRename = promptForConfirm(prompt)
		}
	}
	if applyDefaultPathRename {
		if err := validateTypeDirectoryMoves(vaultPath, defaultPathPlan.Moves); err != nil {
			return handleErrorMsg(
				ErrValidationFailed,
				fmt.Sprintf("cannot rename default directory: %v", err),
				"Use --confirm without --rename-default-path, or resolve destination conflicts and try again",
			)
		}
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

	if applyDefaultPathRename {
		if typeDefAny, exists := types[newName]; exists {
			if typeDefMap, ok := typeDefAny.(map[string]interface{}); ok {
				typeDefMap["default_path"] = defaultPathPlan.NewDefaultPath
				appliedChanges++
			}
		}
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
	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	// 2. Update markdown files
	err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Error != nil {
			return nil //nolint:nilerr
		}

		content, err := os.ReadFile(result.Path)
		if err != nil {
			return nil //nolint:nilerr
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
			if err := atomicfile.WriteFile(result.Path, []byte(newContent), 0o644); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	movedFiles := 0
	updatedReferenceFiles := 0
	if applyDefaultPathRename {
		movedFiles, updatedReferenceFiles, err = applyTypeDirectoryRename(vaultPath, vaultCfg, defaultPathPlan.Moves)
		if err != nil {
			return handleError(ErrFileWriteError, err, "Some files may have been renamed; review the vault and run 'rvn reindex --full'")
		}
		appliedChanges += movedFiles + updatedReferenceFiles
	}

	elapsed = time.Since(start).Milliseconds()

	if isJSONOutput() {
		result := map[string]interface{}{
			"renamed":         true,
			"old_name":        oldName,
			"new_name":        newName,
			"changes_applied": appliedChanges,
			"hint":            "Run 'rvn reindex --full' to update the index",
		}
		if defaultPathPlan != nil {
			result["default_path_rename_available"] = true
			result["default_path_renamed"] = applyDefaultPathRename
			result["default_path_old"] = defaultPathPlan.OldDefaultPath
			result["default_path_new"] = defaultPathPlan.NewDefaultPath
			result["files_moved"] = movedFiles
			result["reference_files_updated"] = updatedReferenceFiles
			if !applyDefaultPathRename {
				result["hint"] = "Run 'rvn reindex --full' to update the index. Use --rename-default-path to also rename the default directory."
			}
		}
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Renamed type '%s' to '%s'\n", oldName, newName)
	fmt.Printf("  Applied %d changes\n", appliedChanges)
	if applyDefaultPathRename {
		fmt.Printf("  Renamed default_path %s → %s\n", defaultPathPlan.OldDefaultPath, defaultPathPlan.NewDefaultPath)
		fmt.Printf("  Moved %d files and updated references in %d files\n", movedFiles, updatedReferenceFiles)
	} else if defaultPathPlan != nil {
		fmt.Printf("  Default path remains %s (use --rename-default-path to rename to %s)\n", defaultPathPlan.OldDefaultPath, defaultPathPlan.NewDefaultPath)
	}
	fmt.Printf("\nRun 'rvn reindex --full' to update the index.\n")
	return nil
}

func suggestRenamedDefaultPath(oldDefaultPath, oldName, newName string) (string, bool) {
	normalized := paths.NormalizeDirRoot(oldDefaultPath)
	if normalized == "" {
		return "", false
	}
	trimmed := strings.TrimSuffix(normalized, "/")
	if trimmed == "" {
		return "", false
	}

	lastSlash := strings.LastIndex(trimmed, "/")
	parent := ""
	base := trimmed
	if lastSlash >= 0 {
		parent = trimmed[:lastSlash]
		base = trimmed[lastSlash+1:]
	}

	newBase := ""
	switch base {
	case oldName:
		newBase = newName
	case oldName + "s":
		newBase = newName + "s"
	default:
		return "", false
	}

	next := newBase
	if parent != "" {
		next = parent + "/" + newBase
	}
	next = paths.NormalizeDirRoot(next)
	if next == normalized {
		return "", false
	}
	return next, true
}

func planTypeDirectoryMove(relPath, newName string, plan *typeDefaultPathRenamePlan, vaultCfg *config.VaultConfig) (typeDirectoryMove, bool) {
	if plan == nil || vaultCfg == nil {
		return typeDirectoryMove{}, false
	}

	sourceRel := filepath.ToSlash(strings.TrimPrefix(relPath, "./"))
	sourceID := vaultCfg.FilePathToObjectID(sourceRel)
	if !strings.HasPrefix(sourceID, plan.OldDefaultPath) {
		return typeDirectoryMove{}, false
	}
	suffix := strings.TrimPrefix(sourceID, plan.OldDefaultPath)
	if suffix == "" {
		return typeDirectoryMove{}, false
	}
	destID := plan.NewDefaultPath + suffix
	destRel := filepath.ToSlash(vaultCfg.ObjectIDToFilePath(destID, newName))
	if sourceRel == destRel {
		return typeDirectoryMove{}, false
	}
	return typeDirectoryMove{
		SourceRelPath:      sourceRel,
		DestinationRelPath: destRel,
		SourceID:           sourceID,
		DestinationID:      destID,
	}, true
}

func validateTypeDirectoryMoves(vaultPath string, moves []typeDirectoryMove) error {
	if len(moves) == 0 {
		return nil
	}

	destinations := make(map[string]string, len(moves))
	sources := make(map[string]struct{}, len(moves))
	for _, move := range moves {
		sourceAbs := filepath.Join(vaultPath, move.SourceRelPath)
		destAbs := filepath.Join(vaultPath, move.DestinationRelPath)
		sources[filepath.Clean(sourceAbs)] = struct{}{}

		if _, err := os.Stat(sourceAbs); err != nil {
			return fmt.Errorf("source file does not exist: %s", move.SourceRelPath)
		}

		if existingSource, exists := destinations[filepath.Clean(destAbs)]; exists && existingSource != move.SourceRelPath {
			return fmt.Errorf("multiple files would move to '%s'", move.DestinationRelPath)
		}
		destinations[filepath.Clean(destAbs)] = move.SourceRelPath
	}

	for _, move := range moves {
		destAbs := filepath.Clean(filepath.Join(vaultPath, move.DestinationRelPath))
		if _, isSource := sources[destAbs]; isSource {
			continue
		}
		if _, err := os.Stat(destAbs); err == nil {
			return fmt.Errorf("destination already exists: %s", move.DestinationRelPath)
		}
	}

	return nil
}

func applyTypeDirectoryRename(vaultPath string, vaultCfg *config.VaultConfig, moves []typeDirectoryMove) (int, int, error) {
	if len(moves) == 0 {
		return 0, 0, nil
	}

	orderedMoves := make([]typeDirectoryMove, len(moves))
	copy(orderedMoves, moves)
	sort.SliceStable(orderedMoves, func(i, j int) bool {
		return len(orderedMoves[i].SourceRelPath) > len(orderedMoves[j].SourceRelPath)
	})

	idMoves := make(map[string]string, len(orderedMoves))
	for _, move := range orderedMoves {
		sourceAbs := filepath.Join(vaultPath, move.SourceRelPath)
		destAbs := filepath.Join(vaultPath, move.DestinationRelPath)

		if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
			return 0, 0, err
		}
		if err := os.Rename(sourceAbs, destAbs); err != nil {
			return 0, 0, err
		}
		idMoves[move.SourceID] = move.DestinationID
	}

	updatedReferenceFiles, err := updateReferencesForTypeDirectoryMoves(vaultPath, vaultCfg, idMoves)
	if err != nil {
		return len(orderedMoves), updatedReferenceFiles, err
	}
	return len(orderedMoves), updatedReferenceFiles, nil
}

func updateReferencesForTypeDirectoryMoves(vaultPath string, vaultCfg *config.VaultConfig, idMoves map[string]string) (int, error) {
	if len(idMoves) == 0 {
		return 0, nil
	}

	objectRoot := ""
	pageRoot := ""
	if vaultCfg != nil {
		objectRoot = vaultCfg.GetObjectsRoot()
		pageRoot = vaultCfg.GetPagesRoot()
	}

	oldIDs := make([]string, 0, len(idMoves))
	for oldID := range idMoves {
		oldIDs = append(oldIDs, oldID)
	}
	sort.SliceStable(oldIDs, func(i, j int) bool {
		return len(oldIDs[i]) > len(oldIDs[j])
	})

	updatedFiles := 0
	err := vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Path == "" {
			return nil
		}

		content, err := os.ReadFile(result.Path)
		if err != nil {
			return nil //nolint:nilerr
		}

		original := string(content)
		updated := original
		for _, oldID := range oldIDs {
			updated = replaceAllRefVariants(updated, oldID, oldID, idMoves[oldID], objectRoot, pageRoot)
		}

		if updated == original {
			return nil
		}
		if err := atomicfile.WriteFile(result.Path, []byte(updated), 0o644); err != nil {
			return err
		}
		updatedFiles++
		return nil
	})
	if err != nil {
		return updatedFiles, err
	}

	return updatedFiles, nil
}

func init() {
	// Add flags to schema add command
	schemaAddCmd.Flags().StringVar(&schemaAddDefaultPath, "default-path", "", "Default path for new type files (default: <type>/)")
	schemaAddCmd.Flags().StringVar(&schemaAddNameField, "name-field", "", "Field to use as display name (auto-created if doesn't exist)")
	schemaAddCmd.Flags().StringVar(&schemaAddDescription, "description", "", "Optional description for the type or field")
	schemaAddCmd.Flags().StringVar(&schemaAddFieldType, "type", "", "Field/trait type (string, number, url, date, datetime, enum, ref, bool)")
	schemaAddCmd.Flags().BoolVar(&schemaAddRequired, "required", false, "Mark field as required")
	schemaAddCmd.Flags().StringVar(&schemaAddDefault, "default", "", "Default value")
	schemaAddCmd.Flags().StringVar(&schemaAddValues, "values", "", "Enum values (comma-separated)")
	schemaAddCmd.Flags().StringVar(&schemaAddTarget, "target", "", "Target type for ref fields")

	// Add flags to schema update command
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateDefaultPath, "default-path", "", "Update default path for type")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateNameField, "name-field", "", "Set/update display name field (use '-' to remove)")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateDescription, "description", "", "Set/update description (use '-' to remove)")
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
	schemaRenameCmd.Flags().BoolVar(&schemaRenameDefaultPathRename, "rename-default-path", false, "Also rename type default_path directory and move matching files (with reference updates)")

	// Add subcommands to schema command
	schemaCmd.AddCommand(schemaAddCmd)
	schemaCmd.AddCommand(schemaUpdateCmd)
	schemaCmd.AddCommand(schemaRemoveCmd)
	schemaCmd.AddCommand(schemaRenameCmd)
	schemaCmd.AddCommand(schemaValidateCmd)
}
