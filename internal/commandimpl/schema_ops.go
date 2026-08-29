package commandimpl

import (
	"context"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/schemamigrate"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// schemaOpKind identifies the category of schema operation.
type schemaOpKind string

const (
	opKindAdd     schemaOpKind = "add"
	opKindUpdate  schemaOpKind = "update"
	opKindRemove  schemaOpKind = "remove"
	opKindRename  schemaOpKind = "rename"
	opKindConvert schemaOpKind = "convert"
)

// schemaOpTarget identifies what schema element the operation applies to.
type schemaOpTarget string

const (
	opTargetType  schemaOpTarget = "type"
	opTargetTrait schemaOpTarget = "trait"
	opTargetField schemaOpTarget = "field"
)

// schemaConversionMapping extracts the --map-json argument.
func schemaConversionMapping(raw interface{}) (map[string]interface{}, bool) {
	switch values := raw.(type) {
	case map[string]interface{}:
		return values, true
	case map[string]string:
		out := make(map[string]interface{}, len(values))
		for key, value := range values {
			out[key] = value
		}
		return out, true
	case nil:
		return nil, true
	default:
		return nil, false
	}
}

// schemaConvertCommandResult builds the command result for convert operations.
func schemaConvertCommandResult(result *schemamigrate.ConvertResult, start time.Time) commandexec.Result {
	meta := &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()}
	if result.Preview {
		return commandexec.Success(commandpayload.SchemaConvertPreviewResult{
			Kind:         result.Kind,
			Name:         result.Name,
			Type:         result.TypeName,
			SourceType:   result.SourceType,
			TargetType:   result.TargetType,
			Hint:         result.Hint,
			Preview:      true,
			TotalChanges: result.TotalChanges,
			Changes:      result.Changes,
		}, meta)
	}
	return commandexec.Success(commandpayload.SchemaConvertResult{
		Kind:           result.Kind,
		Name:           result.Name,
		Type:           result.TypeName,
		SourceType:     result.SourceType,
		TargetType:     result.TargetType,
		Hint:           result.Hint,
		Converted:      true,
		ChangesApplied: result.ChangesApplied,
	}, meta)
}

