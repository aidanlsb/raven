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
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
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
	if containsMarkdownHeading(text) {
		return removedAddHeadingFailure()
	}
	if _, hasHeading := req.Args["heading"]; hasHeading {
		return removedAddHeadingFailure()
	}
	if _, hasCreateHeading := req.Args["create-heading"]; hasCreateHeading {
		return removedAddHeadingFailure()
	}

	objectIDs := commandIDsArg(req.Args, "object_ids")
	stdinMode := boolArg(req.Args, "stdin") || len(objectIDs) > 0

	rt, failure := newRequiredCommandVaultRuntime(vaultPath, false)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	if !stdinMode {
		return runAddSingle(rt, text, strings.TrimSpace(stringArg(req.Args, "to")))
	}
	if len(objectIDs) == 0 {
		return commandexec.Failure("MISSING_ARGUMENT", "no object IDs provided via stdin", nil, "Pipe object IDs to stdin, one per line")
	}

	return runAddBulk(rt, objectIDs, text, req.Confirm)
}

func runAddBulk(rt *vaultruntime.Runtime, ids []string, text string, confirm bool) commandexec.Result {
	vaultPath := rt.VaultPath
	vaultCfg := rt.VaultCfg
	// Section IDs (file#slug) are passed through: bulk add appends within the
	// targeted section instead of at the end of the file.
	var warnings []commandexec.Warning
	request := objectsvc.AddBulkRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		ObjectIDs:    ids,
		Line:         text,
		ParseOptions: rt.ParseOptions,
		Runtime:      rt,
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

	summary, err := objectsvc.ApplyAddBulk(request)
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
	postData, postWarnings := applyChangeSet(rt, summary.ChangeSet)
	data = mergeDataFields(data, postData)
	warnings = appendCommandWarnings(warnings, postWarnings)
	return commandexec.SuccessWithWarnings(data, warnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}

func runAddSingle(rt *vaultruntime.Runtime, text, toRef string) commandexec.Result {
	vaultPath := rt.VaultPath
	vaultCfg := rt.VaultCfg
	captureCfg := vaultCfg.GetCaptureConfig()

	var destPath string
	var isDailyNote bool
	var targetObjectID string

	if strings.TrimSpace(toRef) != "" {
		resolved, err := readsvc.ResolveReferenceWithDynamicDates(toRef, rt, true)
		if err != nil {
			return mapResolveFailure(err, toRef)
		}
		destPath = resolved.FilePath
		targetObjectID = resolved.ObjectID
		isDailyNote = isDailyNoteObjectID(resolved.FileObjectID, vaultCfg)
	} else if captureCfg.Destination == "daily" {
		today := time.Now()
		dateStr := fmt.Sprintf("%04d-%02d-%02d", today.Year(), today.Month(), today.Day())
		destPath = vaultCfg.DailyNotePath(vaultPath, dateStr)
		isDailyNote = true
	} else {
		destPath = filepath.Join(vaultPath, captureCfg.Destination)
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

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return commandexec.Failure("FILE_WRITE_ERROR", err.Error(), nil, "")
	}
	appendResult, err := objectsvc.Append(rt, destPath, text, captureCfg, isDailyNote, targetObjectID)
	if err != nil {
		return mapContentMutationError(err)
	}

	relPath, _ := filepath.Rel(vaultPath, destPath)
	data := map[string]interface{}{
		"file":    filepath.ToSlash(relPath),
		"line":    appendResult.Line,
		"content": text,
	}
	postData, postWarnings := applyChangeSet(rt, appendResult.ChangeSet)
	data = mergeDataFields(data, postData)
	return commandexec.SuccessWithWarnings(data, postWarnings, nil)
}

func removedAddHeadingFailure() commandexec.Result {
	return commandexec.Failure(
		"INVALID_INPUT",
		"rvn add only appends body content; it does not accept or create headings",
		nil,
		`Create the heading with 'rvn section create <file> "<title>" --level N', then append content with 'rvn add <text> --to <file#section>'`,
	)
}

func containsMarkdownHeading(text string) bool {
	extracted, err := parser.ExtractFromAST([]byte(text), 1)
	return err == nil && len(extracted.Headings) > 0
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
