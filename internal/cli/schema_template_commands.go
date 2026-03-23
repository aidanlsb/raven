package cli

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/schemasvc"
)

var (
	schemaTemplateFileFlag         string
	schemaTemplateDescriptionFlag  string
	schemaTemplateTypeFlag         string
	schemaTemplateCoreFlag         string
	schemaTemplateBindDefaultFlag  bool
	schemaTemplateDefaultClearFlag bool
	schemaTemplateUnbindClearFlag  bool
)

type schemaTemplateTarget struct {
	kind string
	name string
}

var schemaTemplateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage schema templates and bindings",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var schemaTemplateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List schema templates or target bindings",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		vaultPath := getVaultPath()

		target, err := resolveSchemaTemplateTarget(false)
		if err != nil {
			return err
		}
		if target == nil {
			return schemaTemplateListDefinitions(vaultPath, start)
		}
		return schemaTemplateListBindings(vaultPath, target.kind, target.name, start)
	},
}

var schemaTemplateGetCmd = &cobra.Command{
	Use:   "get <template_id>",
	Short: "Show a schema template definition",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return schemaTemplateGet(getVaultPath(), args[0], time.Now())
	},
}

var schemaTemplateSetCmd = &cobra.Command{
	Use:   "set <template_id>",
	Short: "Create or update a schema template definition",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return schemaTemplateSet(getVaultPath(), args[0], time.Now())
	},
}

var schemaTemplateRemoveCmd = &cobra.Command{
	Use:   "remove <template_id>",
	Short: "Remove a schema template definition",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return schemaTemplateRemove(getVaultPath(), args[0], time.Now())
	},
}

var schemaTemplateBindCmd = &cobra.Command{
	Use:   "bind <template_id>",
	Short: "Bind a template to a type or core type",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := resolveSchemaTemplateTarget(true)
		if err != nil {
			return err
		}
		return schemaTemplateBindTarget(getVaultPath(), target.kind, target.name, args[0], schemaTemplateBindDefaultFlag, time.Now())
	},
}

var schemaTemplateUnbindCmd = &cobra.Command{
	Use:   "unbind <template_id>",
	Short: "Unbind a template from a type or core type",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := resolveSchemaTemplateTarget(true)
		if err != nil {
			return err
		}
		return schemaTemplateUnbindTarget(getVaultPath(), target.kind, target.name, args[0], schemaTemplateUnbindClearFlag, time.Now())
	},
}

var schemaTemplateDefaultCmd = &cobra.Command{
	Use:   "default [template_id]",
	Short: "Set or clear the default template for a type or core type",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := resolveSchemaTemplateTarget(true)
		if err != nil {
			return err
		}
		templateID := ""
		if len(args) > 0 {
			templateID = args[0]
		}
		return schemaTemplateSetDefaultTarget(getVaultPath(), target.kind, target.name, templateID, schemaTemplateDefaultClearFlag, time.Now())
	},
}

func resolveSchemaTemplateTarget(required bool) (*schemaTemplateTarget, error) {
	typeName := strings.TrimSpace(schemaTemplateTypeFlag)
	coreType := strings.TrimSpace(schemaTemplateCoreFlag)

	switch {
	case typeName != "" && coreType != "":
		return nil, handleErrorMsg(ErrInvalidInput, "specify exactly one of --type or --core", "")
	case typeName != "":
		return &schemaTemplateTarget{kind: "type", name: typeName}, nil
	case coreType != "":
		return &schemaTemplateTarget{kind: "core", name: coreType}, nil
	case required:
		return nil, handleErrorMsg(ErrMissingArgument, "specify --type or --core", "")
	default:
		return nil, nil
	}
}

func schemaTemplateListDefinitions(vaultPath string, start time.Time) error {
	result := executeCanonicalCommand("schema_template_list", vaultPath, nil)
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)
	items, err := decodeSchemaValue[[]schemasvc.TemplateSchema](data["templates"])
	if err != nil {
		return err
	}

	if len(items) == 0 {
		fmt.Println("No schema templates configured.")
		return nil
	}
	fmt.Println("Schema templates:")
	for _, it := range items {
		if it.Description != "" {
			fmt.Printf("  %s -> %s (%s)\n", it.ID, it.File, it.Description)
		} else {
			fmt.Printf("  %s -> %s\n", it.ID, it.File)
		}
	}
	return nil
}

func schemaTemplateListBindings(vaultPath, kind, name string, start time.Time) error {
	args := map[string]interface{}{kind: name}
	result := executeCanonicalCommand("schema_template_list", vaultPath, args)
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)
	templates, err := decodeSchemaValue[[]string](data["templates"])
	if err != nil {
		return err
	}
	defaultTemplate, _ := data["default_template"].(string)

	label := schemaTemplateKindLabel(kind)
	fmt.Printf("%s templates for %s:\n", label, name)
	if len(templates) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, templateID := range templates {
			fmt.Printf("  - %s\n", templateID)
		}
	}
	if defaultTemplate != "" {
		fmt.Printf("Default: %s\n", defaultTemplate)
	} else {
		fmt.Println("Default: (none)")
	}
	return nil
}

