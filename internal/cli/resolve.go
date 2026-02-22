package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

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

	// MatchSource describes how the reference was matched (e.g., "literal_path", "short_name", "alias", "name_field", "object_id", "date", "suffix_match")
	MatchSource string
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
		vaultCfg, err = loadVaultConfigSafe(opts.VaultPath)
		if err != nil {
			return nil, err
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
	if vaultCfg != nil && vaultCfg.GetDailyDirectory() != "" {
		dailyDir = vaultCfg.GetDailyDirectory()
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
			Reference:    reference,
			Matches:      resolved.Matches,
			MatchSources: resolved.MatchSources,
		}
	}

	if resolved.TargetID == "" {
		return nil, &RefNotFoundError{Reference: reference}
	}

	// Build the result
	matchSource := ""
	if resolved.MatchSources != nil {
		matchSource = resolved.MatchSources[resolved.TargetID]
	}
	result := &ResolveResult{
		ObjectID:    resolved.TargetID,
		MatchSource: matchSource,
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
				MatchSource:  "literal_path",
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
	Reference    string
	Matches      []string
	MatchSources map[string]string
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
	var e *AmbiguousRefError
	return errors.As(err, &e)
}

// IsRefNotFound returns true if the error is a reference not found error.
func IsRefNotFound(err error) bool {
	var e *RefNotFoundError
	return errors.As(err, &e)
}

func resolveReferenceWithDynamicDates(reference string, opts ResolveOptions, allowDynamicMissing bool) (*ResolveResult, error) {
	result, err := ResolveReference(reference, opts)
	if err == nil {
		return result, nil
	}
	if !IsRefNotFound(err) {
		return nil, err
	}

	vaultCfg := opts.VaultConfig
	if vaultCfg == nil {
		var loadErr error
		vaultCfg, loadErr = loadVaultConfigSafe(opts.VaultPath)
		if loadErr != nil {
			return nil, loadErr
		}
	}

	dynResult, handled, dynErr := resolveDynamicDateReference(reference, opts.VaultPath, vaultCfg, allowDynamicMissing)
	if !handled {
		return nil, err
	}
	if dynErr != nil {
		return nil, dynErr
	}
	return dynResult, nil
}

func resolveDynamicDateReference(reference, vaultPath string, vaultCfg *config.VaultConfig, allowMissing bool) (*ResolveResult, bool, error) {
	ref := strings.TrimSpace(reference)
	if ref == "" {
		return nil, false, nil
	}

	baseRef := ref
	fragment := ""
	if parts := strings.SplitN(ref, "#", 2); len(parts) == 2 {
		baseRef = parts[0]
		fragment = parts[1]
	}
	if baseRef == "" {
		return nil, false, nil
	}

	keyword := strings.ToLower(strings.TrimSpace(baseRef))
	switch keyword {
	case "today", "tomorrow", "yesterday":
	default:
		return nil, false, nil
	}

	if vaultCfg == nil {
		var loadErr error
		vaultCfg, loadErr = loadVaultConfigSafe(vaultPath)
		if loadErr != nil {
			return nil, true, loadErr
		}
	}

	parsed, err := vault.ParseDateArg(keyword)
	if err != nil {
		return nil, true, err
	}
	dateStr := vault.FormatDateISO(parsed)
	fileObjectID := vaultCfg.DailyNoteID(dateStr)
	objectID := fileObjectID
	if fragment != "" {
		objectID = fileObjectID + "#" + fragment
	}
	filePath := vaultCfg.DailyNotePath(vaultPath, dateStr)

	if !allowMissing {
		if _, err := os.Stat(filePath); err != nil {
			if os.IsNotExist(err) {
				return nil, true, &RefNotFoundError{
					Reference: reference,
					Detail:    fmt.Sprintf("resolved to '%s' but file not found", objectID),
				}
			}
			return nil, true, err
		}
	}

	return &ResolveResult{
		ObjectID:     objectID,
		FilePath:     filePath,
		IsSection:    fragment != "",
		FileObjectID: fileObjectID,
		MatchSource:  "date",
	}, true, nil
}

