package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemasvc"
)

// Flags for schema add commands
var (
	schemaAddTypeDefaultPath string
	schemaAddTypeNameField   string
	schemaAddTypeDescription string

	schemaAddTraitFieldType string
	schemaAddTraitDefault   string
	schemaAddTraitValues    string

	schemaAddFieldTypeFlag    string
	schemaAddFieldRequired    bool
	schemaAddFieldDefault     string
	schemaAddFieldValues      string
	schemaAddFieldTarget      string
	schemaAddFieldDescription string
)

// Flags for schema update commands
var (
	schemaUpdateTypeDefaultPath string
	schemaUpdateTypeNameField   string
	schemaUpdateTypeDescription string
	schemaUpdateTypeAddTrait    string
	schemaUpdateTypeRemoveTrait string

	schemaUpdateTraitFieldType string
	schemaUpdateTraitDefault   string
	schemaUpdateTraitValues    string

	schemaUpdateFieldTypeFlag    string
	schemaUpdateFieldRequired    string // "true", "false", or "" (no change)
	schemaUpdateFieldDefault     string
	schemaUpdateFieldValues      string
	schemaUpdateFieldTarget      string
	schemaUpdateFieldDescription string
)

// Flags for schema remove commands
var (
	schemaRemoveTypeForce  bool
	schemaRemoveTraitForce bool
)

// Flags for schema rename commands
var (
	schemaRenameTypeConfirm           bool
	schemaRenameTypeDefaultPathRename bool
	schemaRenameFieldConfirm          bool
)

var schemaAddCmd = &cobra.Command{
	Use:   "add",
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
}

var schemaAddTypeCmd = &cobra.Command{
	Use:   "type <name>",
	Short: "Add a new type",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return addType(getVaultPath(), args[0], time.Now())
	},
}

var schemaAddTraitCmd = &cobra.Command{
	Use:   "trait <name>",
	Short: "Add a new trait",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return addTrait(getVaultPath(), args[0], time.Now())
	},
}

var schemaAddFieldCmd = &cobra.Command{
	Use:   "field <type> <field>",
	Short: "Add a field to an existing type",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return addField(getVaultPath(), args[0], args[1], time.Now())
	},
}

func addType(vaultPath, typeName string, start time.Time) error {
	// Interactive prompt for name_field if not provided and not in JSON mode
	nameField := schemaAddTypeNameField
	if nameField == "" && !isJSONOutput() {
		fmt.Print("Which field should be the display name? (common: name, title; leave blank for none): ")
		var input string
		fmt.Scanln(&input)
		nameField = strings.TrimSpace(input)
	}

	// Default to a path that matches the type name exactly unless explicitly overridden.
	defaultPath := schemaAddTypeDefaultPath
	if strings.TrimSpace(defaultPath) == "" {
		defaultPath = paths.NormalizeDirRoot(typeName)
	}

	result := executeCanonicalCommand("schema_add_type", vaultPath, map[string]interface{}{
		"name":         typeName,
		"default-path": defaultPath,
		"name-field":   nameField,
		"description":  schemaAddTypeDescription,
	})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)

	fmt.Printf("✓ Added type '%s' to schema.yaml\n", data["name"])
	fmt.Printf("  default_path: %s\n", data["default_path"])
	if description, _ := data["description"].(string); description != "" {
		fmt.Printf("  description: %s\n", description)
	}
	if canonicalNameField, _ := data["name_field"].(string); canonicalNameField != "" {
		fmt.Printf("  name_field: %s (auto-created as required string)\n", canonicalNameField)
	}
	return nil
}

