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
	VaultPath:   getVaultPath,
	RenderHuman: renderQuerySavedList,
})

var querySavedGetCmd = newCanonicalLeafCommand("query_saved_get", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderQuerySavedGet,
})

var querySavedSetCmd = newCanonicalLeafCommand("query_saved_set", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Prepare:     prepareQuerySavedSet,
	RenderHuman: renderQuerySavedSet,
})

var querySavedRemoveCmd = newCanonicalLeafCommand("query_saved_remove", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderQuerySavedRemove,
})

func prepareQuerySavedSet(cmd *cobra.Command, args []string) ([]string, bool, error) {
	// Normalize --arg values via querysvc before building canonical args
	rawArgs, err := cmd.Flags().GetStringArray("arg")
	if err != nil {
		return nil, false, handleError(ErrInternal, err, "")
	}
	normalized, err := querysvc.NormalizeArgs(rawArgs)
	if err != nil {
		return nil, false, handleCanonicalFailure(commandexec.FromServiceError(err))
	}
	// Replace --arg flag values with normalized versions
	if err := cmd.Flags().Set("arg", ""); err != nil {
		return nil, false, handleError(ErrInternal, err, "")
	}
	for _, arg := range normalized {
		if err := cmd.Flags().Set("arg", arg); err != nil {
			return nil, false, handleError(ErrInternal, err, "")
		}
	}
	return args, false, nil
}

func renderQuerySavedList(_ *cobra.Command, result commandexec.Result) error {
	return listSavedQueries(savedQueriesFromResult(canonicalDataMap(result)["queries"]))
}

func renderQuerySavedGet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("%s %s\n", ui.Hint("Name:"), stringValue(data["name"]))
	fmt.Printf("%s %s\n", ui.Hint("Query:"), stringValue(data["query"]))
	if args := stringSliceFromAny(data["args"]); len(args) > 0 {
		fmt.Printf("%s %s\n", ui.Hint("Args:"), strings.Join(args, ", "))
	} else {
		fmt.Printf("%s %s\n", ui.Hint("Args:"), ui.Hint("(none)"))
	}
	if description := stringValue(data["description"]); description != "" {
		fmt.Printf("%s %s\n", ui.Hint("Description:"), description)
	}
	return nil
}

func renderQuerySavedSet(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	status := stringValue(data["status"])
	name := stringValue(data["name"])
	switch status {
	case "created":
		fmt.Println(ui.Checkf("Created saved query '%s'", name))
	case "updated":
		fmt.Println(ui.Checkf("Updated saved query '%s'", name))
	default:
		fmt.Println(ui.Starf("Saved query '%s' unchanged", name))
	}
	fmt.Printf("  %s %s\n", ui.Hint("Run with:"), ui.Bold.Render("rvn query "+name))
	return nil
}

func renderQuerySavedRemove(_ *cobra.Command, result commandexec.Result) error {
	fmt.Println(ui.Checkf("Removed query '%s'", stringValue(canonicalDataMap(result)["name"])))
	return nil
}
