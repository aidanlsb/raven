package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/vault"
)

var (
	addToFlag        string
	addTimestampFlag bool
	addStdin         bool
	addConfirm       bool
)

var addCmd = &cobra.Command{
	Use:   "add <text>",
	Short: "Quick capture - append text to daily note or inbox",
	Long: `Quickly capture a thought, task, or note.

By default, appends to today's daily note. Configure destination in raven.yaml.
Timestamps are OFF by default; use --timestamp to include the current time.
Auto-reindex is ON by default; configure via auto_reindex in raven.yaml.

Bulk operations:
  Use --stdin to read object IDs from stdin and append text to each.
  Bulk operations preview changes by default; use --confirm to apply.

Examples:
  rvn add "Call Odin about the Bifrost"
  rvn add "@due(tomorrow) Send the estimate"
  rvn add "Project idea" --to inbox.md
  rvn add "Meeting notes" --to cursor       # Resolves to companies/cursor.md
  rvn add "Call Odin" --timestamp           # Includes time prefix
  rvn add "Met with [[people/freya]]" --json

Bulk examples:
  rvn query "object:project .status:active" --ids | rvn add --stdin "Review scheduled for Q2"
  rvn query "object:project .status:active" --ids | rvn add --stdin "@reviewed(2026-01-07)" --confirm

Configuration (raven.yaml):
  capture:
    destination: daily      # "daily" or a file path
    heading: "## Captured"  # Optional heading to append under
    timestamp: false        # Prefix with time (default: false)`,
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
	// Text to append is provided as arguments
	if len(args) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no text to add", "Usage: rvn add --stdin <text>")
	}
	text := strings.Join(args, " ")

	// Read IDs from stdin
	ids, embedded, err := ReadIDsFromStdin()
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if len(ids) == 0 && len(embedded) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
	}

	// Build warnings for embedded objects
	var warnings []Warning
	if w := BuildEmbeddedSkipWarning(embedded); w != nil {
		warnings = append(warnings, *w)
	}

	// Load vault config
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	captureCfg := vaultCfg.GetCaptureConfig()
	if addTimestampFlag {
		captureCfg.Timestamp = boolPtr(true)
	}

	// Format the capture line
	line := formatCaptureLine(text, captureCfg)

	// If not confirming, show preview
	if !addConfirm {
		return previewAddBulk(vaultPath, ids, line, warnings, vaultCfg)
	}

	// Apply the additions
	return applyAddBulk(vaultPath, ids, line, warnings, vaultCfg)
}

// previewAddBulk shows a preview of bulk add operations.
func previewAddBulk(vaultPath string, ids []string, line string, warnings []Warning, vaultCfg *config.VaultConfig) error {
	var previewItems []BulkPreviewItem
	var skipped []BulkResult

	for _, id := range ids {
		filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
		if err != nil {
			skipped = append(skipped, BulkResult{
				ID:     id,
				Status: "skipped",
				Reason: "object not found",
			})
			continue
		}

		// Verify file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			skipped = append(skipped, BulkResult{
				ID:     id,
				Status: "skipped",
				Reason: "file not found",
			})
			continue
		}

		previewItems = append(previewItems, BulkPreviewItem{
			ID:      id,
			Action:  "add",
			Details: fmt.Sprintf("append: %s", line),
		})
	}

	preview := &BulkPreview{
		Action:   "add",
		Items:    previewItems,
		Skipped:  skipped,
		Total:    len(ids),
		Warnings: warnings,
	}

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"preview":  true,
			"action":   "add",
			"content":  line,
			"items":    previewItems,
			"skipped":  skipped,
			"total":    len(ids),
			"warnings": warnings,
		}, &Meta{Count: len(previewItems)})
		return nil
	}

	PrintBulkPreview(preview)
	return nil
}

