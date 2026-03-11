package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/vault"
)

var (
	addToFlag      string
	addHeadingFlag string
	addStdin       bool
	addConfirm     bool
)

var addCmd = &cobra.Command{
	Use:   "add <text>",
	Short: "Quick capture - append text to daily note or inbox",
	Long: `Quickly capture a thought, task, or note.

By default, appends to today's daily note. Configure destination in raven.yaml.
Auto-reindex is ON by default; configure via auto_reindex in raven.yaml.

Bulk operations:
  Use --stdin to read object IDs from stdin and append text to each.
  Bulk operations preview changes by default; use --confirm to apply.

Examples:
  rvn add "Call Odin about the Bifrost"
  rvn add "@due(tomorrow) Send the estimate"
  rvn add "Plan for tomorrow" --to tomorrow
  rvn add "Project idea" --to inbox.md
  rvn add "Fix parser edge case" --to project/raven --heading bugs-fixes
  rvn add "Capture under heading" --to project/raven --heading "### Bugs / Fixes"
  rvn add "Structured note" --to project/raven#bugs-fixes
  rvn add "Meeting notes" --to cursor       # Resolves to companies/cursor.md
  rvn add "Met with [[people/freya]]" --json

Bulk examples:
  rvn query "object:project .status==active" --ids | rvn add --stdin "Review scheduled for Q2"
  rvn query "object:project .status==active" --ids | rvn add --stdin "@reviewed(2026-01-07)" --confirm

Configuration (raven.yaml):
  capture:
    destination: daily      # "daily" or a file path
    heading: "## Captured"  # Optional heading to append under`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		// Handle --stdin mode for bulk operations
		if addStdin {
			return runAddBulk(args, vaultPath)
		}

		// Single capture mode - requires text argument
		if len(args) == 0 {
			return handleErrorMsg(ErrMissingArgument, "requires text argument", "Usage: rvn add <text>")
		}

		return addSingleCapture(vaultPath, args)
	},
}

// runAddBulk handles bulk add operations from stdin.
func runAddBulk(args []string, vaultPath string) error {
	headingSpec := effectiveAddHeadingSpec()

	// Text to append is provided as arguments
	if len(args) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no text to add", "Usage: rvn add --stdin <text>")
	}
	text := strings.Join(args, " ")

	// Read IDs from stdin
	fileIDs, embeddedIDs, err := ReadIDsFromStdin()
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	// Combine all IDs - bulk add supports embedded objects (sections) as targets.
	ids := append(fileIDs, embeddedIDs...)

	if len(ids) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
	}

	// Warnings are kept for parity with other bulk ops; add supports embedded targets so no special warning.
	var warnings []Warning

	// Load vault config
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}

	// Format the capture line
	line := formatCaptureLine(text)

	// If not confirming, show preview
	if !addConfirm {
		return previewAddBulk(vaultPath, ids, line, headingSpec, warnings, vaultCfg)
	}

	// Apply the additions
	return applyAddBulk(vaultPath, ids, line, headingSpec, warnings, vaultCfg)
}

// previewAddBulk shows a preview of bulk add operations.
func previewAddBulk(vaultPath string, ids []string, line string, headingSpec string, warnings []Warning, vaultCfg *config.VaultConfig) error {
	parseOpts := buildParseOptions(vaultCfg)
	previewResult, err := objectsvc.PreviewAddBulk(objectsvc.AddBulkRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		ObjectIDs:    ids,
		Line:         line,
		HeadingSpec:  headingSpec,
		ParseOptions: parseOpts,
	})
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	preview := &BulkPreview{
		Action:   "add",
		Items:    make([]BulkPreviewItem, 0, len(previewResult.Items)),
		Skipped:  make([]BulkResult, 0, len(previewResult.Skipped)),
		Total:    previewResult.Total,
		Warnings: warnings,
	}
	for _, item := range previewResult.Items {
		preview.Items = append(preview.Items, BulkPreviewItem{
			ID:      item.ID,
			Action:  item.Action,
			Details: item.Details,
		})
	}
	for _, skip := range previewResult.Skipped {
		preview.Skipped = append(preview.Skipped, BulkResult{
			ID:     skip.ID,
			Status: skip.Status,
			Reason: skip.Reason,
		})
	}

	return outputBulkPreview(preview, map[string]interface{}{
		"content": line,
	})
}

