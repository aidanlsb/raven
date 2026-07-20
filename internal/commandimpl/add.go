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

	rt, failure := newRequiredCommandVaultRuntime(vaultPath, false)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg
	sch := rt.Schema

	headingSpec := strings.TrimSpace(stringArg(req.Args, "heading"))
	createHeading := boolArg(req.Args, "create-heading")
	if createHeading && headingSpec == "" {
		return commandexec.Failure("INVALID_INPUT", "--create-heading requires --heading", nil, `Pass --heading with heading text, e.g. --heading "Team Notes"`)
	}

	if !stdinMode {
		return runAddSingle(vaultPath, vaultCfg, sch, text, strings.TrimSpace(stringArg(req.Args, "to")), headingSpec, createHeading)
	}
	if len(objectIDs) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no object IDs provided via stdin", nil, "Pipe object IDs to stdin, one per line")
	}

	return runAddBulk(vaultPath, vaultCfg, sch, objectIDs, text, headingSpec, createHeading, req.Confirm)
}

func runAddBulk(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, ids []string, text string, headingSpec string, createHeading bool, confirm bool) commandexec.Result {
	// Section IDs (file#slug) are passed through: bulk add appends within the
	// targeted section instead of at the end of the file.
	var warnings []commandexec.Warning
	request := objectsvc.AddBulkRequest{
		VaultPath:     vaultPath,
		VaultConfig:   vaultCfg,
		ObjectIDs:     ids,
		Line:          text,
		HeadingSpec:   headingSpec,
		CreateHeading: createHeading,
		ParseOptions:  buildParseOptions(vaultCfg),
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
	var affectedFiles []string
	summary, err := objectsvc.ApplyAddBulk(request, func(filePath string) {
		reindexWarnings = appendCommandWarnings(reindexWarnings, autoReindexWarnings(vaultPath, vaultCfg, filePath))
		if rel, relErr := filepath.Rel(vaultPath, filePath); relErr == nil {
			affectedFiles = append(affectedFiles, rel)
		}
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	data := map[string]interface{}{
		"ok":      summary.Errors == 0,
		"action":  summary.Action,
		"items":   canonicalAddResults(summary.Results),
		"total":   summary.Total,
		"skipped": summary.Skipped,
		"errors":  summary.Errors,
		"added":   summary.Added,
		"content": text,
	}
	missingData, missingWarnings := missingRefEnvelope(vaultPath, vaultCfg, sch, affectedFiles...)
	data = mergeDataFields(data, missingData)
	warnings = appendCommandWarnings(warnings, reindexWarnings, missingWarnings)
	return commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}

func runAddSingle(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, text, toRef, headingSpec string, createHeading bool) commandexec.Result {
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

	createdHeading := false
	createdSectionID := ""
	if headingSpec != "" {
		if targetObjectID != "" && strings.Contains(targetObjectID, "#") {
			return commandexec.Failure("INVALID_INPUT", "cannot combine --heading with a section reference in --to", nil, "Use either --to <file#section> or --heading")
		}
		resolvedTarget, err := objectsvc.ResolveAddHeadingTarget(vaultPath, destPath, fileObjectID, headingSpec, parseOpts)
		if err != nil {
			if !createHeading || !objectsvc.IsRefNotFound(err) {
				return mapContentMutationError(err)
			}
			createdHeading = true
		} else {
			targetObjectID = resolvedTarget
		}
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return commandexec.Failure("FILE_WRITE_ERROR", err.Error(), nil, "")
	}
	var line int
	var err error
	if createdHeading {
		line, createdSectionID, err = objectsvc.AppendUnderNewHeading(vaultPath, destPath, fileObjectID, text, headingSpec, parseOpts)
	} else {
		line, err = objectsvc.AppendToFile(vaultPath, destPath, text, captureCfg, vaultCfg, isDailyNote, targetObjectID, parseOpts)
	}
	if err != nil {
		return mapContentMutationError(err)
	}

	warnings := autoReindexWarnings(vaultPath, vaultCfg, destPath)
	relPath, _ := filepath.Rel(vaultPath, destPath)
	data := map[string]interface{}{
		"file":    filepath.ToSlash(relPath),
		"line":    line,
		"content": text,
	}
	if createdHeading {
		data["created_heading"] = true
		if createdSectionID != "" {
			data["section"] = createdSectionID
		}
	}
	missingData, missingWarnings := missingRefEnvelope(vaultPath, vaultCfg, sch, relPath)
	data = mergeDataFields(data, missingData)
	warnings = appendCommandWarnings(warnings, missingWarnings)
	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

func isDailyNoteObjectID(objectID string, vaultCfg *config.VaultConfig) bool {
	if objectID == "" {
		return false
	}

	baseID := objectID
	if parts := strings.SplitN(objectID, "#", 2); len(parts) == 2 {
		baseID = parts[0]
	}

	// Canonical daily-note object IDs are bare ISO dates.
	if dates.IsValidDate(baseID) {
		return true
	}

	// Legacy compatibility: daily-directory-prefixed object IDs.
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
