package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/toolexec"
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

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		items, err := workflow.List(vaultPath, vaultCfg)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		if isJSONOutput() {
			// Convert to JSON-friendly format
			workflows := make([]map[string]interface{}, len(items))
			for i, item := range items {
				w := map[string]interface{}{
					"name":        item.Name,
					"description": item.Description,
				}
				if len(item.Inputs) > 0 {
					w["inputs"] = item.Inputs
				}
				workflows[i] = w
			}
			outputSuccess(map[string]interface{}{
				"workflows": workflows,
			}, nil)
			return nil
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

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		wf, err := workflow.Get(vaultPath, name, vaultCfg)
		if err != nil {
			return handleError(ErrQueryNotFound, err, "Use 'rvn workflow list' to see available workflows")
		}

		if isJSONOutput() {
			out := map[string]interface{}{
				"name":        wf.Name,
				"description": wf.Description,
			}
			if len(wf.Inputs) > 0 {
				out["inputs"] = wf.Inputs
			}
			if len(wf.Steps) > 0 {
				out["steps"] = wf.Steps
			}
			outputSuccess(out, nil)
			return nil
		}

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

		step, err := workflow.ParseStepObject(workflowStepAddJSON, true)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}

		if strings.TrimSpace(workflowStepAddBefore) != "" && strings.TrimSpace(workflowStepAddAfter) != "" {
			return handleErrorMsg(ErrInvalidInput, "use either --before or --after, not both", "")
		}

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
		result, err := svc.MutateStep(workflow.StepMutationRequest{
			WorkflowName: workflowName,
			Action:       workflow.StepMutationAdd,
			Step:         step,
			Position: workflow.PositionHint{
				BeforeStepID: strings.TrimSpace(workflowStepAddBefore),
				AfterStepID:  strings.TrimSpace(workflowStepAddAfter),
			},
		})
		if err != nil {
			return handleWorkflowDomainError(err, "Run 'rvn workflow list' to see available workflows")
		}

		payload := map[string]interface{}{
			"workflow_name": workflowName,
			"file":          result.FileRef,
			"action":        "add",
			"step_id":       result.StepID,
			"step":          result.Step,
			"index":         result.Index,
		}
		if isJSONOutput() {
			outputSuccess(payload, nil)
			return nil
		}

		fmt.Printf("Added step '%s' to workflow '%s'.\n", step.ID, workflowName)
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

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		svc := workflow.NewAuthoringService(vaultPath, vaultCfg)

		wf, err := workflow.Get(vaultPath, workflowName, vaultCfg)
		if err != nil {
			return handleError(ErrWorkflowNotFound, err, "Run 'rvn workflow list' to see available workflows")
		}

		targetIdx := workflow.FindStepIndexInSteps(wf.Steps, stepID)
		if targetIdx < 0 {
			return handleErrorWithDetails(
				ErrRefNotFound,
				fmt.Sprintf("step '%s' not found", stepID),
				"Use 'rvn workflow show <name>' to inspect step ids",
				map[string]interface{}{"workflow_name": workflowName, "step_id": stepID},
			)
		}

		updatedStep, err := workflow.ApplyStepPatch(wf.Steps[targetIdx], workflowStepUpdateJSON)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}
		if updatedStep.ID == "" {
			updatedStep.ID = stepID
		}

		if updatedStep.ID != stepID {
			if idx := workflow.FindStepIndexInSteps(wf.Steps, updatedStep.ID); idx >= 0 {
				return handleErrorMsg(
					ErrDuplicateName,
					fmt.Sprintf("step id '%s' already exists", updatedStep.ID),
					"Use a unique step id",
				)
			}
		}

		result, err := svc.MutateStep(workflow.StepMutationRequest{
			WorkflowName: workflowName,
			Action:       workflow.StepMutationUpdate,
			TargetStepID: stepID,
			Step:         updatedStep,
		})
		if err != nil {
			return handleWorkflowDomainError(err, "Run 'rvn workflow list' to see available workflows")
		}

		payload := map[string]interface{}{
			"workflow_name": workflowName,
			"file":          result.FileRef,
			"action":        "update",
			"step_id":       result.StepID,
			"previous_id":   stepID,
			"step":          result.Step,
			"index":         result.Index,
		}
		if isJSONOutput() {
			outputSuccess(payload, nil)
			return nil
		}

		if updatedStep.ID == stepID {
			fmt.Printf("Updated step '%s' in workflow '%s'.\n", stepID, workflowName)
		} else {
			fmt.Printf("Updated step '%s' (renamed from '%s') in workflow '%s'.\n", updatedStep.ID, stepID, workflowName)
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

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
		result, err := svc.MutateStep(workflow.StepMutationRequest{
			WorkflowName: workflowName,
			Action:       workflow.StepMutationRemove,
			TargetStepID: stepID,
		})
		if err != nil {
			return handleWorkflowDomainError(err, "Run 'rvn workflow list' to see available workflows")
		}

		payload := map[string]interface{}{
			"workflow_name": workflowName,
			"file":          result.FileRef,
			"action":        "remove",
			"step_id":       result.StepID,
			"index":         result.Index,
		}
		if isJSONOutput() {
			outputSuccess(payload, nil)
			return nil
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

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		if strings.TrimSpace(workflowAddFile) == "" {
			return handleErrorMsg(ErrMissingArgument, "--file is required", "Use --file <workflow YAML path>")
		}

		svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
		result, err := svc.AddWorkflow(workflow.AddWorkflowRequest{
			Name: name,
			File: workflowAddFile,
		})
		if err != nil {
			if de, ok := workflow.AsDomainError(err); ok && de.Code == workflow.CodeDuplicateName {
				return handleWorkflowDomainError(err, fmt.Sprintf("Use 'rvn workflow remove %s' first to replace it", name))
			}
			return handleWorkflowDomainError(err, fmt.Sprintf("Use a file path like %s<name>.yaml", vaultCfg.GetWorkflowDirectory()))
		}

		if isJSONOutput() {
			out := map[string]interface{}{
				"name":        result.Workflow.Name,
				"description": result.Workflow.Description,
				"source":      result.Source,
				"file":        result.FileRef,
			}
			if len(result.Workflow.Inputs) > 0 {
				out["inputs"] = result.Workflow.Inputs
			}
			if len(result.Workflow.Steps) > 0 {
				out["steps"] = result.Workflow.Steps
			}
			outputSuccess(out, nil)
			return nil
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

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
		result, err := svc.ScaffoldWorkflow(workflow.ScaffoldWorkflowRequest{
			Name:        name,
			File:        workflowScaffoldFile,
			Description: workflowScaffoldDescription,
			Force:       workflowScaffoldForce,
		})
		if err != nil {
			if de, ok := workflow.AsDomainError(err); ok {
				if de.Code == workflow.CodeDuplicateName {
					return handleWorkflowDomainError(
						err,
						fmt.Sprintf("A scaffold file was written to %s. Remove the existing workflow first or use a different name.", workflow.ScaffoldErrorFileRef(vaultCfg, name, workflowScaffoldFile)),
					)
				}
				if de.Code == workflow.CodeFileExists {
					return handleWorkflowDomainError(err, "Use --force to overwrite, or choose a different --file path")
				}
			}
			return handleWorkflowDomainError(err, fmt.Sprintf("Use a file path like %s<name>.yaml", vaultCfg.GetWorkflowDirectory()))
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":        result.Workflow.Name,
				"description": result.Workflow.Description,
				"file":        result.FileRef,
				"source":      result.Source,
				"scaffolded":  result.Scaffolded,
			}, nil)
			return nil
		}

		fmt.Printf("Scaffolded workflow '%s' at %s\n", name, result.FileRef)
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

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
		result, err := svc.RemoveWorkflow(workflow.RemoveWorkflowRequest{Name: name})
		if err != nil {
			return handleWorkflowDomainError(err, "Run 'rvn workflow list' to see available workflows")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":    result.Name,
				"removed": result.Removed,
			}, nil)
			return nil
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

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		name := ""
		if len(args) == 1 {
			name = strings.TrimSpace(args[0])
		}

		svc := workflow.NewAuthoringService(vaultPath, vaultCfg)
		result, err := svc.ValidateWorkflows(workflow.ValidateWorkflowsRequest{Name: name})
		if err != nil {
			return handleWorkflowDomainError(err, "Run 'rvn workflow list' to see available workflows")
		}

		payload := map[string]interface{}{
			"valid":   result.Valid,
			"checked": result.Checked,
			"invalid": result.Invalid,
			"results": result.Results,
		}

		if result.Invalid > 0 {
			return handleErrorWithDetails(
				ErrWorkflowInvalid,
				fmt.Sprintf("%d workflow(s) invalid", result.Invalid),
				"Use 'rvn workflow show <name>' to inspect a workflow definition",
				payload,
			)
		}

		if isJSONOutput() {
			outputSuccess(payload, &Meta{Count: len(result.Results)})
			return nil
		}

		fmt.Printf("All %d workflow(s) are valid.\n", len(result.Results))
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

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		inputs, err := workflow.ParseInputs(workflowInputFile, workflowInputJSON, workflowInputFlags)
		if err != nil {
			return handleError(ErrWorkflowInputInvalid, err, "")
		}

		svc := workflow.NewRunService(vaultPath, vaultCfg, makeToolFunc(vaultPath))
		outcome, err := svc.Start(workflow.StartRunRequest{
			WorkflowName: name,
			Inputs:       inputs,
		})
		if err != nil {
			if de, ok := workflow.AsDomainError(err); ok {
				details := workflow.MergeDetails(de.Details, workflow.OutcomeErrorDetails(outcome, de.StepID))
				return handleErrorWithDetails(workflow.DomainCodeToErrorCode(de.Code), de.Error(), workflow.DomainCodeHint(de.Code), details)
			}
			return handleError(ErrInternal, err, "")
		}
		result := outcome.Result

		if isJSONOutput() {
			outputSuccess(result, nil)
			return nil
		}

		fmt.Printf("run_id: %s\n", result.RunID)
		fmt.Printf("status: %s\n", result.Status)
		fmt.Printf("revision: %d\n\n", result.Revision)

		if result.Next != nil {
			fmt.Println("=== AGENT ===")
			fmt.Println(result.Next.Prompt)
			fmt.Println()
			fmt.Println("=== OUTPUTS ===")
			outputJSON, _ := json.MarshalIndent(result.Next.Outputs, "", "  ")
			fmt.Println(string(outputJSON))
			fmt.Println()
			fmt.Println("=== STEP SUMMARIES ===")
			stepSummariesJSON, _ := json.MarshalIndent(result.StepSummaries, "", "  ")
			fmt.Println(string(stepSummariesJSON))
			fmt.Println()
			fmt.Printf("Use 'rvn workflow runs step %s <step-id>' to fetch a specific step output.\n", result.RunID)
			return nil
		}

		fmt.Println("Workflow completed (no agent steps).")
		fmt.Println()
		fmt.Println("=== STEP SUMMARIES ===")
		stepSummariesJSON, _ := json.MarshalIndent(result.StepSummaries, "", "  ")
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

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		outputEnv, err := workflow.ParseAgentOutputEnvelope(workflowContinueOutputFile, workflowContinueOutputJSON, workflowContinueOutput)
		if err != nil {
			return handleError(ErrWorkflowAgentOutputInvalid, err, "")
		}

		svc := workflow.NewRunService(vaultPath, vaultCfg, makeToolFunc(vaultPath))
		outcome, err := svc.Continue(workflow.ContinueRunRequest{
			RunID:            runID,
			ExpectedRevision: workflowContinueExpectedRevision,
			AgentOutput:      outputEnv,
		})
		if err != nil {
			if de, ok := workflow.AsDomainError(err); ok {
				details := workflow.MergeDetails(de.Details, workflow.OutcomeErrorDetails(outcome, de.StepID))
				return handleErrorWithDetails(workflow.DomainCodeToErrorCode(de.Code), de.Error(), workflow.DomainCodeHint(de.Code), details)
			}
			return handleError(ErrInternal, err, "")
		}
		result := outcome.Result

		if isJSONOutput() {
			outputSuccess(result, nil)
			return nil
		}

		fmt.Printf("run_id: %s\n", result.RunID)
		fmt.Printf("status: %s\n", result.Status)
		fmt.Printf("revision: %d\n\n", result.Revision)

		if result.Next != nil {
			fmt.Println("=== AGENT ===")
			fmt.Println(result.Next.Prompt)
			fmt.Println()
			fmt.Println("=== OUTPUTS ===")
			outputJSON, _ := json.MarshalIndent(result.Next.Outputs, "", "  ")
			fmt.Println(string(outputJSON))
			fmt.Println()
			fmt.Println("=== STEP SUMMARIES ===")
			stepSummariesJSON, _ := json.MarshalIndent(result.StepSummaries, "", "  ")
			fmt.Println(string(stepSummariesJSON))
			fmt.Println()
			fmt.Printf("Use 'rvn workflow runs step %s <step-id>' to fetch a specific step output.\n", result.RunID)
			return nil
		}

		fmt.Println("Workflow completed.")
		fmt.Println()
		fmt.Println("=== STEP SUMMARIES ===")
		stepSummariesJSON, _ := json.MarshalIndent(result.StepSummaries, "", "  ")
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
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		statuses, err := workflow.ParseRunStatusFilter(workflowRunsStatus)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}

		svc := workflow.NewRunService(vaultPath, vaultCfg, makeToolFunc(vaultPath))
		runs, runWarnings, err := svc.ListRuns(workflow.RunListFilter{
			Workflow: workflowRunsWorkflow,
			Statuses: statuses,
		})
		if err != nil {
			if de, ok := workflow.AsDomainError(err); ok {
				return handleError(workflow.DomainCodeToErrorCode(de.Code), de, workflow.DomainCodeHint(de.Code))
			}
			return handleError(ErrInternal, err, "")
		}

		if isJSONOutput() {
			wfCache := map[string]*workflow.Workflow{}
			wfMissing := map[string]bool{}
			outRuns := make([]map[string]interface{}, 0, len(runs))
			for _, run := range runs {
				item := map[string]interface{}{
					"version":       run.Version,
					"run_id":        run.RunID,
					"workflow_name": run.WorkflowName,
					"workflow_hash": run.WorkflowHash,
					"status":        run.Status,
					"cursor":        run.Cursor,
					"revision":      run.Revision,
					"created_at":    run.CreatedAt.Format(time.RFC3339),
					"updated_at":    run.UpdatedAt.Format(time.RFC3339),
					"history":       run.History,
					"failure":       run.Failure,
					"available_steps": func() []string {
						ids := make([]string, 0, len(run.Steps))
						for id := range run.Steps {
							ids = append(ids, id)
						}
						sort.Strings(ids)
						return ids
					}(),
				}
				if run.AwaitingStep != "" {
					item["awaiting_step_id"] = run.AwaitingStep
				}
				if run.CompletedAt != nil {
					item["completed_at"] = run.CompletedAt.Format(time.RFC3339)
				}
				if run.ExpiresAt != nil {
					item["expires_at"] = run.ExpiresAt.Format(time.RFC3339)
				}

				if !wfMissing[run.WorkflowName] {
					wf := wfCache[run.WorkflowName]
					if wf == nil {
						loaded, wfErr := workflow.Get(vaultPath, run.WorkflowName, vaultCfg)
						if wfErr != nil {
							wfMissing[run.WorkflowName] = true
						} else {
							wf = loaded
							wfCache[run.WorkflowName] = wf
						}
					}
					if wf != nil {
						item["step_summaries"] = workflow.BuildStepSummaries(wf, run)
					}
				}

				outRuns = append(outRuns, item)
			}

			data := map[string]interface{}{
				"runs": outRuns,
			}
			warnings := runStoreWarningsToCLIWarnings(runWarnings)
			if len(warnings) > 0 {
				outputSuccessWithWarnings(data, warnings, &Meta{Count: len(outRuns)})
			} else {
				outputSuccess(data, &Meta{Count: len(outRuns)})
			}
			return nil
		}

		for _, w := range runWarnings {
			fmt.Printf("Warning [%s]: %s (%s)\n", w.Code, w.Message, w.File)
		}

		if len(runs) == 0 {
			fmt.Println("No workflow runs.")
			return nil
		}
		for _, run := range runs {
			fmt.Printf("%s  %-18s %-14s rev=%d updated=%s\n",
				run.RunID,
				run.WorkflowName,
				run.Status,
				run.Revision,
				run.UpdatedAt.Format(time.RFC3339),
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

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		svc := workflow.NewRunService(vaultPath, vaultCfg, makeToolFunc(vaultPath))
		paginationRequested := cmd.Flags().Changed("path") || cmd.Flags().Changed("offset") || cmd.Flags().Changed("limit")
		stepResult, err := svc.StepOutput(workflow.StepOutputRequest{
			RunID:      runID,
			StepID:     stepID,
			Paginated:  paginationRequested,
			Path:       workflowRunsStepPath,
			Offset:     workflowRunsStepOffset,
			Limit:      workflowRunsStepLimit,
			IncludeSum: true,
		})
		if err != nil {
			if de, ok := workflow.AsDomainError(err); ok {
				hint := workflow.DomainCodeHint(de.Code)
				if de.Code == workflow.CodeInvalidInput {
					hint = "Use --path for nested fields and provide valid --offset/--limit values"
				}
				return handleErrorWithDetails(
					workflow.DomainCodeToErrorCode(de.Code),
					de.Error(),
					hint,
					de.Details,
				)
			}
			return handleError(ErrInternal, err, "")
		}
		state := stepResult.State

		payload := map[string]interface{}{
			"run_id":        state.RunID,
			"workflow_name": state.WorkflowName,
			"status":        state.Status,
			"revision":      state.Revision,
			"step_id":       stepID,
		}

		if paginationRequested {
			payload["step_output_page"] = stepResult.StepOutputPage
		} else {
			payload["step_output"] = stepResult.StepOutput
		}

		if len(stepResult.Summaries) > 0 {
			payload["step_summaries"] = stepResult.Summaries
		}

		if isJSONOutput() {
			outputSuccess(payload, nil)
			return nil
		}

		fmt.Printf("run_id: %s\n", state.RunID)
		fmt.Printf("workflow: %s\n", state.WorkflowName)
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
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		statuses, err := workflow.ParseRunStatusFilter(workflowRunsPruneStatus)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}
		olderThan, err := workflow.ParseOlderThan(workflowRunsPruneOlderThan)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}

		svc := workflow.NewRunService(vaultPath, vaultCfg, makeToolFunc(vaultPath))
		result, err := svc.PruneRuns(workflow.RunPruneOptions{
			Statuses:  statuses,
			OlderThan: olderThan,
			Apply:     workflowRunsPruneConfirm,
		})
		if err != nil {
			if de, ok := workflow.AsDomainError(err); ok {
				return handleError(workflow.DomainCodeToErrorCode(de.Code), de, workflow.DomainCodeHint(de.Code))
			}
			return handleError(ErrInternal, err, "")
		}

		if isJSONOutput() {
			data := map[string]interface{}{
				"dry_run": !workflowRunsPruneConfirm,
				"prune":   result,
			}
			warnings := runStoreWarningsToCLIWarnings(result.Warnings)
			if len(warnings) > 0 {
				outputSuccessWithWarnings(data, warnings, nil)
			} else {
				outputSuccess(data, nil)
			}
			return nil
		}

		for _, w := range result.Warnings {
			fmt.Printf("Warning [%s]: %s (%s)\n", w.Code, w.Message, w.File)
		}

		if workflowRunsPruneConfirm {
			fmt.Printf("Deleted %d runs (matched %d of %d scanned).\n", result.Deleted, result.Matched, result.Scanned)
		} else {
			fmt.Printf("Would delete %d runs (scanned %d). Use --confirm to apply.\n", result.Matched, result.Scanned)
		}
		return nil
	},
}

