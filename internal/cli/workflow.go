package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/mcp"
	"github.com/aidanlsb/raven/internal/paths"
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

func validateWorkflowCreateName(name string) error {
	if name == "runs" {
		return handleErrorMsg(
			ErrInvalidInput,
			"workflow name 'runs' is reserved for workflows.runs config",
			"Choose a different workflow name",
		)
	}
	return nil
}

func registerWorkflowInConfig(
	vaultPath string,
	vaultCfg *config.VaultConfig,
	name string,
	ref *config.WorkflowRef,
) (*workflow.Workflow, string, error) {
	if ref == nil {
		return nil, ErrWorkflowInvalid, fmt.Errorf("workflow reference is nil")
	}

	if vaultCfg == nil {
		loadedCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return nil, ErrInternal, err
		}
		vaultCfg = loadedCfg
	}
	if vaultCfg.Workflows == nil {
		vaultCfg.Workflows = make(map[string]*config.WorkflowRef)
	}
	if _, exists := vaultCfg.Workflows[name]; exists {
		return nil, ErrDuplicateName, fmt.Errorf("workflow '%s' already exists", name)
	}

	loaded, err := workflow.LoadWithConfig(vaultPath, name, ref, vaultCfg)
	if err != nil {
		return nil, ErrWorkflowInvalid, err
	}

	vaultCfg.Workflows[name] = ref
	if err := config.SaveVaultConfig(vaultPath, vaultCfg); err != nil {
		return nil, ErrInternal, err
	}

	return loaded, "", nil
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
		if err := validateWorkflowCreateName(name); err != nil {
			return err
		}

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		workflowDir := vaultCfg.GetWorkflowDirectory()

		rawFileRef := strings.TrimSpace(workflowAddFile)
		if rawFileRef == "" {
			return handleErrorMsg(ErrMissingArgument, "--file is required", "Use --file <workflow YAML path>")
		}
		fileRef, err := workflow.ResolveWorkflowFileRef(rawFileRef, workflowDir)
		if err != nil {
			return handleErrorMsg(
				ErrInvalidInput,
				err.Error(),
				fmt.Sprintf("Use a file path like %s<name>.yaml", workflowDir),
			)
		}

		ref := &config.WorkflowRef{
			File: fileRef,
		}
		loaded, errCode, err := registerWorkflowInConfig(vaultPath, vaultCfg, name, ref)
		if err != nil {
			if errCode == ErrDuplicateName {
				return handleErrorMsg(errCode, err.Error(), fmt.Sprintf("Use 'rvn workflow remove %s' first to replace it", name))
			}
			return handleError(errCode, err, "")
		}

		if isJSONOutput() {
			out := map[string]interface{}{
				"name":        loaded.Name,
				"description": loaded.Description,
				"source":      "file",
				"file":        ref.File,
			}
			if len(loaded.Inputs) > 0 {
				out["inputs"] = loaded.Inputs
			}
			if len(loaded.Steps) > 0 {
				out["steps"] = loaded.Steps
			}
			outputSuccess(out, nil)
			return nil
		}

		fmt.Printf("Added workflow '%s'.\n", name)
		fmt.Printf("Run with: rvn workflow run %s\n", name)
		return nil
	},
}

