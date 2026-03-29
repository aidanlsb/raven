package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemasvc"
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

var schemaAddTypeCmd = newCanonicalLeafCommand("schema_add_type", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	BuildArgs:   buildSchemaAddTypeArgs,
	RenderHuman: renderSchemaAddType,
})

var schemaAddTraitCmd = newCanonicalLeafCommand("schema_add_trait", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderSchemaAddTrait,
})

var schemaAddFieldCmd = newCanonicalLeafCommand("schema_add_field", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderSchemaAddField,
})

func buildSchemaArgs(commandID string, cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	meta, ok := commands.EffectiveMeta(commandID)
	if !ok {
		return nil, fmt.Errorf("registry metadata missing for %q", commandID)
	}
	return buildCanonicalArgsForMeta(meta, cmd, args)
}

func buildSchemaAddTypeArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	argsMap, err := buildSchemaArgs("schema_add_type", cmd, args)
	if err != nil {
		return nil, err
	}

	nameField, _ := cmd.Flags().GetString("name-field")
	if strings.TrimSpace(nameField) == "" && !isJSONOutput() {
		fmt.Print("Which field should be the display name? (common: name, title; leave blank for none): ")
		var input string
		fmt.Scanln(&input)
		nameField = strings.TrimSpace(input)
		argsMap["name-field"] = nameField
	}

	defaultPath, _ := cmd.Flags().GetString("default-path")
	if strings.TrimSpace(defaultPath) == "" {
		argsMap["default-path"] = paths.NormalizeDirRoot(args[0])
	}

	return argsMap, nil
}

func renderSchemaAddType(_ *cobra.Command, result commandexec.Result) error {
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

func renderSchemaAddTrait(cmd *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)

	fmt.Printf("✓ Added trait '%s' to schema.yaml\n", data["name"])
	fmt.Printf("  type: %s\n", data["type"])
	values, _ := decodeSchemaValue[[]string](data["values"])
	if len(values) > 0 {
		rawValues, _ := cmd.Flags().GetString("values")
		fmt.Printf("  values: %s\n", rawValues)
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

func renderSchemaAddField(_ *cobra.Command, result commandexec.Result) error {
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

var schemaUpdateTypeCmd = newCanonicalLeafCommand("schema_update_type", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderSchemaUpdateType,
})

var schemaUpdateTraitCmd = newCanonicalLeafCommand("schema_update_trait", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderSchemaUpdateTrait,
})

var schemaUpdateFieldCmd = newCanonicalLeafCommand("schema_update_field", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderSchemaUpdateField,
})

func renderSchemaUpdateType(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	changes, err := decodeSchemaValue[[]string](data["changes"])
	if err != nil {
		return err
	}
	printSchemaChangeList(fmt.Sprintf("✓ Updated type '%s'", stringValue(data["name"])), changes)
	return nil
}

func renderSchemaUpdateTrait(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	changes, err := decodeSchemaValue[[]string](data["changes"])
	if err != nil {
		return err
	}
	printSchemaChangeList(fmt.Sprintf("✓ Updated trait '%s'", stringValue(data["name"])), changes)
	return nil
}

func renderSchemaUpdateField(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	changes, err := decodeSchemaValue[[]string](data["changes"])
	if err != nil {
		return err
	}
	printSchemaChangeList(fmt.Sprintf("✓ Updated field '%s' on type '%s'", stringValue(data["field"]), stringValue(data["type"])), changes)
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

var schemaRemoveTypeCmd = newCanonicalLeafCommand("schema_remove_type", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Invoke:      invokeSchemaRemoveType,
	RenderHuman: renderSchemaRemoveType,
})

var schemaRemoveTraitCmd = newCanonicalLeafCommand("schema_remove_trait", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Invoke:      invokeSchemaRemoveTrait,
	RenderHuman: renderSchemaRemoveTrait,
})

var schemaRemoveFieldCmd = newCanonicalLeafCommand("schema_remove_field", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderSchemaRemoveField,
})

func invokeSchemaRemoveType(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	result := executeCanonicalCommand(commandID, vaultPath, args)
	if isJSONOutput() || boolValue(args["force"]) || result.OK || result.Error == nil || result.Error.Code != ErrConfirmationRequired {
		return result
	}

	details, _ := result.Error.Details.(map[string]interface{})
	count := detailInt(details, "affected_count")
	typeName := stringValue(args["name"])
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
		return commandexec.Failure(ErrConfirmationRequired, "operation cancelled", nil, "Use --force to skip confirmation")
	}

	retry := cloneArgsMap(args)
	retry["force"] = true
	return executeCanonicalCommand(commandID, vaultPath, retry)
}

