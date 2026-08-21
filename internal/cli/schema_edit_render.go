package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/ui"
)

func renderSchemaAddType(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.SchemaAddTypeResult](result)
	if err != nil {
		return err
	}

	fmt.Println(ui.Checkf("Added type '%s' to schema.yaml", data.Name))
	fmt.Printf("  %s %s\n", ui.Hint("default_path:"), data.DefaultPath)
	if data.Description != "" {
		fmt.Printf("  %s %s\n", ui.Hint("description:"), data.Description)
	}
	if data.NameField != "" {
		fmt.Printf("  %s %s %s\n", ui.Hint("name_field:"), data.NameField, ui.Hint("(auto-created as required string)"))
	}
	return nil
}

func renderSchemaAddTrait(cmd *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.SchemaAddTraitResult](result)
	if err != nil {
		return err
	}

	fmt.Println(ui.Checkf("Added trait '%s' to schema.yaml", data.Name))
	fmt.Printf("  %s %s\n", ui.Hint("type:"), data.Type)
	if len(data.Values) > 0 {
		rawValues, _ := cmd.Flags().GetString("values")
		fmt.Printf("  %s %s\n", ui.Hint("values:"), rawValues)
	}
	return nil
}

func renderSchemaAddField(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.SchemaAddFieldResult](result)
	if err != nil {
		return err
	}

	fmt.Println(ui.Checkf("Added field '%s' to type '%s'", data.Field, data.Type))
	fmt.Printf("  %s %s\n", ui.Hint("type:"), data.FieldType)
	if data.Required {
		fmt.Printf("  %s true\n", ui.Hint("required:"))
	}
	if data.Description != "" {
		fmt.Printf("  %s %s\n", ui.Hint("description:"), data.Description)
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
	data, err := commandResultData[commandpayload.SchemaValidateResult](result)
	if err != nil {
		return err
	}

	if len(data.Issues) > 0 {
		fmt.Println(ui.Warningf("Schema validation found %d issues:", len(data.Issues)))
		for _, issue := range data.Issues {
			fmt.Printf("  %s\n", ui.Warning(issue))
		}
		return nil
	}

	fmt.Println(ui.Checkf("Schema is valid (%d types, %d traits)", data.Types, data.Traits))
	return nil
}

func renderSchemaUpdateType(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.SchemaUpdateResult](result)
	if err != nil {
		return err
	}
	return renderSchemaUpdate(data, fmt.Sprintf("✓ Updated type '%s'", data.Name))
}

func renderSchemaUpdateTrait(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.SchemaUpdateResult](result)
	if err != nil {
		return err
	}
	return renderSchemaUpdate(data, fmt.Sprintf("✓ Updated trait '%s'", data.Name))
}

func renderSchemaUpdateField(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.SchemaUpdateResult](result)
	if err != nil {
		return err
	}
	return renderSchemaUpdate(data, fmt.Sprintf("✓ Updated field '%s' on type '%s'", data.Field, data.Type))
}

func renderSchemaUpdate(data commandpayload.SchemaUpdateResult, header string) error {
	printSchemaChangeList(header, data.Changes)
	return nil
}

func renderSchemaRemoveType(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.SchemaRemoveResult](result)
	if err != nil {
		return err
	}
	fmt.Println(ui.Checkf("Removed type '%s' from schema.yaml", data.Name))
	return nil
}

func renderSchemaRemoveTrait(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.SchemaRemoveResult](result)
	if err != nil {
		return err
	}
	fmt.Println(ui.Checkf("Removed trait '%s' from schema.yaml", data.Name))
	return nil
}

func renderSchemaRemoveField(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[commandpayload.SchemaRemoveResult](result)
	if err != nil {
		return err
	}
	fmt.Println(ui.Checkf("Removed field '%s' from type '%s'", data.Field, data.Type))
	return nil
}

func renderSchemaConvert(_ *cobra.Command, result commandexec.Result) error {
	var kind, name, typeName, sourceType, targetType, hint string
	var preview *commandpayload.SchemaConvertPreviewResult
	switch data := result.Data.(type) {
	case commandpayload.SchemaConvertPreviewResult:
		preview = &data
		kind, name, typeName = data.Kind, data.Name, data.Type
		sourceType, targetType, hint = data.SourceType, data.TargetType, data.Hint
	case commandpayload.SchemaConvertResult:
		kind, name, typeName = data.Kind, data.Name, data.Type
		sourceType, targetType, hint = data.SourceType, data.TargetType, data.Hint
	default:
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}
	label := name
	if typeName != "" {
		label = typeName + "." + name
	}

	if preview != nil {
		fmt.Printf("%s\n\n", ui.SectionHeader(fmt.Sprintf("Preview: Convert %s '%s' from %s to %s", kind, label, sourceType, targetType)))
		fmt.Printf("%s\n", ui.Hint(fmt.Sprintf("Changes to be made (%d total):", preview.TotalChanges)))
		printSchemaFileChanges(
			preview.Changes,
			func(change schemasvc.ValueConvertChange) string { return change.FilePath },
			func(change schemasvc.ValueConvertChange) int { return change.Line },
			func(change schemasvc.ValueConvertChange) string { return change.Description },
		)
		fmt.Printf("\n%s\n", ui.Hint("Run with --confirm to apply these changes."))
		return nil
	}

	applied := result.Data.(commandpayload.SchemaConvertResult)
	fmt.Println(ui.Checkf("Converted %s '%s' from %s to %s", kind, label, sourceType, targetType))
	fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Applied %d changes", applied.ChangesApplied)))
	fmt.Printf("\n%s.\n", ui.Hint(hint))
	return nil
}

