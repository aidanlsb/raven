package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

// handleResolveError converts a resolve error to an appropriate CLI error output.
// Returns the error code used.
// =============================================================================
// RESOLVE COMMAND
// =============================================================================

var resolveCmd = newExceptionLeafCommand("resolve", exceptionLeafOptions{
	Prepare:     prepareResolveArgs,
	RenderHuman: renderResolve,
})

func prepareResolveArgs(_ *cobra.Command, args []string) ([]string, bool, error) {
	return prepareInteractiveReferenceArgs(args, "resolve", "reference", "resolve> ", "Select a reference to resolve (Esc to cancel)")
}

func renderResolve(_ *cobra.Command, result commandexec.Result) error {
	switch data := result.Data.(type) {
	case readsvc.ResolveAmbiguousPayload:
		fmt.Println(ui.Warningf("Reference '%s' is ambiguous.", data.Reference))
		fmt.Println(ui.Hint("Matches:"))
		for _, match := range data.Items {
			src := ""
			if match.MatchSource != "" {
				src = " " + ui.Hint("("+match.MatchSource+")")
			}
			fmt.Println(ui.Bullet(fmt.Sprintf("%s%s", match.ObjectID, src)))
		}
		return nil
	case readsvc.ResolveMissPayload:
		fmt.Println(ui.Starf("Reference '%s' not found.", data.Reference))
		return nil
	case readsvc.ResolveHitPayload:
		fmt.Printf("%s %s\n", ui.SectionHeader("Resolved"), ui.Bold.Render(data.ObjectID))
		if data.Type != "" {
			fmt.Printf("  %s %s\n", ui.Hint("Type:"), data.Type)
		}
		fmt.Printf("  %s %s\n", ui.Hint("File:"), ui.FilePath(data.FilePath))
		if data.IsSection {
			fmt.Printf("  %s %s\n", ui.Hint("Parent:"), data.FileObjectID)
		}
		if data.MatchSource != "" {
			fmt.Printf("  %s %s\n", ui.Hint("Matched via:"), data.MatchSource)
		}
		return nil
	default:
		return handleErrorMsg(ErrInternal, "unexpected resolve result shape", "")
	}
}

func init() {
	resolveCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: true,
		DisableWhenStdin:    false,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	})
	rootCmd.AddCommand(resolveCmd)
}