func addTrait(vaultPath, traitName string, start time.Time) error {
	result := executeCanonicalCommand("schema_add_trait", vaultPath, map[string]interface{}{
		"name":    traitName,
		"type":    schemaAddTraitFieldType,
		"values":  schemaAddTraitValues,
		"default": schemaAddTraitDefault,
	})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)

	fmt.Printf("✓ Added trait '%s' to schema.yaml\n", data["name"])
	fmt.Printf("  type: %s\n", data["type"])
	values, _ := decodeSchemaValue[[]string](data["values"])
	if len(values) > 0 {
		fmt.Printf("  values: %s\n", schemaAddTraitValues)
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

	// Check if this looks like a schema type name (common mistake).
	// Do not reject names that are also valid field types (e.g., built-in "date").
	if sch != nil {
		if _, isSchemaType := sch.Types[baseType]; isSchemaType && !validFieldTypes[baseType] {
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
		if _, isSchemaType := sch.Types[strings.TrimSuffix(baseType, "[]")]; isSchemaType && !validFieldTypes[strings.TrimSuffix(baseType, "[]")] {
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
	result := executeCanonicalCommand("schema_add_field", vaultPath, map[string]interface{}{
		"type_name":   typeName,
		"field_name":  fieldName,
		"type":        schemaAddFieldTypeFlag,
		"required":    schemaAddFieldRequired,
		"default":     schemaAddFieldDefault,
		"values":      schemaAddFieldValues,
		"target":      schemaAddFieldTarget,
		"description": schemaAddFieldDescription,
	})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)

	fmt.Printf("✓ Added field '%s' to type '%s'\n", data["field"], data["type"])
	fmt.Printf("  type: %s\n", data["field_type"])
	if boolValue(data["required"]) {
		fmt.Println("  required: true")
	}
	if description, _ := data["description"].(string); description != "" {
		fmt.Printf("  description: %s\n", description)
	}
	return nil
}

func printSchemaChangeList(header string, changes []string) {
	fmt.Println(header)
	for _, c := range changes {
		fmt.Printf("  %s\n", c)
	}
}

var schemaValidateCmd = newCanonicalLeafCommand("schema_validate", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderSchemaValidate,
})

func renderSchemaValidate(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	issues, err := decodeSchemaValue[[]string](data["issues"])
	if err != nil {
		return err
	}
	types, err := decodeSchemaCount(data["types"])
	if err != nil {
		return err
	}
	traits, err := decodeSchemaCount(data["traits"])
	if err != nil {
		return err
	}

	if len(issues) > 0 {
		fmt.Printf("Schema validation found %d issues:\n", len(issues))
		for _, issue := range issues {
			fmt.Printf("  ⚠ %s\n", issue)
		}
		return nil
	}

	fmt.Printf("✓ Schema is valid (%d types, %d traits)\n", types, traits)
	return nil
}

// =============================================================================
// UPDATE COMMANDS
// =============================================================================

var schemaUpdateCmd = &cobra.Command{
	Use:   "update",
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
}

var schemaUpdateTypeCmd = &cobra.Command{
	Use:   "type <name>",
	Short: "Update an existing type",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return updateType(getVaultPath(), args[0], time.Now())
	},
}

var schemaUpdateTraitCmd = &cobra.Command{
	Use:   "trait <name>",
	Short: "Update an existing trait",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return updateTrait(getVaultPath(), args[0], time.Now())
	},
}

var schemaUpdateFieldCmd = &cobra.Command{
	Use:   "field <type> <field>",
	Short: "Update a field on an existing type",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return updateField(getVaultPath(), args[0], args[1], time.Now())
	},
}

func updateType(vaultPath, typeName string, start time.Time) error {
	result := executeCanonicalCommand("schema_update_type", vaultPath, map[string]interface{}{
		"name":         typeName,
		"default-path": schemaUpdateTypeDefaultPath,
		"name-field":   schemaUpdateTypeNameField,
		"description":  schemaUpdateTypeDescription,
		"add-trait":    schemaUpdateTypeAddTrait,
		"remove-trait": schemaUpdateTypeRemoveTrait,
	})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)
	changes, err := decodeSchemaValue[[]string](data["changes"])
	if err != nil {
		return err
	}
	printSchemaChangeList(fmt.Sprintf("✓ Updated type '%s'", typeName), changes)
	return nil
}

func updateTrait(vaultPath, traitName string, start time.Time) error {
	result := executeCanonicalCommand("schema_update_trait", vaultPath, map[string]interface{}{
		"name":    traitName,
		"type":    schemaUpdateTraitFieldType,
		"values":  schemaUpdateTraitValues,
		"default": schemaUpdateTraitDefault,
	})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)
	changes, err := decodeSchemaValue[[]string](data["changes"])
	if err != nil {
		return err
	}
	printSchemaChangeList(fmt.Sprintf("✓ Updated trait '%s'", traitName), changes)
	return nil
}

