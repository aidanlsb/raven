package cli

import (
	"encoding/json"
	"fmt"

	"github.com/ravenscroftj/raven/internal/index"
	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query <query-string>",
	Short: "Query objects",
	Long: `Query objects using a query string.

Examples:
  rvn query "type:person"
  rvn query "type:meeting tags:planning"
  rvn query "type:project status:active"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		queryStr := args[0]

		db, err := index.Open(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		qb := index.NewQueryBuilder().Parse(queryStr)
		results, err := db.QueryObjects(qb)
		if err != nil {
			return fmt.Errorf("failed to query: %w", err)
		}

		if len(results) == 0 {
			fmt.Println("No results found.")
			return nil
		}

		fmt.Printf("Found %d result(s):\n\n", len(results))

		for _, result := range results {
			var fields map[string]interface{}
			json.Unmarshal([]byte(result.Fields), &fields)

			fmt.Printf("• %s [%s]\n", result.ID, result.Type)
			fmt.Printf("  %s:%d\n", result.FilePath, result.LineStart)

			// Print a few key fields
			for k, v := range fields {
				if k != "tags" {
					fmt.Printf("  %s: %v\n", k, v)
				}
			}
			fmt.Println()
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(queryCmd)
}
