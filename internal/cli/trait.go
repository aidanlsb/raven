package cli

import (
	"fmt"
	"strings"

	"github.com/ravenscroftj/raven/internal/index"
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
  rvn trait highlight              # All highlighted items`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		traitName := args[0]

		valueFilter, _ := cmd.Flags().GetString("value")

		db, err := index.Open(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		var filter *string
		if valueFilter != "" {
			filter = &valueFilter
		}

		results, err := db.QueryTraits(traitName, filter)
		if err != nil {
			return fmt.Errorf("failed to query traits: %w", err)
		}

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

// Helper to display traits grouped by content
func printGroupedResults(results []index.TraitResult) {
	type contentKey struct {
		filePath string
		line     int
	}

	grouped := make(map[contentKey][]index.TraitResult)
	var order []contentKey

	for _, r := range results {
		key := contentKey{r.FilePath, r.Line}
		if _, exists := grouped[key]; !exists {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], r)
	}

	for _, key := range order {
		traits := grouped[key]
		content := traits[0].Content

		var traitStrs []string
		for _, t := range traits {
			if t.Value != nil && *t.Value != "" {
				traitStrs = append(traitStrs, fmt.Sprintf("@%s(%s)", t.TraitType, *t.Value))
			} else {
				traitStrs = append(traitStrs, fmt.Sprintf("@%s", t.TraitType))
			}
		}

		fmt.Printf("• %s\n", content)
		fmt.Printf("  %s  %s:%d\n", strings.Join(traitStrs, " "), key.filePath, key.line)
	}
}

func init() {
	traitCmd.Flags().String("value", "", "Filter by trait value (supports: today, past, this-week, or specific values)")
	rootCmd.AddCommand(traitCmd)
}
