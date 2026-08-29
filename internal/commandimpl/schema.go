package commandimpl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/templatesvc"
)

// HandleSchema executes the canonical `schema` command.
func HandleSchema(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	subcommand := strings.TrimSpace(stringArg(req.Args, "subcommand"))
	name := strings.TrimSpace(stringArg(req.Args, "name"))
	runtimeBuilder := newSchemaMutationCommandVaultRuntime
	if subcommand == "" {
		runtimeBuilder = newSchemaFirstCommandVaultRuntime
	}
	rt, failure := runtimeBuilder(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	if subcommand == "" {
		result, err := schemasvc.FullSchema(rt)
		if err != nil {
			return commandexec.FromServiceError(err)
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
		result, err := schemasvc.Types(rt)
		if err != nil {
			return commandexec.FromServiceError(err)
		}
		data := map[string]interface{}{"types": result.Types}
		if result.Hint != nil {
			data["hint"] = result.Hint
		}
		return commandexec.Success(data, &commandexec.Meta{Count: len(result.Types), QueryTimeMs: time.Since(start).Milliseconds()})
	case "traits":
		result, err := schemasvc.Traits(rt)
		if err != nil {
			return commandexec.FromServiceError(err)
		}
		return commandexec.Success(map[string]interface{}{"traits": result.Traits}, &commandexec.Meta{Count: len(result.Traits), QueryTimeMs: time.Since(start).Milliseconds()})
	case "core":
		if name == "" {
			result, err := schemasvc.CoreList(rt)
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(map[string]interface{}{"core": result.Core}, &commandexec.Meta{Count: len(result.Core), QueryTimeMs: time.Since(start).Milliseconds()})
		}
		result, err := schemasvc.CoreByName(rt, name)
		if err != nil {
			return commandexec.FromServiceError(err)
		}
		return commandexec.Success(map[string]interface{}{"core": result.Core}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	case "type":
		if name == "" {
			return commandexec.Failure("MISSING_ARGUMENT", "specify a type name", nil, "Usage: rvn schema type <name>")
		}
		result, err := schemasvc.TypeByName(rt, name)
		if err != nil {
			return commandexec.FromServiceError(err)
		}
		return commandexec.Success(map[string]interface{}{"type": result.Type}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	case "trait":
		if name == "" {
			return commandexec.Failure("MISSING_ARGUMENT", "specify a trait name", nil, "Usage: rvn schema trait <name>")
		}
		result, err := schemasvc.TraitByName(rt, name)
		if err != nil {
			return commandexec.FromServiceError(err)
		}
		return commandexec.Success(map[string]interface{}{"trait": result.Trait}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	default:
		return commandexec.Failure("INVALID_INPUT", fmt.Sprintf("unknown schema subcommand: %s", subcommand), nil, "Use: types, traits, type <name>, trait <name>, core [name], or template ...")
	}
}

// HandleSchemaValidate executes the canonical `schema_validate` command.
func HandleSchemaValidate(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	rt, failure := newSchemaMutationCommandVaultRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	result, err := schemasvc.Validate(rt)
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.SchemaValidateResult{
		Valid:  result.Valid,
		Issues: result.Issues,
		Types:  result.Types,
		Traits: result.Traits,
	}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaTemplateList executes the canonical `schema_template_list` command.
func HandleSchemaTemplateList(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	rt, failure := newSchemaMutationCommandVaultRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
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
			state, err = schemasvc.ListTypeTemplates(rt, scopeValue)
		case "core":
			state, err = schemasvc.ListCoreTemplates(rt, scopeValue)
		default:
			return commandexec.Failure("INVALID_INPUT", "unknown template target", nil, "")
		}
		if err != nil {
			return commandexec.FromServiceError(err)
		}
		return commandexec.Success(map[string]interface{}{
			scopeKey:           scopeValue,
			"templates":        state.Templates,
			"default_template": state.DefaultTemplate,
		}, &commandexec.Meta{Count: len(state.Templates), QueryTimeMs: time.Since(start).Milliseconds()})
	}

	items, err := schemasvc.ListTemplates(rt)
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(map[string]interface{}{"templates": items}, &commandexec.Meta{Count: len(items), QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaTemplateGet executes the canonical `schema_template_get` command.
func HandleSchemaTemplateGet(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	rt, failure := newSchemaMutationCommandVaultRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	item, err := schemasvc.GetTemplate(rt, stringArg(req.Args, "template_id"))
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(schemaTemplateDefinitionPayload(item.ID, item.File, item.Description), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaTemplateSet executes the canonical `schema_template_set` command.
func HandleSchemaTemplateSet(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	description := stringArg(req.Args, "description")
	rt, failure := newConfigCommandVaultRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	item, err := schemasvc.SetTemplate(rt, schemasvc.SetTemplateRequest{
		VaultPath:   req.VaultPath,
		TemplateID:  stringArg(req.Args, "template_id"),
		File:        stringArg(req.Args, "file"),
		Description: description,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(schemaTemplateDefinitionPayload(item.ID, item.File, strings.TrimSpace(description)), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaTemplateRemove executes the canonical `schema_template_remove` command.
func HandleSchemaTemplateRemove(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	templateID := strings.TrimSpace(stringArg(req.Args, "template_id"))
	rt, failure := newSchemaMutationCommandVaultRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	if err := schemasvc.RemoveTemplate(rt, templateID); err != nil {
		return commandexec.FromServiceError(err)
	}
	return commandexec.Success(commandpayload.SchemaTemplateRemoveResult{
		Removed: true,
		ID:      templateID,
	}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaTemplateBind executes the canonical `schema_template_bind` command.
func HandleSchemaTemplateBind(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	rt, runtimeFailure := newSchemaMutationCommandVaultRuntime(req.VaultPath)
	if runtimeFailure.Error != nil {
		return runtimeFailure
	}
	defer rt.Close()
	targetKind, _, scopeValue, _, failure := schemaTemplateTarget(req.Args, true)
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
		result, err = schemasvc.AddTypeTemplate(rt, scopeValue, templateID)
		if err == nil && setDefault {
			_, err = schemasvc.SetTypeDefaultTemplate(rt, scopeValue, templateID)
		}
	case "core":
		result, err = schemasvc.AddCoreTemplate(rt, scopeValue, templateID)
		if err == nil && setDefault {
			_, err = schemasvc.SetCoreDefaultTemplate(rt, scopeValue, templateID)
		}
	default:
		return commandexec.Failure("INVALID_INPUT", "unknown template target", nil, "")
	}
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	data := commandpayload.SchemaTemplateBindResult{
		TemplateID: templateID,
	}
	if targetKind == "type" {
		data.Type = scopeValue
	} else {
		data.Core = scopeValue
	}
	if result != nil && result.AlreadySet {
		data.AlreadySet = true
		data.DefaultMatch = &result.DefaultMatch
	}
	if setDefault {
		data.DefaultTemplate = templateID
	}
	return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleSchemaTemplateUnbind executes the canonical `schema_template_unbind` command.
func HandleSchemaTemplateUnbind(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	rt, runtimeFailure := newSchemaMutationCommandVaultRuntime(req.VaultPath)
	if runtimeFailure.Error != nil {
		return runtimeFailure
	}
	defer rt.Close()
	targetKind, _, scopeValue, _, failure := schemaTemplateTarget(req.Args, true)
	if failure.Error != nil {
		return failure
	}

	templateID := strings.TrimSpace(stringArg(req.Args, "template_id"))
	clearDefault := boolArg(req.Args, "clear-default")
	var err error
	switch targetKind {
	case "type":
		err = schemasvc.RemoveTypeTemplate(rt, scopeValue, templateID, clearDefault)
	case "core":
		err = schemasvc.RemoveCoreTemplate(rt, scopeValue, templateID, clearDefault)
	default:
		return commandexec.Failure("INVALID_INPUT", "unknown template target", nil, "")
	}
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	data := commandpayload.SchemaTemplateUnbindResult{
		TemplateID: templateID,
		Removed:    true,
	}
	if targetKind == "type" {
		data.Type = scopeValue
	} else {
		data.Core = scopeValue
	}
	if clearDefault {
		data.DefaultCleared = true
	}
	return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleTemplateList executes the canonical `template_list` command.
func HandleTemplateList(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	rt, failure := newConfigOnlyCommandVaultRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg

	result, err := templatesvc.List(rt, templatesvc.ListRequest{
		VaultPath:   req.VaultPath,
		TemplateDir: vaultCfg.GetTemplateDirectory(),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	return commandexec.Success(map[string]interface{}{
		"template_dir": result.TemplateDir,
		"templates":    result.Templates,
	}, &commandexec.Meta{Count: len(result.Templates), QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleTemplateWrite executes the canonical `template_write` command.
func HandleTemplateWrite(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	rt, failure := newConfigCommandVaultRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg
	if boolArg(req.Args, "edit") {
		if _, ok := req.Args["content"]; !ok {
			return commandexec.Failure(codes.ErrInvalidInput, "template write --edit is only available in the interactive CLI", nil, "Use --content when invoking template_write non-interactively")
		}
	}
	result, err := templatesvc.Write(rt, templatesvc.WriteRequest{
		VaultPath:   req.VaultPath,
		TemplateDir: vaultCfg.GetTemplateDirectory(),
		Path:        stringArg(req.Args, "path"),
		Content:     stringArg(req.Args, "content"),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	return commandexec.SuccessWithWarnings(commandpayload.TemplateWriteResult{
		Path:        result.Path,
		Status:      result.Status,
		TemplateDir: result.TemplateDir,
	}, canonicalTemplateWarnings(result.Warnings), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleTemplateDelete executes the canonical `template_delete` command.
func HandleTemplateDelete(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
	rt, failure := newConfigCommandVaultRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg
	result, err := templatesvc.Delete(rt, templatesvc.DeleteRequest{
		VaultPath:   req.VaultPath,
		TemplateDir: vaultCfg.GetTemplateDirectory(),
		Path:        stringArg(req.Args, "path"),
		Force:       boolArg(req.Args, "force"),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	data := commandpayload.TemplateDeleteResult{
		Deleted:     result.DeletedPath,
		TrashPath:   result.TrashPath,
		Forced:      result.Forced,
		TemplateIDs: result.TemplateIDs,
	}
	warnings := canonicalTemplateWarnings(result.Warnings)
	if len(warnings) > 0 {
		return commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
	}
	return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

func canonicalSchemaWarnings(serviceWarnings []schemasvc.Warning) []commandexec.Warning {
	if len(serviceWarnings) == 0 {
		return nil
	}
	warnings := make([]commandexec.Warning, 0, len(serviceWarnings))
	for _, warning := range serviceWarnings {
		warnings = append(warnings, commandexec.Warning{Code: warning.Code, Message: warning.Message})
	}
	return warnings
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

func schemaTemplateDefinitionPayload(id, file, description string) commandpayload.SchemaTemplateDefinitionResult {
	return commandpayload.SchemaTemplateDefinitionResult{
		ID:          id,
		File:        file,
		Description: description,
	}
}
