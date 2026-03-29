package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
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
		fmt.Println("No workflows defined in raven.yaml")
		return nil
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	for _, item := range items {
		fmt.Printf("%-20s %s\n", item.Name, item.Description)
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

	fmt.Printf("name: %s\n", wf.Name)
	if wf.Description != "" {
		fmt.Printf("description: %s\n", wf.Description)
	}
	if len(wf.Inputs) > 0 {
		fmt.Println("\ninputs:")
		for name, input := range wf.Inputs {
			req := ""
			if input.Required {
				req = " (required)"
			}
			fmt.Printf("  %s: %s%s\n", name, input.Type, req)
			if input.Description != "" {
				fmt.Printf("    %s\n", input.Description)
			}
		}
	}
	if len(wf.Steps) > 0 {
		fmt.Println("\nsteps:")
		for i, step := range wf.Steps {
			if step == nil {
				fmt.Printf("  %d. (nil)\n", i+1)
				continue
			}
			fmt.Printf("  %d. %s (%s)\n", i+1, step.ID, step.Type)
			if step.Description != "" {
				fmt.Printf("     %s\n", step.Description)
			}
			switch step.Type {
			case "agent":
				fmt.Printf("     outputs: %d\n", len(step.Outputs))
			case "tool":
				fmt.Printf("     tool: %s\n", step.Tool)
			case "foreach":
				if step.ForEach != nil {
					fmt.Printf("     items: %s\n", step.ForEach.Items)
					fmt.Printf("     nested_steps: %d\n", len(step.ForEach.Steps))
					if step.ForEach.OnError != "" {
						fmt.Printf("     on_error: %s\n", step.ForEach.OnError)
					}
				}
			case "switch":
				if step.Switch != nil {
					fmt.Printf("     value: %s\n", step.Switch.Value)
					fmt.Printf("     cases: %d\n", len(step.Switch.Cases))
					if step.Switch.Default != nil {
						fmt.Printf("     has_default: true\n")
					}
					if len(step.Switch.Outputs) > 0 {
						fmt.Printf("     outputs: %d\n", len(step.Switch.Outputs))
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
	fmt.Printf("Added step '%s' to workflow '%s'.\n", data["step_id"], data["workflow_name"])
	return nil
}

func renderWorkflowStepUpdate(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	workflowName := stringValue(data["workflow_name"])
	previousStepID := stringValue(data["previous_step_id"])
	stepID := stringValue(data["step_id"])
	if stepID == previousStepID || previousStepID == "" {
		fmt.Printf("Updated step '%s' in workflow '%s'.\n", stepID, workflowName)
		return nil
	}
	fmt.Printf("Updated step '%s' (renamed from '%s') in workflow '%s'.\n", stepID, previousStepID, workflowName)
	return nil
}

func renderWorkflowStepRemove(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("Removed step '%s' from workflow '%s'.\n", data["step_id"], data["workflow_name"])
	return nil
}

func renderWorkflowAdd(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	name := stringValue(data["name"])
	fmt.Printf("Added workflow '%s'.\n", name)
	fmt.Printf("Run with: rvn workflow run %s\n", name)
	return nil
}

func renderWorkflowScaffold(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	name := stringValue(data["name"])
	fmt.Printf("Scaffolded workflow '%s' at %s\n", name, data["file"])
	fmt.Printf("Run with: rvn workflow run %s --input topic=\"...\"\n", name)
	return nil
}

func renderWorkflowRemove(_ *cobra.Command, result commandexec.Result) error {
	fmt.Printf("Removed workflow '%s'.\n", canonicalDataMap(result)["name"])
	return nil
}

func renderWorkflowValidate(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	results, err := decodeSchemaValue[[]workflow.ValidationItem](data["results"])
	if err != nil {
		return err
	}
	fmt.Printf("All %d workflow(s) are valid.\n", len(results))
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

	fmt.Printf("run_id: %s\n", runResult.RunID)
	fmt.Printf("status: %s\n", runResult.Status)
	fmt.Printf("revision: %d\n\n", runResult.Revision)
	if runResult.Next != nil {
		fmt.Println("=== AGENT ===")
		fmt.Println(runResult.Next.Prompt)
		fmt.Println()
		fmt.Println("=== OUTPUTS ===")
		outputJSON, _ := json.MarshalIndent(runResult.Next.Outputs, "", "  ")
		fmt.Println(string(outputJSON))
		fmt.Println()
		fmt.Println("=== STEP SUMMARIES ===")
		stepSummariesJSON, _ := json.MarshalIndent(runResult.StepSummaries, "", "  ")
		fmt.Println(string(stepSummariesJSON))
		fmt.Println()
		fmt.Printf("Use 'rvn workflow runs step %s <step-id>' to fetch a specific step output.\n", runResult.RunID)
		return nil
	}

	fmt.Println(completionMessage)
	fmt.Println()
	fmt.Println("=== STEP SUMMARIES ===")
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
		fmt.Printf("Warning [%s]: %s (%s)\n", w.Code, w.Message, w.Ref)
	}
	if len(runs) == 0 {
		fmt.Println("No workflow runs.")
		return nil
	}
	for _, run := range runs {
		revision, err := decodeSchemaCount(run["revision"])
		if err != nil {
			return err
		}
		fmt.Printf("%s  %-18s %-14s rev=%d updated=%s\n",
			run["run_id"],
			run["workflow_name"],
			run["status"],
			revision,
			run["updated_at"],
		)
	}
	return nil
}

func renderWorkflowRunsStep(cmd *cobra.Command, result commandexec.Result) error {
	payload := canonicalDataMap(result)
	paginationRequested := cmd.Flags().Changed("path") || cmd.Flags().Changed("offset") || cmd.Flags().Changed("limit")

	fmt.Printf("run_id: %s\n", payload["run_id"])
	fmt.Printf("workflow: %s\n", payload["workflow_name"])
	fmt.Printf("step_id: %s\n\n", payload["step_id"])
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
		fmt.Printf("Warning [%s]: %s (%s)\n", w.Code, w.Message, w.Ref)
	}
	confirm, _ := cmd.Flags().GetBool("confirm")
	if confirm {
		fmt.Printf("Deleted %d runs (matched %d of %d scanned).\n", prune.Deleted, prune.Matched, prune.Scanned)
		return nil
	}
	fmt.Printf("Would delete %d runs (scanned %d). Use --confirm to apply.\n", prune.Matched, prune.Scanned)
	return nil
}

func init() {
	_ = workflowStepAddCmd.MarkFlagRequired("step-json")
	_ = workflowStepUpdateCmd.MarkFlagRequired("step-json")

	workflowCmd.AddCommand(workflowListCmd)
	workflowCmd.AddCommand(workflowAddCmd)
	workflowCmd.AddCommand(workflowScaffoldCmd)
	workflowCmd.AddCommand(workflowRemoveCmd)
	workflowCmd.AddCommand(workflowValidateCmd)
	workflowCmd.AddCommand(workflowShowCmd)
	workflowStepCmd.AddCommand(workflowStepAddCmd)
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