// handleResolveError converts a resolve error to an appropriate CLI error output.
// Returns the error code used.
func handleResolveError(err error, reference string) error {
	var ae *AmbiguousRefError
	if errors.As(err, &ae) {
		return handleErrorMsg(ErrRefAmbiguous,
			ae.Error(),
			"Use a more specific path to disambiguate")
	}

	var nfe *RefNotFoundError
	if errors.As(err, &nfe) {
		return handleErrorMsg(ErrRefNotFound,
			nfe.Error(),
			"Check the reference and try again")
	}

	return handleErrorMsg(ErrInternal,
		fmt.Sprintf("failed to resolve '%s': %v", reference, err),
		"Run 'rvn reindex' if the database is out of date")
}

// =============================================================================
// RESOLVE COMMAND
// =============================================================================

var resolveCmd = &cobra.Command{
	Use:   "resolve <reference>",
	Short: "Resolve a reference to its target object",
	Long: `Resolve a reference (short name, alias, path, date, etc.) and return
information about the target object.

This is a pure query — it does not modify anything.

Examples:
  rvn resolve freya --json
  rvn resolve people/freya --json
  rvn resolve today --json
  rvn resolve "The Prose Edda" --json`,
	Args: cobra.ExactArgs(1),
	ValidArgsFunction: completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: true,
		DisableWhenStdin:    false,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	}),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		reference := args[0]

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}

		// Attempt resolution (including dynamic date keywords like "today")
		result, err := resolveReferenceWithDynamicDates(reference, ResolveOptions{
			VaultPath:   vaultPath,
			VaultConfig: vaultCfg,
		}, true) // allowDynamicMissing=true so "today" resolves even without a file

		elapsed := time.Since(start).Milliseconds()

		// Handle ambiguous reference
		var ae *AmbiguousRefError
		if errors.As(err, &ae) {
			matches := make([]map[string]interface{}, 0, len(ae.Matches))
			for _, m := range ae.Matches {
				entry := map[string]interface{}{
					"object_id": m,
				}
				if ae.MatchSources != nil {
					if src, ok := ae.MatchSources[m]; ok {
						entry["match_source"] = src
					}
				}
				matches = append(matches, entry)
			}

			if isJSONOutput() {
				outputSuccess(map[string]interface{}{
					"resolved":  false,
					"ambiguous": true,
					"reference": reference,
					"matches":   matches,
				}, &Meta{QueryTimeMs: elapsed})
				return nil
			}

			fmt.Printf("Reference '%s' is ambiguous. Matches:\n", reference)
			for _, m := range matches {
				src := ""
				if s, ok := m["match_source"].(string); ok {
					src = fmt.Sprintf(" (%s)", s)
				}
				fmt.Printf("  %s%s\n", m["object_id"], src)
			}
			return nil
		}

		// Handle not found
		if err != nil {
			if isJSONOutput() {
				outputSuccess(map[string]interface{}{
					"resolved":  false,
					"reference": reference,
				}, &Meta{QueryTimeMs: elapsed})
				return nil
			}

			fmt.Printf("Reference '%s' not found.\n", reference)
			return nil
		}

		// Successful resolution — enrich with type from the index
		objectType := ""
		relFilePath := ""

		if result.FilePath != "" {
			rel, relErr := filepath.Rel(vaultPath, result.FilePath)
			if relErr == nil {
				relFilePath = rel
			} else {
				relFilePath = result.FilePath
			}
		}

		// Look up the object type from the database
		db, dbErr := index.Open(vaultPath)
		if dbErr == nil {
			defer db.Close()
			obj, objErr := db.GetObject(result.ObjectID)
			if objErr == nil && obj != nil {
				objectType = obj.Type
			}
		}

		if isJSONOutput() {
			data := map[string]interface{}{
				"resolved":   true,
				"object_id":  result.ObjectID,
				"file_path":  relFilePath,
				"is_section": result.IsSection,
			}
			if objectType != "" {
				data["type"] = objectType
			}
			if result.MatchSource != "" {
				data["match_source"] = result.MatchSource
			}
			if result.IsSection {
				data["file_object_id"] = result.FileObjectID
			}
			outputSuccess(data, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		// Human-readable output
		fmt.Printf("Resolved: %s\n", result.ObjectID)
		if objectType != "" {
			fmt.Printf("  Type: %s\n", objectType)
		}
		fmt.Printf("  File: %s\n", relFilePath)
		if result.IsSection {
			fmt.Printf("  Parent: %s\n", result.FileObjectID)
		}
		if result.MatchSource != "" {
			fmt.Printf("  Matched via: %s\n", result.MatchSource)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(resolveCmd)
}
