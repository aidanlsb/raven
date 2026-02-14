package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/mcp"
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

var workflowInputFlags []string

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
			return handleError(ErrQueryNotFound, err, "Use 'rvn workflow list' to see available workflows")
		}

		inputs := make(map[string]string)
		for _, f := range workflowInputFlags {
			parts := strings.SplitN(f, "=", 2)
			if len(parts) == 2 {
				inputs[parts[0]] = parts[1]
			}
		}

		runner := workflow.NewRunner(vaultPath, vaultCfg)
		runner.ToolFunc = makeToolFunc(vaultPath)

		result, err := runner.Run(wf, inputs)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}

		if isJSONOutput() {
			outputSuccess(result, nil)
			return nil
		}

		if result.Next != nil {
			fmt.Println("=== AGENT ===")
			fmt.Println(result.Next.Prompt)
			fmt.Println()
			fmt.Println("=== OUTPUTS ===")
			outputJSON, _ := json.MarshalIndent(result.Next.Outputs, "", "  ")
			fmt.Println(string(outputJSON))
			return nil
		}

		fmt.Println("Workflow completed (no agent steps).")
		fmt.Println()
		fmt.Println("=== STEPS ===")
		stepsJSON, _ := json.MarshalIndent(result.Steps, "", "  ")
		fmt.Println(string(stepsJSON))
		return nil
	},
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

func init() {
	workflowRunCmd.Flags().StringArrayVar(&workflowInputFlags, "input", nil, "Set input value (can be repeated): --input name=value")

	workflowCmd.AddCommand(workflowListCmd)
	workflowCmd.AddCommand(workflowShowCmd)
	workflowCmd.AddCommand(workflowRunCmd)
	rootCmd.AddCommand(workflowCmd)
}
