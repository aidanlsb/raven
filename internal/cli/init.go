package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/initsvc"
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

		info := currentVersionInfo()
		result, err := initsvc.Initialize(initsvc.InitializeRequest{
			Path:       path,
			CLIVersion: info.Version,
		})
		if err != nil {
			return mapInitSvcError(err)
		}

		if isJSONOutput() {
			data := map[string]interface{}{
				"path":            result.Path,
				"status":          result.Status,
				"created_config":  result.CreatedConfig,
				"created_schema":  result.CreatedSchema,
				"gitignore_state": result.GitignoreState,
				"docs":            result.Docs,
			}
			if len(result.Warnings) > 0 {
				warnings := make([]Warning, 0, len(result.Warnings))
				for _, warning := range result.Warnings {
					warnings = append(warnings, Warning{Code: warning.Code, Message: warning.Message})
				}
				outputSuccessWithWarnings(data, warnings, nil)
				return nil
			}
			outputSuccess(data, nil)
			return nil
		}

		if result.CreatedConfig {
			fmt.Println("✓ Created raven.yaml (vault configuration)")
		} else {
			fmt.Println("• raven.yaml already exists (kept)")
		}

		if result.CreatedSchema {
			fmt.Println("✓ Created schema.yaml (types and traits)")
		} else {
			fmt.Println("• schema.yaml already exists (kept)")
		}

		fmt.Println("✓ Ensured .raven/ directory exists")

		switch result.GitignoreState {
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
		} else if result.Docs.Fetched {
			fmt.Printf("✓ Fetched docs into %s (%d files)\n", result.Docs.StorePath, result.Docs.FileCount)
		}

		if result.Status == "initialized" {
			fmt.Println("\nVault initialized! Start adding markdown files.")
		} else {
			fmt.Println("\nExisting vault detected. Configuration preserved.")
		}

		return nil
	},
}

func mapInitSvcError(err error) error {
	svcErr, ok := initsvc.AsError(err)
	if !ok {
		return handleError(ErrInternal, err, "")
	}

	switch svcErr.Code {
	case initsvc.CodeInvalidInput:
		return handleErrorMsg(ErrInvalidInput, svcErr.Message, svcErr.Suggestion)
	case initsvc.CodeFileWriteError:
		return handleError(ErrFileWriteError, svcErr, svcErr.Suggestion)
	default:
		return handleError(ErrInternal, svcErr, svcErr.Suggestion)
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
}
