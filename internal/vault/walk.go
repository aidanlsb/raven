package vault

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ravenscroftj/raven/internal/parser"
)

// WalkResult contains the result of processing a markdown file.
type WalkResult struct {
	Path         string
	RelativePath string
	Document     *parser.ParsedDocument
	Error        error
}

// WalkMarkdownFiles walks all markdown files in a vault and calls the handler for each.
// It automatically:
// - Skips the .raven directory
// - Only processes .md files
// - Verifies files are within the vault (security check)
// - Parses each document
func WalkMarkdownFiles(vaultPath string, handler func(result WalkResult) error) error {
	canonicalVault, err := filepath.Abs(vaultPath)
	if err != nil {
		canonicalVault = vaultPath
	}

	return filepath.WalkDir(vaultPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip directories, but skip .raven entirely
		if d.IsDir() {
			if d.Name() == ".raven" {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process .md files
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		// Security: verify file is within vault
		canonicalFile, err := filepath.Abs(path)
		if err != nil {
			return nil
		}
		if !strings.HasPrefix(canonicalFile, canonicalVault) {
			return nil
		}

		relativePath, _ := filepath.Rel(vaultPath, path)

		// Read file
		content, err := os.ReadFile(path)
		if err != nil {
			return handler(WalkResult{
				Path:         path,
				RelativePath: relativePath,
				Error:        err,
			})
		}

		// Parse document
		doc, err := parser.ParseDocument(string(content), path, vaultPath)
		if err != nil {
			return handler(WalkResult{
				Path:         path,
				RelativePath: relativePath,
				Error:        err,
			})
		}

		return handler(WalkResult{
			Path:         path,
			RelativePath: relativePath,
			Document:     doc,
		})
	})
}

// CollectDocuments walks all markdown files and returns parsed documents.
// Returns the documents and any files that had errors.
func CollectDocuments(vaultPath string) ([]*parser.ParsedDocument, []WalkResult, error) {
	var docs []*parser.ParsedDocument
	var errors []WalkResult

	err := WalkMarkdownFiles(vaultPath, func(result WalkResult) error {
		if result.Error != nil {
			errors = append(errors, result)
		} else {
			docs = append(docs, result.Document)
		}
		return nil
	})

	return docs, errors, err
}
