package commandimpl

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schemapayload"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/templatesvc"
)

// HandleSchema executes the canonical `schema` command.
func HandleSchema(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	subcommand := strings.TrimSpace(stringArg(req.Args, "subcommand"))
	name := strings.TrimSpace(stringArg(req.Args, "name"))

	if subcommand == "" {
		result, err := schemasvc.FullSchema(req.VaultPath)
		if err != nil {
			return mapSchemaFailure(err)
		}

		data := map[string]interface{}{
			"version": result.Version,
			"types":   result.Types,
			"traits":  result.Traits,
		}
		if len(result.Core) > 0 {
			data["core"] = result.Core
		}
		if len(result.Templates) > 0 {
			data["templates"] = result.Templates
		}
		if len(result.Queries) > 0 {
			data["queries"] = result.Queries
		}
		return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	}

	switch subcommand {
	case "types":
		result, err := schemasvc.Types(req.VaultPath)
		if err != nil {
			return mapSchemaFailure(err)
		}
		data := map[string]interface{}{"types": result.Types}
		if result.Hint != nil {
			data["hint"] = result.Hint
		}
		return commandexec.Success(data, &commandexec.Meta{Count: len(result.Types), QueryTimeMs: time.Since(start).Milliseconds()})
	case "traits":
		result, err := schemasvc.Traits(req.VaultPath)
		if err != nil {
			return mapSchemaFailure(err)
		}
		return commandexec.Success(map[string]interface{}{"traits": result.Traits}, &commandexec.Meta{Count: len(result.Traits), QueryTimeMs: time.Since(start).Milliseconds()})
	case "core":
		if name == "" {
			result, err := schemasvc.CoreList(req.VaultPath)
			if err != nil {
				return mapSchemaFailure(err)
			}
			return commandexec.Success(map[string]interface{}{"core": result.Core}, &commandexec.Meta{Count: len(result.Core), QueryTimeMs: time.Since(start).Milliseconds()})
		}
		result, err := schemasvc.CoreByName(req.VaultPath, name)
		if err != nil {
			return mapSchemaFailure(err)
		}
		return commandexec.Success(map[string]interface{}{"core": result.Core}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	case "type":
		if name == "" {
			return commandexec.Failure("MISSING_ARGUMENT", "specify a type name", nil, "Usage: rvn schema type <name>")
		}
		result, err := schemasvc.TypeByName(req.VaultPath, name)
		if err != nil {
			return mapSchemaFailure(err)
		}
		return commandexec.Success(map[string]interface{}{"type": result.Type}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	case "trait":
		if name == "" {
			return commandexec.Failure("MISSING_ARGUMENT", "specify a trait name", nil, "Usage: rvn schema trait <name>")
		}
		result, err := schemasvc.TraitByName(req.VaultPath, name)
		if err != nil {
			return mapSchemaFailure(err)
		}
		return commandexec.Success(map[string]interface{}{"trait": result.Trait}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	default:
		return commandexec.Failure("INVALID_INPUT", fmt.Sprintf("unknown schema subcommand: %s", subcommand), nil, "Use: types, traits, type <name>, trait <name>, core [name], or template ...")
	}
}

// HandleSchemaValidate executes the canonical `schema_validate` command.
func HandleSchemaValidate(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	result, err := schemasvc.Validate(req.VaultPath)
	if err != nil {
		return mapSchemaFailure(err)
	}
	return commandexec.Success(schemapayload.Validate(result), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaAddType executes the canonical `schema_add_type` command.
func HandleSchemaAddType(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	result, err := schemasvc.AddType(schemasvc.AddTypeRequest{
		VaultPath:   req.VaultPath,
		TypeName:    stringArg(req.Args, "name"),
		DefaultPath: stringArg(req.Args, "default-path"),
		NameField:   stringArg(req.Args, "name-field"),
		Description: stringArg(req.Args, "description"),
	})
	if err != nil {
		return mapSchemaFailure(err)
	}
	return commandexec.Success(schemapayload.AddType(result), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaAddTrait executes the canonical `schema_add_trait` command.
func HandleSchemaAddTrait(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	result, err := schemasvc.AddTrait(schemasvc.AddTraitRequest{
		VaultPath: req.VaultPath,
		TraitName: stringArg(req.Args, "name"),
		TraitType: stringArg(req.Args, "type"),
		Values:    stringArg(req.Args, "values"),
		Default:   stringArg(req.Args, "default"),
	})
	if err != nil {
		return mapSchemaFailure(err)
	}
	return commandexec.Success(schemapayload.AddTrait(result), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaAddField executes the canonical `schema_add_field` command.
func HandleSchemaAddField(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	result, err := schemasvc.AddField(schemasvc.AddFieldRequest{
		VaultPath:   req.VaultPath,
		TypeName:    stringArg(req.Args, "type_name"),
		FieldName:   stringArg(req.Args, "field_name"),
		FieldType:   stringArg(req.Args, "type"),
		Required:    boolArg(req.Args, "required"),
		Default:     stringArg(req.Args, "default"),
		Values:      stringArg(req.Args, "values"),
		Target:      stringArg(req.Args, "target"),
		Description: stringArg(req.Args, "description"),
	})
	if err != nil {
		return mapSchemaFailure(err)
	}
	return commandexec.Success(schemapayload.AddField(result), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaUpdateType executes the canonical `schema_update_type` command.
func HandleSchemaUpdateType(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	name := stringArg(req.Args, "name")
	result, err := schemasvc.UpdateType(schemasvc.UpdateTypeRequest{
		VaultPath:   req.VaultPath,
		TypeName:    name,
		DefaultPath: stringArg(req.Args, "default-path"),
		NameField:   stringArg(req.Args, "name-field"),
		Description: stringArg(req.Args, "description"),
		AddTrait:    stringArg(req.Args, "add-trait"),
		RemoveTrait: stringArg(req.Args, "remove-trait"),
	})
	if err != nil {
		return mapSchemaFailure(err)
	}
	return commandexec.Success(schemapayload.Update("type", name, "", "", result.Changes), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaUpdateTrait executes the canonical `schema_update_trait` command.
func HandleSchemaUpdateTrait(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	name := stringArg(req.Args, "name")
	result, err := schemasvc.UpdateTrait(schemasvc.UpdateTraitRequest{
		VaultPath: req.VaultPath,
		TraitName: name,
		TraitType: stringArg(req.Args, "type"),
		Values:    stringArg(req.Args, "values"),
		Default:   stringArg(req.Args, "default"),
	})
	if err != nil {
		return mapSchemaFailure(err)
	}
	return commandexec.Success(schemapayload.Update("trait", name, "", "", result.Changes), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaUpdateField executes the canonical `schema_update_field` command.
func HandleSchemaUpdateField(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	typeName := stringArg(req.Args, "type_name")
	fieldName := stringArg(req.Args, "field_name")
	result, err := schemasvc.UpdateField(schemasvc.UpdateFieldRequest{
		VaultPath:   req.VaultPath,
		TypeName:    typeName,
		FieldName:   fieldName,
		FieldType:   stringArg(req.Args, "type"),
		Required:    stringArg(req.Args, "required"),
		Default:     stringArg(req.Args, "default"),
		Values:      stringArg(req.Args, "values"),
		Target:      stringArg(req.Args, "target"),
		Description: stringArg(req.Args, "description"),
	})
	if err != nil {
		return mapSchemaFailure(err)
	}
	return commandexec.Success(schemapayload.Update("field", "", typeName, fieldName, result.Changes), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaRemoveType executes the canonical `schema_remove_type` command.
func HandleSchemaRemoveType(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	result, err := schemasvc.RemoveType(schemasvc.RemoveTypeRequest{
		VaultPath:   req.VaultPath,
		TypeName:    stringArg(req.Args, "name"),
		Force:       boolArg(req.Args, "force") || req.Confirm,
		Interactive: false,
	})
	if err != nil {
		return mapSchemaFailure(err)
	}
	data := schemapayload.Remove("type", stringArg(req.Args, "name"), "", "")
	warnings := canonicalSchemaWarnings(result.Warnings)
	if len(warnings) > 0 {
		return commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	}
	return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaRemoveTrait executes the canonical `schema_remove_trait` command.
func HandleSchemaRemoveTrait(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	result, err := schemasvc.RemoveTrait(schemasvc.RemoveTraitRequest{
		VaultPath:   req.VaultPath,
		TraitName:   stringArg(req.Args, "name"),
		Force:       boolArg(req.Args, "force") || req.Confirm,
		Interactive: false,
	})
	if err != nil {
		return mapSchemaFailure(err)
	}
	data := schemapayload.Remove("trait", stringArg(req.Args, "name"), "", "")
	warnings := canonicalSchemaWarnings(result.Warnings)
	if len(warnings) > 0 {
		return commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	}
	return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaRemoveField executes the canonical `schema_remove_field` command.
func HandleSchemaRemoveField(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	typeName := stringArg(req.Args, "type_name")
	fieldName := stringArg(req.Args, "field_name")
	if _, err := schemasvc.RemoveField(schemasvc.RemoveFieldRequest{
		VaultPath: req.VaultPath,
		TypeName:  typeName,
		FieldName: fieldName,
	}); err != nil {
		return mapSchemaFailure(err)
	}
	return commandexec.Success(schemapayload.Remove("field", "", typeName, fieldName), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaRenameType executes the canonical `schema_rename_type` command.
func HandleSchemaRenameType(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	result, err := schemasvc.RenameType(schemasvc.RenameTypeRequest{
		VaultPath:         req.VaultPath,
		OldName:           stringArg(req.Args, "old_name"),
		NewName:           stringArg(req.Args, "new_name"),
		Confirm:           boolArg(req.Args, "confirm") || req.Confirm,
		RenameDefaultPath: boolArg(req.Args, "rename-default-path"),
	})
	if err != nil {
		return mapSchemaFailure(err)
	}
	return commandexec.Success(schemapayload.RenameType(result), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaRenameField executes the canonical `schema_rename_field` command.
func HandleSchemaRenameField(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	result, err := schemasvc.RenameField(schemasvc.RenameFieldRequest{
		VaultPath: req.VaultPath,
		TypeName:  stringArg(req.Args, "type_name"),
		OldField:  stringArg(req.Args, "old_field"),
		NewField:  stringArg(req.Args, "new_field"),
		Confirm:   boolArg(req.Args, "confirm") || req.Confirm,
	})
	if err != nil {
		return mapSchemaFailure(err)
	}
	return commandexec.Success(schemapayload.RenameField(result), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaTemplateList executes the canonical `schema_template_list` command.
func HandleSchemaTemplateList(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	targetKind, scopeKey, scopeValue, hasTarget, failure := schemaTemplateTarget(req.Args, false)
	if failure.Error != nil {
		return failure
	}
	if hasTarget {
		var (
			state *schemasvc.TemplateBindingState
			err   error
		)
		switch targetKind {
		case "type":
			state, err = schemasvc.ListTypeTemplates(req.VaultPath, scopeValue)
		case "core":
			state, err = schemasvc.ListCoreTemplates(req.VaultPath, scopeValue)
		default:
			return commandexec.Failure("INVALID_INPUT", "unknown template target", nil, "")
		}
		if err != nil {
			return mapSchemaFailure(err)
		}
		return commandexec.Success(map[string]interface{}{
			scopeKey:           scopeValue,
			"templates":        state.Templates,
			"default_template": state.DefaultTemplate,
		}, &commandexec.Meta{Count: len(state.Templates), QueryTimeMs: time.Since(start).Milliseconds()})
	}

	items, err := schemasvc.ListTemplates(req.VaultPath)
	if err != nil {
		return mapSchemaFailure(err)
	}
	return commandexec.Success(map[string]interface{}{"templates": items}, &commandexec.Meta{Count: len(items), QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaTemplateGet executes the canonical `schema_template_get` command.
func HandleSchemaTemplateGet(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	item, err := schemasvc.GetTemplate(req.VaultPath, stringArg(req.Args, "template_id"))
	if err != nil {
		return mapSchemaFailure(err)
	}
	return commandexec.Success(schemaTemplateDefinitionPayload(item.ID, item.File, item.Description), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaTemplateSet executes the canonical `schema_template_set` command.
func HandleSchemaTemplateSet(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	description := stringArg(req.Args, "description")
	item, err := schemasvc.SetTemplate(schemasvc.SetTemplateRequest{
		VaultPath:   req.VaultPath,
		TemplateID:  stringArg(req.Args, "template_id"),
		File:        stringArg(req.Args, "file"),
		Description: description,
	})
	if err != nil {
		return mapSchemaFailure(err)
	}
	return commandexec.Success(schemaTemplateDefinitionPayload(item.ID, item.File, strings.TrimSpace(description)), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaTemplateRemove executes the canonical `schema_template_remove` command.
func HandleSchemaTemplateRemove(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	templateID := strings.TrimSpace(stringArg(req.Args, "template_id"))
	if err := schemasvc.RemoveTemplate(req.VaultPath, templateID); err != nil {
		return mapSchemaFailure(err)
	}
	return commandexec.Success(map[string]interface{}{"removed": true, "id": templateID}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaTemplateBind executes the canonical `schema_template_bind` command.
func HandleSchemaTemplateBind(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	targetKind, scopeKey, scopeValue, _, failure := schemaTemplateTarget(req.Args, true)
	if failure.Error != nil {
		return failure
	}

	templateID := strings.TrimSpace(stringArg(req.Args, "template_id"))
	setDefault := boolArg(req.Args, "default")

	var (
		result *schemasvc.AddTemplateBindingResult
		err    error
	)
	switch targetKind {
	case "type":
		result, err = schemasvc.AddTypeTemplate(req.VaultPath, scopeValue, templateID)
		if err == nil && setDefault {
			_, err = schemasvc.SetTypeDefaultTemplate(req.VaultPath, scopeValue, templateID, false)
		}
	case "core":
		result, err = schemasvc.AddCoreTemplate(req.VaultPath, scopeValue, templateID)
		if err == nil && setDefault {
			_, err = schemasvc.SetCoreDefaultTemplate(req.VaultPath, scopeValue, templateID, false)
		}
	default:
		return commandexec.Failure("INVALID_INPUT", "unknown template target", nil, "")
	}
	if err != nil {
		return mapSchemaFailure(err)
	}

	data := map[string]interface{}{
		scopeKey:      scopeValue,
		"template_id": templateID,
	}
	if result != nil && result.AlreadySet {
		data["already_set"] = true
		data["default_match"] = result.DefaultMatch
	}
	if setDefault {
		data["default_template"] = templateID
	}
	return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaTemplateUnbind executes the canonical `schema_template_unbind` command.
func HandleSchemaTemplateUnbind(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	targetKind, scopeKey, scopeValue, _, failure := schemaTemplateTarget(req.Args, true)
	if failure.Error != nil {
		return failure
	}

	templateID := strings.TrimSpace(stringArg(req.Args, "template_id"))
	clearDefault := boolArg(req.Args, "clear-default")
	var err error
	switch targetKind {
	case "type":
		err = schemasvc.RemoveTypeTemplate(req.VaultPath, scopeValue, templateID, clearDefault)
	case "core":
		err = schemasvc.RemoveCoreTemplate(req.VaultPath, scopeValue, templateID, clearDefault)
	default:
		return commandexec.Failure("INVALID_INPUT", "unknown template target", nil, "")
	}
	if err != nil {
		return mapSchemaFailure(err)
	}

	data := map[string]interface{}{
		scopeKey:      scopeValue,
		"template_id": templateID,
		"removed":     true,
	}
	if clearDefault {
		data["default_cleared"] = true
	}
	return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaTemplateDefault executes the canonical `schema_template_default` command.
func HandleSchemaTemplateDefault(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	targetKind, scopeKey, scopeValue, _, failure := schemaTemplateTarget(req.Args, true)
	if failure.Error != nil {
		return failure
	}

	templateID := strings.TrimSpace(stringArg(req.Args, "template_id"))
	clearDefault := boolArg(req.Args, "clear")

	var (
		newDefault string
		err        error
	)
	switch targetKind {
	case "type":
		newDefault, err = schemasvc.SetTypeDefaultTemplate(req.VaultPath, scopeValue, templateID, clearDefault)
	case "core":
		newDefault, err = schemasvc.SetCoreDefaultTemplate(req.VaultPath, scopeValue, templateID, clearDefault)
	default:
		return commandexec.Failure("INVALID_INPUT", "unknown template target", nil, "")
	}
	if err != nil {
		return mapSchemaFailure(err)
	}

	return commandexec.Success(map[string]interface{}{
		scopeKey:           scopeValue,
		"default_template": newDefault,
	}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleTemplateList executes the canonical `template_list` command.
func HandleTemplateList(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	vaultCfg, failure := loadVaultConfigResult(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	result, err := templatesvc.List(templatesvc.ListRequest{
		VaultPath:   req.VaultPath,
		TemplateDir: vaultCfg.GetTemplateDirectory(),
	})
	if err != nil {
		return mapTemplateFailure(err)
	}

	return commandexec.Success(map[string]interface{}{
		"template_dir": result.TemplateDir,
		"templates":    result.Templates,
	}, &commandexec.Meta{Count: len(result.Templates), QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleTemplateWrite executes the canonical `template_write` command.
func HandleTemplateWrite(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	vaultCfg, failure := loadVaultConfigResult(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	result, err := templatesvc.Write(templatesvc.WriteRequest{
		VaultPath:   req.VaultPath,
		TemplateDir: vaultCfg.GetTemplateDirectory(),
		Path:        stringArg(req.Args, "path"),
		Content:     stringArg(req.Args, "content"),
	})
	if err != nil {
		return mapTemplateFailure(err)
	}

	if result.Changed && result.ChangedPath != "" {
		autoReindexFile(req.VaultPath, filepath.Clean(result.ChangedPath), vaultCfg)
	}

	return commandexec.Success(map[string]interface{}{
		"path":         result.Path,
		"status":       result.Status,
		"template_dir": result.TemplateDir,
	}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleTemplateDelete executes the canonical `template_delete` command.
func HandleTemplateDelete(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	vaultCfg, failure := loadVaultConfigResult(req.VaultPath)
	if failure.Error != nil {
		return failure
	}

	result, err := templatesvc.Delete(templatesvc.DeleteRequest{
		VaultPath:   req.VaultPath,
		TemplateDir: vaultCfg.GetTemplateDirectory(),
		Path:        stringArg(req.Args, "path"),
		Force:       boolArg(req.Args, "force"),
	})
	if err != nil {
		return mapTemplateFailure(err)
	}

	data := map[string]interface{}{
		"deleted":      result.DeletedPath,
		"trash_path":   result.TrashPath,
		"forced":       result.Forced,
		"template_ids": result.TemplateIDs,
	}
	warnings := canonicalTemplateWarnings(result.Warnings)
	if len(warnings) > 0 {
		return commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	}
	return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

func mapSchemaFailure(err error) commandexec.Result {
	var svcErr *schemasvc.Error
	if errors.As(err, &svcErr) {
		return commandexec.Failure(string(svcErr.Code), svcErr.Message, svcErr.Details, svcErr.Suggestion)
	}
	return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
}

func mapTemplateFailure(err error) commandexec.Result {
	svcErr, ok := templatesvc.AsError(err)
	if !ok {
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
	}
	return commandexec.Failure(string(svcErr.Code), svcErr.Message, nil, svcErr.Suggestion)
}

func canonicalSchemaWarnings(serviceWarnings []schemasvc.Warning) []commandexec.Warning {
	return schemapayload.MapWarnings(serviceWarnings, func(code, message string) commandexec.Warning {
		return commandexec.Warning{Code: code, Message: message}
	})
}

func canonicalTemplateWarnings(serviceWarnings []templatesvc.Warning) []commandexec.Warning {
	if len(serviceWarnings) == 0 {
		return nil
	}
	warnings := make([]commandexec.Warning, 0, len(serviceWarnings))
	for _, warning := range serviceWarnings {
		warnings = append(warnings, commandexec.Warning{
			Code:    warning.Code,
			Message: warning.Message,
			Ref:     warning.Ref,
		})
	}
	return warnings
}

func loadVaultConfigResult(vaultPath string) (*config.VaultConfig, commandexec.Result) {
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return nil, commandexec.Failure("CONFIG_INVALID", "failed to load vault config", nil, "Fix raven.yaml and try again")
	}
	return vaultCfg, commandexec.Result{}
}

func schemaTemplateTarget(args map[string]interface{}, requireTarget bool) (string, string, string, bool, commandexec.Result) {
	typeName := strings.TrimSpace(stringArg(args, "type"))
	coreType := strings.TrimSpace(stringArg(args, "core"))

	switch {
	case typeName != "" && coreType != "":
		return "", "", "", false, commandexec.Failure("INVALID_INPUT", "specify exactly one of --type or --core", nil, "")
	case typeName != "":
		return "type", "type", typeName, true, commandexec.Result{}
	case coreType != "":
		return "core", "core", coreType, true, commandexec.Result{}
	case requireTarget:
		return "", "", "", false, commandexec.Failure("MISSING_ARGUMENT", "specify --type or --core", nil, "")
	default:
		return "", "", "", false, commandexec.Result{}
	}
}

func schemaTemplateDefinitionPayload(id, file, description string) map[string]interface{} {
	return map[string]interface{}{
		"id":          id,
		"file":        file,
		"description": description,
	}
}