func updateField(vaultPath, typeName, fieldName string, start time.Time) error {
	result := executeCanonicalCommand("schema_update_field", vaultPath, map[string]interface{}{
		"type_name":   typeName,
		"field_name":  fieldName,
		"type":        schemaUpdateFieldTypeFlag,
		"required":    schemaUpdateFieldRequired,
		"default":     schemaUpdateFieldDefault,
		"values":      schemaUpdateFieldValues,
		"target":      schemaUpdateFieldTarget,
		"description": schemaUpdateFieldDescription,
	})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)
	changes, err := decodeSchemaValue[[]string](data["changes"])
	if err != nil {
		return err
	}
	printSchemaChangeList(fmt.Sprintf("✓ Updated field '%s' on type '%s'", fieldName, typeName), changes)
	return nil
}

// =============================================================================
// REMOVE COMMANDS
// =============================================================================

var schemaRemoveCmd = &cobra.Command{
	Use:   "remove",
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
}

var schemaRemoveTypeCmd = &cobra.Command{
	Use:   "type <name>",
	Short: "Remove a type from the schema",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return removeType(getVaultPath(), args[0], time.Now())
	},
}

var schemaRemoveTraitCmd = &cobra.Command{
	Use:   "trait <name>",
	Short: "Remove a trait from the schema",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return removeTrait(getVaultPath(), args[0], time.Now())
	},
}

var schemaRemoveFieldCmd = &cobra.Command{
	Use:   "field <type> <field>",
	Short: "Remove a field from a type",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return removeField(getVaultPath(), args[0], args[1], time.Now())
	},
}

func removeType(vaultPath, typeName string, start time.Time) error {
	result := executeCanonicalCommand("schema_remove_type", vaultPath, map[string]interface{}{
		"name":  typeName,
		"force": schemaRemoveTypeForce,
	})
	if !isJSONOutput() && result.Error != nil && result.Error.Code == ErrConfirmationRequired && !schemaRemoveTypeForce {
		details, _ := result.Error.Details.(map[string]interface{})
		count := detailInt(details, "affected_count")
		if count > 0 {
			fmt.Printf("Warning: %d files of type '%s' will become 'page' type:\n", count, typeName)
			for _, filePath := range detailStringSlice(details, "affected_files") {
				fmt.Printf("  - %s\n", filePath)
			}
			if remaining := detailInt(details, "remaining_count"); remaining > 0 {
				fmt.Printf("  ... and %d more\n", remaining)
			}
		}
		if !promptForConfirm("Continue?") {
			return handleErrorMsg(ErrConfirmationRequired, "operation cancelled", "Use --force to skip confirmation")
		}
		result = executeCanonicalCommand("schema_remove_type", vaultPath, map[string]interface{}{"name": typeName, "force": true})
	}
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}

	fmt.Printf("✓ Removed type '%s' from schema.yaml\n", typeName)
	return nil
}

func removeTrait(vaultPath, traitName string, start time.Time) error {
	result := executeCanonicalCommand("schema_remove_trait", vaultPath, map[string]interface{}{
		"name":  traitName,
		"force": schemaRemoveTraitForce,
	})
	if !isJSONOutput() && result.Error != nil && result.Error.Code == ErrConfirmationRequired && !schemaRemoveTraitForce {
		details, _ := result.Error.Details.(map[string]interface{})
		count := detailInt(details, "affected_count")
		if count > 0 {
			fmt.Printf("Warning: %d instances of @%s will remain in files (no longer indexed)\n", count, traitName)
		}
		if !promptForConfirm("Continue?") {
			return handleErrorMsg(ErrConfirmationRequired, "operation cancelled", "Use --force to skip confirmation")
		}
		result = executeCanonicalCommand("schema_remove_trait", vaultPath, map[string]interface{}{"name": traitName, "force": true})
	}
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}

	fmt.Printf("✓ Removed trait '%s' from schema.yaml\n", traitName)
	return nil
}

func removeField(vaultPath, typeName, fieldName string, start time.Time) error {
	result := executeCanonicalCommand("schema_remove_field", vaultPath, map[string]interface{}{
		"type_name":  typeName,
		"field_name": fieldName,
	})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}

	fmt.Printf("✓ Removed field '%s' from type '%s'\n", fieldName, typeName)
	return nil
}

func detailInt(details map[string]interface{}, key string) int {
	if details == nil {
		return 0
	}
	value, ok := details[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func detailStringSlice(details map[string]interface{}, key string) []string {
	if details == nil {
		return nil
	}
	raw, ok := details[key]
	if !ok {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return typed
	case []interface{}:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				values = append(values, str)
			}
		}
		return values
	default:
		return nil
	}
}

