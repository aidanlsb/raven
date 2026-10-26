package vault

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/aidanlsb/raven/internal/config"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/slugs"
)

// WalkResult contains the result of processing a markdown file.
type WalkResult struct {
	Path         string
	RelativePath string
	Document     *parser.ParsedDocument
	FileMtime    int64 // File modification time as Unix timestamp
	ParseSkipped bool  // True when ShouldParse skipped reading and parsing the file
	Error        error
}

// WalkOptions contains options for walking markdown files.
type WalkOptions struct {
	// ParseOptions are passed to the parser for each file.
	ParseOptions *parser.ParseOptions
	// ExcludeMatcher skips paths that are not managed by Raven.
	ExcludeMatcher *ravenignore.Matcher
	// ShouldParse is called after a file is validated and statted, but before it
	// is read. Returning false skips parsing while still calling the handler
	// with ParseSkipped set. When nil, all discovered markdown files are parsed.
	ShouldParse func(relativePath string, fileMtime int64) bool
	// Context cancels discovery and parsing when cancelled.
	Context context.Context
	// ParseWorkers is the number of parallel parse workers. 0 means GOMAXPROCS.
	// 1 forces sequential parse (useful for tests).
	ParseWorkers int
}

// WalkMarkdownFiles walks all markdown files in a vault and calls the handler for each.
// It automatically:
// - Skips the .raven directory
// - Only processes .md files
// - Verifies files are within the vault (security check)
// - Parses each document
func WalkMarkdownFiles(vaultPath string, handler func(result WalkResult) error) error {
	return WalkMarkdownFilesWithOptions(vaultPath, nil, handler)
}

type discoveredFile struct {
	path         string
	relativePath string
	fileMtime    int64
	parseSkipped bool
	err          error
}

// WalkMarkdownFilesWithOptions walks all markdown files with custom options.
func WalkMarkdownFilesWithOptions(vaultPath string, opts *WalkOptions, handler func(result WalkResult) error) error {
	var parseOpts *parser.ParseOptions
	ctx := context.Background()
	if opts != nil {
		parseOpts = opts.ParseOptions
		if opts.Context != nil {
			ctx = opts.Context
		}
	}

	files, err := collectMarkdownFiles(ctx, vaultPath, opts)
	if err != nil {
		return err
	}

	results := parseDiscoveredFiles(ctx, vaultPath, parseOpts, files, parseWorkerCount(opts))
	for _, result := range results {
		if err := handler(result); err != nil {
			return err
		}
	}
	return nil
}

func parseWorkerCount(opts *WalkOptions) int {
	if opts != nil && opts.ParseWorkers > 0 {
		return opts.ParseWorkers
	}
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		return 1
	}
	return n
}

