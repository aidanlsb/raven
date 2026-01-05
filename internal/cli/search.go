package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/index"
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
		var results []index.SearchResult
		if searchType != "" {
			results, err = db.SearchWithType(query, searchType, searchLimit)
		} else {
			results, err = db.Search(query, searchLimit)
		}
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		// Output results
		if jsonOutput {
			outputSuccess(map[string]interface{}{
				"query":   query,
				"results": formatSearchResults(results),
			}, &Meta{Count: len(results)})
			return nil
		}

		if len(results) == 0 {
			fmt.Printf("No results found for: %s\n", query)
			return nil
		}

		fmt.Printf("Found %d results for: %s\n\n", len(results), query)
		for i, result := range results {
			fmt.Printf("%d. %s\n", i+1, result.Title)
			fmt.Printf("   %s\n", result.FilePath)
			if result.Snippet != "" {
				// Clean up snippet for display
				snippet := strings.ReplaceAll(result.Snippet, "\n", " ")
				snippet = strings.TrimSpace(snippet)
				if len(snippet) > 120 {
					snippet = snippet[:120] + "..."
				}
				fmt.Printf("   %s\n", snippet)
			}
			fmt.Println()
		}

		return nil
	},
}

func formatSearchResults(results []index.SearchResult) []map[string]interface{} {
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