// applyAddBulk applies bulk add operations.
func applyAddBulk(vaultPath string, ids []string, line string, warnings []Warning, vaultCfg *config.VaultConfig) error {
	var results []BulkResult
	added := 0
	skipped := 0
	errors := 0

	captureCfg := vaultCfg.GetCaptureConfig()

	for _, id := range ids {
		result := BulkResult{ID: id}

		filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
		if err != nil {
			result.Status = "skipped"
			result.Reason = "object not found"
			skipped++
			results = append(results, result)
			continue
		}

		// Append to file (never create for bulk operations)
		if err := appendToFile(vaultPath, filePath, line, captureCfg, vaultCfg, false); err != nil {
			result.Status = "error"
			result.Reason = fmt.Sprintf("append failed: %v", err)
			errors++
			results = append(results, result)
			continue
		}

		// Reindex if configured
		if vaultCfg.IsAutoReindexEnabled() {
			reindexFile(vaultPath, filePath)
		}

		result.Status = "added"
		added++
		results = append(results, result)
	}

	summary := &BulkSummary{
		Action:  "add",
		Results: results,
		Total:   len(ids),
		Added:   added,
		Skipped: skipped,
		Errors:  errors,
	}

	if isJSONOutput() {
		data := map[string]interface{}{
			"ok":      errors == 0,
			"action":  "add",
			"content": line,
			"results": results,
			"total":   len(ids),
			"added":   added,
			"skipped": skipped,
			"errors":  errors,
		}
		if len(warnings) > 0 {
			outputSuccessWithWarnings(data, warnings, &Meta{Count: added})
		} else {
			outputSuccess(data, &Meta{Count: added})
		}
		return nil
	}

	PrintBulkSummary(summary)
	for _, w := range warnings {
		fmt.Printf("⚠ %s\n", w.Message)
	}
	return nil
}

// addSingleCapture handles single capture mode (non-bulk).
func addSingleCapture(vaultPath string, args []string) error {
	start := time.Now()

	// Load vault config
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	captureCfg := vaultCfg.GetCaptureConfig()

	// Override timestamp if flag is set
	if addTimestampFlag {
		captureCfg.Timestamp = boolPtr(true)
	}

	// Join all args as the capture text
	text := strings.Join(args, " ")

	// Determine destination file
	var destPath string
	var isDailyNote bool
	if addToFlag != "" {
		// Resolve --to flag using unified resolver
		resolvedPath, err := resolveToPath(vaultPath, addToFlag, vaultCfg)
		if err != nil {
			return handleResolveError(err, addToFlag)
		}
		destPath = resolvedPath
	} else if captureCfg.Destination == "daily" {
		// Use today's daily note (auto-created if needed)
		today := vault.FormatDateISO(time.Now())
		destPath = vaultCfg.DailyNotePath(vaultPath, today)
		isDailyNote = true
	} else {
		// Use configured destination
		destPath = filepath.Join(vaultPath, captureCfg.Destination)

		// Check if configured destination exists
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			return handleErrorMsg(ErrFileNotFound,
				fmt.Sprintf("Configured capture destination '%s' does not exist", captureCfg.Destination),
				"Create the file first or change capture.destination in raven.yaml")
		}
	}

	// Security: verify path is within vault
	absVault, err := filepath.Abs(vaultPath)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}
	absDest, err := filepath.Abs(destPath)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}
	if !strings.HasPrefix(absDest, absVault+string(filepath.Separator)) && absDest != absVault {
		return handleErrorMsg(ErrFileOutsideVault, fmt.Sprintf("cannot capture outside vault: %s", destPath), "")
	}

	// Create parent directories if needed
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	// Format the capture line
	line := formatCaptureLine(text, captureCfg)

	// Append to file (create if daily note)
	if err := appendToFile(vaultPath, destPath, line, captureCfg, vaultCfg, isDailyNote); err != nil {
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
	if vaultCfg.IsAutoReindexEnabled() {
		if err := reindexFile(vaultPath, destPath); err != nil {
			if !isJSONOutput() {
				fmt.Printf("  (reindex failed: %v)\n", err)
			}
		}
	}

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

func formatCaptureLine(text string, cfg *config.CaptureConfig) string {
	var parts []string

	// Add timestamp if configured
	if cfg.Timestamp != nil && *cfg.Timestamp {
		parts = append(parts, time.Now().Format("15:04"))
	}

	parts = append(parts, text)

	return "- " + strings.Join(parts, " ")
}

func appendToFile(vaultPath, destPath, line string, cfg *config.CaptureConfig, vaultCfg *config.VaultConfig, isDailyNote bool) error {
	// Check if file exists
	fileExists := true
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		fileExists = false
	}

	// If file doesn't exist and it's a daily note, create it
	if !fileExists {
		if isDailyNote {
			// Extract date from path and create using pages package
			base := filepath.Base(destPath)
			dateStr := strings.TrimSuffix(base, ".md")
			t, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				t = time.Now()
				dateStr = vault.FormatDateISO(t)
			}
			friendlyTitle := vault.FormatDateFriendly(t)
			dailyDir := vaultCfg.DailyDirectory
			if dailyDir == "" {
				dailyDir = "daily"
			}
			if _, err := pages.CreateDailyNoteWithTemplate(vaultPath, dailyDir, dateStr, friendlyTitle, vaultCfg.DailyTemplate); err != nil {
				return fmt.Errorf("failed to create daily note: %w", err)
			}
			fileExists = true
		} else {
			// This shouldn't happen - we check earlier, but just in case
			return fmt.Errorf("file does not exist: %s", destPath)
		}
	}

	// If heading is configured, find or create it
	if cfg.Heading != "" {
		return appendUnderHeading(destPath, line, cfg.Heading)
	}

	// Simple append to end of file
	f, err := os.OpenFile(destPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	// Ensure we're on a new line
	stat, _ := f.Stat()
	if stat.Size() > 0 {
		// Check if file ends with newline
		content, _ := os.ReadFile(destPath)
		if len(content) > 0 && content[len(content)-1] != '\n' {
			if _, err := f.WriteString("\n"); err != nil {
				return fmt.Errorf("failed to write newline: %w", err)
			}
		}
	}

	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("failed to write capture: %w", err)
	}

	return nil
}

