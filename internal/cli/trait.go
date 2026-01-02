package cli

import (
	"fmt"
	"time"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/spf13/cobra"
)

var traitCmd = &cobra.Command{
	Use:   "trait <name> [--value <filter>]",
	Short: "Query traits by type",
	Long: `Query traits of a specific type with optional value filter.

Examples:
  rvn trait due                    # All items with @due
  rvn trait due --value past       # Overdue items
  rvn trait status --value todo    # Items with @status(todo)
  rvn trait highlight              # All highlighted items
  rvn trait due --json             # JSON output for agents`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		traitName := args[0]

		valueFilter, _ := cmd.Flags().GetString("value")

		start := time.Now()

		db, err := index.Open(vaultPath)
		if err != nil {
			return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
		}
		defer db.Close()

		var filter *string
		if valueFilter != "" {
			filter = &valueFilter
		}

		results, err := db.QueryTraits(traitName, filter)
		if err != nil {
			return handleError(ErrDatabaseError, err, "")
		}

		elapsed := time.Since(start).Milliseconds()

		if isJSONOutput() {
			items := make([]TraitResult, len(results))
			for i, r := range results {
				items[i] = TraitResult{
					ID:          fmt.Sprintf("%s:%d", r.FilePath, r.Line),
					TraitType:   r.TraitType,
					Value:       r.Value,
					Content:     r.Content,
					ContentText: r.Content, // Same for now
					ObjectID:    r.ParentID,
					FilePath:    r.FilePath,
					Line:        r.Line,
				}
			}
			outputSuccess(map[string]interface{}{
				"trait": traitName,
				"items": items,
			}, &Meta{Count: len(items), QueryTimeMs: elapsed})
			return nil
		}

		// Human-readable output
		if len(results) == 0 {
			fmt.Printf("No @%s traits found.\n", traitName)
			return nil
		}

		printResults(results)
		return nil
	},
}

func printResults(results []index.TraitResult) {
	for _, result := range results {
		// Format value display
		valueStr := ""
		if result.Value != nil && *result.Value != "" {
			valueStr = fmt.Sprintf("(%s)", *result.Value)
		}

		fmt.Printf("• %s\n", result.Content)
		fmt.Printf("  @%s%s  %s:%d\n", result.TraitType, valueStr, result.FilePath, result.Line)
	}
}

func init() {
	traitCmd.Flags().String("value", "", "Filter by trait value (supports: today, past, this-week, or specific values)")
	rootCmd.AddCommand(traitCmd)
}
