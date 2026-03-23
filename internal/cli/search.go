package cli

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
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
		query := strings.Join(args, " ")
		result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
			CommandID: "search",
			VaultPath: vaultPath,
			Caller:    commandexec.CallerCLI,
			Args: map[string]interface{}{
				"query": query,
				"limit": searchLimit,
				"type":  searchType,
			},
		})
		if !result.OK {
			if isJSONOutput() {
				outputJSON(result)
				return nil
			}
			if result.Error != nil {
				return handleErrorWithDetails(mapSearchCode(result.Error.Code), result.Error.Message, result.Error.Suggestion, result.Error.Details)
			}
			return handleErrorMsg(ErrInternal, "command execution failed", "")
		}

		if isJSONOutput() {
			outputJSON(result)
			return nil
		}

		data, _ := result.Data.(map[string]interface{})
		resultQuery, _ := data["query"].(string)
		printSearchResults(resultQuery, searchMatchesFromResult(data["results"]))

		return nil
	},
}

func searchMatchesFromResult(raw interface{}) []model.SearchMatch {
	rows, ok := raw.([]map[string]interface{})
	if ok {
		matches := make([]model.SearchMatch, 0, len(rows))
		for _, row := range rows {
			matches = append(matches, searchMatchFromMap(row))
		}
		return matches
	}

	genericRows, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	matches := make([]model.SearchMatch, 0, len(genericRows))
	for _, row := range genericRows {
		rowMap, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		matches = append(matches, searchMatchFromMap(rowMap))
	}
	return matches
}

func searchMatchFromMap(row map[string]interface{}) model.SearchMatch {
	match := model.SearchMatch{}
	if row == nil {
		return match
	}
	match.ObjectID, _ = row["object_id"].(string)
	match.Title, _ = row["title"].(string)
	match.FilePath, _ = row["file_path"].(string)
	match.Snippet, _ = row["snippet"].(string)
	switch rank := row["rank"].(type) {
	case float64:
		match.Rank = rank
	case float32:
		match.Rank = float64(rank)
	}
	return match
}

func mapSearchCode(code string) string {
	switch code {
	case "DATABASE_ERROR":
		return ErrDatabaseError
	case "INVALID_ARGS", "INVALID_INPUT":
		return ErrInvalidInput
	default:
		return ErrInternal
	}
}

func init() {
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 20, "Maximum number of results")
	searchCmd.Flags().StringVarP(&searchType, "type", "t", "", "Filter by object type")
	rootCmd.AddCommand(searchCmd)
}