var workflowRunsCmd = &cobra.Command{
	Use:   "runs",
	Short: "Manage persisted workflow run records",
}

// makeToolFunc executes workflow tool steps through the same registry-driven
// CLI argument mapping used by MCP.
func makeToolFunc(vaultPath string) func(tool string, args map[string]interface{}) (interface{}, error) {
	executable, err := os.Executable()
	resolveErr := err
	if strings.TrimSpace(executable) == "" && resolveErr == nil {
		resolveErr = fmt.Errorf("os.Executable returned empty path")
	}

	return func(tool string, args map[string]interface{}) (interface{}, error) {
		if resolveErr != nil {
			return nil, fmt.Errorf("failed to resolve current executable path: %w", resolveErr)
		}
		envelope, err := toolexec.Execute(vaultPath, executable, tool, args)
		if err != nil {
			return nil, err
		}
		return envelope, nil
	}
}

func runStoreWarningsToCLIWarnings(runWarnings []workflow.RunStoreWarning) []Warning {
	if len(runWarnings) == 0 {
		return nil
	}
	warnings := make([]Warning, 0, len(runWarnings))
	for _, w := range runWarnings {
		warnings = append(warnings, Warning{
			Code:    w.Code,
			Message: w.Message,
			Ref:     w.File,
		})
	}
	return warnings
}

func handleWorkflowDomainError(err error, fallbackHint string) error {
	de, ok := workflow.AsDomainError(err)
	if !ok {
		return handleError(ErrInternal, err, "")
	}
	code := workflow.DomainCodeToErrorCode(de.Code)
	hint := workflow.DomainCodeHint(de.Code)
	if hint == "" {
		hint = fallbackHint
	}
	if len(de.Details) > 0 {
		return handleErrorWithDetails(code, de.Error(), hint, de.Details)
	}
	return handleError(code, de, hint)
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
