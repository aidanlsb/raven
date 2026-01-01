package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ravenscroftj/raven/internal/index"
	"github.com/spf13/cobra"
)

var traitCmd = &cobra.Command{
	Use:   "trait <name> [--field=value ...]",
	Short: "Query traits",
	Long:  `Query traits of a specific type with optional field filters.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		traitName := args[0]

		// Parse field filters from remaining args
		fieldFilters := make(map[string]string)
		for _, arg := range args[1:] {
			// Support both --field=value and field=value
			arg = strings.TrimPrefix(arg, "--")
			if idx := strings.Index(arg, "="); idx > 0 {
				key := arg[:idx]
				value := arg[idx+1:]
				fieldFilters[key] = value
			}
		}

		// Special handling for tasks (built-in alias)
		if traitName == "task" {
			return runTaskQuery(vaultPath, fieldFilters)
		}

		db, err := index.Open(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		results, err := db.QueryTraits(traitName, fieldFilters)
		if err != nil {
			return fmt.Errorf("failed to query traits: %w", err)
		}

		if len(results) == 0 {
			fmt.Printf("No '%s' traits found.\n", traitName)
			return nil
		}

		for _, result := range results {
			var fields map[string]interface{}
			json.Unmarshal([]byte(result.Fields), &fields)

			fmt.Printf("• %s\n", result.Content)
			fmt.Printf("  %s:%d\n", result.FilePath, result.Line)

			// Print key fields
			for k, v := range fields {
				fmt.Printf("  %s: %v\n", k, v)
			}
			fmt.Println()
		}

		return nil
	},
}

func runTaskQuery(vaultPath string, filters map[string]string) error {
	db, err := index.Open(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	var statusPtr *string
	if status, ok := filters["status"]; ok {
		statusPtr = &status
	}

	includeDone := false
	if _, ok := filters["all"]; ok {
		includeDone = true
	}

	tasks, err := db.QueryTasks(statusPtr, nil, includeDone)
	if err != nil {
		return fmt.Errorf("failed to query tasks: %w", err)
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks found.")
		return nil
	}

	for _, task := range tasks {
		var fields map[string]interface{}
		json.Unmarshal([]byte(task.Fields), &fields)

		status := getStringField(fields, "status", "todo")
		due := getStringField(fields, "due", "-")

		statusIcon := "○"
		switch status {
		case "todo":
			statusIcon = "○"
		case "in_progress":
			statusIcon = "◐"
		case "done":
			statusIcon = "●"
		}

		fmt.Printf("%s %s\n", statusIcon, task.Content)
		fmt.Printf("  due: %s | %s\n", due, task.FilePath)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(traitCmd)
}
