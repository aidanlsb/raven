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
	HandleError: handleCanonicalQueryFailure,
	RenderHuman: renderQuerySavedList,
})

var querySavedGetCmd = newCanonicalLeafCommand("query_saved_get", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	HandleError: handleCanonicalQueryFailure,
	RenderHuman: renderQuerySavedGet,
})

var querySavedSetCmd = newCanonicalLeafCommand("query_saved_set", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	BuildArgs:   buildQuerySavedSetArgs,
	HandleError: handleCanonicalQueryFailure,
	RenderHuman: renderQuerySavedSet,
})

var querySavedRemoveCmd = newCanonicalLeafCommand("query_saved_remove", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	HandleError: handleCanonicalQueryFailure,
	RenderHuman: renderQuerySavedRemove,
})

func buildQuerySavedSetArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	declaredArgs, err := normalizeSavedQueryArgsForCommand(cmd)
	if err != nil {
		return nil, err
	}
	description, _ := cmd.Flags().GetString("description")
	argsMap := map[string]interface{}{
		"name":         args[0],
		"query_string": args[1],
		"arg":          declaredArgs,
		"description":  description,
	}
	addSavedQueryOptionArgs(cmd, argsMap)
	return argsMap, nil
}

func addSavedQueryOptionArgs(cmd *cobra.Command, argsMap map[string]interface{}) {
	if cmd.Flags().Changed("refresh") {
		value, _ := cmd.Flags().GetBool("refresh")
		argsMap["refresh"] = value
	}
	if cmd.Flags().Changed("ids") {
		value, _ := cmd.Flags().GetBool("ids")
		argsMap["ids"] = value
	}
	if cmd.Flags().Changed("limit") {
		value, _ := cmd.Flags().GetInt("limit")
		argsMap["limit"] = value
	}
	if cmd.Flags().Changed("offset") {
		value, _ := cmd.Flags().GetInt("offset")
		argsMap["offset"] = value
	}
	if cmd.Flags().Changed("count-only") {
		value, _ := cmd.Flags().GetBool("count-only")
		argsMap["count-only"] = value
	}
	if cmd.Flags().Changed("apply") {
		value, _ := cmd.Flags().GetStringArray("apply")
		argsMap["apply"] = value
	}
	if cmd.Flags().Changed("confirm") {
		value, _ := cmd.Flags().GetBool("confirm")
		argsMap["confirm"] = value
	}
	if cmd.Flags().Changed("pipe") {
		value, _ := cmd.Flags().GetBool("pipe")
		argsMap["pipe"] = value
	} else if cmd.Flags().Changed("no-pipe") {
		noPipe, _ := cmd.Flags().GetBool("no-pipe")
		if noPipe {
			argsMap["pipe"] = false
		}
	}
	if cmd.Flags().Changed("browse") {
		value, _ := cmd.Flags().GetBool("browse")
		argsMap["browse"] = value
	}
}

func handleCanonicalQueryFailure(result commandexec.Result) error {
	if result.Error == nil {
		return nil
	}
	return handleErrorWithDetails(mapQueryCode(result.Error.Code), result.Error.Message, result.Error.Suggestion, result.Error.Details)
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

func normalizeSavedQueryArgsForCommand(cmd *cobra.Command) ([]string, error) {
	rawArgs, err := cmd.Flags().GetStringArray("arg")
	if err != nil {
		return nil, handleError(ErrInternal, err, "")
	}
	normalized, err := querysvc.NormalizeArgs(rawArgs)
	if err != nil {
		return nil, mapQuerySvcError(err)
	}
	return normalized, nil
}
