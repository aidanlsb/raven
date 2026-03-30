package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/workflow"
)

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage and run workflows",
	Long:  `Workflows are multi-step pipelines with deterministic tool steps and agent steps.`,
}

var workflowListCmd = newCanonicalLeafCommand("workflow_list", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderWorkflowList,
})

var workflowShowCmd = newCanonicalLeafCommand("workflow_show", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderWorkflowShow,
})

var workflowStepCmd = &cobra.Command{
	Use:   "step",
	Short: "Edit workflow definition steps",
	Long:  `Edit step definitions in a workflow YAML file without manual YAML editing.`,
}

var workflowStepAddCmd = newCanonicalLeafCommand("workflow_step_add", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	BuildArgs:   buildWorkflowStepAddArgs,
	RenderHuman: renderWorkflowStepAdd,
})

var workflowStepBatchCmd = newCanonicalLeafCommand("workflow_step_batch", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderWorkflowStepBatch,
})

var workflowStepUpdateCmd = newCanonicalLeafCommand("workflow_step_update", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderWorkflowStepUpdate,
})

var workflowStepRemoveCmd = newCanonicalLeafCommand("workflow_step_remove", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderWorkflowStepRemove,
})

var workflowAddCmd = newCanonicalLeafCommand("workflow_add", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	BuildArgs:   buildWorkflowAddArgs,
	RenderHuman: renderWorkflowAdd,
})

var workflowScaffoldCmd = newCanonicalLeafCommand("workflow_scaffold", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderWorkflowScaffold,
})

var workflowRemoveCmd = newCanonicalLeafCommand("workflow_remove", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderWorkflowRemove,
})

var workflowValidateCmd = newCanonicalLeafCommand("workflow_validate", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderWorkflowValidate,
})

var workflowRunCmd = newCanonicalLeafCommand("workflow_run", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	HandleError: handleWorkflowRunFailure,
	RenderHuman: renderWorkflowRun,
})

var workflowContinueCmd = newCanonicalLeafCommand("workflow_continue", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	HandleError: handleWorkflowContinueFailure,
	RenderHuman: renderWorkflowContinue,
})

var workflowRunsListCmd = newCanonicalLeafCommand("workflow_runs_list", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderWorkflowRunsList,
})

var workflowRunsStepCmd = newCanonicalLeafCommand("workflow_runs_step", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderWorkflowRunsStep,
})

var workflowRunsPruneCmd = newCanonicalLeafCommand("workflow_runs_prune", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderWorkflowRunsPrune,
})

var workflowRunsCmd = &cobra.Command{
	Use:   "runs",
	Short: "Manage persisted workflow run records",
}

func workflowRunResultFromCanonical(result commandexec.Result) (*workflow.RunResult, error) {
	if runResult, ok := result.Data.(*workflow.RunResult); ok && runResult != nil {
		return runResult, nil
	}
	data := canonicalDataMap(result)
	runResult, err := decodeSchemaValue[workflow.RunResult](data)
	if err != nil {
		return nil, err
	}
	return &runResult, nil
}

func renderWorkflowList(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	items, err := decodeSchemaValue[[]workflow.ListItem](data["workflows"])
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Println(ui.Star("No workflows defined in raven.yaml."))
		return nil
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	for _, item := range items {
		line := ui.Bold.Render(item.Name)
		if strings.TrimSpace(item.Description) != "" {
			line = fmt.Sprintf("%s %s", line, ui.Hint("— "+item.Description))
		}
		fmt.Println(ui.Bullet(line))
	}
	return nil
}

