package keep

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
)

// WalkResult contains the result of processing a markdown file.
type WalkResult struct {
	Path         string
	RelativePath string
	Document     *parser.ParsedDocument
	FileMtime    int64 // File modification time as Unix timestamp
	Error        error
}

// WalkOptions contains options for walking markdown files.
type WalkOptions struct {
	// ParseOptions are passed to the parser for each file.
	ParseOptions *parser.ParseOptions
}

// WalkMarkdownFiles walks all markdown files in a keep and calls the handler for each.
// It automatically:
// - Skips the .raven directory
// - Only processes .md files
// - Verifies files are within the keep (security check)
// - Parses each document
func WalkMarkdownFiles(keepPath string, handler func(result WalkResult) error) error {
	return WalkMarkdownFilesWithOptions(keepPath, nil, handler)
}

// WalkMarkdownFilesWithOptions walks all markdown files with custom options.
func WalkMarkdownFilesWithOptions(keepPath string, opts *WalkOptions, handler func(result WalkResult) error) error {
	var parseOpts *parser.ParseOptions
	if opts != nil {
		parseOpts = opts.ParseOptions
	}

	return filepath.WalkDir(keepPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			relativePath, _ := filepath.Rel(keepPath, path)
			return handler(WalkResult{
				Path:         path,
				RelativePath: relativePath,
				Error:        err,
			})
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

		// Security: verify file is within keep
		if err := paths.ValidateWithinKeep(keepPath, path); err != nil {
			if errors.Is(err, paths.ErrPathOutsideKeep) {
				return nil
			}
			relativePath, _ := filepath.Rel(keepPath, path)
			return handler(WalkResult{
				Path:         path,
				RelativePath: relativePath,
				Error:        err,
			})
		}

		relativePath, _ := filepath.Rel(keepPath, path)

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

		// Parse document with options
		doc, err := parser.ParseDocumentWithOptions(string(content), path, keepPath, parseOpts)
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
func CollectDocuments(keepPath string) ([]*parser.ParsedDocument, []WalkResult, error) {
	var docs []*parser.ParsedDocument
	var errors []WalkResult

	err := WalkMarkdownFiles(keepPath, func(result WalkResult) error {
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
func ResolveObjectToFile(keepPath, objectID string) (string, error) {
	return ResolveObjectToFileWithRoots(keepPath, objectID, "", "")
}

// ResolveObjectToFileWithConfig resolves a reference/object ID to an absolute file path,
// using keep directory roots when configured.
func ResolveObjectToFileWithConfig(keepPath, ref string, keepCfg *config.KeepConfig) (string, error) {
	objectsRoot := ""
	pagesRoot := ""
	if keepCfg != nil && keepCfg.HasDirectoriesConfig() {
		if dirs := keepCfg.GetDirectoriesConfig(); dirs != nil {
			objectsRoot = dirs.Object
			pagesRoot = dirs.Page
		}
	}
	return ResolveObjectToFileWithRoots(keepPath, ref, objectsRoot, pagesRoot)
}

// ResolveObjectToFileWithRoots resolves a reference/object ID to an absolute file path,
// using the provided objects/pages roots for both direct candidate paths and fuzzy matching.
func ResolveObjectToFileWithRoots(keepPath, ref, objectsRoot, pagesRoot string) (string, error) {
	// Try direct candidates first (literal + rooted).
	for _, rel := range paths.CandidateFilePaths(ref, objectsRoot, pagesRoot) {
		filePath := filepath.Join(keepPath, rel)
		if _, err := os.Stat(filePath); err == nil {
			return filePath, nil
		}
	}

	wantID := paths.FilePathToObjectID(ref, objectsRoot, pagesRoot)

	// Fall back to walking the keep and using exact/slugified matching.
	var foundPath string
	err := filepath.WalkDir(keepPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Best-effort: we still want to resolve if some files/dirs are unreadable.
			return nil //nolint:nilerr
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

		relPath, _ := filepath.Rel(keepPath, path)
		relID := paths.FilePathToObjectID(relPath, objectsRoot, pagesRoot)

		// Exact match
		if relID == wantID {
			foundPath = path
			return filepath.SkipAll
		}

		// Slugified match
		if pages.SlugifyPath(relID) == pages.SlugifyPath(wantID) {
			foundPath = path
			return filepath.SkipAll
		}

		return nil
	})
	if err != nil && !errors.Is(err, filepath.SkipAll) {
		return "", err
	}
	if foundPath != "" {
		return foundPath, nil
	}
	return "", fmt.Errorf("object not found: %s", strings.TrimSuffix(ref, ".md"))
}
