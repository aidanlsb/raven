package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

// ResolveResult contains the resolved reference information.
type ResolveResult struct {
	// ObjectID is the canonical object ID (e.g., "people/freya", "daily/2025-02-01#standup")
	ObjectID string

	// FilePath is the absolute path to the file
	FilePath string

	// IsSection is true if this is an embedded section reference (contains #)
	IsSection bool

	// FileObjectID is the parent file's object ID (for sections, this differs from ObjectID)
	FileObjectID string
}

// ResolveOptions configures reference resolution behavior.
type ResolveOptions struct {
	// VaultPath is the root path of the vault (required)
	VaultPath string

	// VaultConfig is the vault configuration (optional, will be loaded if nil)
	VaultConfig *config.VaultConfig

	// AllowMissing if true, returns a result for date references even if the file doesn't exist
	// This is useful for commands that may create the file (like daily notes)
	AllowMissing bool
}

// ResolveReference resolves any reference to its target object and file.
//
// Supports:
//   - Literal paths: "people/freya", "people/freya.md"
//   - Short names: "freya" → "people/freya"
//   - Aliases: "The Queen" → "people/freya"
//   - Name field values: "The Prose Edda" → "books/the-prose-edda"
//   - Date references: "2025-02-01" → "daily/2025-02-01"
//   - Section references: "projects/website#tasks"
//
// Returns an error if the reference is not found or is ambiguous.
func ResolveReference(reference string, opts ResolveOptions) (*ResolveResult, error) {
	if opts.VaultPath == "" {
		return nil, fmt.Errorf("vault path is required")
	}

	// Load vault config if not provided
	vaultCfg := opts.VaultConfig
	if vaultCfg == nil {
		var err error
		vaultCfg, err = config.LoadVaultConfig(opts.VaultPath)
		if err != nil {
			vaultCfg = &config.VaultConfig{}
		}
	}

	// Fast path: try literal path first (most common case for explicit paths)
	if result := tryLiteralPath(reference, opts.VaultPath, vaultCfg); result != nil {
		return result, nil
	}

	// Use the database resolver for full resolution
	db, err := index.Open(opts.VaultPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w (run 'rvn reindex' to rebuild)", err)
	}
	defer db.Close()

	// Get resolver with schema support for name_field resolution
	dailyDir := "daily"
	if vaultCfg != nil && vaultCfg.DailyDirectory != "" {
		dailyDir = vaultCfg.DailyDirectory
	}

	sch, _ := schema.Load(opts.VaultPath)
	res, err := db.Resolver(index.ResolverOptions{
		DailyDirectory: dailyDir,
		Schema:         sch, // nil is fine, aliases still work
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: %w", err)
	}

	// Resolve the reference
	resolved := res.Resolve(reference)

	if resolved.Ambiguous {
		return nil, &AmbiguousRefError{
			Reference: reference,
			Matches:   resolved.Matches,
		}
	}

	if resolved.TargetID == "" {
		return nil, &RefNotFoundError{Reference: reference}
	}

	// Build the result
	result := &ResolveResult{
		ObjectID: resolved.TargetID,
	}

	// Handle section references
	if idx := strings.Index(resolved.TargetID, "#"); idx >= 0 {
		result.IsSection = true
		result.FileObjectID = resolved.TargetID[:idx]
	} else {
		result.FileObjectID = resolved.TargetID
	}

	// Resolve the file path
	filePath, err := vault.ResolveObjectToFileWithConfig(opts.VaultPath, result.FileObjectID, vaultCfg)
	if err != nil {
		// For date references with AllowMissing, construct the expected path
		if opts.AllowMissing && strings.HasPrefix(result.FileObjectID, dailyDir+"/") {
			expectedPath := filepath.Join(opts.VaultPath, result.FileObjectID+".md")
			result.FilePath = expectedPath
			return result, nil
		}
		return nil, &RefNotFoundError{
			Reference: reference,
			Detail:    fmt.Sprintf("resolved to '%s' but file not found", resolved.TargetID),
		}
	}

	result.FilePath = filePath
	return result, nil
}

// tryLiteralPath attempts to resolve a reference as a literal file path.
// Returns nil if the path doesn't exist.
func tryLiteralPath(reference string, vaultPath string, vaultCfg *config.VaultConfig) *ResolveResult {
	// Try the reference as-is
	candidates := []string{reference}

	// Try with .md extension if not present
	if !strings.HasSuffix(reference, ".md") {
		candidates = append(candidates, reference+".md")
	}

	for _, candidate := range candidates {
		fullPath := filepath.Join(vaultPath, candidate)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			// Found it - build the result
			objectID := strings.TrimSuffix(candidate, ".md")
			if vaultCfg != nil {
				objectID = vaultCfg.FilePathToObjectID(objectID)
			}

			return &ResolveResult{
				ObjectID:     objectID,
				FilePath:     fullPath,
				IsSection:    false,
				FileObjectID: objectID,
			}
		}
	}

	return nil
}

// ResolveReferenceToFile is a convenience function that resolves a reference
// and returns just the file path. This is the most common use case.
func ResolveReferenceToFile(reference string, opts ResolveOptions) (string, error) {
	result, err := ResolveReference(reference, opts)
	if err != nil {
		return "", err
	}
	return result.FilePath, nil
}

// ResolveReferenceToObjectID is a convenience function that resolves a reference
// and returns the canonical object ID.
func ResolveReferenceToObjectID(reference string, opts ResolveOptions) (string, error) {
	result, err := ResolveReference(reference, opts)
	if err != nil {
		return "", err
	}
	return result.ObjectID, nil
}

// AmbiguousRefError is returned when a reference matches multiple objects.
type AmbiguousRefError struct {
	Reference string
	Matches   []string
}

func (e *AmbiguousRefError) Error() string {
	return fmt.Sprintf("reference '%s' is ambiguous, matches: %v", e.Reference, e.Matches)
}

// RefNotFoundError is returned when a reference cannot be resolved.
type RefNotFoundError struct {
	Reference string
	Detail    string
}

func (e *RefNotFoundError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("reference '%s' not found: %s", e.Reference, e.Detail)
	}
	return fmt.Sprintf("reference '%s' not found", e.Reference)
}

// IsAmbiguousRef returns true if the error is an ambiguous reference error.
func IsAmbiguousRef(err error) bool {
	_, ok := err.(*AmbiguousRefError)
	return ok
}

// IsRefNotFound returns true if the error is a reference not found error.
func IsRefNotFound(err error) bool {
	_, ok := err.(*RefNotFoundError)
	return ok
}

// handleResolveError converts a resolve error to an appropriate CLI error output.
// Returns the error code used.
func handleResolveError(err error, reference string) error {
	switch e := err.(type) {
	case *AmbiguousRefError:
		return handleErrorMsg(ErrRefAmbiguous,
			e.Error(),
			"Use a more specific path to disambiguate")
	case *RefNotFoundError:
		return handleErrorMsg(ErrRefNotFound,
			e.Error(),
			"Check the reference and try again")
	default:
		return handleErrorMsg(ErrInternal,
			fmt.Sprintf("failed to resolve '%s': %v", reference, err),
			"Run 'rvn reindex' if the database is out of date")
	}
}