func buildWorkflowScaffoldYAML(name, description string) string {
	desc := strings.TrimSpace(description)
	if desc == "" {
		desc = fmt.Sprintf("Scaffolded workflow: %s", name)
	}

	return fmt.Sprintf(`description: %q
inputs:
  topic:
    type: string
    required: true
    description: "Question or topic to analyze"
steps:
  - id: context
    type: tool
    tool: raven_search
    arguments:
      query: "{{inputs.topic}}"
      limit: 10
  - id: compose
    type: agent
    outputs:
      markdown:
        type: markdown
        required: true
    prompt: |
      Return JSON: {"outputs":{"markdown":"..."}}

      Answer this request using my notes:
      {{inputs.topic}}

      ## Relevant context
      {{steps.context.data.results}}
`, desc)
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
		if err := validateWorkflowCreateName(name); err != nil {
			return err
		}

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		workflowDir := vaultCfg.GetWorkflowDirectory()

		fileRef := strings.TrimSpace(workflowScaffoldFile)
		if fileRef == "" {
			fileRef = fmt.Sprintf("%s%s.yaml", workflowDir, name)
		}
		fileRef, err = workflow.ResolveWorkflowFileRef(fileRef, workflowDir)
		if err != nil {
			return handleErrorMsg(
				ErrInvalidInput,
				err.Error(),
				fmt.Sprintf("Use a file path like %s<name>.yaml", workflowDir),
			)
		}

		fullPath := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
		if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
			return handleError(ErrFileOutsideVault, err, "Workflow files must be within the vault")
		}

		if _, err := os.Stat(fullPath); err == nil && !workflowScaffoldForce {
			return handleErrorMsg(
				ErrFileExists,
				fmt.Sprintf("workflow file already exists: %s", fileRef),
				"Use --force to overwrite, or choose a different --file path",
			)
		}

		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		content := buildWorkflowScaffoldYAML(name, workflowScaffoldDescription)
		if err := atomicfile.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		ref := &config.WorkflowRef{File: fileRef}
		loaded, errCode, err := registerWorkflowInConfig(vaultPath, vaultCfg, name, ref)
		if err != nil {
			if errCode == ErrDuplicateName {
				return handleErrorMsg(
					errCode,
					err.Error(),
					fmt.Sprintf("A scaffold file was written to %s. Remove the existing workflow first or use a different name.", fileRef),
				)
			}
			return handleError(errCode, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":        loaded.Name,
				"description": loaded.Description,
				"file":        fileRef,
				"source":      "file",
				"scaffolded":  true,
			}, nil)
			return nil
		}

		fmt.Printf("Scaffolded workflow '%s' at %s\n", name, fileRef)
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

		if vaultCfg.Workflows == nil {
			return handleErrorMsg(
				ErrWorkflowNotFound,
				fmt.Sprintf("workflow '%s' not found", name),
				"Run 'rvn workflow list' to see available workflows",
			)
		}
		if _, exists := vaultCfg.Workflows[name]; !exists {
			return handleErrorMsg(
				ErrWorkflowNotFound,
				fmt.Sprintf("workflow '%s' not found", name),
				"Run 'rvn workflow list' to see available workflows",
			)
		}

		delete(vaultCfg.Workflows, name)
		if len(vaultCfg.Workflows) == 0 {
			vaultCfg.Workflows = nil
		}
		if err := config.SaveVaultConfig(vaultPath, vaultCfg); err != nil {
			return handleError(ErrInternal, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"name":    name,
				"removed": true,
			}, nil)
			return nil
		}

		fmt.Printf("Removed workflow '%s'.\n", name)
		return nil
	},
}

