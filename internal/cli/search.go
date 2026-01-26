package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/lastresults"
	"github.com/aidanlsb/raven/internal/model"
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

		// Open database
		db, _, err := index.OpenWithRebuild(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		// Join all args as the search query
		query := strings.Join(args, " ")

		// Perform search
		var results []model.SearchMatch
		if searchType != "" {
			results, err = db.SearchWithType(query, searchType, searchLimit)
		} else {
			results, err = db.Search(query, searchLimit)
		}
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		saveLastSearchResults(vaultPath, query, results)

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

func saveLastSearchResults(vaultPath, query string, results []model.SearchMatch) {
	modelResults := make([]model.Result, len(results))
	for i, r := range results {
		modelResults[i] = r
	}
	lr, err := lastresults.NewFromResults(lastresults.SourceSearch, query, "", modelResults)
	if err != nil {
		return
	}
	_ = lastresults.Write(vaultPath, lr)
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
