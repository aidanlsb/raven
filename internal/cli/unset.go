package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/ui"
)

var unsetCmd = newCanonicalLeafCommand("unset", canonicalLeafOptions{
	VaultPath: getVaultPath,
	RenderHuman: func(_ *cobra.Command, result commandexec.Result) error {
		return renderCanonicalUnsetResult(result)
	},
})


func renderCanonicalUnsetResult(result commandexec.Result) error {
	data, ok := result.Data.(commandpayload.UnsetResult)
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	if data.Modified {
		fmt.Println(ui.Checkf("Updated %s", ui.FilePath(data.File)))
	} else {
		fmt.Println(ui.Hint(fmt.Sprintf("No fields removed from %s", data.File)))
	}

	fieldNames := make([]string, 0, len(data.RemovedFields))
	for name := range data.RemovedFields {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)
	for _, name := range fieldNames {
		fmt.Printf("  - %s: %s\n", name, ui.Muted.Render(data.RemovedFields[name]))
	}

	missingFields := append([]string(nil), data.MissingFields...)
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
