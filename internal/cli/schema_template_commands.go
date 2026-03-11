package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/schemasvc"
)

var (
	schemaTemplateFileFlag        string
	schemaTemplateDescriptionFlag string
	schemaTypeTemplateClearFlag   bool
)

func runSchemaTemplateCommand(vaultPath string, args []string, start time.Time) error {
	if len(args) == 0 {
		return handleErrorMsg(ErrMissingArgument, "missing schema template subcommand", "Use: rvn schema template list|get|set|remove ...")
	}

	switch args[0] {
	case "list":
		return schemaTemplateList(vaultPath, start)
	case "get":
		if len(args) != 2 {
			return handleErrorMsg(ErrInvalidInput, "template get requires template_id", "Use: rvn schema template get <template_id>")
		}
		return schemaTemplateGet(vaultPath, args[1], start)
	case "set":
		if len(args) != 2 {
			return handleErrorMsg(ErrInvalidInput, "template set requires template_id", "Use: rvn schema template set <template_id> --file <path>")
		}
		return schemaTemplateSet(vaultPath, args[1], start)
	case "remove":
		if len(args) != 2 {
			return handleErrorMsg(ErrInvalidInput, "template remove requires template_id", "Use: rvn schema template remove <template_id>")
		}
		return schemaTemplateRemove(vaultPath, args[1], start)
	default:
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema template subcommand: %s", args[0]), "Use: list, get, set, or remove")
	}
}