func decodeSchemaCount(raw interface{}) (int, error) {
	switch typed := raw.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		return int(typed), nil
	case json.Number:
		value, err := typed.Int64()
		return int(value), err
	default:
		return 0, fmt.Errorf("unexpected numeric value type %T", raw)
	}
}

// =============================================================================
// RENAME COMMANDS
// =============================================================================

var schemaRenameCmd = &cobra.Command{
	Use:   "rename",
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
}

var schemaRenameTypeCmd = &cobra.Command{
	Use:   "type <old_name> <new_name>",
	Short: "Rename a type and update references",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return renameType(getVaultPath(), args[0], args[1], time.Now())
	},
}

var schemaRenameFieldCmd = &cobra.Command{
	Use:   "field <type> <old_field> <new_field>",
	Short: "Rename a field on a type and update downstream uses",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return renameField(getVaultPath(), args[0], args[1], args[2], time.Now())
	},
}

func renameField(vaultPath, typeName, oldField, newField string, start time.Time) error {
	result := executeCanonicalCommand("schema_rename_field", vaultPath, map[string]interface{}{
		"type_name": typeName,
		"old_field": oldField,
		"new_field": newField,
		"confirm":   schemaRenameFieldConfirm,
	})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)

	if boolValue(data["preview"]) {
		changes, err := decodeSchemaValue[[]schemasvc.FieldRenameChange](data["changes"])
		if err != nil {
			return err
		}
		totalChanges, err := decodeSchemaCount(data["total_changes"])
		if err != nil {
			return err
		}
		fmt.Printf("Preview: Rename field '%s.%s' to '%s.%s'\n\n", typeName, oldField, typeName, newField)
		fmt.Printf("Changes to be made (%d total):\n", totalChanges)
		printFieldRenameChanges(changes)
		fmt.Printf("\nRun with --confirm to apply these changes.\n")
		return nil
	}

	changesApplied, err := decodeSchemaCount(data["changes_applied"])
	if err != nil {
		return err
	}
	fmt.Printf("✓ Renamed field '%s.%s' to '%s.%s'\n", typeName, oldField, typeName, newField)
	fmt.Printf("  Applied %d changes\n", changesApplied)
	fmt.Printf("\n%s.\n", data["hint"])
	return nil
}

func renameType(vaultPath, oldName, newName string, start time.Time) error {
	preview := executeCanonicalCommand("schema_rename_type", vaultPath, map[string]interface{}{
		"old_name": oldName,
		"new_name": newName,
	})
	if isJSONOutput() && !schemaRenameTypeConfirm {
		outputCanonicalResultJSON(preview)
		return nil
	}
	if err := handleCanonicalFailure(preview); err != nil {
		return err
	}
	previewData := canonicalDataMap(preview)

	if !schemaRenameTypeConfirm {
		changes, err := decodeSchemaValue[[]schemasvc.TypeRenameChange](previewData["changes"])
		if err != nil {
			return err
		}
		totalChanges, err := decodeSchemaCount(previewData["total_changes"])
		if err != nil {
			return err
		}
		fmt.Printf("Preview: Rename type '%s' to '%s'\n\n", oldName, newName)
		fmt.Printf("Changes to be made (%d total):\n", totalChanges)
		printTypeRenameChanges(changes)
		if boolValue(previewData["default_path_rename_available"]) {
			optionalChanges, err := decodeSchemaCount(previewData["optional_total_changes"])
			if err != nil {
				return err
			}
			filesToMove, err := decodeSchemaCount(previewData["files_to_move"])
			if err != nil {
				return err
			}
			fmt.Printf("\nOptional default directory rename (%d changes):\n", optionalChanges)
			fmt.Printf("  default_path: %s → %s\n", previewData["default_path_old"], previewData["default_path_new"])
			if filesToMove > 0 {
				fmt.Printf("  files to move: %d\n", filesToMove)
			}
			fmt.Printf("  (add --rename-default-path to apply these optional changes)\n")
		}
		fmt.Printf("\nRun with --confirm to apply these changes.\n")
		return nil
	}

	applyDefaultPathRename := schemaRenameTypeDefaultPathRename
	if boolValue(previewData["default_path_rename_available"]) && !applyDefaultPathRename && shouldPromptForConfirm() {
		defaultPathOld, _ := previewData["default_path_old"].(string)
		defaultPathNew, _ := previewData["default_path_new"].(string)
		filesToMove, err := decodeSchemaCount(previewData["files_to_move"])
		if err != nil {
			return err
		}
		prompt := fmt.Sprintf("Also rename default_path '%s' -> '%s'?", defaultPathOld, defaultPathNew)
		if filesToMove > 0 {
			prompt = fmt.Sprintf(
				"Also rename default_path '%s' -> '%s' and move %d files with reference updates?",
				defaultPathOld,
				defaultPathNew,
				filesToMove,
			)
		}
		applyDefaultPathRename = promptForConfirm(prompt)
	}

	result := executeCanonicalCommand("schema_rename_type", vaultPath, map[string]interface{}{
		"old_name":            oldName,
		"new_name":            newName,
		"confirm":             true,
		"rename-default-path": applyDefaultPathRename,
	})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)
	changesApplied, err := decodeSchemaCount(data["changes_applied"])
	if err != nil {
		return err
	}

	fmt.Printf("✓ Renamed type '%s' to '%s'\n", oldName, newName)
	fmt.Printf("  Applied %d changes\n", changesApplied)
	if boolValue(data["default_path_rename_available"]) {
		if boolValue(data["default_path_renamed"]) {
			filesMoved, err := decodeSchemaCount(data["files_moved"])
			if err != nil {
				return err
			}
			refFilesUpdated, err := decodeSchemaCount(data["reference_files_updated"])
			if err != nil {
				return err
			}
			fmt.Printf("  Renamed default_path %s → %s\n", data["default_path_old"], data["default_path_new"])
			fmt.Printf("  Moved %d files and updated references in %d files\n", filesMoved, refFilesUpdated)
		} else {
			fmt.Printf("  Default path remains %s (use --rename-default-path to rename to %s)\n", data["default_path_old"], data["default_path_new"])
		}
	}
	fmt.Printf("\n%s.\n", data["hint"])
	return nil
}

