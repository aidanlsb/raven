package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/ui"
)

var searchCmd = newCanonicalLeafCommand("search", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Prepare:     prepareSearchArgs,
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

	if canUseRavenInteractive() {
		vaultPath := getVaultPath()
		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return nil, false, handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		selectedPath, selected, err := cliSelector.vaultFile(vaultPath, vaultCfg, "search> ", "Search indexed files")
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


func handleCanonicalSearchFailure(result commandexec.Result) error {
	if result.Error == nil {
		return nil
	}
	return handleErrorWithDetails(mapSearchCode(result.Error.Code), result.Error.Message, result.Error.Suggestion, result.Error.Details)
}

func renderSearch(_ *cobra.Command, result commandexec.Result) error {
	payload, ok := result.Data.(commandpayload.SearchResult)
	if !ok {
		return handleErrorMsg(ErrInternal, "unexpected search result shape", "")
	}
	printSearchResults(payload.Query, searchMatchesFromItems(payload.Items))
	return nil
}

// searchMatchesFromItems adapts the typed search payload items into the model
// rows the shared retrieval renderer consumes. It is a thin field copy with no
// map decoding: the canonical handler emits typed commandpayload items
// in-process.
func searchMatchesFromItems(items []commandpayload.SearchMatchItem) []model.SearchMatch {
	matches := make([]model.SearchMatch, 0, len(items))
	for _, item := range items {
		matches = append(matches, model.SearchMatch{
			ObjectID:       item.ObjectID,
			Title:          item.Title,
			FilePath:       item.FilePath,
			Snippet:        item.Snippet,
			Rank:           item.Rank,
			IsSection:      item.IsSection,
			FileObjectID:   item.FileObjectID,
			LineStart:      item.LineStart,
			LineEnd:        item.LineEnd,
			SubtreeLineEnd: item.SubtreeLineEnd,
		})
	}
	return matches
}

func mapSearchCode(code codes.ErrorCode) codes.ErrorCode {
	switch code {
	case codes.ErrMissingArgument:
		return ErrMissingArgument
	case codes.ErrDatabase:
		return ErrDatabase
	case codes.ErrInvalidArgs, codes.ErrInvalidInput:
		return ErrInvalidInput
	default:
		return ErrInternal
	}
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
