package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
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
  rvn add "Plan for tomorrow" --to tomorrow
  rvn add "Project idea" --to inbox.md
  rvn add "Meeting notes" --to cursor       # Resolves to companies/cursor.md
  rvn add "Call Odin" --timestamp           # Includes time prefix
  rvn add "Met with [[people/freya]]" --json

Bulk examples:
  rvn query "object:project .status==active" --ids | rvn add --stdin "Review scheduled for Q2"
  rvn query "object:project .status==active" --ids | rvn add --stdin "@reviewed(2026-01-07)" --confirm

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
	parseOpts := buildParseOptions(vaultCfg)

	preview := buildBulkPreview("add", ids, warnings, func(id string) (*BulkPreviewItem, *BulkResult) {
		// Resolve to a file path. For embedded IDs, resolve their parent file.
		fileID := id
		if baseID, _, isEmbedded := paths.ParseEmbeddedID(id); isEmbedded {
			fileID = baseID
		}
		filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileID, vaultCfg)
		if err != nil {
			return nil, &BulkResult{ID: id, Status: "skipped", Reason: "object not found"}
		}
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return nil, &BulkResult{ID: id, Status: "skipped", Reason: "file not found"}
		}

		// For embedded IDs, ensure the target section exists in the file.
		if IsEmbeddedID(id) {
			content, err := os.ReadFile(filePath)
			if err != nil {
				return nil, &BulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("read error: %v", err)}
			}
			doc, err := parser.ParseDocumentWithOptions(string(content), filePath, vaultPath, parseOpts)
			if err != nil {
				return nil, &BulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("parse error: %v", err)}
			}
			found := false
			for _, obj := range doc.Objects {
				if obj != nil && obj.ID == id {
					found = true
					break
				}
			}
			if !found {
				return nil, &BulkResult{ID: id, Status: "skipped", Reason: "embedded object not found"}
			}
		}

		details := fmt.Sprintf("append: %s", line)
		if IsEmbeddedID(id) {
			details = fmt.Sprintf("append within %s: %s", id, line)
		}
		return &BulkPreviewItem{
			ID:      id,
			Action:  "add",
			Details: details,
		}, nil
	})

	return outputBulkPreview(preview, map[string]interface{}{
		"content": line,
	})
}

