package commandimpl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/traitsvc"
)

const warnEmbeddedSkipped = "EMBEDDED_SKIPPED"

type canonicalBulkResult struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Details string `json:"details,omitempty"`
}

type canonicalBulkPreviewItem struct {
	ID      string            `json:"id"`
	Changes map[string]string `json:"changes,omitempty"`
	Action  string            `json:"action"`
	Details string            `json:"details,omitempty"`
}

// HandleSet executes the canonical `set` command.
func HandleSet(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return commandexec.Failure("SCHEMA_INVALID", "failed to load schema", nil, "Fix schema.yaml and try again")
	}

	updates, err := parseKeyValueArgs(req.Args["fields"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", "invalid fields payload", nil, err.Error())
	}

	typedUpdates, err := parseTypedFieldValues(req.Args["fields-json"])
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", "invalid --fields-json payload", nil, "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
	}

	objectIDs := commandIDsArg(req.Args, "object_ids")
	stdinMode := boolArg(req.Args, "stdin") || len(objectIDs) > 0
	if stdinMode {
		if _, ok := req.Args["fields-json"]; ok {
			return commandexec.Failure("INVALID_INPUT", "--fields-json is not supported with --stdin", nil, "Use positional field=value updates when using --stdin")
		}
		if len(objectIDs) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no object IDs provided via stdin", nil, "Pipe object IDs to stdin, one per line")
		}
		if len(updates) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no fields to set", nil, "Usage: rvn set --stdin field=value...")
		}
		return runSetBulk(vaultPath, vaultCfg, sch, objectIDs, updates, req.Confirm || boolArg(req.Args, "confirm"))
	}

	reference := strings.TrimSpace(stringArg(req.Args, "object_id"))
	if reference == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires object-id", nil, "Usage: rvn set <object-id> field=value...")
	}
	if len(updates) == 0 && len(typedUpdates) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no fields to set", nil, "Usage: rvn set <object-id> field=value... or --fields-json '{...}'")
	}

	serviceResult, err := objectsvc.SetByReference(objectsvc.SetByReferenceRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		Schema:       sch,
		Reference:    reference,
		Updates:      updates,
		TypedUpdates: typedUpdates,
		ParseOptions: parseOptionsFromVaultConfig(vaultCfg),
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	maybeReindexFile(vaultPath, serviceResult.FilePath, vaultCfg)

	data := map[string]interface{}{
		"file":           serviceResult.RelativePath,
		"object_id":      serviceResult.ObjectID,
		"type":           serviceResult.ObjectType,
		"updated_fields": serviceResult.ResolvedUpdates,
	}
	if len(serviceResult.PreviousFields) > 0 {
		data["previous_fields"] = serviceResult.PreviousFields
	}
	if serviceResult.Embedded {
		data["embedded"] = true
		if serviceResult.EmbeddedSlug != "" {
			data["embedded_slug"] = serviceResult.EmbeddedSlug
		}
	}

	return commandexec.SuccessWithWarnings(
		data,
		warningMessagesToCommandWarnings(serviceResult.WarningMessages, "UNKNOWN_FIELD"),
		nil,
	)
}

// HandleAdd executes the canonical `add` command.
func HandleAdd(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	text := strings.TrimSpace(stringArg(req.Args, "text"))
	if text == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires text argument", nil, "Usage: rvn add <text>")
	}

	objectIDs := commandIDsArg(req.Args, "object_ids")
	stdinMode := boolArg(req.Args, "stdin") || len(objectIDs) > 0

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return commandexec.Failure("SCHEMA_INVALID", "failed to load schema", nil, "Fix schema.yaml and try again")
	}

	if !stdinMode {
		return runAddSingle(vaultPath, vaultCfg, sch, text, strings.TrimSpace(stringArg(req.Args, "to")), strings.TrimSpace(stringArg(req.Args, "heading")))
	}
	if len(objectIDs) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no object IDs provided via stdin", nil, "Pipe object IDs to stdin, one per line")
	}

	return runAddBulk(vaultPath, vaultCfg, objectIDs, text, strings.TrimSpace(stringArg(req.Args, "heading")), req.Confirm || boolArg(req.Args, "confirm"))
}

