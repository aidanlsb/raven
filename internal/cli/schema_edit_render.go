package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/ui"
)

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
	for _, change := range changes {
		fmt.Printf("  %s\n", ui.Hint(change))
	}
}

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

func renderSchemaUpdateType(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	return renderSchemaUpdate(result, fmt.Sprintf("✓ Updated type '%s'", stringValue(data["name"])))
}

func renderSchemaUpdateTrait(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	return renderSchemaUpdate(result, fmt.Sprintf("✓ Updated trait '%s'", stringValue(data["name"])))
}

func renderSchemaUpdateField(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	return renderSchemaUpdate(result, fmt.Sprintf("✓ Updated field '%s' on type '%s'", stringValue(data["field"]), stringValue(data["type"])))
}

func renderSchemaUpdate(result commandexec.Result, header string) error {
	changes, err := decodeSchemaValue[[]string](canonicalDataMap(result)["changes"])
	if err != nil {
		return err
	}
	printSchemaChangeList(header, changes)
	return nil
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
		printSchemaFileChanges(
			changes,
			func(change schemasvc.ValueConvertChange) string { return change.FilePath },
			func(change schemasvc.ValueConvertChange) int { return change.Line },
			func(change schemasvc.ValueConvertChange) string { return change.Description },
		)
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
		printSchemaFileChanges(
			changes,
			func(change schemasvc.FieldRenameChange) string { return change.FilePath },
			func(change schemasvc.FieldRenameChange) int { return change.Line },
			func(change schemasvc.FieldRenameChange) string { return change.Description },
		)
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
		printSchemaFileChanges(
			changes,
			func(change schemasvc.TypeRenameChange) string { return change.FilePath },
			func(change schemasvc.TypeRenameChange) int { return change.Line },
			func(change schemasvc.TypeRenameChange) string { return change.Description },
		)
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

func printSchemaFileChanges[T any](
	changes []T,
	filePath func(T) string,
	line func(T) int,
	description func(T) string,
) {
	byFile := make(map[string][]T)
	for _, change := range changes {
		path := filePath(change)
		byFile[path] = append(byFile[path], change)
	}

	files := make([]string, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}
	sort.Strings(files)

	for _, file := range files {
		fmt.Printf("\n  %s:\n", ui.FilePath(file))
		for _, change := range byFile[file] {
			if line(change) > 0 {
				fmt.Printf("    %s %s\n", ui.Hint(fmt.Sprintf("Line %d:", line(change))), description(change))
			} else {
				fmt.Printf("    %s\n", description(change))
			}
		}
	}
}
