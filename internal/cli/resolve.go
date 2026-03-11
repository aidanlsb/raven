package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/readsvc"
)

// ResolveResult contains the resolved reference information.
type ResolveResult = readsvc.ResolveResult

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

func ResolveReference(reference string, opts ResolveOptions) (*ResolveResult, error) {
	if opts.VaultPath == "" {
		return nil, fmt.Errorf("vault path is required")
	}

	rt, err := readsvc.NewRuntime(opts.VaultPath, readsvc.RuntimeOptions{OpenDB: false})
	if err != nil {
		return nil, err
	}
	if opts.VaultConfig != nil {
		rt.VaultCfg = opts.VaultConfig
	}

	return readsvc.ResolveReference(reference, rt, opts.AllowMissing)
}

// ResolveReferenceToFile is a convenience function that resolves a reference
// and returns just the file path. This is the most common use case.
func ResolveReferenceToFile(reference string, opts ResolveOptions) (string, error) {
	rt, err := readsvc.NewRuntime(opts.VaultPath, readsvc.RuntimeOptions{OpenDB: false})
	if err != nil {
		return "", err
	}
	if opts.VaultConfig != nil {
		rt.VaultCfg = opts.VaultConfig
	}
	return readsvc.ResolveReferenceToFile(reference, rt, opts.AllowMissing)
}

// ResolveReferenceToObjectID is a convenience function that resolves a reference
// and returns the canonical object ID.
func ResolveReferenceToObjectID(reference string, opts ResolveOptions) (string, error) {
	rt, err := readsvc.NewRuntime(opts.VaultPath, readsvc.RuntimeOptions{OpenDB: false})
	if err != nil {
		return "", err
	}
	if opts.VaultConfig != nil {
		rt.VaultCfg = opts.VaultConfig
	}
	return readsvc.ResolveReferenceToObjectID(reference, rt, opts.AllowMissing)
}

// AmbiguousRefError is returned when a reference matches multiple objects.
type AmbiguousRefError = readsvc.AmbiguousRefError

// RefNotFoundError is returned when a reference cannot be resolved.
type RefNotFoundError = readsvc.RefNotFoundError

// IsAmbiguousRef returns true if the error is an ambiguous reference error.
func IsAmbiguousRef(err error) bool {
	return readsvc.IsAmbiguousRef(err)
}

// IsRefNotFound returns true if the error is a reference not found error.
func IsRefNotFound(err error) bool {
	return readsvc.IsRefNotFound(err)
}

func resolveReferenceWithDynamicDates(reference string, opts ResolveOptions, allowDynamicMissing bool) (*ResolveResult, error) {
	rt, err := readsvc.NewRuntime(opts.VaultPath, readsvc.RuntimeOptions{OpenDB: false})
	if err != nil {
		return nil, err
	}
	if opts.VaultConfig != nil {
		rt.VaultCfg = opts.VaultConfig
	}
	return readsvc.ResolveReferenceWithDynamicDates(reference, rt, allowDynamicMissing)
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