// applyAddBulk applies bulk add operations.
func applyAddBulk(vaultPath string, ids []string, line string, headingSpec string, warnings []Warning, vaultCfg *config.VaultConfig) error {
	parseOpts := buildParseOptions(vaultCfg)
	bulkResult, err := objectsvc.ApplyAddBulk(objectsvc.AddBulkRequest{
		VaultPath:    vaultPath,
		VaultConfig:  vaultCfg,
		ObjectIDs:    ids,
		Line:         line,
		HeadingSpec:  headingSpec,
		ParseOptions: parseOpts,
	}, func(filePath string) {
		maybeReindex(vaultPath, filePath, vaultCfg)
	})
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	results := make([]BulkResult, 0, len(bulkResult.Results))
	for _, result := range bulkResult.Results {
		results = append(results, BulkResult{
			ID:     result.ID,
			Status: result.Status,
			Reason: result.Reason,
		})
	}

	summary := buildBulkSummary("add", results, warnings)
	return outputBulkSummary(summary, warnings, map[string]interface{}{
		"content": line,
	})
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

// addSingleCapture handles single capture mode (non-bulk).
func addSingleCapture(vaultPath string, args []string) error {
	start := time.Now()
	headingSpec := effectiveAddHeadingSpec()

	// Load vault config
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}
	captureCfg := vaultCfg.GetCaptureConfig()
	parseOpts := buildParseOptions(vaultCfg)

	// Join all args as the capture text
	text := strings.Join(args, " ")

	// Determine destination file
	var destPath string
	var isDailyNote bool
	var targetObjectID string
	var fileObjectID string
	if addToFlag != "" {
		// Resolve --to flag using unified resolver (supports section refs like file#section).
		// If not found, fall back to dynamic date keywords (today/tomorrow/yesterday).
		result, err := resolveReferenceWithDynamicDates(addToFlag, ResolveOptions{
			VaultPath:    vaultPath,
			VaultConfig:  vaultCfg,
			AllowMissing: true,
		}, true)
		if err != nil {
			return handleResolveError(err, addToFlag)
		}
		destPath = result.FilePath
		targetObjectID = result.ObjectID
		fileObjectID = result.FileObjectID
		isDailyNote = isDailyNoteObjectID(result.FileObjectID, vaultCfg)
	} else if captureCfg.Destination == "daily" {
		// Use today's daily note (auto-created if needed)
		today := vault.FormatDateISO(time.Now())
		destPath = vaultCfg.DailyNotePath(vaultPath, today)
		fileObjectID = vaultCfg.DailyNoteID(today)
		isDailyNote = true
	} else {
		// Use configured destination
		destPath = filepath.Join(vaultPath, captureCfg.Destination)
		fileObjectID = vaultCfg.FilePathToObjectID(captureCfg.Destination)

		// Check if configured destination exists
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			return handleErrorMsg(ErrFileNotFound,
				fmt.Sprintf("Configured capture destination '%s' does not exist", captureCfg.Destination),
				"Create the file first or change capture.destination in raven.yaml")
		}
	}

	// Security: verify path is within vault
	if err := paths.ValidateWithinVault(vaultPath, destPath); err != nil {
		if errors.Is(err, paths.ErrPathOutsideVault) {
			return handleErrorMsg(ErrFileOutsideVault, fmt.Sprintf("cannot capture outside vault: %s", destPath), "")
		}
		return handleError(ErrInternal, err, "")
	}

	if headingSpec != "" {
		if targetObjectID != "" && strings.Contains(targetObjectID, "#") {
			return handleErrorMsg(ErrInvalidInput, "cannot combine --heading with a section reference in --to", "Use either --to <file#section> or --heading")
		}
		resolvedTarget, err := resolveAddHeadingTarget(vaultPath, destPath, fileObjectID, headingSpec, vaultCfg, parseOpts)
		if err != nil {
			return handleErrorMsg(ErrRefNotFound, err.Error(), "Use an existing section slug/id or heading text")
		}
		targetObjectID = resolvedTarget
	}

	// Create parent directories if needed
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	// Format the capture line
	line := formatCaptureLine(text)

	// Append to file (create if daily note)
	if err := appendToFile(vaultPath, destPath, line, captureCfg, vaultCfg, isDailyNote, targetObjectID); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	// Get line count for response
	lineNum := getFileLineCount(destPath)

	relPath, _ := filepath.Rel(vaultPath, destPath)

	// Check for broken references and build warnings
	var warnings []Warning
	refs := parser.ExtractRefs(text, 1)
	if len(refs) > 0 {
		warnings = append(warnings, validateRefs(vaultPath, refs, vaultCfg)...)
	}

	// Reindex if configured
	maybeReindex(vaultPath, destPath, vaultCfg)

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccessWithWarnings(CaptureResult{
			File:    relPath,
			Line:    lineNum,
			Content: line,
		}, warnings, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Println(ui.Checkf("Added to %s", ui.FilePath(relPath)))
	for _, w := range warnings {
		fmt.Printf("  ⚠ %s: %s\n", w.Code, w.Message)
		if w.CreateCommand != "" {
			fmt.Printf("    → %s\n", w.CreateCommand)
		}
	}

	return nil
}