func appendUnderHeading(destPath, line, heading string) error {
	content, err := os.ReadFile(destPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")

	// Find the heading
	headingIdx := -1
	nextHeadingIdx := -1
	headingLevel := strings.Count(strings.Split(heading, " ")[0], "#")

	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == heading {
			headingIdx = i
			continue
		}
		// If we found our heading, look for the next heading of same or higher level
		if headingIdx >= 0 && strings.HasPrefix(trimmed, "#") {
			level := 0
			for _, c := range trimmed {
				if c == '#' {
					level++
				} else {
					break
				}
			}
			if level <= headingLevel {
				nextHeadingIdx = i
				break
			}
		}
	}

	var newLines []string
	if headingIdx == -1 {
		// Heading doesn't exist, add it at the end
		newLines = append(lines, "", heading, line)
	} else if nextHeadingIdx == -1 {
		// Heading exists, no next heading, append at end
		// But insert before any trailing empty lines
		insertIdx := len(lines)
		for insertIdx > headingIdx+1 && strings.TrimSpace(lines[insertIdx-1]) == "" {
			insertIdx--
		}
		newLines = append(lines[:insertIdx], line)
		newLines = append(newLines, lines[insertIdx:]...)
	} else {
		// Insert before the next heading
		insertIdx := nextHeadingIdx
		// Skip back over empty lines
		for insertIdx > headingIdx+1 && strings.TrimSpace(lines[insertIdx-1]) == "" {
			insertIdx--
		}
		newLines = append(lines[:insertIdx], line)
		newLines = append(newLines, lines[insertIdx:]...)
	}

	return os.WriteFile(destPath, []byte(strings.Join(newLines, "\n")), 0644)
}

func reindexFile(vaultPath, filePath string) error {
	// Load schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return err
	}

	// Read and parse the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	doc, err := parser.ParseDocument(string(content), filePath, vaultPath)
	if err != nil {
		return err
	}

	// Open database and index
	db, err := index.Open(vaultPath)
	if err != nil {
		return err
	}
	defer db.Close()

	return db.IndexDocument(doc, sch)
}

// readLastLine reads the last line of a file to check if it ends with newline
func readLastLine(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lastLine string
	for scanner.Scan() {
		lastLine = scanner.Text()
	}
	return lastLine, scanner.Err()
}

// getFileLineCount returns the number of lines in a file.
func getFileLineCount(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(content), "\n")
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
	if vaultCfg != nil && vaultCfg.DailyDirectory != "" {
		dailyDir = vaultCfg.DailyDirectory
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
				warning.CreateCommand = fmt.Sprintf("rvn object create %s --title \"%s\" --json",
					suggestedType, filepath.Base(ref.TargetRaw))
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
	vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Document != nil {
			for _, obj := range result.Document.Objects {
				objectIDs = append(objectIDs, obj.ID)
			}
		}
		return nil
	})

	// Build resolver with existing objects (no alias support)
	res := resolver.NewWithDailyDir(objectIDs, dailyDir)

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
				warning.CreateCommand = fmt.Sprintf("rvn object create %s --title \"%s\" --json",
					suggestedType, filepath.Base(ref.TargetRaw))
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

// boolPtr returns a pointer to the given bool value.
func boolPtr(b bool) *bool {
	return &b
}

// resolveToPath resolves the --to flag value to a file path using the unified resolver.
func resolveToPath(vaultPath, toValue string, vaultCfg *config.VaultConfig) (string, error) {
	return ResolveReferenceToFile(toValue, ResolveOptions{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
	})
}

func init() {
	addCmd.Flags().StringVar(&addToFlag, "to", "", "Target file (path or reference like 'cursor')")
	addCmd.Flags().BoolVar(&addTimestampFlag, "timestamp", false, "Prefix with current time (HH:MM)")
	addCmd.Flags().BoolVar(&addStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	addCmd.Flags().BoolVar(&addConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
	rootCmd.AddCommand(addCmd)
}