func printFieldRenameChanges(changes []schemasvc.FieldRenameChange) {
	byFile := make(map[string][]schemasvc.FieldRenameChange)
	for _, change := range changes {
		byFile[change.FilePath] = append(byFile[change.FilePath], change)
	}

	files := make([]string, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}
	sort.Strings(files)

	for _, file := range files {
		fileChanges := byFile[file]
		fmt.Printf("\n  %s:\n", file)
		for _, change := range fileChanges {
			if change.Line > 0 {
				fmt.Printf("    Line %d: %s\n", change.Line, change.Description)
			} else {
				fmt.Printf("    %s\n", change.Description)
			}
		}
	}
}

func printTypeRenameChanges(changes []schemasvc.TypeRenameChange) {
	byFile := make(map[string][]schemasvc.TypeRenameChange)
	for _, change := range changes {
		byFile[change.FilePath] = append(byFile[change.FilePath], change)
	}

	files := make([]string, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}
	sort.Strings(files)

	for _, file := range files {
		fileChanges := byFile[file]
		fmt.Printf("\n  %s:\n", file)
		for _, change := range fileChanges {
			if change.Line > 0 {
				fmt.Printf("    Line %d: %s\n", change.Line, change.Description)
			} else {
				fmt.Printf("    %s\n", change.Description)
			}
		}
	}
}