type workflowValidationItem struct {
	Name  string `json:"name"`
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
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

		if vaultCfg.Workflows == nil || len(vaultCfg.Workflows) == 0 {
			if isJSONOutput() {
				outputSuccess(map[string]interface{}{
					"valid":   true,
					"checked": 0,
					"results": []workflowValidationItem{},
				}, &Meta{Count: 0})
				return nil
			}
			fmt.Println("No workflows defined in raven.yaml")
			return nil
		}

		var names []string
		if len(args) == 1 {
			name := strings.TrimSpace(args[0])
			if _, ok := vaultCfg.Workflows[name]; !ok {
				return handleErrorMsg(
					ErrWorkflowNotFound,
					fmt.Sprintf("workflow '%s' not found", name),
					"Run 'rvn workflow list' to see available workflows",
				)
			}
			names = []string{name}
		} else {
			names = make([]string, 0, len(vaultCfg.Workflows))
			for name := range vaultCfg.Workflows {
				names = append(names, name)
			}
			sort.Strings(names)
		}

		results := make([]workflowValidationItem, 0, len(names))
		invalidCount := 0
		for _, name := range names {
			_, loadErr := workflow.LoadWithConfig(vaultPath, name, vaultCfg.Workflows[name], vaultCfg)
			item := workflowValidationItem{
				Name:  name,
				Valid: loadErr == nil,
			}
			if loadErr != nil {
				item.Error = loadErr.Error()
				invalidCount++
			}
			results = append(results, item)
		}

		payload := map[string]interface{}{
			"valid":   invalidCount == 0,
			"checked": len(results),
			"invalid": invalidCount,
			"results": results,
		}

		if invalidCount > 0 {
			return handleErrorWithDetails(
				ErrWorkflowInvalid,
				fmt.Sprintf("%d workflow(s) invalid", invalidCount),
				"Use 'rvn workflow show <name>' to inspect a workflow definition",
				payload,
			)
		}

		if isJSONOutput() {
			outputSuccess(payload, &Meta{Count: len(results)})
			return nil
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

		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		wf, err := workflow.Get(vaultPath, name, vaultCfg)
		if err != nil {
			return handleError(ErrWorkflowNotFound, err, "Use 'rvn workflow list' to see available workflows")
		}

		inputs, err := parseWorkflowInputs(workflowInputFile, workflowInputJSON, workflowInputFlags)
		if err != nil {
			return handleError(ErrWorkflowInputInvalid, err, "")
		}

		runner := workflow.NewRunner(vaultPath, vaultCfg)
		runner.ToolFunc = makeToolFunc(vaultPath)

		runCfg := vaultCfg.GetWorkflowRunsConfig()
		_, _ = workflow.AutoPruneRunStates(vaultPath, runCfg)

		state, err := workflow.NewRunState(wf, inputs)
		if err != nil {
			return handleError(ErrWorkflowInvalid, err, "")
		}

		result, err := runner.RunWithState(wf, state)
		if err != nil {
			errCode, stepID := classifyRunnerError(err)
			markRunFailed(state, errCode, stepID, err)
			workflow.ApplyRetentionExpiry(state, runCfg, time.Now().UTC())
			_ = workflow.SaveRunState(vaultPath, runCfg, state)
			return handleErrorWithDetails(errCode, err.Error(), "", runStateErrorDetails(wf, state, stepID))
		}

		workflow.ApplyRetentionExpiry(state, runCfg, time.Now().UTC())
		if err := workflow.SaveRunState(vaultPath, runCfg, state); err != nil {
			return handleError(ErrInternal, err, "")
		}

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
		runCfg := vaultCfg.GetWorkflowRunsConfig()

		_, _ = workflow.AutoPruneRunStates(vaultPath, runCfg)

		state, err := workflow.LoadRunState(vaultPath, runCfg, runID)
		if err != nil {
			code := ErrWorkflowRunNotFound
			if strings.Contains(err.Error(), "parse run state") {
				code = ErrWorkflowStateCorrupt
			}
			return handleError(code, err, "")
		}

		if workflowContinueExpectedRevision > 0 && state.Revision != workflowContinueExpectedRevision {
			return handleErrorWithDetails(
				ErrWorkflowConflict,
				fmt.Sprintf("revision mismatch: expected %d, got %d", workflowContinueExpectedRevision, state.Revision),
				"Fetch latest run state and retry",
				map[string]interface{}{
					"run_id":            state.RunID,
					"workflow_name":     state.WorkflowName,
					"expected_revision": workflowContinueExpectedRevision,
					"revision":          state.Revision,
				},
			)
		}

		wf, err := workflow.Get(vaultPath, state.WorkflowName, vaultCfg)
		if err != nil {
			return handleError(ErrWorkflowNotFound, err, "")
		}

		currentHash, err := workflow.WorkflowHash(wf)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		if state.WorkflowHash != "" && currentHash != state.WorkflowHash {
			return handleErrorMsg(
				ErrWorkflowChanged,
				"workflow definition changed since run started",
				"Start a new run to use the latest workflow definition",
			)
		}

		outputEnv, err := parseAgentOutputEnvelope(workflowContinueOutputFile, workflowContinueOutputJSON)
		if err != nil {
			return handleError(ErrWorkflowAgentOutputInvalid, err, "")
		}

		if err := workflow.ApplyAgentOutputs(wf, state, outputEnv); err != nil {
			code := classifyContinueValidationError(state, err)
			return handleErrorWithDetails(code, err.Error(), "", runStateErrorDetails(wf, state, ""))
		}

		state.Revision++
		runner := workflow.NewRunner(vaultPath, vaultCfg)
		runner.ToolFunc = makeToolFunc(vaultPath)

		result, err := runner.RunWithState(wf, state)
		if err != nil {
			errCode, stepID := classifyRunnerError(err)
			markRunFailed(state, errCode, stepID, err)
			state.Revision++
			workflow.ApplyRetentionExpiry(state, runCfg, time.Now().UTC())
			_ = workflow.SaveRunState(vaultPath, runCfg, state)
			return handleErrorWithDetails(errCode, err.Error(), "", runStateErrorDetails(wf, state, stepID))
		}

		workflow.ApplyRetentionExpiry(state, runCfg, time.Now().UTC())
		if err := workflow.SaveRunState(vaultPath, runCfg, state); err != nil {
			return handleError(ErrInternal, err, "")
		}

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

var workflowRunsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List persisted workflow runs",
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		statuses, err := parseRunStatusFilter(workflowRunsStatus)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}

		runs, err := workflow.ListRunStates(vaultPath, vaultCfg.GetWorkflowRunsConfig(), workflow.RunListFilter{
			Workflow: workflowRunsWorkflow,
			Statuses: statuses,
		})
		if err != nil {
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

			outputSuccess(map[string]interface{}{
				"runs": outRuns,
			}, &Meta{Count: len(outRuns)})
			return nil
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
		runCfg := vaultCfg.GetWorkflowRunsConfig()

		state, err := workflow.LoadRunState(vaultPath, runCfg, runID)
		if err != nil {
			code := ErrWorkflowRunNotFound
			if strings.Contains(err.Error(), "parse run state") {
				code = ErrWorkflowStateCorrupt
			}
			return handleError(code, err, "")
		}

		stepOutput, ok := state.Steps[stepID]
		if !ok {
			available := make([]string, 0, len(state.Steps))
			for id := range state.Steps {
				available = append(available, id)
			}
			sort.Strings(available)
			return handleErrorWithDetails(
				ErrRefNotFound,
				fmt.Sprintf("step '%s' not found in run '%s'", stepID, runID),
				"Use one of the available step IDs",
				map[string]interface{}{
					"run_id":          state.RunID,
					"workflow_name":   state.WorkflowName,
					"available_steps": available,
				},
			)
		}

		var summaries []workflow.RunStepSummary
		if wf, wfErr := workflow.Get(vaultPath, state.WorkflowName, vaultCfg); wfErr == nil {
			summaries = workflow.BuildStepSummaries(wf, state)
		}

		payload := map[string]interface{}{
			"run_id":        state.RunID,
			"workflow_name": state.WorkflowName,
			"status":        state.Status,
			"revision":      state.Revision,
			"step_id":       stepID,
			"step_output":   stepOutput,
		}
		if len(summaries) > 0 {
			payload["step_summaries"] = summaries
		}

		if isJSONOutput() {
			outputSuccess(payload, nil)
			return nil
		}

		fmt.Printf("run_id: %s\n", state.RunID)
		fmt.Printf("workflow: %s\n", state.WorkflowName)
		fmt.Printf("step_id: %s\n\n", stepID)
		stepJSON, _ := json.MarshalIndent(stepOutput, "", "  ")
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

		statuses, err := parseRunStatusFilter(workflowRunsPruneStatus)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}
		olderThan, err := parseOlderThan(workflowRunsPruneOlderThan)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}

		result, err := workflow.PruneRunStates(vaultPath, vaultCfg.GetWorkflowRunsConfig(), workflow.RunPruneOptions{
			Statuses:  statuses,
			OlderThan: olderThan,
			Apply:     workflowRunsPruneConfirm,
		})
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"dry_run": !workflowRunsPruneConfirm,
				"prune":   result,
			}, nil)
			return nil
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
	return func(tool string, args map[string]interface{}) (interface{}, error) {
		cliArgs := mcp.BuildCLIArgs(tool, args)
		if len(cliArgs) == 0 {
			return nil, fmt.Errorf("unknown tool: %s", tool)
		}

		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve executable: %w", err)
		}

		cmdArgs := append([]string{"--vault-path", vaultPath}, cliArgs...)
		cmd := exec.Command(exe, cmdArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			trimmed := strings.TrimSpace(string(output))
			var env map[string]interface{}
			if json.Unmarshal(output, &env) == nil {
				return nil, fmt.Errorf("tool '%s' failed: %s", tool, trimmed)
			}
			return nil, fmt.Errorf("tool '%s' execution error: %w (%s)", tool, err, trimmed)
		}

		var env map[string]interface{}
		if err := json.Unmarshal(output, &env); err != nil {
			return nil, fmt.Errorf("tool '%s' returned non-JSON output: %w", tool, err)
		}

		if okVal, exists := env["ok"].(bool); exists && !okVal {
			b, _ := json.Marshal(env)
			return nil, fmt.Errorf("tool '%s' returned error: %s", tool, string(b))
		}

		return env, nil
	}
}