// applyAddBulk applies bulk add operations.
func applyAddBulk(vaultPath string, ids []string, line string, warnings []Warning, vaultCfg *config.VaultConfig) error {
	captureCfg := vaultCfg.GetCaptureConfig()

	results := applyBulk(ids, func(id string) BulkResult {
		result := BulkResult{ID: id}
		// Resolve to a file path. For embedded IDs, resolve their parent file and
		// pass the embedded ID through so appendToFile can target the correct section.
		fileID := id
		targetObjectID := ""
		if baseID, _, isEmbedded := paths.ParseEmbeddedID(id); isEmbedded {
			fileID = baseID
			targetObjectID = id
		}

		filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileID, vaultCfg)
		if err != nil {
			result.Status = "skipped"
			result.Reason = "object not found"
			return result
		}

		// Append to file (never create for bulk operations)
		if err := appendToFile(vaultPath, filePath, line, captureCfg, vaultCfg, false, targetObjectID); err != nil {
			result.Status = "error"
			result.Reason = fmt.Sprintf("append failed: %v", err)
			return result
		}

		// Reindex if configured
		maybeReindex(vaultPath, filePath, vaultCfg)

		result.Status = "added"
		return result
	})

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
	if vaultCfg != nil && vaultCfg.DailyDirectory != "" {
		dailyDir = vaultCfg.DailyDirectory
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

	// Load vault config
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
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
	var targetObjectID string
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
		isDailyNote = isDailyNoteObjectID(result.FileObjectID, vaultCfg)
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
	if err := paths.ValidateWithinVault(vaultPath, destPath); err != nil {
		if errors.Is(err, paths.ErrPathOutsideVault) {
			return handleErrorMsg(ErrFileOutsideVault, fmt.Sprintf("cannot capture outside vault: %s", destPath), "")
		}
		return handleError(ErrInternal, err, "")
	}

	// Create parent directories if needed
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	// Format the capture line
	line := formatCaptureLine(text, captureCfg)

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

func formatCaptureLine(text string, cfg *config.CaptureConfig) string {
	var parts []string

	// Add timestamp if configured
	if cfg.Timestamp != nil && *cfg.Timestamp {
		parts = append(parts, time.Now().Format("15:04"))
	}

	parts = append(parts, text)

	return "- " + strings.Join(parts, " ")
}

func appendToFile(vaultPath, destPath, line string, cfg *config.CaptureConfig, vaultCfg *config.VaultConfig, isDailyNote bool, targetObjectID string) error {
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
			t, err := time.Parse(dates.DateLayout, dateStr)
			if err != nil {
				t = time.Now()
				dateStr = vault.FormatDateISO(t)
			}
			friendlyTitle := vault.FormatDateFriendly(t)
			dailyDir := vaultCfg.DailyDirectory
			if dailyDir == "" {
				dailyDir = "daily"
			}
			if _, err := pages.CreateDailyNoteWithTemplate(vaultPath, dailyDir, dateStr, friendlyTitle, vaultCfg.DailyTemplate, vaultCfg.GetTemplateDirectory()); err != nil {
				return fmt.Errorf("failed to create daily note: %w", err)
			}
		} else {
			// This shouldn't happen - we check earlier, but just in case
			return fmt.Errorf("file does not exist: %s", destPath)
		}
	}

	// If a specific embedded/section object is targeted, append within that object's range.
	// This overrides capture.heading behavior for this operation.
	if targetObjectID != "" && strings.Contains(targetObjectID, "#") {
		return appendWithinObject(vaultPath, destPath, line, targetObjectID, vaultCfg)
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

func appendWithinObject(vaultPath, destPath, line, objectID string, vaultCfg *config.VaultConfig) error {
	contentBytes, err := os.ReadFile(destPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	content := string(contentBytes)
	lines := strings.Split(content, "\n")

	var opts *parser.ParseOptions
	if vaultCfg != nil {
		opts = &parser.ParseOptions{
			ObjectsRoot: vaultCfg.GetObjectsRoot(),
			PagesRoot:   vaultCfg.GetPagesRoot(),
		}
	}

	doc, err := parser.ParseDocumentWithOptions(content, destPath, vaultPath, opts)
	if err != nil {
		return fmt.Errorf("failed to parse document: %w", err)
	}

	var target *parser.ParsedObject
	for _, obj := range doc.Objects {
		if obj != nil && obj.ID == objectID {
			target = obj
			break
		}
	}
	if target == nil {
		return fmt.Errorf("target section not found: %s", objectID)
	}

	// Insertion point is just before the next object's heading (or EOF).
	// LineEnd is 1-indexed (inclusive); inserting before the next heading corresponds
	// to inserting at index == LineEnd (since slice indices are 0-indexed).
	insertIdx := len(lines)
	if target.LineEnd != nil {
		insertIdx = *target.LineEnd
		if insertIdx < 0 {
			insertIdx = 0
		}
		if insertIdx > len(lines) {
			insertIdx = len(lines)
		}
	}

	// Avoid inserting above the heading itself. LineStart is 1-indexed; the earliest
	// valid insertion point is the line after the heading: index == LineStart.
	minInsertIdx := target.LineStart
	if minInsertIdx < 0 {
		minInsertIdx = 0
	}
	if minInsertIdx > len(lines) {
		minInsertIdx = len(lines)
	}

	// Insert before trailing empty lines at the boundary (mirrors appendUnderHeading behavior).
	for insertIdx > minInsertIdx && strings.TrimSpace(lines[insertIdx-1]) == "" {
		insertIdx--
	}

	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, line)
	newLines = append(newLines, lines[insertIdx:]...)

	return atomicfile.WriteFile(destPath, []byte(strings.Join(newLines, "\n")), 0o644)
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

	return atomicfile.WriteFile(destPath, []byte(strings.Join(newLines, "\n")), 0o644)
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

func init() {
	addCmd.Flags().StringVar(&addToFlag, "to", "", "Target file (path or reference like 'cursor')")
	addCmd.Flags().BoolVar(&addTimestampFlag, "timestamp", false, "Prefix with current time (HH:MM)")
	addCmd.Flags().BoolVar(&addStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	addCmd.Flags().BoolVar(&addConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
	rootCmd.AddCommand(addCmd)
}
