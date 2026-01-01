package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ravenscroftj/raven/internal/check"
	"github.com/ravenscroftj/raven/internal/parser"
	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/spf13/cobra"
)

var (
	checkStrict bool
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate the vault",
	Long:  `Checks all files for errors and warnings (type mismatches, broken references, etc.)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		fmt.Printf("Checking vault: %s\n", vaultPath)

		// Load schema
		s, err := schema.Load(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to load schema: %w", err)
		}

		var errorCount, warningCount, fileCount int
		var allDocs []*parser.ParsedDocument
		var allObjectIDs []string

		// Get canonical vault path
		canonicalVault, err := filepath.Abs(vaultPath)
		if err != nil {
			canonicalVault = vaultPath
		}

		// First pass: parse all documents and collect object IDs
		err = filepath.WalkDir(vaultPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			if d.IsDir() {
				if d.Name() == ".raven" {
					return filepath.SkipDir
				}
				return nil
			}

			if !strings.HasSuffix(path, ".md") {
				return nil
			}

			// Security check
			canonicalFile, err := filepath.Abs(path)
			if err != nil {
				return nil
			}
			if !strings.HasPrefix(canonicalFile, canonicalVault) {
				return nil
			}

			fileCount++

			content, err := os.ReadFile(path)
			if err != nil {
				relativePath, _ := filepath.Rel(vaultPath, path)
				fmt.Printf("ERROR: %s - Failed to read: %v\n", relativePath, err)
				errorCount++
				return nil
			}

			doc, err := parser.ParseDocument(string(content), path, vaultPath)
			if err != nil {
				relativePath, _ := filepath.Rel(vaultPath, path)
				fmt.Printf("ERROR: %s - Parse error: %v\n", relativePath, err)
				errorCount++
				return nil
			}

			allDocs = append(allDocs, doc)
			for _, obj := range doc.Objects {
				allObjectIDs = append(allObjectIDs, obj.ID)
			}

			return nil
		})

		if err != nil {
			return fmt.Errorf("error walking vault: %w", err)
		}

		// Second pass: validate with full context
		validator := check.NewValidator(s, allObjectIDs)

		for _, doc := range allDocs {
			issues := validator.ValidateDocument(doc)

			for _, issue := range issues {
				prefix := "ERROR"
				if issue.Level == check.LevelWarning {
					prefix = "WARN"
					warningCount++
				} else {
					errorCount++
				}

				fmt.Printf("%s:  %s:%d - %s\n", prefix, issue.FilePath, issue.Line, issue.Message)
			}
		}

		fmt.Println()
		if errorCount == 0 && warningCount == 0 {
			fmt.Printf("✓ No issues found in %d files.\n", fileCount)
		} else {
			fmt.Printf("Found %d error(s), %d warning(s) in %d files.\n", errorCount, warningCount, fileCount)
		}

		if errorCount > 0 || (checkStrict && warningCount > 0) {
			os.Exit(1)
		}

		return nil
	},
}

func init() {
	checkCmd.Flags().BoolVar(&checkStrict, "strict", false, "Treat warnings as errors")
	rootCmd.AddCommand(checkCmd)
}
