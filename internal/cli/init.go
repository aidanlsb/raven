package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
)

var initCmd = &cobra.Command{
	Use:   "init <path>",
	Short: "Initialize a new vault",
	Long: `Creates a new vault at the specified path with default configuration files.

Creates:
  - raven.yaml   (vault configuration)
  - schema.yaml  (types and traits)
  - .raven/      (index directory)
  - .gitignore   (ignores derived files)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]
		if !isJSONOutput() {
			fmt.Printf("Initializing vault at: %s\n", path)
		}

		result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
			CommandID: "init",
			Caller:    commandexec.CallerCLI,
			Args: map[string]interface{}{
				"path": path,
			},
		})
		if !result.OK {
			if isJSONOutput() {
				outputJSON(result)
				return nil
			}
			if result.Error != nil {
				return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
			}
			return handleErrorMsg(ErrInternal, "command execution failed", "")
		}

		if isJSONOutput() {
			outputJSON(result)
			return nil
		}

		data, _ := result.Data.(map[string]interface{})
		createdConfig, _ := data["created_config"].(bool)
		createdSchema, _ := data["created_schema"].(bool)
		gitignoreState, _ := data["gitignore_state"].(string)
		status, _ := data["status"].(string)
		docs, _ := data["docs"].(map[string]interface{})

		if createdConfig {
			fmt.Println("✓ Created raven.yaml (vault configuration)")
		} else {
			fmt.Println("• raven.yaml already exists (kept)")
		}

		if createdSchema {
			fmt.Println("✓ Created schema.yaml (types and traits)")
		} else {
			fmt.Println("• schema.yaml already exists (kept)")
		}

		fmt.Println("✓ Ensured .raven/ directory exists")

		switch gitignoreState {
		case "created":
			fmt.Println("✓ Created .gitignore")
		case "updated":
			fmt.Println("✓ Updated .gitignore (added Raven entries)")
		default:
			fmt.Println("• .gitignore already has Raven entries")
		}

		if len(result.Warnings) > 0 {
			for _, warning := range result.Warnings {
				fmt.Printf("! %s\n", warning.Message)
			}
		} else if fetched, _ := docs["fetched"].(bool); fetched {
			fmt.Printf("✓ Fetched docs into %s (%d files)\n", stringFromMap(docs, "store_path"), intFromMap(docs, "file_count"))
		}

		if status == "initialized" {
			fmt.Println("\nVault initialized! Start adding markdown files.")
		} else {
			fmt.Println("\nExisting vault detected. Configuration preserved.")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
