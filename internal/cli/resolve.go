package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

// handleResolveError converts a resolve error to an appropriate CLI error output.
// Returns the error code used.
// =============================================================================
// RESOLVE COMMAND
// =============================================================================

var resolveCmd = newCanonicalLeafCommand("resolve", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Args:        cobra.MaximumNArgs(1),
	Prepare:     prepareResolveArgs,
	BuildArgs:   buildResolveArgs,
	RenderHuman: renderResolve,
})

func prepareResolveArgs(_ *cobra.Command, args []string) ([]string, bool, error) {
	return prepareInteractiveReferenceArgs(args, "resolve", "reference", "resolve> ", "Select a reference to resolve (Esc to cancel)")
}

func buildResolveArgs(_ *cobra.Command, args []string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"reference": args[0],
	}, nil
}

func renderResolve(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	reference := stringValue(data["reference"])
	if ambiguous, _ := data["ambiguous"].(bool); ambiguous {
		fmt.Println(ui.Warningf("Reference '%s' is ambiguous.", reference))
		fmt.Println(ui.Hint("Matches:"))
		if matches, ok := data["matches"].([]map[string]interface{}); ok {
			for _, match := range matches {
				src := ""
				if s, ok := match["match_source"].(string); ok {
					src = " " + ui.Hint("("+s+")")
				}
				fmt.Println(ui.Bullet(fmt.Sprintf("%s%s", match["object_id"], src)))
			}
		}
		return nil
	}

	if resolved, _ := data["resolved"].(bool); !resolved {
		fmt.Println(ui.Starf("Reference '%s' not found.", reference))
		return nil
	}

	objectID, _ := data["object_id"].(string)
	objectType, _ := data["type"].(string)
	relFilePath, _ := data["file_path"].(string)
	isSection, _ := data["is_section"].(bool)
	fileObjectID, _ := data["file_object_id"].(string)
	matchSource, _ := data["match_source"].(string)

	fmt.Printf("%s %s\n", ui.SectionHeader("Resolved"), ui.Bold.Render(objectID))
	if objectType != "" {
		fmt.Printf("  %s %s\n", ui.Hint("Type:"), objectType)
	}
	fmt.Printf("  %s %s\n", ui.Hint("File:"), ui.FilePath(relFilePath))
	if isSection {
		fmt.Printf("  %s %s\n", ui.Hint("Parent:"), fileObjectID)
	}
	if matchSource != "" {
		fmt.Printf("  %s %s\n", ui.Hint("Matched via:"), matchSource)
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
