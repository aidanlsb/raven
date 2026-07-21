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
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/ui"
)

const schemaAddLong = `Add new definitions to schema.yaml.

Subcommands:
  type <name>              Add a new type
  trait <name>             Add a new trait
  field <type> <field>     Add a field to an existing type

Examples:
  rvn schema add type event --default-path event/
  rvn schema add trait priority --type enum --values high,medium,low
  rvn schema add field person email --type string --required`

// schemaAddCmd is the "schema add" subtree, generated from registry metadata
// (CLIPath) via buildRegistrySubtree. Leaf render/build hooks are wired through
// the spec below; adding a new schema_add_* entry needs no new Cobra vars.
var schemaAddCmd = buildRegistrySubtree(registrySubtreeSpec{
	Prefix:    []string{"schema", "add"},
	VaultPath: getVaultPath,
	Root: registryGroup{
		Use:        "add",
		Short:      "Add a type, trait, or field to the schema",
		Long:       schemaAddLong,
		ParentOnly: true,
	},
	Renders: map[string]func(*cobra.Command, commandexec.Result) error{
		"schema_add_type":  renderSchemaAddType,
		"schema_add_trait": renderSchemaAddTrait,
		"schema_add_field": renderSchemaAddField,
	},
	Leaves: map[string]canonicalLeafOptions{
		"schema_add_type": {BuildArgs: buildSchemaAddTypeArgs},
	},
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

	fmt.Println(ui.Checkf("Added type '%s' to schema.yaml", data["name"]))
	fmt.Printf("  %s %s\n", ui.Hint("default_path:"), data["default_path"])
	if description, _ := data["description"].(string); description != "" {
		fmt.Printf("  %s %s\n", ui.Hint("description:"), description)
	}
	if canonicalNameField, _ := data["name_field"].(string); canonicalNameField != "" {
		fmt.Printf("  %s %s %s\n", ui.Hint("name_field:"), canonicalNameField, ui.Hint("(auto-created as required string)"))
	}
	return nil
}

func renderSchemaAddTrait(cmd *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)

	fmt.Println(ui.Checkf("Added trait '%s' to schema.yaml", data["name"]))
	fmt.Printf("  %s %s\n", ui.Hint("type:"), data["type"])
	values, _ := decodeSchemaValue[[]string](data["values"])
	if len(values) > 0 {
		rawValues, _ := cmd.Flags().GetString("values")
		fmt.Printf("  %s %s\n", ui.Hint("values:"), rawValues)
	}
	return nil
}

func renderSchemaAddField(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)

	fmt.Println(ui.Checkf("Added field '%s' to type '%s'", data["field"], data["type"]))
	fmt.Printf("  %s %s\n", ui.Hint("type:"), data["field_type"])
	if boolValue(data["required"]) {
		fmt.Printf("  %s true\n", ui.Hint("required:"))
	}
	if description, _ := data["description"].(string); description != "" {
		fmt.Printf("  %s %s\n", ui.Hint("description:"), description)
	}
	return nil
}

func printSchemaChangeList(header string, changes []string) {
	fmt.Println(ui.Check(header))
	for _, c := range changes {
		fmt.Printf("  %s\n", ui.Hint(c))
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
		fmt.Println(ui.Warningf("Schema validation found %d issues:", len(issues)))
		for _, issue := range issues {
			fmt.Printf("  %s\n", ui.Warning(issue))
		}
		return nil
	}

	fmt.Println(ui.Checkf("Schema is valid (%d types, %d traits)", types, traits))
	return nil
}

// =============================================================================
// UPDATE COMMANDS
// =============================================================================

const schemaUpdateLong = `Update existing definitions in schema.yaml.

Subcommands:
  type <name>              Update an existing type
  trait <name>             Update an existing trait  
  field <type> <field>     Update a field on an existing type

Examples:
  rvn schema update type person --default-path person/
  rvn schema update trait priority --values critical,high,medium,low
  rvn schema update field person email --required=true
  rvn schema update type meeting --add-trait due`

