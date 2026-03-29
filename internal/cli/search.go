package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
)

var searchCmd = newCanonicalLeafCommand("search", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Args:        cobra.MinimumNArgs(1),
	BuildArgs:   buildSearchArgs,
	HandleError: handleCanonicalSearchFailure,
	RenderHuman: renderSearch,
})

func buildSearchArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	meta, _ := cmd.Flags().GetString("type")
	limit, _ := cmd.Flags().GetInt("limit")
	return map[string]interface{}{
		"query": strings.Join(args, " "),
		"limit": limit,
		"type":  meta,
	}, nil
}

func handleCanonicalSearchFailure(result commandexec.Result) error {
	if result.Error == nil {
		return nil
	}
	return handleErrorWithDetails(mapSearchCode(result.Error.Code), result.Error.Message, result.Error.Suggestion, result.Error.Details)
}

func renderSearch(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	resultQuery, _ := data["query"].(string)
	printSearchResults(resultQuery, searchMatchesFromResult(data["results"]))
	return nil
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
	rootCmd.AddCommand(searchCmd)
}