func init() {
	schemaAddTypeCmd.Flags().StringVar(&schemaAddTypeDefaultPath, "default-path", "", "Default path for new type files (default: <type>/)")
	schemaAddTypeCmd.Flags().StringVar(&schemaAddTypeNameField, "name-field", "", "Field to use as display name (auto-created if doesn't exist)")
	schemaAddTypeCmd.Flags().StringVar(&schemaAddTypeDescription, "description", "", "Optional description for the type")

	schemaAddTraitCmd.Flags().StringVar(&schemaAddTraitFieldType, "type", "", "Trait type (string, number, url, date, datetime, enum, ref, bool)")
	schemaAddTraitCmd.Flags().StringVar(&schemaAddTraitDefault, "default", "", "Default value")
	schemaAddTraitCmd.Flags().StringVar(&schemaAddTraitValues, "values", "", "Enum values (comma-separated)")

	schemaAddFieldCmd.Flags().StringVar(&schemaAddFieldTypeFlag, "type", "", "Field type (string, number, url, date, datetime, enum, ref, bool)")
	schemaAddFieldCmd.Flags().BoolVar(&schemaAddFieldRequired, "required", false, "Mark field as required")
	schemaAddFieldCmd.Flags().StringVar(&schemaAddFieldDefault, "default", "", "Default value")
	schemaAddFieldCmd.Flags().StringVar(&schemaAddFieldValues, "values", "", "Enum values (comma-separated)")
	schemaAddFieldCmd.Flags().StringVar(&schemaAddFieldTarget, "target", "", "Target type for ref fields")
	schemaAddFieldCmd.Flags().StringVar(&schemaAddFieldDescription, "description", "", "Optional description for the field")

	schemaUpdateTypeCmd.Flags().StringVar(&schemaUpdateTypeDefaultPath, "default-path", "", "Update default path for type")
	schemaUpdateTypeCmd.Flags().StringVar(&schemaUpdateTypeNameField, "name-field", "", "Set/update display name field (use '-' to remove)")
	schemaUpdateTypeCmd.Flags().StringVar(&schemaUpdateTypeDescription, "description", "", "Set/update description (use '-' to remove)")
	schemaUpdateTypeCmd.Flags().StringVar(&schemaUpdateTypeAddTrait, "add-trait", "", "Add a trait to the type")
	schemaUpdateTypeCmd.Flags().StringVar(&schemaUpdateTypeRemoveTrait, "remove-trait", "", "Remove a trait from the type")

	schemaUpdateTraitCmd.Flags().StringVar(&schemaUpdateTraitFieldType, "type", "", "Update trait type")
	schemaUpdateTraitCmd.Flags().StringVar(&schemaUpdateTraitDefault, "default", "", "Update default value")
	schemaUpdateTraitCmd.Flags().StringVar(&schemaUpdateTraitValues, "values", "", "Update enum values (comma-separated)")

	schemaUpdateFieldCmd.Flags().StringVar(&schemaUpdateFieldTypeFlag, "type", "", "Update field type")
	schemaUpdateFieldCmd.Flags().StringVar(&schemaUpdateFieldRequired, "required", "", "Update required status (true/false)")
	schemaUpdateFieldCmd.Flags().StringVar(&schemaUpdateFieldDefault, "default", "", "Update default value")
	schemaUpdateFieldCmd.Flags().StringVar(&schemaUpdateFieldValues, "values", "", "Update enum values (comma-separated)")
	schemaUpdateFieldCmd.Flags().StringVar(&schemaUpdateFieldTarget, "target", "", "Update target type for ref fields")
	schemaUpdateFieldCmd.Flags().StringVar(&schemaUpdateFieldDescription, "description", "", "Set/update description (use '-' to remove)")

	schemaRemoveTypeCmd.Flags().BoolVar(&schemaRemoveTypeForce, "force", false, "Skip confirmation prompts")
	schemaRemoveTraitCmd.Flags().BoolVar(&schemaRemoveTraitForce, "force", false, "Skip confirmation prompts")

	schemaRenameTypeCmd.Flags().BoolVar(&schemaRenameTypeConfirm, "confirm", false, "Apply the rename (default: preview only)")
	schemaRenameTypeCmd.Flags().BoolVar(&schemaRenameTypeDefaultPathRename, "rename-default-path", false, "Also rename type default_path directory and move matching files (with reference updates)")
	schemaRenameFieldCmd.Flags().BoolVar(&schemaRenameFieldConfirm, "confirm", false, "Apply the rename (default: preview only)")

	schemaAddCmd.AddCommand(schemaAddTypeCmd)
	schemaAddCmd.AddCommand(schemaAddTraitCmd)
	schemaAddCmd.AddCommand(schemaAddFieldCmd)
	schemaUpdateCmd.AddCommand(schemaUpdateTypeCmd)
	schemaUpdateCmd.AddCommand(schemaUpdateTraitCmd)
	schemaUpdateCmd.AddCommand(schemaUpdateFieldCmd)
	schemaRemoveCmd.AddCommand(schemaRemoveTypeCmd)
	schemaRemoveCmd.AddCommand(schemaRemoveTraitCmd)
	schemaRemoveCmd.AddCommand(schemaRemoveFieldCmd)
	schemaRenameCmd.AddCommand(schemaRenameTypeCmd)
	schemaRenameCmd.AddCommand(schemaRenameFieldCmd)

	schemaCmd.AddCommand(schemaAddCmd)
	schemaCmd.AddCommand(schemaUpdateCmd)
	schemaCmd.AddCommand(schemaRemoveCmd)
	schemaCmd.AddCommand(schemaRenameCmd)
	schemaCmd.AddCommand(schemaValidateCmd)
}
