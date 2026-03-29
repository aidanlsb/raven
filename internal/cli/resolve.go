package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
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
// =============================================================================
// RESOLVE COMMAND
// =============================================================================

var resolveCmd = newCanonicalLeafCommand("resolve", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderResolve,
})

func renderResolve(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	reference := stringValue(data["reference"])
	if ambiguous, _ := data["ambiguous"].(bool); ambiguous {
		fmt.Printf("Reference '%s' is ambiguous. Matches:\n", reference)
		if matches, ok := data["matches"].([]map[string]interface{}); ok {
			for _, match := range matches {
				src := ""
				if s, ok := match["match_source"].(string); ok {
					src = fmt.Sprintf(" (%s)", s)
				}
				fmt.Printf("  %s%s\n", match["object_id"], src)
			}
		}
		return nil
	}

	if resolved, _ := data["resolved"].(bool); !resolved {
		fmt.Printf("Reference '%s' not found.\n", reference)
		return nil
	}

	objectID, _ := data["object_id"].(string)
	objectType, _ := data["type"].(string)
	relFilePath, _ := data["file_path"].(string)
	isSection, _ := data["is_section"].(bool)
	fileObjectID, _ := data["file_object_id"].(string)
	matchSource, _ := data["match_source"].(string)

	fmt.Printf("Resolved: %s\n", objectID)
	if objectType != "" {
		fmt.Printf("  Type: %s\n", objectType)
	}
	fmt.Printf("  File: %s\n", relFilePath)
	if isSection {
		fmt.Printf("  Parent: %s\n", fileObjectID)
	}
	if matchSource != "" {
		fmt.Printf("  Matched via: %s\n", matchSource)
	}
	return nil
}

func init() {
	resolveCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: true,
		DisableWhenStdin:    false,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	})
	rootCmd.AddCommand(resolveCmd)
}