func renderWorkflowShow(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	wf := &workflow.Workflow{
		Name:        stringValue(data["name"]),
		Description: stringValue(data["description"]),
	}
	wf.Inputs, _ = decodeSchemaValue[map[string]*config.WorkflowInput](data["inputs"])
	wf.Steps, _ = decodeSchemaValue[[]*config.WorkflowStep](data["steps"])
	display := ui.NewDisplayContext()

	fmt.Printf("%s %s\n", ui.SectionHeader("Workflow"), ui.Bold.Render(wf.Name))
	if wf.Description != "" {
		fmt.Printf("%s %s\n", ui.Hint("Description:"), wf.Description)
	}
	if len(wf.Inputs) > 0 {
		fmt.Printf("\n%s\n", ui.DividerWithAccentLabel("inputs", display.TermWidth))
		for name, input := range wf.Inputs {
			req := ""
			if input.Required {
				req = " (required)"
			}
			fmt.Println(ui.Bullet(fmt.Sprintf("%s: %s%s", name, input.Type, req)))
			if input.Description != "" {
				fmt.Println(ui.Indent(2, ui.Hint(input.Description)))
			}
		}
	}
	if len(wf.Steps) > 0 {
		fmt.Printf("\n%s\n", ui.DividerWithAccentLabel("steps", display.TermWidth))
		for i, step := range wf.Steps {
			if step == nil {
				fmt.Println(ui.Bullet(fmt.Sprintf("%d. (nil)", i+1)))
				continue
			}
			fmt.Println(ui.Bullet(fmt.Sprintf("%d. %s (%s)", i+1, step.ID, step.Type)))
			if step.Description != "" {
				fmt.Println(ui.Indent(2, ui.Hint(step.Description)))
			}
			switch step.Type {
			case "agent":
				fmt.Println(ui.Indent(2, fmt.Sprintf("outputs: %d", len(step.Outputs))))
			case "tool":
				fmt.Println(ui.Indent(2, fmt.Sprintf("tool: %s", step.Tool)))
			case "foreach":
				if step.ForEach != nil {
					fmt.Println(ui.Indent(2, fmt.Sprintf("items: %s", step.ForEach.Items)))
					fmt.Println(ui.Indent(2, fmt.Sprintf("nested_steps: %d", len(step.ForEach.Steps))))
					if step.ForEach.OnError != "" {
						fmt.Println(ui.Indent(2, fmt.Sprintf("on_error: %s", step.ForEach.OnError)))
					}
				}
			case "switch":
				if step.Switch != nil {
					fmt.Println(ui.Indent(2, fmt.Sprintf("value: %s", step.Switch.Value)))
					fmt.Println(ui.Indent(2, fmt.Sprintf("cases: %d", len(step.Switch.Cases))))
					if step.Switch.Default != nil {
						fmt.Println(ui.Indent(2, "has_default: true"))
					}
					if len(step.Switch.Outputs) > 0 {
						fmt.Println(ui.Indent(2, fmt.Sprintf("outputs: %d", len(step.Switch.Outputs))))
					}
				}
			}
		}
	}
	return nil
}

func buildWorkflowStepAddArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	before, _ := cmd.Flags().GetString("before")
	after, _ := cmd.Flags().GetString("after")
	if strings.TrimSpace(before) != "" && strings.TrimSpace(after) != "" {
		return nil, handleErrorMsg(ErrInvalidInput, "use either --before or --after, not both", "")
	}
	argsMap := map[string]interface{}{
		"workflow-name": args[0],
	}
	if cmd.Flags().Changed("step-json") {
		raw, _ := cmd.Flags().GetString("step-json")
		var decoded interface{}
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			return nil, fmt.Errorf("invalid --step-json JSON: %w", err)
		}
		argsMap["step-json"] = decoded
	}
	if strings.TrimSpace(before) != "" {
		argsMap["before"] = before
	}
	if strings.TrimSpace(after) != "" {
		argsMap["after"] = after
	}
	return argsMap, nil
}

func buildWorkflowAddArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	file, _ := cmd.Flags().GetString("file")
	if strings.TrimSpace(file) == "" {
		return nil, handleErrorMsg(ErrMissingArgument, "--file is required", "Use --file <workflow YAML path>")
	}
	return map[string]interface{}{
		"name": args[0],
		"file": file,
	}, nil
}

func renderWorkflowStepAdd(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Println(ui.Checkf("Added step '%s' to workflow '%s'.", data["step_id"], data["workflow_name"]))
	return nil
}

func renderWorkflowStepBatch(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	count, err := decodeSchemaCount(data["count"])
	if err != nil {
		return err
	}
	fmt.Println(ui.Checkf("Applied %d step edits to workflow '%s'.", count, data["workflow_name"]))
	return nil
}