func formatCaptureLine(text string) string {
	return text
}

func effectiveAddHeadingSpec() string {
	return strings.TrimSpace(addHeadingFlag)
}

func resolveAddHeadingTarget(
	vaultPath string,
	destPath string,
	fileObjectID string,
	headingSpec string,
	vaultCfg *config.VaultConfig,
	parseOpts *parser.ParseOptions,
) (string, error) {
	return objectsvc.ResolveAddHeadingTarget(vaultPath, destPath, fileObjectID, headingSpec, parseOpts)
}

func parseHeadingTextFromSpec(spec string) (string, bool) {
	trimmed := strings.TrimSpace(spec)
	if !strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	i := 0
	for i < len(trimmed) && trimmed[i] == '#' {
		i++
	}
	if i == 0 || i >= len(trimmed) || trimmed[i] != ' ' {
		return "", false
	}
	headingText := strings.TrimSpace(trimmed[i:])
	if headingText == "" {
		return "", false
	}
	return headingText, true
}

func appendToFile(vaultPath, destPath, line string, cfg *config.CaptureConfig, vaultCfg *config.VaultConfig, isDailyNote bool, targetObjectID string) error {
	parseOpts := buildParseOptions(vaultCfg)
	return objectsvc.AppendToFile(vaultPath, destPath, line, cfg, vaultCfg, isDailyNote, targetObjectID, parseOpts)
}

// getFileLineCount returns the number of lines in a file.
func getFileLineCount(path string) int {
	return objectsvc.FileLineCount(path)
}

