package vault

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
)

// WalkResult contains the result of processing a markdown file.
type WalkResult struct {
	Path         string
	RelativePath string
	Document     *parser.ParsedDocument
	FileMtime    int64 // File modification time as Unix timestamp
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

		// Skip directories, but skip .raven and .trash entirely
		if d.IsDir() {
			name := d.Name()
			if name == ".raven" || name == ".trash" {
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

		// Get file mtime
		info, err := d.Info()
		if err != nil {
			return handler(WalkResult{
				Path:         path,
				RelativePath: relativePath,
				Error:        err,
			})
		}
		fileMtime := info.ModTime().Unix()

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
			FileMtime:    fileMtime,
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

// ResolveObjectToFile resolves an object ID to an absolute file path.
// Supports exact matches and slugified matching (e.g., "people/Sif" -> "people/sif.md").
func ResolveObjectToFile(vaultPath, objectID string) (string, error) {
	// Normalize the object ID
	objectID = strings.TrimSuffix(objectID, ".md")

	// Try direct path first
	filePath := filepath.Join(vaultPath, objectID+".md")
	if _, err := os.Stat(filePath); err == nil {
		return filePath, nil
	}

	// Try with different casing/slugification by walking the vault
	var foundPath string
	err := filepath.WalkDir(vaultPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if d.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		// Get relative path and compare
		relPath, _ := filepath.Rel(vaultPath, path)
		relID := strings.TrimSuffix(relPath, ".md")

		// Exact match
		if relID == objectID {
			foundPath = path
			return filepath.SkipAll
		}

		// Slugified match
		if pages.SlugifyPath(relID) == pages.SlugifyPath(objectID) {
			foundPath = path
			return filepath.SkipAll
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return "", err
	}

	if foundPath != "" {
		return foundPath, nil
	}

	return "", fmt.Errorf("object not found: %s", objectID)
}