func collectMarkdownFiles(ctx context.Context, vaultPath string, opts *WalkOptions) ([]discoveredFile, error) {
	var files []discoveredFile
	err := filepath.WalkDir(vaultPath, func(path string, d fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			relativePath, _ := filepath.Rel(vaultPath, path)
			files = append(files, discoveredFile{
				path:         path,
				relativePath: relativePath,
				err:          err,
			})
			return nil //nolint:nilerr // keep walking to collect all per-file errors
		}

		relativePath, _ := filepath.Rel(vaultPath, path)
		relativePath = filepath.ToSlash(relativePath)

		if d.IsDir() {
			name := d.Name()
			if name == ".raven" || name == ".trash" || name == ".git" {
				return filepath.SkipDir
			}
			if relativePath != "." && opts != nil && opts.ExcludeMatcher.Match(relativePath, true) {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		if err := paths.ValidateWithinVault(vaultPath, path); err != nil {
			if errors.Is(err, paths.ErrPathOutsideVault) {
				return nil
			}
			relativePath, _ := filepath.Rel(vaultPath, path)
			files = append(files, discoveredFile{
				path:         path,
				relativePath: relativePath,
				err:          err,
			})
			return nil
		}

		if opts != nil && opts.ExcludeMatcher.Match(relativePath, false) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			files = append(files, discoveredFile{
				path:         path,
				relativePath: relativePath,
				err:          err,
			})
			return nil //nolint:nilerr // keep walking to collect all per-file errors
		}
		fileMtime := info.ModTime().Unix()

		if opts != nil && opts.ShouldParse != nil && !opts.ShouldParse(relativePath, fileMtime) {
			files = append(files, discoveredFile{
				path:         path,
				relativePath: relativePath,
				fileMtime:    fileMtime,
				parseSkipped: true,
			})
			return nil
		}

		files = append(files, discoveredFile{
			path:         path,
			relativePath: relativePath,
			fileMtime:    fileMtime,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func parseDiscoveredFiles(ctx context.Context, vaultPath string, parseOpts *parser.ParseOptions, files []discoveredFile, workers int) []WalkResult {
	results := make([]WalkResult, len(files))
	if len(files) == 0 {
		return results
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(files) {
		workers = len(files)
	}

	var wg sync.WaitGroup
	jobs := make(chan int)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = parseDiscoveredFile(ctx, vaultPath, parseOpts, files[i])
			}
		}()
	}
	for i := range files {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}

func parseDiscoveredFile(ctx context.Context, vaultPath string, parseOpts *parser.ParseOptions, file discoveredFile) WalkResult {
	result := WalkResult{
		Path:         file.path,
		RelativePath: file.relativePath,
		FileMtime:    file.fileMtime,
		ParseSkipped: file.parseSkipped,
		Error:        file.err,
	}
	if file.err != nil || file.parseSkipped {
		return result
	}
	if err := ctx.Err(); err != nil {
		result.Error = err
		return result
	}

	content, err := os.ReadFile(file.path)
	if err != nil {
		result.Error = err
		return result
	}

	doc, err := parser.ParseDocumentWithOptions(string(content), file.path, vaultPath, parseOpts)
	if err != nil {
		result.Error = err
		return result
	}
	result.Document = doc
	return result
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

// ResolveObjectToFileWithConfig resolves a reference/object ID to an absolute file path,
// using vault directory roots when configured.
func ResolveObjectToFileWithConfig(vaultPath, ref string, vaultCfg *config.VaultConfig) (string, error) {
	objectsRoot := ""
	pagesRoot := ""
	dailyRoot := ""
	if vaultCfg != nil {
		if vaultCfg.HasDirectoriesConfig() {
			if dirs := vaultCfg.GetDirectoriesConfig(); dirs != nil {
				objectsRoot = dirs.Object
				pagesRoot = dirs.Page
			}
		}
		dailyRoot = vaultCfg.GetDailyDirectory()
	}
	return ResolveObjectToFileWithRoots(vaultPath, ref, objectsRoot, pagesRoot, dailyRoot)
}

// ResolveObjectToFileWithRoots resolves a reference/object ID to an absolute file path,
// using the provided objects/pages/daily roots for both direct candidate paths and fuzzy matching.
func ResolveObjectToFileWithRoots(vaultPath, ref, objectsRoot, pagesRoot, dailyRoot string) (string, error) {
	// Try direct candidates first (literal + rooted).
	for _, rel := range paths.CandidateFilePaths(ref, objectsRoot, pagesRoot, dailyRoot) {
		filePath := filepath.Join(vaultPath, rel)
		if _, err := os.Stat(filePath); err == nil {
			return filePath, nil
		}
	}

	wantID := paths.FilePathToObjectID(ref, objectsRoot, pagesRoot, dailyRoot)

	// Fall back to walking the vault and using exact/slugified matching.
	var foundPath string
	err := filepath.WalkDir(vaultPath, func(path string, d fs.DirEntry, err error) error {
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

		relPath, _ := filepath.Rel(vaultPath, path)
		relID := paths.FilePathToObjectID(relPath, objectsRoot, pagesRoot, dailyRoot)

		// Exact match
		if relID == wantID {
			foundPath = path
			return filepath.SkipAll
		}

		// Slugified match
		if slugs.PathSlug(relID) == slugs.PathSlug(wantID) {
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
