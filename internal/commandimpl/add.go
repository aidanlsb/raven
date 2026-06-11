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
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/schema"
)

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

	return runAddBulk(vaultPath, vaultCfg, objectIDs, text, strings.TrimSpace(stringArg(req.Args, "heading")), req.Confirm)
}

func runAddBulk(vaultPath string, vaultCfg *config.VaultConfig, ids []string, text string, headingSpec string, confirm bool) commandexec.Result {
	fileIDs, sectionIDs := splitSectionIDs(ids)
	warnings := sectionSkipWarnings(sectionIDs)
	request := objectsvc.AddBulkRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		ObjectIDs:    fileIDs,
		Line:         text,
		HeadingSpec:  headingSpec,
		ParseOptions: buildParseOptions(vaultCfg),
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

	var reindexWarnings []commandexec.Warning
	summary, err := objectsvc.ApplyAddBulk(request, func(filePath string) {
		reindexWarnings = appendCommandWarnings(reindexWarnings, autoReindexWarnings(vaultPath, vaultCfg, filePath))
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	warnings = appendCommandWarnings(warnings, reindexWarnings)
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
	parseOpts := buildParseOptions(vaultCfg)

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
	if err := objectsvc.ValidateContentMutationFilePath(vaultPath, vaultCfg, destPath); err != nil {
		return mapContentMutationError(err)
	}

	if headingSpec != "" {
		if targetObjectID != "" && strings.Contains(targetObjectID, "#") {
			return commandexec.Failure("INVALID_INPUT", "cannot combine --heading with a section reference in --to", nil, "Use either --to <file#section> or --heading")
		}
		resolvedTarget, err := objectsvc.ResolveAddHeadingTarget(vaultPath, destPath, fileObjectID, headingSpec, parseOpts)
		if err != nil {
			return mapContentMutationError(err)
		}
		targetObjectID = resolvedTarget
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return commandexec.Failure("FILE_WRITE_ERROR", err.Error(), nil, "")
	}
	line, err := objectsvc.AppendToFile(vaultPath, destPath, text, captureCfg, vaultCfg, isDailyNote, targetObjectID, parseOpts)
	if err != nil {
		return mapContentMutationError(err)
	}

	warnings := autoReindexWarnings(vaultPath, vaultCfg, destPath)
	relPath, _ := filepath.Rel(vaultPath, destPath)
	return commandexec.SuccessWithWarnings(map[string]interface{}{
		"file":    filepath.ToSlash(relPath),
		"line":    line,
		"content": text,
	}, warnings, nil)
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
