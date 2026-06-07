package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

var unsetCmd = newCanonicalLeafCommand("unset", canonicalLeafOptions{
	VaultPath: getVaultPath,
	Args:      cobra.ArbitraryArgs,
	BuildArgs: buildUnsetArgs,
	RenderHuman: func(_ *cobra.Command, result commandexec.Result) error {
		return renderCanonicalUnsetResult(result)
	},
})

func buildUnsetArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	if len(args) < 1 {
		return nil, handleErrorMsg(ErrMissingArgument, "requires object-id", "Usage: rvn unset <object-id> <field>...")
	}

	fields := append([]string{}, args[1:]...)
	flagFields, _ := cmd.Flags().GetStringArray("fields")
	fields = append(fields, flagFields...)
	fields = normalizedUnsetCLIFields(fields)
	if len(fields) == 0 {
		return nil, handleErrorMsg(ErrMissingArgument, "no fields to unset", "Usage: rvn unset <object-id> <field>...")
	}

	return map[string]interface{}{
		"object_id": args[0],
		"fields":    stringsToAny(fields),
	}, nil
}

func normalizedUnsetCLIFields(fields []string) []string {
	seen := make(map[string]struct{}, len(fields))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

func renderCanonicalUnsetResult(result commandexec.Result) error {
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	relativePath, _ := data["file"].(string)
	modified := boolValue(data["modified"])
	if modified {
		fmt.Println(ui.Checkf("Updated %s", ui.FilePath(relativePath)))
	} else {
		fmt.Println(ui.Hint(fmt.Sprintf("No fields removed from %s", relativePath)))
	}

	removedFields := stringMapFromAny(data["removed_fields"])
	fieldNames := make([]string, 0, len(removedFields))
	for name := range removedFields {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)
	for _, name := range fieldNames {
		fmt.Printf("  - %s: %s\n", name, ui.Muted.Render(removedFields[name]))
	}

	missingFields := stringSliceFromAny(data["missing_fields"])
	sort.Strings(missingFields)
	for _, name := range missingFields {
		fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("%s was already absent", name)))
	}

	for _, warning := range result.Warnings {
		fmt.Printf("  %s\n", ui.Warning(warning.Message))
	}
	return nil
}

func init() {
	unsetCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	})
	rootCmd.AddCommand(unsetCmd)
}