func renderWorkflowStepUpdate(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	workflowName := stringValue(data["workflow_name"])
	previousStepID := stringValue(data["previous_step_id"])
	stepID := stringValue(data["step_id"])
	if stepID == previousStepID || previousStepID == "" {
		fmt.Println(ui.Checkf("Updated step '%s' in workflow '%s'.", stepID, workflowName))
		return nil
	}
	fmt.Println(ui.Checkf("Updated step '%s' (renamed from '%s') in workflow '%s'.", stepID, previousStepID, workflowName))
	return nil
}

func renderWorkflowStepRemove(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Println(ui.Checkf("Removed step '%s' from workflow '%s'.", data["step_id"], data["workflow_name"]))
	return nil
}

func renderWorkflowAdd(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	name := stringValue(data["name"])
	fmt.Println(ui.Checkf("Added workflow '%s'.", name))
	fmt.Printf("%s\n", ui.Hint(fmt.Sprintf("Run with: rvn workflow run %s", name)))
	return nil
}

func renderWorkflowScaffold(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	name := stringValue(data["name"])
	fmt.Println(ui.Checkf("Scaffolded workflow '%s' at %s", name, ui.FilePath(stringValue(data["file"]))))
	fmt.Printf("%s\n", ui.Hint(fmt.Sprintf("Run with: rvn workflow run %s --input topic=\"...\"", name)))
	return nil
}

func renderWorkflowRemove(_ *cobra.Command, result commandexec.Result) error {
	fmt.Println(ui.Checkf("Removed workflow '%s'.", canonicalDataMap(result)["name"]))
	return nil
}

func renderWorkflowValidate(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	results, err := decodeSchemaValue[[]workflow.ValidationItem](data["results"])
	if err != nil {
		return err
	}
	fmt.Println(ui.Starf("All %d workflow(s) are valid.", len(results)))
	return nil
}

func handleWorkflowRunFailure(result commandexec.Result) error {
	if result.Error != nil && result.Error.Code == ErrWorkflowInputInvalid {
		return handleError(ErrWorkflowInputInvalid, fmt.Errorf("%s", result.Error.Message), result.Error.Suggestion)
	}
	return handleCanonicalFailure(result)
}

func handleWorkflowContinueFailure(result commandexec.Result) error {
	if result.Error != nil && result.Error.Code == ErrWorkflowAgentOutputInvalid {
		return handleError(ErrWorkflowAgentOutputInvalid, fmt.Errorf("%s", result.Error.Message), result.Error.Suggestion)
	}
	return handleCanonicalFailure(result)
}

func renderWorkflowRun(_ *cobra.Command, result commandexec.Result) error {
	return renderWorkflowRunResult(result, "Workflow completed (no agent steps).")
}

func renderWorkflowContinue(_ *cobra.Command, result commandexec.Result) error {
	return renderWorkflowRunResult(result, "Workflow completed.")
}

func renderWorkflowRunResult(result commandexec.Result, completionMessage string) error {
	runResult, err := workflowRunResultFromCanonical(result)
	if err != nil {
		return err
	}
	display := ui.NewDisplayContext()

	fmt.Printf("%s %s\n", ui.SectionHeader("Run"), ui.Bold.Render(runResult.RunID))
	fmt.Printf("%s %s\n", ui.Hint("Status:"), runResult.Status)
	fmt.Printf("%s %d\n\n", ui.Hint("Revision:"), runResult.Revision)
	if runResult.Next != nil {
		fmt.Println(ui.DividerWithAccentLabel("agent", display.TermWidth))
		fmt.Println(runResult.Next.Prompt)
		fmt.Println()
		fmt.Println(ui.DividerWithAccentLabel("outputs", display.TermWidth))
		outputJSON, _ := json.MarshalIndent(runResult.Next.Outputs, "", "  ")
		fmt.Println(string(outputJSON))
		fmt.Println()
		fmt.Println(ui.DividerWithAccentLabel("step summaries", display.TermWidth))
		stepSummariesJSON, _ := json.MarshalIndent(runResult.StepSummaries, "", "  ")
		fmt.Println(string(stepSummariesJSON))
		fmt.Println()
		fmt.Println(ui.Hint(fmt.Sprintf("Use 'rvn workflow runs step %s <step-id>' to fetch a specific step output.", runResult.RunID)))
		return nil
	}

	fmt.Println(ui.Star(completionMessage))
	fmt.Println()
	fmt.Println(ui.DividerWithAccentLabel("step summaries", display.TermWidth))
	stepSummariesJSON, _ := json.MarshalIndent(runResult.StepSummaries, "", "  ")
	fmt.Println(string(stepSummariesJSON))
	return nil
}