// schemaOperation defines a single schema mutation operation and how to execute it.
type schemaOperation struct {
	Kind   schemaOpKind
	Target schemaOpTarget

	// Execute runs the operation. It receives the runtime, request, and start time,
	// and returns the result envelope.
	Execute func(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result
}

// schemaOperationTable maps command IDs to their operation definitions.
var schemaOperationTable = map[string]schemaOperation{
	"schema_add_type": {
		Kind:   opKindAdd,
		Target: opTargetType,
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
			result, err := schemasvc.AddType(rt, schemasvc.AddTypeRequest{
				VaultPath:   req.VaultPath,
				TypeName:    stringArg(req.Args, "name"),
				DefaultPath: stringArg(req.Args, "default-path"),
				NameField:   stringArg(req.Args, "name-field"),
				Description: stringArg(req.Args, "description"),
			})
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			data := commandpayload.SchemaAddTypeResult{
				Added:       "type",
				Name:        result.Name,
				DefaultPath: result.DefaultPath,
				Description: result.Description,
				NameField:   result.NameField,
			}
			if result.NameField != "" {
				data.AutoCreatedField = &result.AutoCreatedField
			}
			return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
		},
	},

	"schema_add_trait": {
		Kind:   opKindAdd,
		Target: opTargetTrait,
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
			result, err := schemasvc.AddTrait(rt, schemasvc.AddTraitRequest{
				VaultPath: req.VaultPath,
				TraitName: stringArg(req.Args, "name"),
				TraitType: stringArg(req.Args, "type"),
				Values:    commaStringArg(req.Args, "values"),
				Default:   stringArg(req.Args, "default"),
			})
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.SchemaAddTraitResult{
				Added:  "trait",
				Name:   result.Name,
				Type:   result.Type,
				Values: result.Values,
			}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
		},
	},

	"schema_add_field": {
		Kind:   opKindAdd,
		Target: opTargetField,
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
			result, err := schemasvc.AddField(rt, schemasvc.AddFieldRequest{
				VaultPath:   req.VaultPath,
				TypeName:    stringArg(req.Args, "type_name"),
				FieldName:   stringArg(req.Args, "field_name"),
				FieldType:   stringArg(req.Args, "type"),
				Required:    boolArg(req.Args, "required"),
				Default:     stringArg(req.Args, "default"),
				Values:      commaStringArg(req.Args, "values"),
				Target:      stringArg(req.Args, "target"),
				Description: stringArg(req.Args, "description"),
			})
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.SchemaAddFieldResult{
				Added:       "field",
				Type:        result.TypeName,
				Field:       result.FieldName,
				FieldType:   result.FieldType,
				Required:    result.Required,
				Description: result.Description,
			}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
		},
	},

	"schema_update_type": {
		Kind:   opKindUpdate,
		Target: opTargetType,
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
			name := stringArg(req.Args, "name")
			result, err := schemasvc.UpdateType(rt, schemasvc.UpdateTypeRequest{
				VaultPath:   req.VaultPath,
				TypeName:    name,
				DefaultPath: stringArg(req.Args, "default-path"),
				NameField:   stringArg(req.Args, "name-field"),
				Description: stringArg(req.Args, "description"),
				AddTrait:    stringArg(req.Args, "add-trait"),
				RemoveTrait: stringArg(req.Args, "remove-trait"),
			})
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.SchemaUpdateResult{
				Updated: "type",
				Changes: result.Changes,
				Name:    name,
			}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
		},
	},

	"schema_update_trait": {
		Kind:   opKindUpdate,
		Target: opTargetTrait,
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
			name := stringArg(req.Args, "name")
			result, err := schemasvc.UpdateTrait(rt, schemasvc.UpdateTraitRequest{
				VaultPath: req.VaultPath,
				TraitName: name,
				TraitType: stringArg(req.Args, "type"),
				Values:    commaStringArg(req.Args, "values"),
				Default:   stringArg(req.Args, "default"),
			})
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.SchemaUpdateResult{
				Updated: "trait",
				Changes: result.Changes,
				Name:    name,
			}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
		},
	},

	"schema_update_field": {
		Kind:   opKindUpdate,
		Target: opTargetField,
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
			typeName := stringArg(req.Args, "type_name")
			fieldName := stringArg(req.Args, "field_name")
			result, err := schemasvc.UpdateField(rt, schemasvc.UpdateFieldRequest{
				VaultPath:   req.VaultPath,
				TypeName:    typeName,
				FieldName:   fieldName,
				FieldType:   stringArg(req.Args, "type"),
				Required:    stringArg(req.Args, "required"),
				Default:     stringArg(req.Args, "default"),
				Values:      commaStringArg(req.Args, "values"),
				Target:      stringArg(req.Args, "target"),
				Description: stringArg(req.Args, "description"),
			})
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.SchemaUpdateResult{
				Updated: "field",
				Changes: result.Changes,
				Type:    typeName,
				Field:   fieldName,
			}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
		},
	},

	"schema_remove_type": {
		Kind:   opKindRemove,
		Target: opTargetType,
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
			result, err := schemasvc.RemoveType(rt, schemasvc.RemoveTypeRequest{
				VaultPath:   req.VaultPath,
				TypeName:    stringArg(req.Args, "name"),
				Force:       boolArg(req.Args, "force") || req.Confirm,
				Interactive: false,
			})
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			data := commandpayload.SchemaRemoveResult{Removed: "type", Name: stringArg(req.Args, "name")}
			warnings := canonicalSchemaWarnings(result.Warnings)
			if len(warnings) > 0 {
				return commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
			}
			return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
		},
	},

	"schema_remove_trait": {
		Kind:   opKindRemove,
		Target: opTargetTrait,
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
			result, err := schemasvc.RemoveTrait(rt, schemasvc.RemoveTraitRequest{
				VaultPath:   req.VaultPath,
				TraitName:   stringArg(req.Args, "name"),
				Force:       boolArg(req.Args, "force") || req.Confirm,
				Interactive: false,
			})
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			data := commandpayload.SchemaRemoveResult{Removed: "trait", Name: stringArg(req.Args, "name")}
			warnings := canonicalSchemaWarnings(result.Warnings)
			if len(warnings) > 0 {
				return commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
			}
			return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
		},
	},

	"schema_remove_field": {
		Kind:   opKindRemove,
		Target: opTargetField,
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
			typeName := stringArg(req.Args, "type_name")
			fieldName := stringArg(req.Args, "field_name")
			if _, err := schemasvc.RemoveField(rt, schemasvc.RemoveFieldRequest{
				VaultPath: req.VaultPath,
				TypeName:  typeName,
				FieldName: fieldName,
			}); err != nil {
				return commandexec.FromServiceError(err)
			}
			return commandexec.Success(commandpayload.SchemaRemoveResult{
				Removed: "field",
				Type:    typeName,
				Field:   fieldName,
			}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
		},
	},

	"schema_rename_type": {
		Kind:   opKindRename,
		Target: opTargetType,
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
			result, err := schemamigrate.RenameType(rt, schemamigrate.RenameTypeRequest{
				VaultPath:         req.VaultPath,
				OldName:           stringArg(req.Args, "old_name"),
				NewName:           stringArg(req.Args, "new_name"),
				Description:       stringArg(req.Args, "description"),
				Confirm:           req.Confirm,
				RenameDefaultPath: boolArg(req.Args, "rename-default-path"),
			})
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			if result.Preview {
				data := commandpayload.SchemaRenameTypePreviewResult{
					Preview:      true,
					OldName:      result.OldName,
					NewName:      result.NewName,
					TotalChanges: result.TotalChanges,
					Changes:      result.Changes,
					Hint:         result.Hint,
				}
				if result.DefaultPathRenameAvailable {
					data.DefaultPathRenameAvailable = &result.DefaultPathRenameAvailable
					data.DefaultPathOld = &result.DefaultPathOld
					data.DefaultPathNew = &result.DefaultPathNew
					data.OptionalTotalChanges = &result.OptionalTotalChanges
					data.OptionalChanges = &result.OptionalChanges
					data.FilesToMove = &result.FilesToMove
				}
				return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
			}
			data := commandpayload.SchemaRenameTypeResult{
				Renamed:        true,
				OldName:        result.OldName,
				NewName:        result.NewName,
				ChangesApplied: result.ChangesApplied,
				Hint:           result.Hint,
			}
			if result.DefaultPathRenameAvailable {
				data.DefaultPathRenameAvailable = &result.DefaultPathRenameAvailable
				data.DefaultPathRenamed = &result.DefaultPathRenamed
				data.DefaultPathOld = &result.DefaultPathOld
				data.DefaultPathNew = &result.DefaultPathNew
				data.FilesMoved = &result.FilesMoved
				data.ReferenceFilesUpdated = &result.ReferenceFilesUpdated
			}
			return commandexec.Success(data, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
		},
	},

	"schema_rename_field": {
		Kind:   opKindRename,
		Target: opTargetField,
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
			result, err := schemamigrate.RenameField(rt, schemamigrate.RenameFieldRequest{
				VaultPath: req.VaultPath,
				TypeName:  stringArg(req.Args, "type_name"),
				OldField:  stringArg(req.Args, "old_field"),
				NewField:  stringArg(req.Args, "new_field"),
				Confirm:   req.Confirm,
			})
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			if result.Preview {
				return commandexec.Success(commandpayload.SchemaRenameFieldPreviewResult{
					Preview:      true,
					Type:         result.TypeName,
					OldField:     result.OldField,
					NewField:     result.NewField,
					TotalChanges: result.TotalChanges,
					Changes:      result.Changes,
					Hint:         result.Hint,
				}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
			}
			return commandexec.Success(commandpayload.SchemaRenameFieldResult{
				Renamed:        true,
				Type:           result.TypeName,
				OldField:       result.OldField,
				NewField:       result.NewField,
				ChangesApplied: result.ChangesApplied,
				Hint:           result.Hint,
			}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
		},
	},

	"schema_convert_trait": {
		Kind:   opKindConvert,
		Target: opTargetTrait,
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
			mapping, ok := schemaConversionMapping(req.Args["map-json"])
			if !ok {
				return commandexec.Failure(codes.ErrInvalidInput, "--map-json must be a JSON object", nil, `Provide an object such as {"high":true,"low":false}`)
			}
			result, err := schemamigrate.ConvertTrait(rt, schemamigrate.ConvertTraitRequest{
				VaultPath:  req.VaultPath,
				TraitName:  stringArg(req.Args, "name"),
				TargetType: stringArg(req.Args, "type"),
				Mapping:    mapping,
				Confirm:    req.Confirm,
			})
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return schemaConvertCommandResult(result, start)
		},
	},

	"schema_convert_field": {
		Kind:   opKindConvert,
		Target: opTargetField,
		Execute: func(rt *vaultruntime.Runtime, req commandexec.Request, start time.Time) commandexec.Result {
			mapping, ok := schemaConversionMapping(req.Args["map-json"])
			if !ok {
				return commandexec.Failure(codes.ErrInvalidInput, "--map-json must be a JSON object", nil, `Provide an object such as {"true":"done","false":"todo"}`)
			}
			result, err := schemamigrate.ConvertField(rt, schemamigrate.ConvertFieldRequest{
				VaultPath:  req.VaultPath,
				TypeName:   stringArg(req.Args, "type_name"),
				FieldName:  stringArg(req.Args, "field_name"),
				TargetType: stringArg(req.Args, "type"),
				Mapping:    mapping,
				Confirm:    req.Confirm,
			})
			if err != nil {
				return commandexec.FromServiceError(err)
			}
			return schemaConvertCommandResult(result, start)
		},
	},
}

