package cli

import (
	"errors"
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
	items, err := schemasvc.ListTemplates(vaultPath)
	if err != nil {
		return mapSchemaTemplateServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()
	if isJSONOutput() {
		outputSuccess(map[string]interface{}{"templates": items}, &Meta{Count: len(items), QueryTimeMs: elapsed})
		return nil
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
	var (
		state *schemasvc.TemplateBindingState
		err   error
	)
	switch kind {
	case "type":
		state, err = schemasvc.ListTypeTemplates(vaultPath, name)
	case "core":
		state, err = schemasvc.ListCoreTemplates(vaultPath, name)
	default:
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown template target kind: %s", kind), "")
	}
	if err != nil {
		return mapSchemaTemplateServiceError(err)
	}

	key := schemaTemplateTargetKey(kind)
	elapsed := time.Since(start).Milliseconds()
	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			key:                name,
			"templates":        state.Templates,
			"default_template": state.DefaultTemplate,
		}, &Meta{Count: len(state.Templates), QueryTimeMs: elapsed})
		return nil
	}

	label := schemaTemplateKindLabel(kind)
	fmt.Printf("%s templates for %s:\n", label, name)
	if len(state.Templates) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, templateID := range state.Templates {
			fmt.Printf("  - %s\n", templateID)
		}
	}
	if state.DefaultTemplate != "" {
		fmt.Printf("Default: %s\n", state.DefaultTemplate)
	} else {
		fmt.Println("Default: (none)")
	}
	return nil
}

func schemaTemplateGet(vaultPath, templateID string, start time.Time) error {
	templateDef, err := schemasvc.GetTemplate(vaultPath, templateID)
	if err != nil {
		return mapSchemaTemplateServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()
	result := map[string]interface{}{
		"id":          templateDef.ID,
		"file":        templateDef.File,
		"description": templateDef.Description,
	}
	if isJSONOutput() {
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}
	fmt.Printf("Template: %s\n", templateDef.ID)
	fmt.Printf("  File: %s\n", templateDef.File)
	if templateDef.Description != "" {
		fmt.Printf("  Description: %s\n", templateDef.Description)
	}
	return nil
}

func schemaTemplateSet(vaultPath, templateID string, start time.Time) error {
	templateDef, err := schemasvc.SetTemplate(schemasvc.SetTemplateRequest{
		VaultPath:   vaultPath,
		TemplateID:  templateID,
		File:        schemaTemplateFileFlag,
		Description: schemaTemplateDescriptionFlag,
	})
	if err != nil {
		return mapSchemaTemplateServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()
	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"id":          templateDef.ID,
			"file":        templateDef.File,
			"description": strings.TrimSpace(schemaTemplateDescriptionFlag),
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}
	fmt.Printf("Set schema template %s -> %s\n", templateDef.ID, templateDef.File)
	return nil
}

func schemaTemplateRemove(vaultPath, templateID string, start time.Time) error {
	if err := schemasvc.RemoveTemplate(vaultPath, templateID); err != nil {
		return mapSchemaTemplateServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()
	if isJSONOutput() {
		outputSuccess(map[string]interface{}{"removed": true, "id": strings.TrimSpace(templateID)}, &Meta{QueryTimeMs: elapsed})
		return nil
	}
	fmt.Printf("Removed schema template %s\n", strings.TrimSpace(templateID))
	return nil
}

func schemaTemplateBindTarget(vaultPath, kind, name, templateID string, setDefault bool, start time.Time) error {
	var (
		result *schemasvc.AddTemplateBindingResult
		err    error
	)
	switch kind {
	case "type":
		result, err = schemasvc.AddTypeTemplate(vaultPath, name, templateID)
	case "core":
		result, err = schemasvc.AddCoreTemplate(vaultPath, name, templateID)
	default:
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown template target kind: %s", kind), "")
	}
	if err != nil {
		return mapSchemaTemplateServiceError(err)
	}

	if setDefault {
		switch kind {
		case "type":
			_, err = schemasvc.SetTypeDefaultTemplate(vaultPath, name, templateID, false)
		case "core":
			_, err = schemasvc.SetCoreDefaultTemplate(vaultPath, name, templateID, false)
		}
		if err != nil {
			return mapSchemaTemplateServiceError(err)
		}
	}

	key := schemaTemplateTargetKey(kind)
	elapsed := time.Since(start).Milliseconds()
	payload := map[string]interface{}{
		key:           name,
		"template_id": strings.TrimSpace(templateID),
	}
	if result != nil && result.AlreadySet {
		payload["already_set"] = true
		payload["default_match"] = result.DefaultMatch
	}
	if setDefault {
		payload["default_template"] = strings.TrimSpace(templateID)
	}
	if isJSONOutput() {
		outputSuccess(payload, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	if result != nil && result.AlreadySet {
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
	var err error
	switch kind {
	case "type":
		err = schemasvc.RemoveTypeTemplate(vaultPath, name, templateID, clearDefault)
	case "core":
		err = schemasvc.RemoveCoreTemplate(vaultPath, name, templateID, clearDefault)
	default:
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown template target kind: %s", kind), "")
	}
	if err != nil {
		return mapSchemaTemplateServiceError(err)
	}

	key := schemaTemplateTargetKey(kind)
	elapsed := time.Since(start).Milliseconds()
	payload := map[string]interface{}{
		key:           name,
		"template_id": strings.TrimSpace(templateID),
		"removed":     true,
	}
	if clearDefault {
		payload["default_cleared"] = true
	}
	if isJSONOutput() {
		outputSuccess(payload, &Meta{QueryTimeMs: elapsed})
		return nil
	}
	if clearDefault {
		fmt.Printf("Cleared default template and unbound %s from %s %s\n", strings.TrimSpace(templateID), kind, name)
		return nil
	}
	fmt.Printf("Unbound template %s from %s %s\n", strings.TrimSpace(templateID), kind, name)
	return nil
}

func schemaTemplateSetDefaultTarget(vaultPath, kind, name, templateID string, clearDefault bool, start time.Time) error {
	var (
		newDefault string
		err        error
	)
	switch kind {
	case "type":
		newDefault, err = schemasvc.SetTypeDefaultTemplate(vaultPath, name, templateID, clearDefault)
	case "core":
		newDefault, err = schemasvc.SetCoreDefaultTemplate(vaultPath, name, templateID, clearDefault)
	default:
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown template target kind: %s", kind), "")
	}
	if err != nil {
		return mapSchemaTemplateServiceError(err)
	}

	key := schemaTemplateTargetKey(kind)
	elapsed := time.Since(start).Milliseconds()
	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			key:                name,
			"default_template": newDefault,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}
	if clearDefault {
		fmt.Printf("Cleared default template for %s %s\n", kind, name)
		return nil
	}
	fmt.Printf("Set default template for %s %s -> %s\n", kind, name, newDefault)
	return nil
}

func schemaTemplateTargetKey(kind string) string {
	if kind == "core" {
		return "core_type"
	}
	return "type"
}

func schemaTemplateKindLabel(kind string) string {
	if kind == "" {
		return kind
	}
	runes := []rune(kind)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func mapSchemaTemplateServiceError(err error) error {
	var svcErr *schemasvc.Error
	if errors.As(err, &svcErr) {
		return handleErrorMsg(string(svcErr.Code), svcErr.Message, svcErr.Suggestion)
	}
	return handleError(ErrInternal, err, "")
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
