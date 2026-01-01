package cli

import (
	"encoding/json"
	"fmt"

	"github.com/ravenscroftj/raven/internal/index"
	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/spf13/cobra"
)

var (
	tasksStatus string
	tasksDue    string
	tasksAll    bool
)

var tasksCmd = &cobra.Command{
	Use:   "tasks",
	Short: "List tasks (alias for 'rvn trait task')",
	Long: `Lists all tasks. This is a schema-defined alias for 'rvn trait task'.

The behavior of this command is controlled by the 'task' trait definition
in your schema.yaml. If no 'task' trait with cli.alias: tasks is defined,
this command will show an error.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		// Load schema to find the trait with alias "tasks"
		sch, err := schema.Load(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to load schema: %w", err)
		}

		// Find trait with cli.alias: tasks
		var traitName string
		var traitDef *schema.TraitDefinition
		for name, def := range sch.Traits {
			if def.CLI != nil && def.CLI.Alias == "tasks" {
				traitName = name
				traitDef = def
				break
			}
		}

		if traitName == "" {
			return fmt.Errorf(`no trait with 'cli.alias: tasks' found in schema.yaml

To enable the 'rvn tasks' command, add a trait with a CLI alias:

traits:
  task:
    fields:
      due:
        type: date
      status:
        type: enum
        values: [todo, in_progress, done]
        default: todo
    cli:
      alias: tasks
      default_query: "status:todo OR status:in_progress"`)
		}

		db, err := index.Open(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		// Build query from schema's default_query and CLI flags
		// The --all flag means skip the default status filter
		var statusFilter, dueFilter *string
		if tasksStatus != "" {
			statusFilter = &tasksStatus
		} else if !tasksAll && traitDef.CLI != nil && traitDef.CLI.DefaultQuery != "" {
			// Apply schema's default query behavior (filter to non-done)
			// Include NULL/empty status since schema default is typically 'todo'
			defaultStatus := "todo,in_progress,"
			statusFilter = &defaultStatus
		}
		if tasksDue != "" {
			dueFilter = &tasksDue
		}

		tasks, err := db.QueryTraitsByType(traitName, statusFilter, dueFilter)
		if err != nil {
			return fmt.Errorf("failed to query %s: %w", traitName, err)
		}

		if len(tasks) == 0 {
			fmt.Printf("No %s found.\n", traitName)
			return nil
		}

		// Display tasks using field names from schema
		for _, task := range tasks {
			var fields map[string]interface{}
			json.Unmarshal([]byte(task.Fields), &fields)

			status := getStringField(fields, "status", "")
			due := getStringField(fields, "due", "-")
			priority := getStringField(fields, "priority", "")

			// Status indicator
			statusIcon := "○"
			switch status {
			case "todo":
				statusIcon = "○"
			case "in_progress":
				statusIcon = "◐"
			case "done":
				statusIcon = "●"
			}

			// Priority coloring (if priority field exists in this trait)
			priorityColor := ""
			reset := ""
			if priority != "" {
				switch priority {
				case "high":
					priorityColor = "\033[31m" // red
					reset = "\033[0m"
				case "low":
					priorityColor = "\033[90m" // gray
					reset = "\033[0m"
				}
			}

			fmt.Printf("%s %s%s%s\n", statusIcon, priorityColor, task.Content, reset)
			if due != "-" {
				fmt.Printf("  due: %s | %s\n", due, task.FilePath)
			} else {
				fmt.Printf("  %s\n", task.FilePath)
			}
		}

		return nil
	},
}

func getStringField(fields map[string]interface{}, key, defaultVal string) string {
	if val, ok := fields[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return defaultVal
}

func init() {
	tasksCmd.Flags().StringVar(&tasksStatus, "status", "", "Filter by status")
	tasksCmd.Flags().StringVar(&tasksDue, "due", "", "Filter by due date")
	tasksCmd.Flags().BoolVar(&tasksAll, "all", false, "Show all (ignore default_query filter)")
	rootCmd.AddCommand(tasksCmd)
}