// handleSchemaMutation is a generic handler for schema mutation operations.
// It looks up the operation in the table and executes it.
func handleSchemaMutation(_ context.Context, req commandexec.Request, commandID string) commandexec.Result {
	op, exists := schemaOperationTable[commandID]
	if !exists {
		return commandexec.Failure(codes.ErrInvalidInput, "unknown schema operation: "+commandID, nil, "")
	}

	start := time.Now()
	rt, failure := newSchemaMutationCommandVaultRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	return op.Execute(rt, req, start)
}

// Replace existing handlers with table-driven versions:

// HandleSchemaAddType executes the canonical `schema_add_type` command.
func HandleSchemaAddType(ctx context.Context, req commandexec.Request) commandexec.Result {
	return handleSchemaMutation(ctx, req, "schema_add_type")
}

// HandleSchemaAddTrait executes the canonical `schema_add_trait` command.
func HandleSchemaAddTrait(ctx context.Context, req commandexec.Request) commandexec.Result {
	return handleSchemaMutation(ctx, req, "schema_add_trait")
}

// HandleSchemaAddField executes the canonical `schema_add_field` command.
func HandleSchemaAddField(ctx context.Context, req commandexec.Request) commandexec.Result {
	return handleSchemaMutation(ctx, req, "schema_add_field")
}