// HandleDelete executes the canonical `delete` command.
func HandleDelete(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	objectIDs := commandIDsArg(req.Args, "object_ids")
	stdinMode := boolArg(req.Args, "stdin") || len(objectIDs) > 0
	if stdinMode {
		if len(objectIDs) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no object IDs provided via stdin", nil, "Pipe object IDs to stdin, one per line")
		}
		return runDeleteBulk(vaultPath, vaultCfg, objectIDs, req.Confirm || boolArg(req.Args, "confirm"))
	}

	reference := strings.TrimSpace(stringArg(req.Args, "object_id"))
	if reference == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires object-id argument", nil, "Usage: rvn delete <object-id>")
	}

	sch, _ := schema.Load(vaultPath)
	deletionCfg := vaultCfg.GetDeletionConfig()
	if req.Preview {
		preview, err := objectsvc.PreviewDeleteByReference(objectsvc.DeleteByReferenceRequest{
			VaultPath:   vaultPath,
			VaultConfig: vaultCfg,
			Schema:      sch,
			Reference:   reference,
			Behavior:    deletionCfg.Behavior,
			TrashDir:    deletionCfg.TrashDir,
		})
		if err != nil {
			return mapContentMutationError(err)
		}

		warnings := deleteBacklinkCommandWarnings(preview.Backlinks)
		return commandexec.SuccessWithWarnings(map[string]interface{}{
			"preview":   true,
			"object_id": preview.ObjectID,
			"behavior":  preview.Behavior,
			"trash_dir": deletionCfg.TrashDir,
			"backlinks": preview.Backlinks,
		}, warnings, nil)
	}

	serviceResult, err := objectsvc.DeleteByReference(objectsvc.DeleteByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		Schema:      sch,
		Reference:   reference,
		Behavior:    deletionCfg.Behavior,
		TrashDir:    deletionCfg.TrashDir,
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	warnings := make([]commandexec.Warning, 0, len(serviceResult.WarningMessages)+1)
	warnings = append(warnings, deleteBacklinkCommandWarnings(serviceResult.Backlinks)...)
	warnings = append(warnings, warningMessagesToCommandWarnings(serviceResult.WarningMessages, "INDEX_UPDATE_FAILED")...)

	data := map[string]interface{}{
		"deleted":  serviceResult.ObjectID,
		"behavior": serviceResult.Behavior,
	}
	if serviceResult.TrashPath != "" {
		relDest, relErr := filepath.Rel(vaultPath, serviceResult.TrashPath)
		if relErr == nil {
			data["trash_path"] = filepath.ToSlash(relDest)
		}
	}

	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

// HandleMove executes the canonical `move` command.
func HandleMove(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		sch = schema.New()
	}

	objectIDs := commandIDsArg(req.Args, "object_ids")
	stdinMode := boolArg(req.Args, "stdin") || len(objectIDs) > 0
	if stdinMode {
		destination := strings.TrimSpace(stringArg(req.Args, "destination"))
		if destination == "" {
			destination = strings.TrimSpace(stringArg(req.Args, "source"))
		}
		if len(objectIDs) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no object IDs provided via stdin", nil, "Provide object IDs when using bulk move")
		}
		return runMoveBulk(vaultPath, vaultCfg, sch, objectIDs, destination, boolArgDefault(req.Args, "update-refs", true), req.Confirm || boolArg(req.Args, "confirm"))
	}

	source := strings.TrimSpace(stringArg(req.Args, "source"))
	destination := strings.TrimSpace(stringArg(req.Args, "destination"))
	if source == "" || destination == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires source and destination arguments", nil, "Usage: rvn move <source> <destination>")
	}

	serviceResult, err := objectsvc.MoveByReference(objectsvc.MoveByReferenceRequest{
		VaultPath:      vaultPath,
		VaultConfig:    vaultCfg,
		Schema:         sch,
		Reference:      source,
		Destination:    destination,
		UpdateRefs:     boolArgDefault(req.Args, "update-refs", true),
		SkipTypeCheck:  boolArg(req.Args, "skip-type-check"),
		ParseOptions:   parseOptionsFromVaultConfig(vaultCfg),
		FailOnIndexErr: true,
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	if serviceResult.NeedsConfirm && serviceResult.TypeMismatch != nil {
		mismatch := serviceResult.TypeMismatch
		return commandexec.SuccessWithWarnings(map[string]interface{}{
			"source":        serviceResult.SourceID,
			"destination":   serviceResult.DestinationID,
			"needs_confirm": true,
			"reason":        serviceResult.Reason,
		}, []commandexec.Warning{{
			Code: "TYPE_DIRECTORY_MISMATCH",
			Message: fmt.Sprintf("Moving to '%s/' which is the default directory for type '%s', but file has type '%s'",
				mismatch.DestinationDir, mismatch.ExpectedType, mismatch.ActualType),
			Ref: fmt.Sprintf("Use --skip-type-check to proceed, or change the file's type to '%s'", mismatch.ExpectedType),
		}}, nil)
	}

	warnings := warningMessagesToCommandWarnings(serviceResult.WarningMessages, "INDEX_UPDATE_FAILED")
	data := map[string]interface{}{
		"source":      serviceResult.SourceID,
		"destination": serviceResult.DestinationID,
	}
	if len(serviceResult.UpdatedRefs) > 0 {
		data["updated_refs"] = serviceResult.UpdatedRefs
	}

	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

// HandleUpdate executes the canonical `update` command.
func HandleUpdate(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	newValue := strings.TrimSpace(stringArg(req.Args, "value"))
	if newValue == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "no value specified", nil, "Usage: rvn update <trait_id> <new_value>")
	}

	traitIDs := commandIDsArg(req.Args, "object_ids")
	stdinMode := boolArg(req.Args, "stdin") || len(traitIDs) > 0
	confirm := req.Confirm || boolArg(req.Args, "confirm")
	if !stdinMode {
		singleID := strings.TrimSpace(stringArg(req.Args, "trait_id"))
		if singleID == "" {
			return commandexec.Failure("MISSING_ARGUMENT", "requires trait-id and new value arguments", nil, "Usage: rvn update <trait_id> <new_value>")
		}
		if !strings.Contains(singleID, ":trait:") {
			return commandexec.Failure("INVALID_INPUT", "invalid trait ID format", nil, "Trait IDs look like: path/file.md:trait:N")
		}
		traitIDs = []string{singleID}
		confirm = true
	}
	if len(traitIDs) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no trait IDs provided via stdin", nil, "Provide trait IDs when using bulk update")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return commandexec.Failure("SCHEMA_INVALID", "failed to load schema", nil, "Fix schema.yaml and try again")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		return commandexec.Failure("DATABASE_ERROR", err.Error(), nil, "Run 'rvn reindex' to rebuild the database")
	}
	defer db.Close()
	db.SetDailyDirectory(vaultCfg.GetDailyDirectory())

	traits, skipped, err := traitsvc.ResolveTraitIDs(db, traitIDs)
	if err != nil {
		return mapTraitMutationError(err)
	}

	if !confirm {
		preview, err := traitsvc.BuildPreview(traits, newValue, sch, skipped)
		if err != nil {
			return mapTraitMutationError(err)
		}
		return commandexec.Success(map[string]interface{}{
			"preview": true,
			"action":  preview.Action,
			"items":   preview.Items,
			"skipped": preview.Skipped,
			"total":   preview.Total,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := traitsvc.ApplyUpdates(vaultPath, traits, newValue, sch, skipped)
	if err != nil {
		return mapTraitMutationError(err)
	}

	reindexed := make(map[string]struct{}, len(summary.ChangedFilePaths))
	for _, filePath := range summary.ChangedFilePaths {
		if filePath == "" {
			continue
		}
		if _, ok := reindexed[filePath]; ok {
			continue
		}
		reindexed[filePath] = struct{}{}
		maybeReindexFile(vaultPath, filePath, vaultCfg)
	}

	return commandexec.Success(map[string]interface{}{
		"action":   summary.Action,
		"results":  summary.Results,
		"total":    summary.Total,
		"modified": summary.Modified,
		"skipped":  summary.Skipped,
		"errors":   summary.Errors,
	}, &commandexec.Meta{Count: summary.Modified})
}

func runSetBulk(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, ids []string, updates map[string]string, confirm bool) commandexec.Result {
	request := objectsvc.SetBulkRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		Schema:       sch,
		ObjectIDs:    ids,
		Updates:      updates,
		ParseOptions: parseOptionsFromVaultConfig(vaultCfg),
	}

	if !confirm {
		preview, err := objectsvc.PreviewSetBulk(request)
		if err != nil {
			return mapContentMutationError(err)
		}
		return commandexec.Success(map[string]interface{}{
			"preview":  true,
			"action":   preview.Action,
			"items":    canonicalSetPreviewItems(preview.Items),
			"skipped":  canonicalSetResults(preview.Skipped),
			"total":    preview.Total,
			"warnings": nil,
			"fields":   updates,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := objectsvc.ApplySetBulk(request, func(filePath string) {
		maybeReindexFile(vaultPath, filePath, vaultCfg)
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	return commandexec.Success(map[string]interface{}{
		"ok":       summary.Errors == 0,
		"action":   summary.Action,
		"results":  canonicalSetResults(summary.Results),
		"total":    summary.Total,
		"skipped":  summary.Skipped,
		"errors":   summary.Errors,
		"modified": summary.Modified,
		"fields":   updates,
	}, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}

func runAddBulk(vaultPath string, vaultCfg *config.VaultConfig, ids []string, text string, headingSpec string, confirm bool) commandexec.Result {
	fileIDs, embeddedIDs := splitEmbeddedIDs(ids)
	warnings := embeddedSkipWarnings(embeddedIDs)
	request := objectsvc.AddBulkRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		ObjectIDs:    fileIDs,
		Line:         text,
		HeadingSpec:  headingSpec,
		ParseOptions: parseOptionsFromVaultConfig(vaultCfg),
	}

	if !confirm {
		preview, err := objectsvc.PreviewAddBulk(request)
		if err != nil {
			return mapContentMutationError(err)
		}
		return commandexec.Success(map[string]interface{}{
			"preview":  true,
			"action":   "add",
			"items":    canonicalAddPreviewItems(preview.Items),
			"skipped":  canonicalAddResults(preview.Skipped),
			"total":    preview.Total,
			"warnings": warnings,
			"content":  text,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := objectsvc.ApplyAddBulk(request, func(filePath string) {
		maybeReindexFile(vaultPath, filePath, vaultCfg)
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	return commandexec.SuccessWithWarnings(map[string]interface{}{
		"ok":      summary.Errors == 0,
		"action":  summary.Action,
		"results": canonicalAddResults(summary.Results),
		"total":   summary.Total,
		"skipped": summary.Skipped,
		"errors":  summary.Errors,
		"added":   summary.Added,
		"content": text,
	}, warnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}

func runAddSingle(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, text, toRef, headingSpec string) commandexec.Result {
	captureCfg := vaultCfg.GetCaptureConfig()
	parseOpts := parseOptionsFromVaultConfig(vaultCfg)

	var destPath string
	var isDailyNote bool
	var targetObjectID string
	var fileObjectID string

	if strings.TrimSpace(toRef) != "" {
		rt := &readsvc.Runtime{
			VaultPath: vaultPath,
			VaultCfg:  vaultCfg,
			Schema:    sch,
		}
		resolved, err := readsvc.ResolveReferenceWithDynamicDates(toRef, rt, true)
		if err != nil {
			return mapResolveFailure(err, toRef)
		}
		destPath = resolved.FilePath
		targetObjectID = resolved.ObjectID
		fileObjectID = resolved.FileObjectID
		isDailyNote = isDailyNoteObjectID(resolved.FileObjectID, vaultCfg)
	} else if captureCfg.Destination == "daily" {
		today := time.Now()
		dateStr := fmt.Sprintf("%04d-%02d-%02d", today.Year(), today.Month(), today.Day())
		destPath = vaultCfg.DailyNotePath(vaultPath, dateStr)
		fileObjectID = vaultCfg.DailyNoteID(dateStr)
		isDailyNote = true
	} else {
		destPath = filepath.Join(vaultPath, captureCfg.Destination)
		fileObjectID = vaultCfg.FilePathToObjectID(captureCfg.Destination)
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			return commandexec.Failure("FILE_NOT_FOUND", fmt.Sprintf("Configured capture destination '%s' does not exist", captureCfg.Destination), nil, "Create the file first or change capture.destination in raven.yaml")
		}
	}

	if err := paths.ValidateWithinVault(vaultPath, destPath); err != nil {
		if errors.Is(err, paths.ErrPathOutsideVault) {
			return commandexec.Failure("FILE_OUTSIDE_VAULT", fmt.Sprintf("cannot capture outside vault: %s", destPath), nil, "")
		}
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
	}

	if headingSpec != "" {
		if targetObjectID != "" && strings.Contains(targetObjectID, "#") {
			return commandexec.Failure("INVALID_INPUT", "cannot combine --heading with a section reference in --to", nil, "Use either --to <file#section> or --heading")
		}
		resolvedTarget, err := objectsvc.ResolveAddHeadingTarget(vaultPath, destPath, fileObjectID, headingSpec, parseOpts)
		if err != nil {
			return commandexec.Failure("REF_NOT_FOUND", err.Error(), nil, "Use an existing section slug/id or heading text")
		}
		targetObjectID = resolvedTarget
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return commandexec.Failure("FILE_WRITE_ERROR", err.Error(), nil, "")
	}
	if err := objectsvc.AppendToFile(vaultPath, destPath, text, captureCfg, vaultCfg, isDailyNote, targetObjectID, parseOpts); err != nil {
		return commandexec.Failure("FILE_WRITE_ERROR", err.Error(), nil, "")
	}

	maybeReindexFile(vaultPath, destPath, vaultCfg)
	relPath, _ := filepath.Rel(vaultPath, destPath)
	return commandexec.Success(map[string]interface{}{
		"file":    filepath.ToSlash(relPath),
		"line":    objectsvc.FileLineCount(destPath),
		"content": text,
	}, nil)
}

func runDeleteBulk(vaultPath string, vaultCfg *config.VaultConfig, ids []string, confirm bool) commandexec.Result {
	fileIDs, embeddedIDs := splitEmbeddedIDs(ids)
	warnings := embeddedSkipWarnings(embeddedIDs)
	deletionCfg := vaultCfg.GetDeletionConfig()
	request := objectsvc.DeleteBulkRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		ObjectIDs:   fileIDs,
		Behavior:    deletionCfg.Behavior,
		TrashDir:    deletionCfg.TrashDir,
	}

	if !confirm {
		preview, err := objectsvc.PreviewDeleteBulk(request)
		if err != nil {
			return mapContentMutationError(err)
		}
		return commandexec.Success(map[string]interface{}{
			"preview":  true,
			"action":   preview.Action,
			"items":    canonicalDeletePreviewItems(preview.Items),
			"skipped":  canonicalDeleteResults(preview.Skipped),
			"total":    preview.Total,
			"warnings": warnings,
			"behavior": preview.Behavior,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := objectsvc.ApplyDeleteBulk(request)
	if err != nil {
		return mapContentMutationError(err)
	}

	allWarnings := append([]commandexec.Warning{}, warnings...)
	allWarnings = append(allWarnings, warningMessagesToCommandWarnings(summary.WarningMessages, "INDEX_UPDATE_FAILED")...)
	return commandexec.SuccessWithWarnings(map[string]interface{}{
		"ok":       summary.Errors == 0,
		"action":   summary.Action,
		"results":  canonicalDeleteResults(summary.Results),
		"total":    summary.Total,
		"skipped":  summary.Skipped,
		"errors":   summary.Errors,
		"deleted":  summary.Deleted,
		"behavior": summary.Behavior,
	}, allWarnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}

func runMoveBulk(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, ids []string, destination string, updateRefs bool, confirm bool) commandexec.Result {
	if strings.TrimSpace(destination) == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "no destination provided", nil, "Usage: rvn move --stdin <destination-directory/>")
	}
	if !strings.HasSuffix(destination, "/") {
		return commandexec.Failure("INVALID_INPUT", "destination must be a directory (end with /)", nil, "Example: rvn move --stdin archive/projects/")
	}

	fileIDs, embeddedIDs := splitEmbeddedIDs(ids)
	warnings := embeddedSkipWarnings(embeddedIDs)
	request := objectsvc.MoveBulkRequest{
		VaultPath:      vaultPath,
		VaultConfig:    vaultCfg,
		Schema:         sch,
		ObjectIDs:      fileIDs,
		DestinationDir: destination,
		UpdateRefs:     updateRefs,
		ParseOptions:   parseOptionsFromVaultConfig(vaultCfg),
	}

	if !confirm {
		preview, err := objectsvc.PreviewMoveBulk(request)
		if err != nil {
			return mapContentMutationError(err)
		}
		return commandexec.Success(map[string]interface{}{
			"preview":     true,
			"action":      preview.Action,
			"items":       canonicalMovePreviewItems(preview.Items),
			"skipped":     canonicalMoveResults(preview.Skipped),
			"total":       preview.Total,
			"warnings":    warnings,
			"destination": preview.Destination,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := objectsvc.ApplyMoveBulk(request)
	if err != nil {
		return mapContentMutationError(err)
	}

	allWarnings := append([]commandexec.Warning{}, warnings...)
	allWarnings = append(allWarnings, warningMessagesToCommandWarnings(summary.WarningMessages, "INDEX_UPDATE_FAILED")...)
	return commandexec.SuccessWithWarnings(map[string]interface{}{
		"ok":          summary.Errors == 0,
		"action":      summary.Action,
		"results":     canonicalMoveResults(summary.Results),
		"total":       summary.Total,
		"skipped":     summary.Skipped,
		"errors":      summary.Errors,
		"moved":       summary.Moved,
		"destination": summary.Destination,
	}, allWarnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}

func commandIDsArg(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, raw := range stringSliceArg(args[key]) {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if strings.Contains(id, "\t") {
			parts := strings.SplitN(id, "\t", 3)
			if len(parts) >= 2 {
				id = strings.TrimSpace(parts[1])
			}
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func stringSliceArg(raw any) []string {
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		parts := strings.Split(s, "\n")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		return out
	default:
		return nil
	}
}

func splitEmbeddedIDs(ids []string) ([]string, []string) {
	fileIDs := make([]string, 0, len(ids))
	embeddedIDs := make([]string, 0)
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, _, ok := paths.ParseEmbeddedID(id); ok {
			embeddedIDs = append(embeddedIDs, id)
			continue
		}
		fileIDs = append(fileIDs, id)
	}
	return fileIDs, embeddedIDs
}

func embeddedSkipWarnings(embeddedIDs []string) []commandexec.Warning {
	if len(embeddedIDs) == 0 {
		return nil
	}
	return []commandexec.Warning{{
		Code:    warnEmbeddedSkipped,
		Message: fmt.Sprintf("Skipped %d embedded object(s) - bulk operations only support file-level objects", len(embeddedIDs)),
		Ref:     strings.Join(embeddedIDs, ", "),
	}}
}

func canonicalAddPreviewItems(items []objectsvc.AddBulkPreviewItem) []canonicalBulkPreviewItem {
	out := make([]canonicalBulkPreviewItem, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkPreviewItem{
			ID:      item.ID,
			Action:  item.Action,
			Details: item.Details,
		})
	}
	return out
}

func canonicalAddResults(items []objectsvc.AddBulkResult) []canonicalBulkResult {
	out := make([]canonicalBulkResult, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkResult{
			ID:     item.ID,
			Status: item.Status,
			Reason: item.Reason,
		})
	}
	return out
}

func canonicalSetPreviewItems(items []objectsvc.SetBulkPreviewItem) []canonicalBulkPreviewItem {
	out := make([]canonicalBulkPreviewItem, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkPreviewItem{
			ID:      item.ID,
			Action:  item.Action,
			Changes: item.Changes,
		})
	}
	return out
}

func canonicalSetResults(items []objectsvc.SetBulkResult) []canonicalBulkResult {
	out := make([]canonicalBulkResult, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkResult{
			ID:     item.ID,
			Status: item.Status,
			Reason: item.Reason,
		})
	}
	return out
}

func canonicalDeletePreviewItems(items []objectsvc.DeleteBulkPreviewItem) []canonicalBulkPreviewItem {
	out := make([]canonicalBulkPreviewItem, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkPreviewItem{
			ID:      item.ID,
			Action:  item.Action,
			Details: item.Details,
			Changes: item.Changes,
		})
	}
	return out
}

func canonicalDeleteResults(items []objectsvc.DeleteBulkResult) []canonicalBulkResult {
	out := make([]canonicalBulkResult, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkResult{
			ID:     item.ID,
			Status: item.Status,
			Reason: item.Reason,
		})
	}
	return out
}

func canonicalMovePreviewItems(items []objectsvc.MoveBulkPreviewItem) []canonicalBulkPreviewItem {
	out := make([]canonicalBulkPreviewItem, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkPreviewItem{
			ID:      item.ID,
			Action:  item.Action,
			Details: item.Details,
		})
	}
	return out
}

func canonicalMoveResults(items []objectsvc.MoveBulkResult) []canonicalBulkResult {
	out := make([]canonicalBulkResult, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkResult{
			ID:      item.ID,
			Status:  item.Status,
			Reason:  item.Reason,
			Details: item.Details,
		})
	}
	return out
}

func deleteBacklinkCommandWarnings(backlinks []model.Reference) []commandexec.Warning {
	if len(backlinks) == 0 {
		return nil
	}

	backlinkIDs := make([]string, 0, len(backlinks))
	for _, bl := range backlinks {
		backlinkIDs = append(backlinkIDs, bl.SourceID)
	}

	return []commandexec.Warning{{
		Code:    "HAS_BACKLINKS",
		Message: fmt.Sprintf("Object is referenced by %d other objects", len(backlinks)),
		Ref:     strings.Join(backlinkIDs, ", "),
	}}
}

func boolArgDefault(args map[string]any, key string, defaultValue bool) bool {
	if args == nil {
		return defaultValue
	}
	if _, ok := args[key]; !ok {
		return defaultValue
	}
	return boolArg(args, key)
}

func isDailyNoteObjectID(objectID string, vaultCfg *config.VaultConfig) bool {
	if objectID == "" {
		return false
	}

	baseID := objectID
	if parts := strings.SplitN(objectID, "#", 2); len(parts) == 2 {
		baseID = parts[0]
	}

	dailyDir := "daily"
	if vaultCfg != nil && vaultCfg.GetDailyDirectory() != "" {
		dailyDir = vaultCfg.GetDailyDirectory()
	}
	if !strings.HasPrefix(baseID, dailyDir+"/") {
		return false
	}

	dateStr := strings.TrimPrefix(baseID, dailyDir+"/")
	return dates.IsValidDate(dateStr)
}

func mapTraitMutationError(err error) commandexec.Result {
	var validationErr *traitsvc.ValueValidationError
	if errors.As(err, &validationErr) {
		return commandexec.Failure("VALIDATION_FAILED", validationErr.Error(), nil, validationErr.Suggestion())
	}
	if svcErr, ok := traitsvc.AsError(err); ok {
		return commandexec.Failure(string(svcErr.Code), svcErr.Message, svcErr.Details, svcErr.Suggestion)
	}
	return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
}