func parseWorkflowInputs(inputFile, inputJSON string, kvFlags []string) (map[string]interface{}, error) {
	inputs := map[string]interface{}{}

	if inputFile != "" {
		obj, err := readJSONFileObject(inputFile)
		if err != nil {
			return nil, fmt.Errorf("parse --input-file: %w", err)
		}
		mergeObject(inputs, obj)
	}
	if strings.TrimSpace(inputJSON) != "" {
		obj, err := parseJSONObject(inputJSON)
		if err != nil {
			return nil, fmt.Errorf("parse --input-json: %w", err)
		}
		mergeObject(inputs, obj)
	}
	for _, f := range kvFlags {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --input value: %s (expected key=value)", f)
		}
		k := strings.TrimSpace(parts[0])
		if k == "" {
			return nil, fmt.Errorf("input key cannot be empty")
		}
		inputs[k] = parts[1]
	}
	return inputs, nil
}

func parseAgentOutputEnvelope(outputFile, outputJSON string) (workflow.AgentOutputEnvelope, error) {
	if outputFile == "" && strings.TrimSpace(outputJSON) == "" {
		return workflow.AgentOutputEnvelope{}, fmt.Errorf("provide --agent-output-json or --agent-output-file")
	}

	var obj map[string]interface{}
	var err error
	if outputFile != "" {
		obj, err = readJSONFileObject(outputFile)
		if err != nil {
			return workflow.AgentOutputEnvelope{}, err
		}
	}
	if strings.TrimSpace(outputJSON) != "" {
		obj, err = parseJSONObject(outputJSON)
		if err != nil {
			return workflow.AgentOutputEnvelope{}, err
		}
	}

	rawOutputs, ok := obj["outputs"]
	if !ok {
		return workflow.AgentOutputEnvelope{}, fmt.Errorf("agent output must contain an 'outputs' key")
	}
	outputs, ok := rawOutputs.(map[string]interface{})
	if !ok {
		return workflow.AgentOutputEnvelope{}, fmt.Errorf("'outputs' must be an object")
	}
	return workflow.AgentOutputEnvelope{Outputs: outputs}, nil
}