// HandleSchemaUpdateType executes the canonical `schema_update_type` command.
func HandleSchemaUpdateType(ctx context.Context, req commandexec.Request) commandexec.Result {
	return handleSchemaMutation(ctx, req, "schema_update_type")
}

// HandleSchemaUpdateTrait executes the canonical `schema_update_trait` command.
func HandleSchemaUpdateTrait(ctx context.Context, req commandexec.Request) commandexec.Result {
	return handleSchemaMutation(ctx, req, "schema_update_trait")
}

// HandleSchemaUpdateField executes the canonical `schema_update_field` command.
func HandleSchemaUpdateField(ctx context.Context, req commandexec.Request) commandexec.Result {
	return handleSchemaMutation(ctx, req, "schema_update_field")
}

// HandleSchemaRemoveType executes the canonical `schema_remove_type` command.
func HandleSchemaRemoveType(ctx context.Context, req commandexec.Request) commandexec.Result {
	return handleSchemaMutation(ctx, req, "schema_remove_type")
}

// HandleSchemaRemoveTrait executes the canonical `schema_remove_trait` command.
func HandleSchemaRemoveTrait(ctx context.Context, req commandexec.Request) commandexec.Result {
	return handleSchemaMutation(ctx, req, "schema_remove_trait")
}

// HandleSchemaRemoveField executes the canonical `schema_remove_field` command.
func HandleSchemaRemoveField(ctx context.Context, req commandexec.Request) commandexec.Result {
	return handleSchemaMutation(ctx, req, "schema_remove_field")
}

// HandleSchemaRenameType executes the canonical `schema_rename_type` command.
func HandleSchemaRenameType(ctx context.Context, req commandexec.Request) commandexec.Result {
	return handleSchemaMutation(ctx, req, "schema_rename_type")
}

// HandleSchemaRenameField executes the canonical `schema_rename_field` command.
func HandleSchemaRenameField(ctx context.Context, req commandexec.Request) commandexec.Result {
	return handleSchemaMutation(ctx, req, "schema_rename_field")
}

// HandleSchemaConvertTrait executes the canonical `schema_convert_trait` command.
func HandleSchemaConvertTrait(ctx context.Context, req commandexec.Request) commandexec.Result {
	return handleSchemaMutation(ctx, req, "schema_convert_trait")
}

// HandleSchemaConvertField executes the canonical `schema_convert_field` command.
func HandleSchemaConvertField(ctx context.Context, req commandexec.Request) commandexec.Result {
	return handleSchemaMutation(ctx, req, "schema_convert_field")
}
