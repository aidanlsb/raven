package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/ui"
)

var searchCmd = newCanonicalLeafCommand("search", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Args:        cobra.ArbitraryArgs,
	Prepare:     prepareSearchArgs,
	BuildArgs:   buildSearchArgs,
	HandleError: handleCanonicalSearchFailure,
	RenderHuman: renderSearch,
})

func prepareSearchArgs(_ *cobra.Command, args []string) ([]string, bool, error) {
	if len(args) > 0 {
		return args, false, nil
	}
	if isJSONOutput() {
		return args, false, nil
	}

	if canUseFZFInteractive() {
		vaultPath := getVaultPath()
		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return nil, false, handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		selectedPath, selected, err := pickVaultFileWithFZF(vaultPath, vaultCfg, "search> ", "Search indexed files (Esc to cancel)")
		if err != nil {
			return nil, false, handleError(ErrInternal, err, "Run 'rvn reindex' to refresh indexed files")
		}
		if !selected {
			return nil, true, nil
		}
		fmt.Println(ui.SectionHeader("Selected"))
		fmt.Println(ui.Bullet(ui.FilePath(selectedPath)))
		return nil, true, nil
	}

	err := handleErrorMsg(
		ErrMissingArgument,
		"specify a search query",
		interactivePickerMissingArgSuggestion("search", "rvn search <query>"),
	)
	return nil, err == nil, err
}

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
	match.IsSection, _ = row["is_section"].(bool)
	match.FileObjectID, _ = row["file_object_id"].(string)
	match.LineStart = intFromAny(row["line_start"])
	match.LineEnd = intPointerFromAny(row["line_end"])
	match.SubtreeLineEnd = intPointerFromAny(row["subtree_line_end"])
	switch rank := row["rank"].(type) {
	case float64:
		match.Rank = rank
	case float32:
		match.Rank = float64(rank)
	}
	return match
}

func mapSearchCode(code codes.ErrorCode) codes.ErrorCode {
	switch code {
	case codes.ErrMissingArgument:
		return ErrMissingArgument
	case codes.ErrDatabase:
		return ErrDatabaseError
	case codes.ErrInvalidArgs, codes.ErrInvalidInput:
		return ErrInvalidInput
	default:
		return ErrInternal
	}
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
