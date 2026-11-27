package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/querysvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var querySavedCmd = &cobra.Command{
	Use:   "saved",
	Short: "Manage saved queries",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var querySavedListCmd = newCanonicalLeafCommand("query_saved_list", canonicalLeafOptions{
	RenderHuman: renderQuerySavedList,
})

var querySavedGetCmd = newCanonicalLeafCommand("query_saved_get", canonicalLeafOptions{
	RenderHuman: renderQuerySavedGet,
})

var querySavedSetCmd = newCanonicalLeafCommand("query_saved_set", canonicalLeafOptions{
	BuildArgs:   buildQuerySavedSetArgs,
	RenderHuman: renderQuerySavedSet,
})

var querySavedRemoveCmd = newCanonicalLeafCommand("query_saved_remove", canonicalLeafOptions{
	RenderHuman: renderQuerySavedRemove,
})

func buildQuerySavedSetArgs(cmd *cobra.Command, _ []string, argsMap map[string]interface{}) error {
	rawArgs, err := cmd.Flags().GetStringArray("arg")
	if err != nil {
		return handleError(ErrInternal, err, "")
	}
	normalized, err := querysvc.NormalizeArgs(rawArgs)
	if err != nil {
		result := commandexec.FromServiceError(err)
		if isJSONOutput() {
			return outputCanonicalResultJSON(result)
		}
		return handleCanonicalFailure(result)
	}
	if len(normalized) > 0 {
		argsMap["arg"] = normalized
	}
	return nil
}

func renderQuerySavedList(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[querysvc.SavedQueryListResult](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	return listSavedQueries(data.Queries)
}

func renderQuerySavedGet(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[querysvc.SavedQueryPayload](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	fmt.Printf("%s %s\n", ui.Hint("Name:"), data.Name)
	fmt.Printf("%s %s\n", ui.Hint("Query:"), data.Query)
	if len(data.Args) > 0 {
		fmt.Printf("%s %s\n", ui.Hint("Args:"), strings.Join(data.Args, ", "))
	} else {
		fmt.Printf("%s %s\n", ui.Hint("Args:"), ui.Hint("(none)"))
	}
	if data.Description != "" {
		fmt.Printf("%s %s\n", ui.Hint("Description:"), data.Description)
	}
	return nil
}

func renderQuerySavedSet(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[querysvc.SavedQuerySetResult](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	switch data.Status {
	case "created":
		fmt.Println(ui.Checkf("Created saved query '%s'", data.Name))
	case "updated":
		fmt.Println(ui.Checkf("Updated saved query '%s'", data.Name))
	default:
		fmt.Println(ui.Starf("Saved query '%s' unchanged", data.Name))
	}
	fmt.Printf("  %s %s\n", ui.Hint("Run with:"), ui.Bold.Render("rvn query "+data.Name))
	return nil
}

func renderQuerySavedRemove(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[querysvc.SavedQueryRemoveResult](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	fmt.Println(ui.Checkf("Removed query '%s'", data.Name))
	return nil
}