func parseJSONObject(raw string) (map[string]interface{}, error) {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return obj, nil
}

func readJSONFileObject(path string) (map[string]interface{}, error) {
	absPath := path
	if !filepath.IsAbs(path) {
		absPath = filepath.Clean(path)
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(content, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return obj, nil
}

func mergeObject(dst, src map[string]interface{}) {
	for k, v := range src {
		dst[k] = v
	}
}

func parseRunStatusFilter(raw string) (map[workflow.RunStatus]bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	statuses := map[workflow.RunStatus]bool{}
	for _, part := range strings.Split(raw, ",") {
		s := workflow.RunStatus(strings.TrimSpace(part))
		switch s {
		case workflow.RunStatusRunning, workflow.RunStatusAwaitingAgent, workflow.RunStatusCompleted, workflow.RunStatusFailed, workflow.RunStatusCancelled:
			statuses[s] = true
		default:
			return nil, fmt.Errorf("unknown status: %s", part)
		}
	}
	return statuses, nil
}

func parseOlderThan(raw string) (*time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasSuffix(raw, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		if err != nil {
			return nil, fmt.Errorf("invalid days duration: %s", raw)
		}
		dur := time.Duration(days) * 24 * time.Hour
		return &dur, nil
	}
	dur, err := time.ParseDuration(raw)
	if err != nil {
		return nil, err
	}
	return &dur, nil
}

func markRunFailed(state *workflow.WorkflowRunState, code, stepID string, runErr error) {
	if state == nil {
		return
	}
	now := time.Now().UTC()
	state.Status = workflow.RunStatusFailed
	state.Failure = &workflow.RunFailure{
		Code:    code,
		Message: runErr.Error(),
		StepID:  stepID,
		At:      now,
	}
	state.CompletedAt = &now
	state.UpdatedAt = now
	state.AwaitingStep = ""
}

func classifyRunnerError(err error) (code string, stepID string) {
	if err == nil {
		return ErrWorkflowInvalid, ""
	}
	msg := err.Error()
	stepID = extractStepID(msg)
	switch {
	case strings.Contains(msg, "unknown variable:"),
		strings.Contains(msg, "invalid inputs reference"):
		return ErrWorkflowInterpolationError, stepID
	case strings.Contains(msg, "tool '"),
		strings.Contains(msg, "tool function not configured"):
		return ErrWorkflowToolExecutionFailed, stepID
	case strings.Contains(msg, "missing required inputs"),
		strings.Contains(msg, "unknown workflow input"),
		strings.Contains(msg, "workflow input '"):
		return ErrWorkflowInputInvalid, stepID
	default:
		return ErrWorkflowInvalid, stepID
	}
}

func classifyContinueValidationError(state *workflow.WorkflowRunState, err error) string {
	if state != nil {
		switch state.Status {
		case workflow.RunStatusCompleted, workflow.RunStatusFailed, workflow.RunStatusCancelled:
			return ErrWorkflowTerminalState
		}
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if strings.Contains(msg, "not awaiting agent output") {
		return ErrWorkflowNotAwaitingAgent
	}
	return ErrWorkflowAgentOutputInvalid
}

func extractStepID(msg string) string {
	const marker = "step '"
	start := strings.Index(msg, marker)
	if start < 0 {
		return ""
	}
	rest := msg[start+len(marker):]
	end := strings.Index(rest, "'")
	if end <= 0 {
		return ""
	}
	return rest[:end]
}

func runStateErrorDetails(wf *workflow.Workflow, state *workflow.WorkflowRunState, failedStepID string) map[string]interface{} {
	if state == nil {
		return nil
	}
	available := make([]string, 0, len(state.Steps))
	for id := range state.Steps {
		available = append(available, id)
	}
	sort.Strings(available)

	details := map[string]interface{}{
		"run_id":          state.RunID,
		"workflow_name":   state.WorkflowName,
		"status":          state.Status,
		"revision":        state.Revision,
		"cursor":          state.Cursor,
		"available_steps": available,
		"updated_at":      state.UpdatedAt.Format(time.RFC3339),
	}
	if wf != nil {
		details["step_summaries"] = workflow.BuildStepSummaries(wf, state)
	}
	if failedStepID != "" {
		details["failed_step_id"] = failedStepID
	}
	if state.Failure != nil {
		details["failure"] = state.Failure
	}
	return details
}

func init() {
	workflowAddCmd.Flags().StringVar(&workflowAddFile, "file", "", "Path to external workflow YAML file (relative to vault root)")
	workflowScaffoldCmd.Flags().StringVar(&workflowScaffoldFile, "file", "", "Path for the scaffolded workflow YAML file (relative to vault root)")
	workflowScaffoldCmd.Flags().StringVar(&workflowScaffoldDescription, "description", "", "Description for the scaffolded workflow")
	workflowScaffoldCmd.Flags().BoolVar(&workflowScaffoldForce, "force", false, "Overwrite scaffold file if it already exists")

	workflowRunCmd.Flags().StringArrayVar(&workflowInputFlags, "input", nil, "Set input value (can be repeated): --input name=value")
	workflowRunCmd.Flags().StringVar(&workflowInputJSON, "input-json", "", "Set workflow inputs as a JSON object")
	workflowRunCmd.Flags().StringVar(&workflowInputFile, "input-file", "", "Read workflow inputs from JSON file")

	workflowContinueCmd.Flags().StringVar(&workflowContinueOutputJSON, "agent-output-json", "", "Agent output JSON object")
	workflowContinueCmd.Flags().StringVar(&workflowContinueOutputFile, "agent-output-file", "", "Path to JSON file containing agent output")
	workflowContinueCmd.Flags().IntVar(&workflowContinueExpectedRevision, "expected-revision", 0, "Expected run revision for optimistic concurrency")

	workflowRunsListCmd.Flags().StringVar(&workflowRunsStatus, "status", "", "Filter by status (comma-separated)")
	workflowRunsListCmd.Flags().StringVar(&workflowRunsWorkflow, "workflow", "", "Filter by workflow name")

	workflowRunsPruneCmd.Flags().StringVar(&workflowRunsPruneStatus, "status", "", "Prune only statuses (comma-separated)")
	workflowRunsPruneCmd.Flags().StringVar(&workflowRunsPruneOlderThan, "older-than", "", "Prune records older than duration (e.g. 72h, 14d)")
	workflowRunsPruneCmd.Flags().BoolVar(&workflowRunsPruneConfirm, "confirm", false, "Apply deletion (without this flag, preview only)")

	workflowCmd.AddCommand(workflowListCmd)
	workflowCmd.AddCommand(workflowAddCmd)
	workflowCmd.AddCommand(workflowScaffoldCmd)
	workflowCmd.AddCommand(workflowRemoveCmd)
	workflowCmd.AddCommand(workflowValidateCmd)
	workflowCmd.AddCommand(workflowShowCmd)
	workflowCmd.AddCommand(workflowRunCmd)
	workflowCmd.AddCommand(workflowContinueCmd)
	workflowRunsCmd.AddCommand(workflowRunsListCmd)
	workflowRunsCmd.AddCommand(workflowRunsStepCmd)
	workflowRunsCmd.AddCommand(workflowRunsPruneCmd)
	workflowCmd.AddCommand(workflowRunsCmd)
	rootCmd.AddCommand(workflowCmd)
}