// schemaUpdateCmd is the "schema update" subtree, generated from registry
// metadata via buildRegistrySubtree.
var schemaUpdateCmd = buildRegistrySubtree(registrySubtreeSpec{
	Prefix:    []string{"schema", "update"},
	VaultPath: getVaultPath,
	Root: registryGroup{
		Use:        "update",
		Short:      "Update a type, trait, or field in the schema",
		Long:       schemaUpdateLong,
		ParentOnly: true,
	},
	Renders: map[string]func(*cobra.Command, commandexec.Result) error{
		"schema_update_type":  renderSchemaUpdateType,
		"schema_update_trait": renderSchemaUpdateTrait,
		"schema_update_field": renderSchemaUpdateField,
	},
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

const schemaRemoveLong = `Remove definitions from schema.yaml.

Subcommands:
  type <name>              Remove a type (objects become 'page' type)
  trait <name>             Remove a trait (existing instances remain in files)
  field <type> <field>     Remove a field from a type

By default, warns about affected files. Use --force to skip warnings.

Examples:
  rvn schema remove type event
  rvn schema remove trait priority --force
  rvn schema remove field person nickname`

// schemaRemoveCmd is the "schema remove" subtree, generated from registry
// metadata via buildRegistrySubtree. The type/trait leaves keep their
// interactive confirm flows via per-leaf Invoke hooks.
var schemaRemoveCmd = buildRegistrySubtree(registrySubtreeSpec{
	Prefix:    []string{"schema", "remove"},
	VaultPath: getVaultPath,
	Root: registryGroup{
		Use:        "remove",
		Short:      "Remove a type, trait, or field from the schema",
		Long:       schemaRemoveLong,
		ParentOnly: true,
	},
	Renders: map[string]func(*cobra.Command, commandexec.Result) error{
		"schema_remove_type":  renderSchemaRemoveType,
		"schema_remove_trait": renderSchemaRemoveTrait,
		"schema_remove_field": renderSchemaRemoveField,
	},
	Leaves: map[string]canonicalLeafOptions{
		"schema_remove_type":  {Invoke: invokeSchemaRemoveType},
		"schema_remove_trait": {Invoke: invokeSchemaRemoveTrait},
	},
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
		fmt.Println(ui.Warningf("%d files of type '%s' will become 'page' type:", count, typeName))
		for _, filePath := range detailStringSlice(details, "affected_files") {
			fmt.Println(ui.Bullet(ui.FilePath(filePath)))
		}
		if remaining := detailInt(details, "remaining_count"); remaining > 0 {
			fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("... and %d more", remaining)))
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
		fmt.Println(ui.Warningf("%d instances of @%s will remain in files (no longer indexed)", count, traitName))
	}
	if !promptForConfirm("Continue?") {
		return commandexec.Failure(ErrConfirmationRequired, "operation cancelled", nil, "Use --force to skip confirmation")
	}

	retry := cloneArgsMap(args)
	retry["force"] = true
	return executeCanonicalCommand(commandID, vaultPath, retry)
}

func renderSchemaRemoveType(_ *cobra.Command, result commandexec.Result) error {
	fmt.Println(ui.Checkf("Removed type '%s' from schema.yaml", stringValue(canonicalDataMap(result)["name"])))
	return nil
}

func renderSchemaRemoveTrait(_ *cobra.Command, result commandexec.Result) error {
	fmt.Println(ui.Checkf("Removed trait '%s' from schema.yaml", stringValue(canonicalDataMap(result)["name"])))
	return nil
}

func renderSchemaRemoveField(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Println(ui.Checkf("Removed field '%s' from type '%s'", stringValue(data["field"]), stringValue(data["type"])))
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
// CONVERT COMMANDS
// =============================================================================

const schemaConvertLong = `Convert trait or field values and migrate schema.yaml plus matching vault data.

Subcommands:
  trait <name>
  field <type> <field>

--map-json is required and must exhaustively cover enum/bool allowed values,
the current default, and every observed live value. Conversion is preview-only
unless --confirm is supplied. Array-to-array conversion maps each member;
scalar-to-array mappings use explicit JSON arrays.

Examples:
  rvn schema convert trait priority --type bool --map-json '{"high":true,"medium":true,"low":false}'
  rvn schema convert field project status --type enum --map-json '{"true":"done","false":"todo"}'
  rvn schema convert trait priority --map-json '{"urgent":"critical","high":"high","medium":"medium","low":"low"}'`

var schemaConvertCmd = buildRegistrySubtree(registrySubtreeSpec{
	Prefix:    []string{"schema", "convert"},
	VaultPath: getVaultPath,
	Root: registryGroup{
		Use:        "convert",
		Short:      "Convert schema values and migrate vault data",
		Long:       schemaConvertLong,
		ParentOnly: true,
	},
	Renders: map[string]func(*cobra.Command, commandexec.Result) error{
		"schema_convert_trait": renderSchemaConvert,
		"schema_convert_field": renderSchemaConvert,
	},
})

func renderSchemaConvert(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	kind := stringValue(data["kind"])
	name := stringValue(data["name"])
	label := name
	if typeName := stringValue(data["type"]); typeName != "" {
		label = typeName + "." + name
	}
	sourceType := stringValue(data["source_type"])
	targetType := stringValue(data["target_type"])

	if boolValue(data["preview"]) {
		changes, err := decodeSchemaValue[[]schemasvc.ValueConvertChange](data["changes"])
		if err != nil {
			return err
		}
		totalChanges, err := decodeSchemaCount(data["total_changes"])
		if err != nil {
			return err
		}
		fmt.Printf("%s\n\n", ui.SectionHeader(fmt.Sprintf("Preview: Convert %s '%s' from %s to %s", kind, label, sourceType, targetType)))
		fmt.Printf("%s\n", ui.Hint(fmt.Sprintf("Changes to be made (%d total):", totalChanges)))
		printValueConvertChanges(changes)
		fmt.Printf("\n%s\n", ui.Hint("Run with --confirm to apply these changes."))
		return nil
	}

	changesApplied, err := decodeSchemaCount(data["changes_applied"])
	if err != nil {
		return err
	}
	fmt.Println(ui.Checkf("Converted %s '%s' from %s to %s", kind, label, sourceType, targetType))
	fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Applied %d changes", changesApplied)))
	fmt.Printf("\n%s.\n", ui.Hint(stringValue(data["hint"])))
	return nil
}

func printValueConvertChanges(changes []schemasvc.ValueConvertChange) {
	byFile := make(map[string][]schemasvc.ValueConvertChange)
	for _, change := range changes {
		byFile[change.FilePath] = append(byFile[change.FilePath], change)
	}
	files := make([]string, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		fmt.Printf("\n  %s:\n", ui.FilePath(file))
		for _, change := range byFile[file] {
			if change.Line > 0 {
				fmt.Printf("    %s %s\n", ui.Hint(fmt.Sprintf("Line %d:", change.Line)), change.Description)
			} else {
				fmt.Printf("    %s\n", change.Description)
			}
		}
	}
}

// =============================================================================
// RENAME COMMANDS
// =============================================================================

const schemaRenameLong = `Rename a type or a field in the schema and update downstream usages.

Subcommands:
  type  <old_name> <new_name>
  field <type> <old_field> <new_field>

Rename type updates:
1. Type definition key in schema.yaml
2. All 'type:' frontmatter fields
3. All ref field targets pointing to the old type

Rename field updates:
1. Field key in schema.yaml for the target type
2. If name_field == old_field, updates it to new_field
3. Type templates referencing {{field.old_field}} (template files)
4. Object frontmatter keys for files with type:<type>
5. Saved queries in raven.yaml (best-effort for type:<type> queries)

By default, previews changes. Use --confirm to apply.
When type default_path clearly matches the type name, you can also rename
that directory with --rename-default-path.

Examples:
  rvn schema rename type event meeting
  rvn schema rename type event meeting --confirm

  rvn schema rename field person email email_address
  rvn schema rename field person email email_address --confirm`

// schemaRenameCmd is the "schema rename" subtree, generated from registry
// metadata via buildRegistrySubtree. The type leaf keeps its interactive
// default-path confirm flow via a per-leaf Invoke hook.
var schemaRenameCmd = buildRegistrySubtree(registrySubtreeSpec{
	Prefix:    []string{"schema", "rename"},
	VaultPath: getVaultPath,
	Root: registryGroup{
		Use:        "rename",
		Short:      "Rename a type or field and update references",
		Long:       schemaRenameLong,
		ParentOnly: true,
	},
	Renders: map[string]func(*cobra.Command, commandexec.Result) error{
		"schema_rename_type":  renderSchemaRenameType,
		"schema_rename_field": renderSchemaRenameField,
	},
	Leaves: map[string]canonicalLeafOptions{
		"schema_rename_type": {Invoke: invokeSchemaRenameType},
	},
})

func invokeSchemaRenameType(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	confirm := boolValue(args["confirm"])

	previewArgs := map[string]interface{}{
		"old_name": args["old_name"],
		"new_name": args["new_name"],
	}
	if description, ok := args["description"]; ok {
		previewArgs["description"] = description
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
		fmt.Printf("%s\n\n", ui.SectionHeader(fmt.Sprintf("Preview: Rename field '%s.%s' to '%s.%s'", typeName, oldField, typeName, newField)))
		fmt.Printf("%s\n", ui.Hint(fmt.Sprintf("Changes to be made (%d total):", totalChanges)))
		printFieldRenameChanges(changes)
		fmt.Printf("\n%s\n", ui.Hint("Run with --confirm to apply these changes."))
		return nil
	}

	changesApplied, err := decodeSchemaCount(data["changes_applied"])
	if err != nil {
		return err
	}
	fmt.Println(ui.Checkf("Renamed field '%s.%s' to '%s.%s'", typeName, oldField, typeName, newField))
	fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Applied %d changes", changesApplied)))
	fmt.Printf("\n%s.\n", ui.Hint(stringValue(data["hint"])))
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
		fmt.Printf("%s\n\n", ui.SectionHeader(fmt.Sprintf("Preview: Rename type '%s' to '%s'", oldName, newName)))
		fmt.Printf("%s\n", ui.Hint(fmt.Sprintf("Changes to be made (%d total):", totalChanges)))
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
			fmt.Printf("\n%s\n", ui.Hint(fmt.Sprintf("Optional default directory rename (%d changes):", optionalChanges)))
			fmt.Printf("  %s %s → %s\n", ui.Hint("default_path:"), data["default_path_old"], data["default_path_new"])
			if filesToMove > 0 {
				fmt.Printf("  %s %d\n", ui.Hint("files to move:"), filesToMove)
			}
			fmt.Printf("  %s\n", ui.Hint("(add --rename-default-path to apply these optional changes)"))
		}
		fmt.Printf("\n%s\n", ui.Hint("Run with --confirm to apply these changes."))
		return nil
	}

	changesApplied, err := decodeSchemaCount(data["changes_applied"])
	if err != nil {
		return err
	}

	fmt.Println(ui.Checkf("Renamed type '%s' to '%s'", oldName, newName))
	fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Applied %d changes", changesApplied)))
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
			fmt.Printf("  %s %s → %s\n", ui.Hint("Renamed default_path"), data["default_path_old"], data["default_path_new"])
			fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Moved %d files and updated references in %d files", filesMoved, refFilesUpdated)))
		} else {
			fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Default path remains %s (use --rename-default-path to rename to %s)", data["default_path_old"], data["default_path_new"])))
		}
	}
	fmt.Printf("\n%s.\n", ui.Hint(stringValue(data["hint"])))
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
		fmt.Printf("\n  %s:\n", ui.FilePath(file))
		for _, change := range fileChanges {
			if change.Line > 0 {
				fmt.Printf("    %s %s\n", ui.Hint(fmt.Sprintf("Line %d:", change.Line)), change.Description)
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
		fmt.Printf("\n  %s:\n", ui.FilePath(file))
		for _, change := range fileChanges {
			if change.Line > 0 {
				fmt.Printf("    %s %s\n", ui.Hint(fmt.Sprintf("Line %d:", change.Line)), change.Description)
			} else {
				fmt.Printf("    %s\n", change.Description)
			}
		}
	}
}

func init() {
	// schemaAddCmd/schemaUpdateCmd/schemaRemoveCmd/schemaConvertCmd/schemaRenameCmd are
	// registry-generated subtrees (see buildRegistrySubtree specs above).
	// schema_validate remains a hand-wired direct leaf of schemaCmd.
	schemaCmd.AddCommand(schemaAddCmd)
	schemaCmd.AddCommand(schemaUpdateCmd)
	schemaCmd.AddCommand(schemaRemoveCmd)
	schemaCmd.AddCommand(schemaConvertCmd)
	schemaCmd.AddCommand(schemaRenameCmd)
	schemaCmd.AddCommand(schemaValidateCmd)
}