func invokeSchemaRemoveTrait(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	result := executeCanonicalCommand(commandID, vaultPath, args)
	if isJSONOutput() || boolValue(args["force"]) || result.OK || result.Error == nil || result.Error.Code != ErrConfirmationRequired {
		return result
	}

	details, _ := result.Error.Details.(map[string]interface{})
	count := detailInt(details, "affected_count")
	traitName := stringValue(args["name"])
	if count > 0 {
		fmt.Printf("Warning: %d instances of @%s will remain in files (no longer indexed)\n", count, traitName)
	}
	if !promptForConfirm("Continue?") {
		return commandexec.Failure(ErrConfirmationRequired, "operation cancelled", nil, "Use --force to skip confirmation")
	}

	retry := cloneArgsMap(args)
	retry["force"] = true
	return executeCanonicalCommand(commandID, vaultPath, retry)
}

func renderSchemaRemoveType(_ *cobra.Command, result commandexec.Result) error {
	fmt.Printf("✓ Removed type '%s' from schema.yaml\n", stringValue(canonicalDataMap(result)["name"]))
	return nil
}

func renderSchemaRemoveTrait(_ *cobra.Command, result commandexec.Result) error {
	fmt.Printf("✓ Removed trait '%s' from schema.yaml\n", stringValue(canonicalDataMap(result)["name"]))
	return nil
}

func renderSchemaRemoveField(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("✓ Removed field '%s' from type '%s'\n", stringValue(data["field"]), stringValue(data["type"]))
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

var schemaRenameTypeCmd = newCanonicalLeafCommand("schema_rename_type", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Invoke:      invokeSchemaRenameType,
	RenderHuman: renderSchemaRenameType,
})

var schemaRenameFieldCmd = newCanonicalLeafCommand("schema_rename_field", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderSchemaRenameField,
})

func invokeSchemaRenameType(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	confirm := boolValue(args["confirm"])

	previewArgs := map[string]interface{}{
		"old_name": args["old_name"],
		"new_name": args["new_name"],
	}
	preview := executeCanonicalCommand(commandID, vaultPath, previewArgs)
	if !preview.OK {
		return preview
	}
	if !confirm {
		return preview
	}

	previewData := canonicalDataMap(preview)
	applyDefaultPathRename := boolValue(args["rename-default-path"])
	if boolValue(previewData["default_path_rename_available"]) && !applyDefaultPathRename && shouldPromptForConfirm() {
		defaultPathOld, _ := previewData["default_path_old"].(string)
		defaultPathNew, _ := previewData["default_path_new"].(string)
		filesToMove, err := decodeSchemaCount(previewData["files_to_move"])
		if err != nil {
			return commandexec.Failure(ErrInternal, err.Error(), nil, "")
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

	applyArgs := cloneArgsMap(args)
	applyArgs["rename-default-path"] = applyDefaultPathRename
	applyArgs["confirm"] = true
	return executeCanonicalCommand(commandID, vaultPath, applyArgs)
}

func renderSchemaRenameField(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	typeName := stringValue(data["type"])
	oldField := stringValue(data["old_field"])
	newField := stringValue(data["new_field"])

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

func renderSchemaRenameType(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	oldName := stringValue(data["old_name"])
	newName := stringValue(data["new_name"])

	if boolValue(data["preview"]) {
		changes, err := decodeSchemaValue[[]schemasvc.TypeRenameChange](data["changes"])
		if err != nil {
			return err
		}
		totalChanges, err := decodeSchemaCount(data["total_changes"])
		if err != nil {
			return err
		}
		fmt.Printf("Preview: Rename type '%s' to '%s'\n\n", oldName, newName)
		fmt.Printf("Changes to be made (%d total):\n", totalChanges)
		printTypeRenameChanges(changes)
		if boolValue(data["default_path_rename_available"]) {
			optionalChanges, err := decodeSchemaCount(data["optional_total_changes"])
			if err != nil {
				return err
			}
			filesToMove, err := decodeSchemaCount(data["files_to_move"])
			if err != nil {
				return err
			}
			fmt.Printf("\nOptional default directory rename (%d changes):\n", optionalChanges)
			fmt.Printf("  default_path: %s → %s\n", data["default_path_old"], data["default_path_new"])
			if filesToMove > 0 {
				fmt.Printf("  files to move: %d\n", filesToMove)
			}
			fmt.Printf("  (add --rename-default-path to apply these optional changes)\n")
		}
		fmt.Printf("\nRun with --confirm to apply these changes.\n")
		return nil
	}

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
