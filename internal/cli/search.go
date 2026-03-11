package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/readsvc"
)

var searchLimit int
var searchType string

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: commands.Registry["search"].Description,
	Long:  commands.Registry["search"].LongDesc,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		query := strings.Join(args, " ")

		rt, err := readsvc.NewRuntime(vaultPath, readsvc.RuntimeOptions{OpenDB: true})
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer rt.Close()

		results, err := readsvc.Search(rt, query, searchType, searchLimit)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		readsvc.SaveSearchResults(vaultPath, query, results)

		// Output results
		if jsonOutput {
			outputSuccess(map[string]interface{}{
				"query":   query,
				"results": formatSearchResults(results),
			}, &Meta{Count: len(results)})
			return nil
		}

		printSearchResults(query, results)

		return nil
	},
}

func formatSearchResults(results []model.SearchMatch) []map[string]interface{} {
	formatted := make([]map[string]interface{}, len(results))
	for i, r := range results {
		formatted[i] = map[string]interface{}{
			"object_id": r.ObjectID,
			"title":     r.Title,
			"file_path": r.FilePath,
			"snippet":   r.Snippet,
			"rank":      r.Rank,
		}
	}
	return formatted
}

func init() {
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 20, "Maximum number of results")
	searchCmd.Flags().StringVarP(&searchType, "type", "t", "", "Filter by object type")
	rootCmd.AddCommand(searchCmd)
}
