package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ravenscroftj/raven/internal/audit"
	"github.com/ravenscroftj/raven/internal/config"
	"github.com/ravenscroftj/raven/internal/index"
	"github.com/ravenscroftj/raven/internal/parser"
	"github.com/ravenscroftj/raven/internal/resolver"
	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/ravenscroftj/raven/internal/vault"
	"github.com/spf13/cobra"
)

var (
	addToFlag string
)

// AddResultJSON is the JSON representation of an add command result.
type AddResultJSON struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

var addCmd = &cobra.Command{
	Use:   "add <text>",
	Short: "Quick capture - append text to daily note or inbox",
	Long: `Quickly capture a thought, task, or note.

By default, appends to today's daily note. Configure destination in raven.yaml.

Examples:
  rvn add "Call Alice about the project"
  rvn add "@due(tomorrow) Send the estimate"
  rvn add "Project idea" --to inbox.md
  rvn add "@priority(high) Urgent task" --to projects/ideas.md
  rvn add "Met with [[people/alice]]" --json

Configuration (raven.yaml):
  capture:
    destination: daily      # "daily" or a file path
    heading: "## Captured"  # Optional heading to append under
    timestamp: true         # Prefix with time
    reindex: true           # Reindex after capture`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()

		// Load vault config
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		captureCfg := vaultCfg.GetCaptureConfig()

		// Join all args as the capture text
		text := strings.Join(args, " ")

		// Determine destination file
		var destPath string
		if addToFlag != "" {
			// Override with --to flag
			destPath = filepath.Join(vaultPath, addToFlag)
		} else if captureCfg.Destination == "daily" {
			// Use today's daily note
			today := vault.FormatDateISO(time.Now())
			destPath = vaultCfg.DailyNotePath(vaultPath, today)
		} else {
			// Use configured destination
			destPath = filepath.Join(vaultPath, captureCfg.Destination)
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

		// Append to file
		if err := appendToFile(destPath, line, captureCfg, vaultCfg); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		// Get line count for response
		lineNum := getFileLineCount(destPath)

		relPath, _ := filepath.Rel(vaultPath, destPath)

		// Log to audit log if enabled
		auditLogger := audit.New(vaultPath, vaultCfg.IsAuditLogEnabled())
		auditLogger.LogCapture(relPath, line)

		// Check for broken references and build warnings
		var warnings []Warning
		refs := parser.ExtractRefs(text, 1)
		if len(refs) > 0 {
			warnings = validateRefs(vaultPath, refs)
		}

		// Reindex if configured
		if captureCfg.Reindex != nil && *captureCfg.Reindex {
			if err := reindexFile(vaultPath, destPath); err != nil {
				if !isJSONOutput() {
					fmt.Printf("  (reindex failed: %v)\n", err)
				}
			}
		}

		elapsed := time.Since(start).Milliseconds()

		if isJSONOutput() {
			outputSuccessWithWarnings(AddResultJSON{
				File:    relPath,
				Line:    lineNum,
				Content: line,
			}, warnings, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		// Human-readable output
		fmt.Printf("✓ Added to %s\n", relPath)
		for _, w := range warnings {
			fmt.Printf("  ⚠ %s: %s\n", w.Code, w.Message)
			if w.CreateCommand != "" {
				fmt.Printf("    → %s\n", w.CreateCommand)
			}
		}

		return nil
	},
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

func appendToFile(destPath, line string, cfg *config.CaptureConfig, vaultCfg *config.VaultConfig) error {
	// Check if file exists
	fileExists := true
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		fileExists = false
	}

	// If file doesn't exist, create it appropriately
	if !fileExists {
		// Determine if this is a daily note (destination is "daily" and no --to override)
		isDailyNote := cfg.Destination == "daily" && addToFlag == ""
		if isDailyNote {
			if err := createDailyNote(destPath, vaultCfg); err != nil {
				return err
			}
		} else {
			// Create simple file for non-daily destinations
			if err := os.WriteFile(destPath, []byte(""), 0644); err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
		}
		fileExists = true
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

func createDailyNote(destPath string, vaultCfg *config.VaultConfig) error {
	// Extract date from path
	base := filepath.Base(destPath)
	date := strings.TrimSuffix(base, ".md")

	// Parse the date to get a friendly title
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		// Fall back to just the date string
		t = time.Now()
		date = vault.FormatDateISO(t)
	}

	title := vault.FormatDateFriendly(t)

	content := fmt.Sprintf(`---
type: date
---

# %s

`, title)

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return os.WriteFile(destPath, []byte(content), 0644)
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
func validateRefs(vaultPath string, refs []parser.Reference) []Warning {
	var warnings []Warning

	// Load schema to infer types from default_path
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return warnings
	}

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

	// Build resolver with existing objects
	res := resolver.New(objectIDs)

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

func init() {
	addCmd.Flags().StringVar(&addToFlag, "to", "", "Override destination file (relative to vault)")
	rootCmd.AddCommand(addCmd)
}