// validateRefs checks if references exist and returns warnings for missing ones.
func validateRefs(vaultPath string, refs []parser.Reference, vaultCfg *config.VaultConfig) []Warning {
	var warnings []Warning

	// Load schema to infer types from default_path
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return warnings
	}

	// Determine daily directory from config
	dailyDir := "daily"
	if vaultCfg != nil && vaultCfg.GetDailyDirectory() != "" {
		dailyDir = vaultCfg.GetDailyDirectory()
	}

	// Open database and use its resolver (includes aliases and name_field values)
	db, err := index.Open(vaultPath)
	if err != nil {
		// Fall back to walking vault if database is unavailable
		return validateRefsWithoutDB(vaultPath, refs, sch, dailyDir)
	}
	defer db.Close()

	// Build resolver with aliases and name_field support
	res, err := db.Resolver(index.ResolverOptions{
		DailyDirectory: dailyDir,
		Schema:         sch,
	})
	if err != nil {
		return validateRefsWithoutDB(vaultPath, refs, sch, dailyDir)
	}

	for _, ref := range refs {
		// Try to resolve the reference
		resolved := res.Resolve(ref.TargetRaw)
		if resolved.TargetID == "" {
			// Reference not found - build a warning
			warning := Warning{
				Code:    WarnRefNotFound,
				Message: fmt.Sprintf("Reference [[%s]] does not exist", ref.TargetRaw),
				Ref:     ref.TargetRaw,
			}

			// Try to infer the type from the path
			suggestedType := inferTypeFromPath(sch, ref.TargetRaw)
			if suggestedType != "" {
				warning.SuggestedType = suggestedType
				warning.CreateCommand = buildCreateObjectCommand(suggestedType, ref.TargetRaw)
			}

			warnings = append(warnings, warning)
		}
	}

	return warnings
}

// validateRefsWithoutDB is a fallback for when the database is unavailable.
// It walks the vault files but does not support alias resolution.
func validateRefsWithoutDB(vaultPath string, refs []parser.Reference, sch *schema.Schema, dailyDir string) []Warning {
	var warnings []Warning

	// Collect existing object IDs by walking the vault
	var objectIDs []string
	_ = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Document != nil {
			for _, obj := range result.Document.Objects {
				objectIDs = append(objectIDs, obj.ID)
			}
		}
		return nil
	})

	// Build resolver with existing objects (no alias support needed for validation)
	res := resolver.New(objectIDs, resolver.Options{DailyDirectory: dailyDir})

	for _, ref := range refs {
		// Try to resolve the reference
		resolved := res.Resolve(ref.TargetRaw)
		if resolved.TargetID == "" {
			// Reference not found - build a warning
			warning := Warning{
				Code:    WarnRefNotFound,
				Message: fmt.Sprintf("Reference [[%s]] does not exist", ref.TargetRaw),
				Ref:     ref.TargetRaw,
			}

			// Try to infer the type from the path
			suggestedType := inferTypeFromPath(sch, ref.TargetRaw)
			if suggestedType != "" {
				warning.SuggestedType = suggestedType
				warning.CreateCommand = buildCreateObjectCommand(suggestedType, ref.TargetRaw)
			}

			warnings = append(warnings, warning)
		}
	}

	return warnings
}

// inferTypeFromPath tries to infer the type from a reference path based on default_path.
func inferTypeFromPath(sch *schema.Schema, refPath string) string {
	if sch == nil {
		return ""
	}

	// Check if the path matches any type's default_path
	parts := strings.Split(refPath, "/")
	if len(parts) >= 1 {
		dir := parts[0] + "/"
		for typeName, typeDef := range sch.Types {
			if typeDef != nil && typeDef.DefaultPath == dir {
				return typeName
			}
		}
	}

	return ""
}

func buildCreateObjectCommand(typeName, targetRaw string) string {
	title := filepath.Base(strings.TrimSpace(targetRaw))
	if title == "" || title == "." || title == "/" {
		title = "new-object"
	}
	return fmt.Sprintf("rvn new %s %q --json", typeName, title)
}

func init() {
	addCmd.Flags().StringVar(&addToFlag, "to", "", "Target file (path or reference like 'cursor')")
	addCmd.Flags().StringVar(&addHeadingFlag, "heading", "", "Target heading within destination (heading slug, object#heading ID, or markdown heading text)")
	addCmd.Flags().BoolVar(&addStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	addCmd.Flags().BoolVar(&addConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
	if err := addCmd.RegisterFlagCompletionFunc("to", completeReferenceFlag(true)); err != nil {
		panic(err)
	}
	rootCmd.AddCommand(addCmd)
}
