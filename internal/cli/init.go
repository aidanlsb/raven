package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
)

var initCmd = newCanonicalLeafCommand("init", canonicalLeafOptions{
	Args:         cobra.ExactArgs(1),
	Prepare:      prepareInitArgs,
	HandleResult: handleInitResult,
})

func prepareInitArgs(_ *cobra.Command, args []string) ([]string, bool, error) {
	if !isJSONOutput() {
		fmt.Printf("Initializing vault at: %s\n", args[0])
	}
	return args, false, nil
}

func handleInitResult(_ *cobra.Command, result commandexec.Result) error {
	if isJSONOutput() {
		outputJSON(result)
		return nil
	}

	data := canonicalDataMap(result)
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
}

func init() {
	rootCmd.AddCommand(initCmd)
}
