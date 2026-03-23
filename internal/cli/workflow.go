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

var workflowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available workflows",
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		result := executeCanonicalCommand("workflow_list", vaultPath, nil)
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}
		data := canonicalDataMap(result)
		items, err := decodeSchemaValue[[]workflow.ListItem](data["workflows"])
		if err != nil {
			return err
		}

		if len(items) == 0 {
			fmt.Println("No workflows defined in raven.yaml")
			return nil
		}

		// Sort by name
		sort.Slice(items, func(i, j int) bool {
			return items[i].Name < items[j].Name
		})

		for _, item := range items {
			fmt.Printf("%-20s %s\n", item.Name, item.Description)
		}

		return nil
	},
}

var workflowShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show workflow details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		name := args[0]
		result := executeCanonicalCommand("workflow_show", vaultPath, map[string]interface{}{"name": name})
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}
		data := canonicalDataMap(result)
		wf := &workflow.Workflow{
			Name:        name,
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
	},
}

var workflowAddFile string
var workflowScaffoldFile string
var workflowScaffoldDescription string
var workflowScaffoldForce bool
var workflowStepAddJSON string
var workflowStepAddBefore string
var workflowStepAddAfter string
var workflowStepUpdateJSON string

var workflowStepCmd = &cobra.Command{
	Use:   "step",
	Short: "Edit workflow definition steps",
	Long:  `Edit step definitions in a workflow YAML file without manual YAML editing.`,
}

var workflowStepAddCmd = &cobra.Command{
	Use:   "add <workflow-name>",
	Short: "Add a step to a workflow definition",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		workflowName := strings.TrimSpace(args[0])
		if workflowName == "" {
			return handleErrorMsg(ErrMissingArgument, "workflow name cannot be empty", "")
		}

		if strings.TrimSpace(workflowStepAddBefore) != "" && strings.TrimSpace(workflowStepAddAfter) != "" {
			return handleErrorMsg(ErrInvalidInput, "use either --before or --after, not both", "")
		}
		stepJSON, err := decodeCLIJSONObject(workflowStepAddJSON)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}
		result := executeCanonicalCommand("workflow_step_add", vaultPath, map[string]interface{}{
			"workflow-name": workflowName,
			"step-json":     stepJSON,
			"before":        strings.TrimSpace(workflowStepAddBefore),
			"after":         strings.TrimSpace(workflowStepAddAfter),
		})
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}

		data := canonicalDataMap(result)
		fmt.Printf("Added step '%s' to workflow '%s'.\n", data["step_id"], workflowName)
		return nil
	},
}

var workflowStepUpdateCmd = &cobra.Command{
	Use:   "update <workflow-name> <step-id>",
	Short: "Update a step in a workflow definition",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		workflowName := strings.TrimSpace(args[0])
		stepID := strings.TrimSpace(args[1])
		if workflowName == "" || stepID == "" {
			return handleErrorMsg(ErrMissingArgument, "workflow name and step id are required", "")
		}
		stepJSON, err := decodeCLIJSONObject(workflowStepUpdateJSON)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}
		result := executeCanonicalCommand("workflow_step_update", vaultPath, map[string]interface{}{
			"workflow-name": workflowName,
			"step-id":       stepID,
			"step-json":     stepJSON,
		})
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}

		data := canonicalDataMap(result)
		if stringValue(data["step_id"]) == stepID {
			fmt.Printf("Updated step '%s' in workflow '%s'.\n", stepID, workflowName)
		} else {
			fmt.Printf("Updated step '%s' (renamed from '%s') in workflow '%s'.\n", data["step_id"], stepID, workflowName)
		}
		return nil
	},
}

var workflowStepRemoveCmd = &cobra.Command{
	Use:   "remove <workflow-name> <step-id>",
	Short: "Remove a step from a workflow definition",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		workflowName := strings.TrimSpace(args[0])
		stepID := strings.TrimSpace(args[1])
		if workflowName == "" || stepID == "" {
			return handleErrorMsg(ErrMissingArgument, "workflow name and step id are required", "")
		}
		result := executeCanonicalCommand("workflow_step_remove", vaultPath, map[string]interface{}{
			"workflow-name": workflowName,
			"step-id":       stepID,
		})
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}

		fmt.Printf("Removed step '%s' from workflow '%s'.\n", stepID, workflowName)
		return nil
	},
}

var workflowAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a workflow to raven.yaml",
	Long: `Add a workflow definition to raven.yaml.

Workflow declarations are file references only.
The referenced file must exist under directories.workflow (default: workflows/).

Examples:
  rvn workflow add meeting-prep --file workflows/meeting-prep.yaml`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		name := strings.TrimSpace(args[0])
		if name == "" {
			return handleErrorMsg(ErrMissingArgument, "workflow name cannot be empty", "")
		}
		if strings.TrimSpace(workflowAddFile) == "" {
			return handleErrorMsg(ErrMissingArgument, "--file is required", "Use --file <workflow YAML path>")
		}
		result := executeCanonicalCommand("workflow_add", vaultPath, map[string]interface{}{
			"name": name,
			"file": workflowAddFile,
		})
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}

		fmt.Printf("Added workflow '%s'.\n", name)
		fmt.Printf("Run with: rvn workflow run %s\n", name)
		return nil
	},
}

var workflowScaffoldCmd = &cobra.Command{
	Use:   "scaffold <name>",
	Short: "Scaffold a starter workflow file and config entry",
	Long: `Create a starter workflow file and register it in raven.yaml.

By default this creates <directories.workflow>/<name>.yaml and adds:
  workflows:
    <name>:
      file: <directories.workflow>/<name>.yaml

Use --file to choose a different file path and --force to overwrite an
existing scaffold file.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		name := strings.TrimSpace(args[0])
		if name == "" {
			return handleErrorMsg(ErrMissingArgument, "workflow name cannot be empty", "")
		}
		result := executeCanonicalCommand("workflow_scaffold", vaultPath, map[string]interface{}{
			"name":        name,
			"file":        workflowScaffoldFile,
			"description": workflowScaffoldDescription,
			"force":       workflowScaffoldForce,
		})
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}

		data := canonicalDataMap(result)
		fmt.Printf("Scaffolded workflow '%s' at %s\n", name, data["file"])
		fmt.Printf("Run with: rvn workflow run %s --input topic=\"...\"\n", name)
		return nil
	},
}

var workflowRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a workflow from raven.yaml",
	Long: `Remove a workflow definition from raven.yaml.

Examples:
  rvn workflow remove meeting-prep
  rvn workflow remove daily-brief`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		name := strings.TrimSpace(args[0])
		if name == "" {
			return handleErrorMsg(ErrMissingArgument, "workflow name cannot be empty", "")
		}
		result := executeCanonicalCommand("workflow_remove", vaultPath, map[string]interface{}{"name": name})
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}

		fmt.Printf("Removed workflow '%s'.\n", name)
		return nil
	},
}

var workflowValidateCmd = &cobra.Command{
	Use:   "validate [name]",
	Short: "Validate workflow definitions",
	Long: `Validate one workflow or all workflows defined in raven.yaml.

Examples:
  rvn workflow validate
  rvn workflow validate meeting-prep`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		name := ""
		if len(args) == 1 {
			name = strings.TrimSpace(args[0])
		}
		result := executeCanonicalCommand("workflow_validate", vaultPath, map[string]interface{}{"name": name})
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}
		data := canonicalDataMap(result)
		results, err := decodeSchemaValue[[]workflow.ValidationItem](data["results"])
		if err != nil {
			return err
		}

		fmt.Printf("All %d workflow(s) are valid.\n", len(results))
		return nil
	},
}

var workflowInputFlags []string
var workflowInputJSON string
var workflowInputFile string

var workflowRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Run a workflow until it reaches an agent step",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		name := args[0]
		argsMap, err := workflowRunArgs(name)
		if err != nil {
			return handleError(ErrWorkflowInputInvalid, err, "")
		}
		result := executeCanonicalCommand("workflow_run", vaultPath, argsMap)
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}
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

		fmt.Println("Workflow completed (no agent steps).")
		fmt.Println()
		fmt.Println("=== STEP SUMMARIES ===")
		stepSummariesJSON, _ := json.MarshalIndent(runResult.StepSummaries, "", "  ")
		fmt.Println(string(stepSummariesJSON))
		return nil
	},
}

var workflowContinueOutputJSON string
var workflowContinueOutput string
var workflowContinueOutputFile string
var workflowContinueExpectedRevision int

var workflowContinueCmd = &cobra.Command{
	Use:   "continue <run-id>",
	Short: "Continue a paused workflow run with agent output JSON",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		runID := args[0]
		argsMap, err := workflowContinueArgs(runID)
		if err != nil {
			return handleError(ErrWorkflowAgentOutputInvalid, err, "")
		}
		result := executeCanonicalCommand("workflow_continue", vaultPath, argsMap)
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}
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

		fmt.Println("Workflow completed.")
		fmt.Println()
		fmt.Println("=== STEP SUMMARIES ===")
		stepSummariesJSON, _ := json.MarshalIndent(runResult.StepSummaries, "", "  ")
		fmt.Println(string(stepSummariesJSON))
		return nil
	},
}

var workflowRunsStatus string
var workflowRunsWorkflow string
var workflowRunsStepPath string
var workflowRunsStepOffset int
var workflowRunsStepLimit int

var workflowRunsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List persisted workflow runs",
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		result := executeCanonicalCommand("workflow_runs_list", vaultPath, map[string]interface{}{
			"workflow": workflowRunsWorkflow,
			"status":   workflowRunsStatus,
		})
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}
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
	},
}

var workflowRunsStepCmd = &cobra.Command{
	Use:   "step <run-id> <step-id>",
	Short: "Fetch output for a specific workflow step",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		runID := args[0]
		stepID := strings.TrimSpace(args[1])
		argsMap := map[string]interface{}{
			"run-id":  runID,
			"step-id": stepID,
		}
		if cmd.Flags().Changed("path") {
			argsMap["path"] = workflowRunsStepPath
		}
		if cmd.Flags().Changed("offset") {
			argsMap["offset"] = workflowRunsStepOffset
		}
		if cmd.Flags().Changed("limit") {
			argsMap["limit"] = workflowRunsStepLimit
		}
		result := executeCanonicalCommand("workflow_runs_step", vaultPath, argsMap)
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}
		payload := canonicalDataMap(result)
		paginationRequested := cmd.Flags().Changed("path") || cmd.Flags().Changed("offset") || cmd.Flags().Changed("limit")

		fmt.Printf("run_id: %s\n", payload["run_id"])
		fmt.Printf("workflow: %s\n", payload["workflow_name"])
		fmt.Printf("step_id: %s\n\n", stepID)
		stepValue := payload["step_output"]
		if paginationRequested {
			stepValue = payload["step_output_page"]
		}
		stepJSON, _ := json.MarshalIndent(stepValue, "", "  ")
		fmt.Println(string(stepJSON))
		return nil
	},
}

var workflowRunsPruneStatus string
var workflowRunsPruneOlderThan string
var workflowRunsPruneConfirm bool

var workflowRunsPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Prune persisted workflow runs",
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		result := executeCanonicalCommand("workflow_runs_prune", vaultPath, map[string]interface{}{
			"status":     workflowRunsPruneStatus,
			"older-than": workflowRunsPruneOlderThan,
			"confirm":    workflowRunsPruneConfirm,
		})
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}
		data := canonicalDataMap(result)
		prune, err := decodeSchemaValue[workflow.RunPruneResult](data["prune"])
		if err != nil {
			return err
		}

		for _, w := range result.Warnings {
			fmt.Printf("Warning [%s]: %s (%s)\n", w.Code, w.Message, w.Ref)
		}

		if workflowRunsPruneConfirm {
			fmt.Printf("Deleted %d runs (matched %d of %d scanned).\n", prune.Deleted, prune.Matched, prune.Scanned)
		} else {
			fmt.Printf("Would delete %d runs (scanned %d). Use --confirm to apply.\n", prune.Matched, prune.Scanned)
		}
		return nil
	},
}

var workflowRunsCmd = &cobra.Command{
	Use:   "runs",
	Short: "Manage persisted workflow run records",
}

func workflowRunArgs(name string) (map[string]interface{}, error) {
	args := map[string]interface{}{
		"name":       name,
		"input":      keyValueFlagsToObject(workflowInputFlags),
		"input-file": workflowInputFile,
	}
	if strings.TrimSpace(workflowInputJSON) != "" {
		obj, err := decodeCLIJSONObject(workflowInputJSON)
		if err != nil {
			return nil, err
		}
		args["input-json"] = obj
	}
	return args, nil
}

func workflowContinueArgs(runID string) (map[string]interface{}, error) {
	args := map[string]interface{}{
		"run-id":            runID,
		"agent-output":      workflowContinueOutput,
		"agent-output-file": workflowContinueOutputFile,
		"expected-revision": workflowContinueExpectedRevision,
	}
	if strings.TrimSpace(workflowContinueOutputJSON) != "" {
		obj, err := decodeCLIJSONObject(workflowContinueOutputJSON)
		if err != nil {
			return nil, err
		}
		args["agent-output-json"] = obj
	}
	return args, nil
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

func decodeCLIJSONObject(raw string) (map[string]interface{}, error) {
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func keyValueFlagsToObject(flags []string) map[string]interface{} {
	if len(flags) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(flags))
	for _, item := range flags {
		key, value, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func init() {
	workflowAddCmd.Flags().StringVar(&workflowAddFile, "file", "", "Path to external workflow YAML file (relative to vault root)")
	workflowScaffoldCmd.Flags().StringVar(&workflowScaffoldFile, "file", "", "Path for the scaffolded workflow YAML file (relative to vault root)")
	workflowScaffoldCmd.Flags().StringVar(&workflowScaffoldDescription, "description", "", "Description for the scaffolded workflow")
	workflowScaffoldCmd.Flags().BoolVar(&workflowScaffoldForce, "force", false, "Overwrite scaffold file if it already exists")
	workflowStepAddCmd.Flags().StringVar(&workflowStepAddJSON, "step-json", "", "Step definition JSON object")
	workflowStepAddCmd.Flags().StringVar(&workflowStepAddBefore, "before", "", "Insert new step before this step id")
	workflowStepAddCmd.Flags().StringVar(&workflowStepAddAfter, "after", "", "Insert new step after this step id")
	_ = workflowStepAddCmd.MarkFlagRequired("step-json")
	workflowStepUpdateCmd.Flags().StringVar(&workflowStepUpdateJSON, "step-json", "", "Step patch JSON object")
	_ = workflowStepUpdateCmd.MarkFlagRequired("step-json")

	workflowRunCmd.Flags().StringArrayVar(&workflowInputFlags, "input", nil, "Set input value (can be repeated): --input name=value")
	workflowRunCmd.Flags().StringVar(&workflowInputJSON, "input-json", "", "Set workflow inputs as a JSON object")
	workflowRunCmd.Flags().StringVar(&workflowInputFile, "input-file", "", "Read workflow inputs from JSON file")

	workflowContinueCmd.Flags().StringVar(&workflowContinueOutputJSON, "agent-output-json", "", "Agent output JSON object")
	workflowContinueCmd.Flags().StringVar(&workflowContinueOutput, "agent-output", "", "Agent output JSON object (string form)")
	workflowContinueCmd.Flags().StringVar(&workflowContinueOutputFile, "agent-output-file", "", "Path to JSON file containing agent output")
	workflowContinueCmd.Flags().IntVar(&workflowContinueExpectedRevision, "expected-revision", 0, "Expected run revision for optimistic concurrency")

	workflowRunsListCmd.Flags().StringVar(&workflowRunsStatus, "status", "", "Filter by status (comma-separated)")
	workflowRunsListCmd.Flags().StringVar(&workflowRunsWorkflow, "workflow", "", "Filter by workflow name")
	workflowRunsStepCmd.Flags().StringVar(&workflowRunsStepPath, "path", "", "Dot path within step output to paginate (e.g. data.results)")
	workflowRunsStepCmd.Flags().IntVar(&workflowRunsStepOffset, "offset", 0, "Zero-based offset for paginated step output")
	workflowRunsStepCmd.Flags().IntVar(&workflowRunsStepLimit, "limit", 100, "Page size for paginated step output")

	workflowRunsPruneCmd.Flags().StringVar(&workflowRunsPruneStatus, "status", "", "Prune only statuses (comma-separated)")
	workflowRunsPruneCmd.Flags().StringVar(&workflowRunsPruneOlderThan, "older-than", "", "Prune records older than duration (e.g. 72h, 14d)")
	workflowRunsPruneCmd.Flags().BoolVar(&workflowRunsPruneConfirm, "confirm", false, "Apply deletion (without this flag, preview only)")

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