func renderSchemaRenameField(_ *cobra.Command, result commandexec.Result) error {
	if data, ok := result.Data.(commandpayload.SchemaRenameFieldPreviewResult); ok {
		fmt.Printf("%s\n\n", ui.SectionHeader(fmt.Sprintf("Preview: Rename field '%s.%s' to '%s.%s'", data.Type, data.OldField, data.Type, data.NewField)))
		fmt.Printf("%s\n", ui.Hint(fmt.Sprintf("Changes to be made (%d total):", data.TotalChanges)))
		printSchemaFileChanges(
			data.Changes,
			func(change schemasvc.FieldRenameChange) string { return change.FilePath },
			func(change schemasvc.FieldRenameChange) int { return change.Line },
			func(change schemasvc.FieldRenameChange) string { return change.Description },
		)
		fmt.Printf("\n%s\n", ui.Hint("Run with --confirm to apply these changes."))
		return nil
	}

	data, err := commandResultData[commandpayload.SchemaRenameFieldResult](result)
	if err != nil {
		return err
	}
	fmt.Println(ui.Checkf("Renamed field '%s.%s' to '%s.%s'", data.Type, data.OldField, data.Type, data.NewField))
	fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Applied %d changes", data.ChangesApplied)))
	fmt.Printf("\n%s.\n", ui.Hint(data.Hint))
	return nil
}

func renderSchemaRenameType(_ *cobra.Command, result commandexec.Result) error {
	if data, ok := result.Data.(commandpayload.SchemaRenameTypePreviewResult); ok {
		fmt.Printf("%s\n\n", ui.SectionHeader(fmt.Sprintf("Preview: Rename type '%s' to '%s'", data.OldName, data.NewName)))
		fmt.Printf("%s\n", ui.Hint(fmt.Sprintf("Changes to be made (%d total):", data.TotalChanges)))
		printSchemaFileChanges(
			data.Changes,
			func(change schemasvc.TypeRenameChange) string { return change.FilePath },
			func(change schemasvc.TypeRenameChange) int { return change.Line },
			func(change schemasvc.TypeRenameChange) string { return change.Description },
		)
		if data.DefaultPathRenameAvailable != nil && *data.DefaultPathRenameAvailable {
			fmt.Printf("\n%s\n", ui.Hint(fmt.Sprintf("Optional default directory rename (%d changes):", *data.OptionalTotalChanges)))
			fmt.Printf("  %s %s → %s\n", ui.Hint("default_path:"), *data.DefaultPathOld, *data.DefaultPathNew)
			if *data.FilesToMove > 0 {
				fmt.Printf("  %s %d\n", ui.Hint("files to move:"), *data.FilesToMove)
			}
			fmt.Printf("  %s\n", ui.Hint("(add --rename-default-path to apply these optional changes)"))
		}
		fmt.Printf("\n%s\n", ui.Hint("Run with --confirm to apply these changes."))
		return nil
	}

	data, err := commandResultData[commandpayload.SchemaRenameTypeResult](result)
	if err != nil {
		return err
	}

	fmt.Println(ui.Checkf("Renamed type '%s' to '%s'", data.OldName, data.NewName))
	fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Applied %d changes", data.ChangesApplied)))
	if data.DefaultPathRenameAvailable != nil && *data.DefaultPathRenameAvailable {
		if data.DefaultPathRenamed != nil && *data.DefaultPathRenamed {
			fmt.Printf("  %s %s → %s\n", ui.Hint("Renamed default_path"), *data.DefaultPathOld, *data.DefaultPathNew)
			fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Moved %d files and updated references in %d files", *data.FilesMoved, *data.ReferenceFilesUpdated)))
		} else {
			fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Default path remains %s (use --rename-default-path to rename to %s)", *data.DefaultPathOld, *data.DefaultPathNew)))
		}
	}
	fmt.Printf("\n%s.\n", ui.Hint(data.Hint))
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