func schemaTemplateList(vaultPath string, start time.Time) error {
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

func runSchemaTypeTemplateCommand(vaultPath, typeName string, args []string, start time.Time) error {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return handleErrorMsg(ErrInvalidInput, "type_name cannot be empty", "")
	}
	if len(args) == 0 {
		return handleErrorMsg(ErrMissingArgument, "missing type template subcommand", "Use: rvn schema type <type_name> template list|set|remove|default ...")
	}

	switch args[0] {
	case "list":
		state, err := schemasvc.ListTypeTemplates(vaultPath, typeName)
		if err != nil {
			return mapSchemaTemplateServiceError(err)
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"type":             typeName,
				"templates":        state.Templates,
				"default_template": state.DefaultTemplate,
			}, &Meta{Count: len(state.Templates), QueryTimeMs: elapsed})
			return nil
		}

		fmt.Printf("Type templates for %s:\n", typeName)
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
	case "set":
		if len(args) != 2 {
			return handleErrorMsg(ErrInvalidInput, "set requires template_id", "Use: rvn schema type <type_name> template set <template_id>")
		}

		templateID := strings.TrimSpace(args[1])
		result, err := schemasvc.AddTypeTemplate(vaultPath, typeName, templateID)
		if err != nil {
			return mapSchemaTemplateServiceError(err)
		}

		elapsed := time.Since(start).Milliseconds()
		if result.AlreadySet {
			if isJSONOutput() {
				outputSuccess(map[string]interface{}{
					"type":          typeName,
					"template_id":   templateID,
					"already_set":   true,
					"default_match": result.DefaultMatch,
				}, &Meta{QueryTimeMs: elapsed})
				return nil
			}
			fmt.Printf("Type %s already includes template %s\n", typeName, templateID)
			return nil
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"type":        typeName,
				"template_id": templateID,
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		fmt.Printf("Added template %s to type %s\n", templateID, typeName)
		return nil
	case "remove":
		if len(args) != 2 {
			return handleErrorMsg(ErrInvalidInput, "remove requires template_id", "Use: rvn schema type <type_name> template remove <template_id>")
		}
		templateID := strings.TrimSpace(args[1])

		if err := schemasvc.RemoveTypeTemplate(vaultPath, typeName, templateID); err != nil {
			return mapSchemaTemplateServiceError(err)
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{"type": typeName, "template_id": templateID, "removed": true}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		fmt.Printf("Removed template %s from type %s\n", templateID, typeName)
		return nil
	case "default":
		var (
			templateID string
			err        error
		)

		if schemaTypeTemplateClearFlag {
			templateID, err = schemasvc.SetTypeDefaultTemplate(vaultPath, typeName, "", true)
		} else {
			if len(args) != 2 {
				return handleErrorMsg(ErrInvalidInput, "default requires template_id or --clear", "Use: rvn schema type <type_name> template default <template_id> OR --clear")
			}
			templateID, err = schemasvc.SetTypeDefaultTemplate(vaultPath, typeName, args[1], false)
		}
		if err != nil {
			return mapSchemaTemplateServiceError(err)
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{"type": typeName, "default_template": templateID}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		if schemaTypeTemplateClearFlag {
			fmt.Printf("Cleared default template for type %s\n", typeName)
			return nil
		}
		fmt.Printf("Set default template for type %s -> %s\n", typeName, templateID)
		return nil
	default:
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown type template subcommand: %s", args[0]), "Use: list, set, remove, or default")
	}
}

func runSchemaCoreTemplateCommand(vaultPath, coreTypeName string, args []string, start time.Time) error {
	coreTypeName = strings.TrimSpace(coreTypeName)
	if coreTypeName == "" {
		return handleErrorMsg(ErrInvalidInput, "core_type cannot be empty", "")
	}
	if len(args) == 0 {
		return handleErrorMsg(ErrMissingArgument, "missing core template subcommand", "Use: rvn schema core <core_type> template list|set|remove|default ...")
	}

	switch args[0] {
	case "list":
		state, err := schemasvc.ListCoreTemplates(vaultPath, coreTypeName)
		if err != nil {
			return mapSchemaTemplateServiceError(err)
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"core_type":        coreTypeName,
				"templates":        state.Templates,
				"default_template": state.DefaultTemplate,
			}, &Meta{Count: len(state.Templates), QueryTimeMs: elapsed})
			return nil
		}

		fmt.Printf("Core templates for %s:\n", coreTypeName)
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
	case "set":
		if len(args) != 2 {
			return handleErrorMsg(ErrInvalidInput, "set requires template_id", "Use: rvn schema core <core_type> template set <template_id>")
		}
		templateID := strings.TrimSpace(args[1])

		result, err := schemasvc.AddCoreTemplate(vaultPath, coreTypeName, templateID)
		if err != nil {
			return mapSchemaTemplateServiceError(err)
		}

		elapsed := time.Since(start).Milliseconds()
		if result.AlreadySet {
			if isJSONOutput() {
				outputSuccess(map[string]interface{}{
					"core_type":     coreTypeName,
					"template_id":   templateID,
					"already_set":   true,
					"default_match": result.DefaultMatch,
				}, &Meta{QueryTimeMs: elapsed})
				return nil
			}
			fmt.Printf("Core type %s already includes template %s\n", coreTypeName, templateID)
			return nil
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"core_type":   coreTypeName,
				"template_id": templateID,
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		fmt.Printf("Added template %s to core type %s\n", templateID, coreTypeName)
		return nil
	case "remove":
		if len(args) != 2 {
			return handleErrorMsg(ErrInvalidInput, "remove requires template_id", "Use: rvn schema core <core_type> template remove <template_id>")
		}
		templateID := strings.TrimSpace(args[1])

		if err := schemasvc.RemoveCoreTemplate(vaultPath, coreTypeName, templateID); err != nil {
			return mapSchemaTemplateServiceError(err)
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{"core_type": coreTypeName, "template_id": templateID, "removed": true}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		fmt.Printf("Removed template %s from core type %s\n", templateID, coreTypeName)
		return nil
	case "default":
		var (
			templateID string
			err        error
		)
		if schemaTypeTemplateClearFlag {
			templateID, err = schemasvc.SetCoreDefaultTemplate(vaultPath, coreTypeName, "", true)
		} else {
			if len(args) != 2 {
				return handleErrorMsg(ErrInvalidInput, "default requires template_id or --clear", "Use: rvn schema core <core_type> template default <template_id> OR --clear")
			}
			templateID, err = schemasvc.SetCoreDefaultTemplate(vaultPath, coreTypeName, args[1], false)
		}
		if err != nil {
			return mapSchemaTemplateServiceError(err)
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{"core_type": coreTypeName, "default_template": templateID}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		if schemaTypeTemplateClearFlag {
			fmt.Printf("Cleared default template for core type %s\n", coreTypeName)
			return nil
		}
		fmt.Printf("Set default template for core type %s -> %s\n", coreTypeName, templateID)
		return nil
	default:
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown core template subcommand: %s", args[0]), "Use: list, set, remove, or default")
	}
}

func mapSchemaTemplateServiceError(err error) error {
	var svcErr *schemasvc.Error
	if errors.As(err, &svcErr) {
		return handleErrorMsg(string(svcErr.Code), svcErr.Message, svcErr.Suggestion)
	}
	return handleError(ErrInternal, err, "")
}
