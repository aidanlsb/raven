package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ravenscroftj/raven/internal/commands"
	"github.com/ravenscroftj/raven/internal/index"
	"github.com/ravenscroftj/raven/internal/parser"
	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/spf13/cobra"
)

var editConfirm bool

var editCmd = &cobra.Command{
	Use:   "edit <path> <old_str> <new_str>",
	Short: commands.Registry["edit"].Description,
	Long:  commands.Registry["edit"].LongDesc,
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		relPath := args[0]
		oldStr := args[1]
		newStr := args[2]

		// Resolve file path
		filePath := filepath.Join(vaultPath, relPath)

		// Check file exists
		content, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				return handleErrorWithDetails("FILE_NOT_FOUND",
					fmt.Sprintf("file not found: %s", relPath),
					"Check the path and try again",
					map[string]string{"path": relPath})
			}
			return handleError("READ_ERROR", err, "")
		}

		contentStr := string(content)

		// Count occurrences
		count := strings.Count(contentStr, oldStr)

		if count == 0 {
			return handleErrorWithDetails("STRING_NOT_FOUND",
				"old_str not found in file",
				"Check the exact string including whitespace",
				map[string]string{"path": relPath, "old_str": oldStr})
		}

		if count > 1 {
			return handleErrorWithDetails("MULTIPLE_MATCHES",
				fmt.Sprintf("old_str found %d times in file", count),
				"Include more surrounding context to make the match unique",
				map[string]string{"path": relPath, "count": fmt.Sprintf("%d", count)})
		}

		// Find the line number where the match occurs
		matchIndex := strings.Index(contentStr, oldStr)
		lineNumber := strings.Count(contentStr[:matchIndex], "\n") + 1

		// Generate context (before and after views)
		newContent := strings.Replace(contentStr, oldStr, newStr, 1)

		if !editConfirm {
			// Preview mode
			beforeContext := extractContext(contentStr, matchIndex, len(oldStr))
			afterContext := extractContextAfterReplace(contentStr, oldStr, newStr, matchIndex)

			if jsonOutput {
				outputJSON(Response{
					OK: true,
					Data: map[string]interface{}{
						"status": "preview",
						"path":   relPath,
						"line":   lineNumber,
						"preview": map[string]string{
							"before": beforeContext,
							"after":  afterContext,
						},
					},
					Meta: &Meta{},
				})
				// Add suggestion as a separate print since Meta doesn't have suggestion
				return nil
			}

			fmt.Printf("Preview edit in %s (line %d):\n\n", relPath, lineNumber)
			fmt.Println("BEFORE:")
			fmt.Println(indent(beforeContext, "  "))
			fmt.Println()
			fmt.Println("AFTER:")
			fmt.Println(indent(afterContext, "  "))
			fmt.Println()
			fmt.Println("Run with --confirm to apply this edit")
			return nil
		}

		// Apply the edit
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			return handleError("WRITE_ERROR", err, "")
		}

		// Reindex the file
		sch, err := schema.Load(vaultPath)
		if err == nil {
			db, _, dbErr := index.OpenWithRebuild(vaultPath)
			if dbErr == nil {
				defer db.Close()
				doc, parseErr := parser.ParseDocument(newContent, filePath, vaultPath)
				if parseErr == nil {
					db.IndexDocument(doc, sch)
				}
			}
		}

		// Get context around the edit
		newMatchIndex := strings.Index(newContent, newStr)
		context := ""
		if newMatchIndex >= 0 {
			context = extractContext(newContent, newMatchIndex, len(newStr))
		}

		if jsonOutput {
			outputSuccess(map[string]interface{}{
				"status":  "applied",
				"path":    relPath,
				"line":    lineNumber,
				"old_str": oldStr,
				"new_str": newStr,
				"context": context,
			}, nil)
			return nil
		}

		fmt.Printf("✓ Applied edit in %s (line %d)\n", relPath, lineNumber)
		fmt.Println()
		fmt.Println("Context:")
		fmt.Println(indent(context, "  "))
		return nil
	},
}

// extractContext extracts ~3 lines of context around a match
func extractContext(content string, matchIndex int, matchLen int) string {
	lines := strings.Split(content, "\n")
	
	// Find line containing the match
	charCount := 0
	startLine := 0
	for i, line := range lines {
		if charCount+len(line)+1 > matchIndex {
			startLine = i
			break
		}
		charCount += len(line) + 1 // +1 for newline
	}

	// Get 1 line before and 2 lines after
	contextStart := startLine
	if contextStart > 0 {
		contextStart--
	}
	contextEnd := startLine + 3
	if contextEnd > len(lines) {
		contextEnd = len(lines)
	}

	return strings.Join(lines[contextStart:contextEnd], "\n")
}

// extractContextAfterReplace shows context after the replacement
func extractContextAfterReplace(content, oldStr, newStr string, matchIndex int) string {
	newContent := strings.Replace(content, oldStr, newStr, 1)
	// Find approximate position in new content
	newMatchIndex := matchIndex
	if newMatchIndex > len(newContent) {
		newMatchIndex = len(newContent) - 1
	}
	if newMatchIndex < 0 {
		newMatchIndex = 0
	}
	return extractContext(newContent, newMatchIndex, len(newStr))
}

// indent adds a prefix to each line
func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func init() {
	editCmd.Flags().BoolVar(&editConfirm, "confirm", false, "Apply the edit (default: preview only)")
	rootCmd.AddCommand(editCmd)
}