func renderWorkflowRunsList(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	runs, err := decodeSchemaValue[[]map[string]interface{}](data["runs"])
	if err != nil {
		return err
	}
	for _, w := range result.Warnings {
		fmt.Println(ui.Warningf("[%s] %s (%s)", w.Code, w.Message, w.Ref))
	}
	if len(runs) == 0 {
		fmt.Println(ui.Star("No workflow runs."))
		return nil
	}
	for _, run := range runs {
		revision, err := decodeSchemaCount(run["revision"])
		if err != nil {
			return err
		}
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s %s %s",
			ui.Bold.Render(stringValue(run["run_id"])),
			ui.Hint(fmt.Sprintf("workflow=%s", stringValue(run["workflow_name"]))),
			ui.Hint(fmt.Sprintf("status=%s", stringValue(run["status"]))),
			ui.Hint(fmt.Sprintf("rev=%d updated=%s", revision, stringValue(run["updated_at"]))),
		)))
	}
	return nil
}

func renderWorkflowRunsStep(cmd *cobra.Command, result commandexec.Result) error {
	payload := canonicalDataMap(result)
	paginationRequested := cmd.Flags().Changed("path") || cmd.Flags().Changed("offset") || cmd.Flags().Changed("limit")

	fmt.Printf("%s %s\n", ui.SectionHeader("Workflow step"), ui.Bold.Render(stringValue(payload["step_id"])))
	fmt.Printf("%s %s\n", ui.Hint("Run:"), stringValue(payload["run_id"]))
	fmt.Printf("%s %s\n\n", ui.Hint("Workflow:"), stringValue(payload["workflow_name"]))
	stepValue := payload["step_output"]
	if paginationRequested {
		stepValue = payload["step_output_page"]
	}
	stepJSON, _ := json.MarshalIndent(stepValue, "", "  ")
	fmt.Println(string(stepJSON))
	return nil
}

func renderWorkflowRunsPrune(cmd *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	prune, err := decodeSchemaValue[workflow.RunPruneResult](data["prune"])
	if err != nil {
		return err
	}
	for _, w := range result.Warnings {
		fmt.Println(ui.Warningf("[%s] %s (%s)", w.Code, w.Message, w.Ref))
	}
	confirm, _ := cmd.Flags().GetBool("confirm")
	if confirm {
		fmt.Println(ui.Checkf("Deleted %d runs (matched %d of %d scanned).", prune.Deleted, prune.Matched, prune.Scanned))
		return nil
	}
	fmt.Println(ui.Hint(fmt.Sprintf("Would delete %d runs (scanned %d). Use --confirm to apply.", prune.Matched, prune.Scanned)))
	return nil
}

func init() {
	_ = workflowStepAddCmd.MarkFlagRequired("step-json")
	_ = workflowStepBatchCmd.MarkFlagRequired("mutations-json")
	_ = workflowStepUpdateCmd.MarkFlagRequired("step-json")

	workflowCmd.AddCommand(workflowListCmd)
	workflowCmd.AddCommand(workflowAddCmd)
	workflowCmd.AddCommand(workflowScaffoldCmd)
	workflowCmd.AddCommand(workflowRemoveCmd)
	workflowCmd.AddCommand(workflowValidateCmd)
	workflowCmd.AddCommand(workflowShowCmd)
	workflowStepCmd.AddCommand(workflowStepAddCmd)
	workflowStepCmd.AddCommand(workflowStepBatchCmd)
	workflowStepCmd.AddCommand(workflowStepUpdateCmd)
	workflowStepCmd.AddCommand(workflowStepRemoveCmd)
	workflowCmd.AddCommand(workflowStepCmd)
	workflowCmd.AddCommand(workflowRunCmd)
	workflowCmd.AddCommand(workflowContinueCmd)
	workflowRunsCmd.AddCommand(workflowRunsListCmd)
	workflowRunsCmd.AddCommand(workflowRunsStepCmd)
	workflowRunsCmd.AddCommand(workflowRunsPruneCmd)
	workflowCmd.AddCommand(workflowRunsCmd)
	rootCmd.AddCommand(workflowCmd)
}
