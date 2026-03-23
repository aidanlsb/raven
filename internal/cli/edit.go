package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	editConfirm   bool
	editEditsJSON string
)

var editCmd = &cobra.Command{
	Use:   "edit <reference> [old_str] [new_str]",
	Short: commands.Registry["edit"].Description,
	Long:  commands.Registry["edit"].LongDesc,
	Args:  cobra.RangeArgs(1, 3),
	ValidArgsFunction: completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    false,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	}),
	RunE: func(cmd *cobra.Command, args []string) error {
		argsMap := map[string]interface{}{
			"path":    args[0],
			"confirm": editConfirm,
		}
		if editEditsJSON != "" {
			var payload interface{}
			if err := json.Unmarshal([]byte(editEditsJSON), &payload); err != nil {
				return handleErrorMsg(ErrInvalidInput, "invalid --edits-json payload", `Provide an object like: --edits-json '{"edits":[{"old_str":"from","new_str":"to"}]}'`)
			}
			argsMap["edits-json"] = payload
		}
		if len(args) > 1 {
			argsMap["old_str"] = args[1]
		}
		if len(args) > 2 {
			argsMap["new_str"] = args[2]
		}

		result := executeCanonicalRequest(commandexec.Request{
			CommandID: "edit",
			VaultPath: getVaultPath(),
			Args:      argsMap,
			Confirm:   editConfirm,
		})
		if !result.OK {
			return handleCanonicalEditFailure(result)
		}
		if jsonOutput {
			outputJSON(result)
			return nil
		}

		return renderCanonicalEditResult(result)
	},
}

func handleCanonicalEditFailure(result commandexec.Result) error {
	if result.Error == nil {
		return handleErrorMsg(ErrInternal, "edit failed", "")
	}
	if result.Error.Details != nil {
		return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
	}
	return handleErrorMsg(result.Error.Code, result.Error.Message, result.Error.Suggestion)
}

func renderCanonicalEditResult(result commandexec.Result) error {
	data := canonicalDataMap(result)
	status, _ := data["status"].(string)
	if status == "preview" {
		if edits := editItems(data["edits"]); len(edits) > 0 {
			path, _ := data["path"].(string)
			fmt.Printf("%s %s\n\n", ui.SectionHeader("Preview edits"), ui.FilePath(path))
			for _, edit := range edits {
				line, _ := edit["line"].(float64)
				index, _ := edit["index"].(float64)
				preview := stringMapValue(edit["preview"])
				before := preview["before"]
				after := preview["after"]
				fmt.Println(ui.Muted.Render(fmt.Sprintf("EDIT %d (line %d):", int(index), int(line))))
				fmt.Println(ui.Muted.Render("BEFORE:"))
				fmt.Println(indent(before, "  "))
				fmt.Println()
				fmt.Println(ui.Bold.Render("AFTER:"))
				fmt.Println(indent(after, "  "))
				fmt.Println()
			}
			fmt.Println(ui.Hint("Run with --confirm to apply this edit"))
			return nil
		}

		path, _ := data["path"].(string)
		line, _ := data["line"].(float64)
		preview := stringMapValue(data["preview"])
		before := preview["before"]
		after := preview["after"]
		fmt.Printf("%s %s\n\n", ui.SectionHeader("Preview edit"), ui.FilePath(fmt.Sprintf("%s:%d", path, int(line))))
		fmt.Println(ui.Muted.Render("BEFORE:"))
		fmt.Println(indent(before, "  "))
		fmt.Println()
		fmt.Println(ui.Bold.Render("AFTER:"))
		fmt.Println(indent(after, "  "))
		fmt.Println()
		fmt.Println(ui.Hint("Run with --confirm to apply this edit"))
		return nil
	}

	if edits := editItems(data["edits"]); len(edits) > 0 {
		path, _ := data["path"].(string)
		fmt.Println(ui.Checkf("Applied %d edits in %s", len(edits), ui.FilePath(path)))
		fmt.Println()
		for _, edit := range edits {
			line, _ := edit["line"].(float64)
			index, _ := edit["index"].(float64)
			contextText, _ := edit["context"].(string)
			fmt.Println(ui.Muted.Render(fmt.Sprintf("EDIT %d (line %d):", int(index), int(line))))
			fmt.Println(indent(contextText, "  "))
			fmt.Println()
		}
		return nil
	}

	path, _ := data["path"].(string)
	line, _ := data["line"].(float64)
	contextText, _ := data["context"].(string)
	fmt.Println(ui.Checkf("Applied edit in %s", ui.FilePath(fmt.Sprintf("%s:%d", path, int(line)))))
	fmt.Println()
	fmt.Println(ui.Muted.Render("Context:"))
	fmt.Println(indent(contextText, "  "))
	return nil
}

func editItems(raw interface{}) []map[string]interface{} {
	switch items := raw.(type) {
	case []map[string]interface{}:
		return items
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(items))
		for _, item := range items {
			typed, ok := item.(map[string]interface{})
			if ok {
				out = append(out, typed)
			}
		}
		return out
	default:
		return nil
	}
}

func stringMapValue(raw interface{}) map[string]string {
	switch value := raw.(type) {
	case map[string]string:
		return value
	case map[string]interface{}:
		out := make(map[string]string, len(value))
		for key, item := range value {
			if s, ok := item.(string); ok {
				out[key] = s
			}
		}
		return out
	default:
		return map[string]string{}
	}
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func init() {
	editCmd.Flags().BoolVar(&editConfirm, "confirm", false, "Apply the edit (default: preview only)")
	editCmd.Flags().StringVar(&editEditsJSON, "edits-json", "", "JSON object with ordered edits, e.g. '{\"edits\":[{\"old_str\":\"from\",\"new_str\":\"to\"}]}'")
	rootCmd.AddCommand(editCmd)
}
