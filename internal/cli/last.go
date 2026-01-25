// Package cli implements the command-line interface.
package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/lastquery"
	"github.com/aidanlsb/raven/internal/ui"
)

var lastCmd = &cobra.Command{
	Use:   "last [numbers...]",
	Short: "Show or select results from the last query",
	Long: `Show or select results from the most recent query.

Without arguments, displays all results from the last query with their numbers.
With number arguments, outputs the selected IDs for piping to other commands.

Number formats:
  1         Single result
  1,3,5     Multiple results (comma-separated)
  1-5       Range of results
  1,3-5,7   Mixed format

Examples:
  rvn last                              # Show all results from last query
  rvn last 1,3                          # Output IDs for results 1 and 3
  rvn last 1-5 | rvn update --stdin value=done   # Pipe to update command
  rvn last | fzf --multi | rvn update --stdin value=done  # Interactive selection

With --apply, applies an operation directly to selected results:
  rvn last 1,3 --apply "update value=done"
  rvn last 1-5 --apply "set status=archived"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		// Handle --pipe/--no-pipe flags
		if pipeFlag, _ := cmd.Flags().GetBool("pipe"); pipeFlag {
			t := true
			SetPipeFormat(&t)
		} else if noPipeFlag, _ := cmd.Flags().GetBool("no-pipe"); noPipeFlag {
			f := false
			SetPipeFormat(&f)
		}

		// Load the last query
		lq, err := lastquery.Read(vaultPath)
		if err != nil {
			if err == lastquery.ErrNoLastQuery {
				return handleErrorMsg(ErrMissingArgument,
					"no query results available",
					"Run a query first, then use 'rvn last' to see or select results")
			}
			return handleError(ErrInternal, err, "")
		}

		// Check for --apply flag
		applyStr, _ := cmd.Flags().GetString("apply")
		confirmApply, _ := cmd.Flags().GetBool("confirm")

		// If no number args, display all results
		if len(args) == 0 {
			return displayLastQuery(lq, applyStr, confirmApply, vaultPath)
		}

		// Parse the number arguments
		nums, err := lastquery.ParseNumberArgs(args)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(),
				fmt.Sprintf("Valid range: 1-%d", len(lq.Results)))
		}

		// Get the selected entries
		entries, err := lq.GetByNumbers(nums)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(),
				fmt.Sprintf("Last query returned %d results", len(lq.Results)))
		}

		// If --apply is set, apply the operation
		if applyStr != "" {
			return applyToEntries(vaultPath, entries, applyStr, confirmApply, lq.Type)
		}

		// Output selected IDs (for piping)
		if isJSONOutput() {
			items := make([]map[string]interface{}, len(entries))
			for i, e := range entries {
				items[i] = map[string]interface{}{
					"num":      e.Num,
					"id":       e.ID,
					"kind":     e.Kind,
					"content":  e.Content,
					"location": e.Location,
				}
			}
			outputSuccess(map[string]interface{}{
				"selected": items,
			}, &Meta{Count: len(items)})
			return nil
		}

		// Plain output: one ID per line for piping
		for _, e := range entries {
			fmt.Println(e.ID)
		}
		return nil
	},
}

// displayLastQuery shows the last query results in human-readable format.
func displayLastQuery(lq *lastquery.LastQuery, applyStr string, confirm bool, vaultPath string) error {
	if isJSONOutput() {
		items := make([]map[string]interface{}, len(lq.Results))
		for i, r := range lq.Results {
			items[i] = map[string]interface{}{
				"num":      r.Num,
				"id":       r.ID,
				"kind":     r.Kind,
				"content":  r.Content,
				"location": r.Location,
			}
		}
		outputSuccess(map[string]interface{}{
			"query":     lq.Query,
			"timestamp": lq.Timestamp,
			"type":      lq.Type,
			"items":     items,
		}, &Meta{Count: len(items)})
		return nil
	}

	// Check for pipe mode - output pipe format for fzf integration
	if ShouldUsePipeFormat() {
		pipeItems := make([]PipeableItem, len(lq.Results))
		for _, r := range lq.Results {
			pipeItems[r.Num-1] = PipeableItem{
				Num:      r.Num,
				ID:       r.ID,
				Content:  TruncateContent(r.Content, 60),
				Location: r.Location,
			}
		}
		WritePipeableList(os.Stdout, pipeItems)
		return nil
	}

	// If --apply is set without numbers, apply to all
	if applyStr != "" {
		return applyToEntries(vaultPath, lq.Results, applyStr, confirm, lq.Type)
	}

	// Human-readable display
	if len(lq.Results) == 0 {
		fmt.Println(ui.Starf("Last query returned no results"))
		fmt.Printf("Query: %s\n", lq.Query)
		return nil
	}

	// Header with query info
	ago := formatTimeAgo(lq.Timestamp)
	fmt.Printf("%s %s\n", ui.Header("Last Query"), ui.Hint(fmt.Sprintf("(%s)", ago)))
	fmt.Printf("%s\n\n", ui.Muted.Render(lq.Query))

	// Results table
	if lq.Type == "trait" {
		printLastQueryTraitResults(lq.Results)
	} else {
		printLastQueryObjectResults(lq.Results)
	}

	return nil
}

// printLastQueryTraitResults prints trait results with numbers.
// Matches the format used by printTraitRows in query.go.
func printLastQueryTraitResults(results []lastquery.ResultEntry) {
	// Column widths
	numWidth := len(fmt.Sprintf("%d", len(results)))
	if numWidth < 2 {
		numWidth = 2
	}
	contentWidth := 52

	for i, r := range results {
		numStr := fmt.Sprintf("%*d", numWidth, r.Num)

		content := r.Content
		if len(content) > contentWidth {
			content = content[:contentWidth-3] + "..."
		}

		// Highlight traits in content
		content = ui.HighlightTraits(content)

		// Build trait string (e.g., "@todo(done)")
		// Hide value if it matches the trait type
		value := ""
		if r.TraitValue != nil && *r.TraitValue != r.TraitType {
			value = *r.TraitValue
		}
		traitStr := ui.Trait(r.TraitType, value)

		// Build metadata string: "trait · location"
		metadata := traitStr + " " + ui.Muted.Render("·") + " " + ui.Muted.Render(r.Location)

		fmt.Printf("  %s  %s  %s\n",
			ui.Muted.Render(numStr),
			ui.PadRight(content, contentWidth),
			metadata)

		// Separator between items (except last)
		if i < len(results)-1 {
			fmt.Println(ui.Muted.Render("  " + strings.Repeat("─", 90)))
		}
	}
}

// printLastQueryObjectResults prints object results with numbers.
func printLastQueryObjectResults(results []lastquery.ResultEntry) {
	numWidth := len(fmt.Sprintf("%d", len(results)))
	if numWidth < 2 {
		numWidth = 2
	}

	for _, r := range results {
		numStr := fmt.Sprintf("%*d", numWidth, r.Num)
		fmt.Printf("  %s  %-30s  %s\n",
			ui.Bold.Render(numStr),
			r.Content,
			ui.Muted.Render(r.Location))
	}
}

// applyToEntries applies an operation to the given entries.
func applyToEntries(vaultPath string, entries []lastquery.ResultEntry, applyStr string, confirm bool, queryType string) error {
	if len(entries) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no entries to apply operation to", "")
	}

	// Parse the apply command
	parts := strings.Fields(applyStr)
	if len(parts) == 0 {
		return handleErrorMsg(ErrInvalidInput, "no apply command specified",
			"Use --apply \"update value=done\" or similar")
	}

	applyCmd := parts[0]
	applyArgs := parts[1:]

	// Collect IDs
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}

	// Load vault config for operations
	vaultCfg := loadVaultConfigSafe(vaultPath)

	// For trait queries, only 'update' with 'value=' is supported
	var traitValue string
	if queryType == "trait" {
		if applyCmd != "update" {
			return handleErrorMsg(ErrInvalidInput,
				fmt.Sprintf("'%s' is not supported for trait results", applyCmd),
				"For traits, use: --apply \"update value=<new_value>\"")
		}
		var err error
		traitValue, err = parseTraitValueArgs(applyArgs)
		if err != nil {
			return err
		}
	}

	// Dispatch to appropriate bulk operation
	var warnings []Warning
	switch applyCmd {
	case "set":
		return applySetFromQuery(vaultPath, ids, applyArgs, warnings, nil, vaultCfg, confirm)
	case "update":
		if queryType != "trait" {
			return handleErrorMsg(ErrInvalidInput, "update is only supported for trait results",
				"Use: --apply \"set field=value\" for object results")
		}
		return applyUpdateTraitsByID(vaultPath, ids, traitValue, confirm, vaultCfg)
	case "delete":
		if queryType == "trait" {
			return handleErrorMsg(ErrInvalidInput, "cannot delete traits directly",
				"Traits are part of their containing file")
		}
		return applyDeleteFromQuery(vaultPath, ids, warnings, vaultCfg, confirm)
	case "move":
		if queryType == "trait" {
			return handleErrorMsg(ErrInvalidInput, "cannot move traits directly",
				"Traits are part of their containing file")
		}
		return applyMoveFromQuery(vaultPath, ids, applyArgs, warnings, vaultCfg, confirm)
	default:
		return handleErrorMsg(ErrInvalidInput,
			fmt.Sprintf("unknown apply command: %s", applyCmd),
			"Supported commands: set, delete, move")
	}
}

// formatTimeAgo formats a timestamp as a human-readable "X ago" string.
func formatTimeAgo(t time.Time) string {
	diff := time.Since(t)

	if diff < time.Minute {
		return "just now"
	} else if diff < time.Hour {
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	} else if diff < 24*time.Hour {
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}

	days := int(diff.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}

func init() {
	lastCmd.Flags().String("apply", "", "Apply an operation to selected results (e.g., \"update value=done\")")
	lastCmd.Flags().Bool("confirm", false, "Apply changes (without this flag, shows preview only)")
	lastCmd.Flags().Bool("pipe", false, "Force pipe-friendly output format")
	lastCmd.Flags().Bool("no-pipe", false, "Force human-readable output format")

	rootCmd.AddCommand(lastCmd)
}