func schemaTemplateGet(vaultPath, templateID string, start time.Time) error {
	result := executeCanonicalCommand("schema_template_get", vaultPath, map[string]interface{}{"template_id": templateID})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)
	id, _ := data["id"].(string)
	file, _ := data["file"].(string)
	description, _ := data["description"].(string)
	fmt.Printf("Template: %s\n", id)
	fmt.Printf("  File: %s\n", file)
	if description != "" {
		fmt.Printf("  Description: %s\n", description)
	}
	return nil
}

func schemaTemplateSet(vaultPath, templateID string, start time.Time) error {
	result := executeCanonicalCommand("schema_template_set", vaultPath, map[string]interface{}{
		"template_id": templateID,
		"file":        schemaTemplateFileFlag,
		"description": schemaTemplateDescriptionFlag,
	})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)
	fmt.Printf("Set schema template %s -> %s\n", data["id"], data["file"])
	return nil
}

func schemaTemplateRemove(vaultPath, templateID string, start time.Time) error {
	result := executeCanonicalCommand("schema_template_remove", vaultPath, map[string]interface{}{"template_id": templateID})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	fmt.Printf("Removed schema template %s\n", strings.TrimSpace(templateID))
	return nil
}

func schemaTemplateBindTarget(vaultPath, kind, name, templateID string, setDefault bool, start time.Time) error {
	args := map[string]interface{}{
		"template_id": templateID,
		kind:          name,
		"default":     setDefault,
	}
	result := executeCanonicalCommand("schema_template_bind", vaultPath, args)
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)
	if boolValue(data["already_set"]) {
		fmt.Printf("%s %s already includes template %s\n", schemaTemplateKindLabel(kind), name, strings.TrimSpace(templateID))
		if setDefault {
			fmt.Printf("Set default template for %s %s -> %s\n", kind, name, strings.TrimSpace(templateID))
		}
		return nil
	}

	fmt.Printf("Bound template %s to %s %s\n", strings.TrimSpace(templateID), kind, name)
	if setDefault {
		fmt.Printf("Set default template for %s %s -> %s\n", kind, name, strings.TrimSpace(templateID))
	}
	return nil
}

func schemaTemplateUnbindTarget(vaultPath, kind, name, templateID string, clearDefault bool, start time.Time) error {
	result := executeCanonicalCommand("schema_template_unbind", vaultPath, map[string]interface{}{
		"template_id":   templateID,
		kind:            name,
		"clear-default": clearDefault,
	})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	if clearDefault {
		fmt.Printf("Cleared default template and unbound %s from %s %s\n", strings.TrimSpace(templateID), kind, name)
		return nil
	}
	fmt.Printf("Unbound template %s from %s %s\n", strings.TrimSpace(templateID), kind, name)
	return nil
}

func schemaTemplateSetDefaultTarget(vaultPath, kind, name, templateID string, clearDefault bool, start time.Time) error {
	result := executeCanonicalCommand("schema_template_default", vaultPath, map[string]interface{}{
		"template_id": templateID,
		kind:          name,
		"clear":       clearDefault,
	})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)
	newDefault, _ := data["default_template"].(string)
	if clearDefault {
		fmt.Printf("Cleared default template for %s %s\n", kind, name)
		return nil
	}
	fmt.Printf("Set default template for %s %s -> %s\n", kind, name, newDefault)
	return nil
}

func schemaTemplateKindLabel(kind string) string {
	if kind == "" {
		return kind
	}
	runes := []rune(kind)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func init() {
	schemaTemplateSetCmd.Flags().StringVar(&schemaTemplateFileFlag, "file", "", "Template file path under directories.template")
	schemaTemplateSetCmd.Flags().StringVar(&schemaTemplateDescriptionFlag, "description", "", "Template description (use '-' to clear)")

	for _, cmd := range []*cobra.Command{schemaTemplateListCmd, schemaTemplateBindCmd, schemaTemplateUnbindCmd, schemaTemplateDefaultCmd} {
		cmd.Flags().StringVar(&schemaTemplateTypeFlag, "type", "", "Target schema type")
		cmd.Flags().StringVar(&schemaTemplateCoreFlag, "core", "", "Target core type (date or page)")
	}

	schemaTemplateBindCmd.Flags().BoolVar(&schemaTemplateBindDefaultFlag, "default", false, "Also set this template as the default for the target")
	schemaTemplateUnbindCmd.Flags().BoolVar(&schemaTemplateUnbindClearFlag, "clear-default", false, "Allow unbinding the current default template by clearing the default first")
	schemaTemplateDefaultCmd.Flags().BoolVar(&schemaTemplateDefaultClearFlag, "clear", false, "Clear the target default template")

	schemaTemplateCmd.AddCommand(schemaTemplateListCmd)
	schemaTemplateCmd.AddCommand(schemaTemplateGetCmd)
	schemaTemplateCmd.AddCommand(schemaTemplateSetCmd)
	schemaTemplateCmd.AddCommand(schemaTemplateRemoveCmd)
	schemaTemplateCmd.AddCommand(schemaTemplateBindCmd)
	schemaTemplateCmd.AddCommand(schemaTemplateUnbindCmd)
	schemaTemplateCmd.AddCommand(schemaTemplateDefaultCmd)
	schemaCmd.AddCommand(schemaTemplateCmd)
}
