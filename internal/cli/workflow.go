package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/workflow"
)

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage and run workflows",
	Long:  `Workflows are reusable prompt templates for agents.`,
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
				if item.Inputs != nil && len(item.Inputs) > 0 {
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
			outputSuccess(map[string]interface{}{
				"name":        wf.Name,
				"description": wf.Description,
				"inputs":      wf.Inputs,
				"context":     formatContextQueries(wf.Context),
				"prompt":      wf.Prompt,
			}, nil)
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

		if len(wf.Context) > 0 {
			fmt.Println("\ncontext:")
			for name, query := range wf.Context {
				fmt.Printf("  %s: %s\n", name, describeContextQuery(query))
			}
		}

		return nil
	},
}

var workflowInputFlags []string

var workflowRenderCmd = &cobra.Command{
	Use:   "render <name>",
	Short: "Render a workflow with context",
	Long: `Renders a workflow and returns the prompt with pre-gathered context.

This command:
1. Loads the workflow definition
2. Validates inputs
3. Runs all context queries
4. Renders the template
5. Outputs the complete prompt with context`,
	Args: cobra.ExactArgs(1),
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

		// Parse --input flags
		inputs := make(map[string]string)
		for _, f := range workflowInputFlags {
			parts := strings.SplitN(f, "=", 2)
			if len(parts) == 2 {
				inputs[parts[0]] = parts[1]
			}
		}

		// Create renderer with query functions
		renderer := workflow.NewRenderer(vaultPath, vaultCfg)
		renderer.ReadFunc = makeReadFunc(vaultPath)
		renderer.QueryFunc = makeQueryFunc(vaultPath)
		renderer.BacklinksFunc = makeBacklinksFunc(vaultPath)
		renderer.SearchFunc = makeSearchFunc(vaultPath)

		// Render the workflow
		result, err := renderer.Render(wf, inputs)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}

		if isJSONOutput() {
			outputSuccess(result, nil)
			return nil
		}

		fmt.Println("=== PROMPT ===")
		fmt.Println(result.Prompt)
		fmt.Println()
		fmt.Println("=== CONTEXT ===")
		contextJSON, _ := json.MarshalIndent(result.Context, "", "  ")
		fmt.Println(string(contextJSON))

		return nil
	},
}

// formatContextQueries converts context queries to a JSON-friendly format.
func formatContextQueries(queries map[string]*config.ContextQuery) map[string]interface{} {
	if queries == nil {
		return nil
	}
	result := make(map[string]interface{})
	for name, q := range queries {
		result[name] = describeContextQuery(q)
	}
	return result
}

// describeContextQuery returns a string description of a context query.
func describeContextQuery(q *config.ContextQuery) string {
	if q.Read != "" {
		return fmt.Sprintf("read: %s", q.Read)
	}
	if q.Query != "" {
		return fmt.Sprintf("query: %s", q.Query)
	}
	if q.Backlinks != "" {
		return fmt.Sprintf("backlinks: %s", q.Backlinks)
	}
	if q.Search != "" {
		if q.Limit > 0 {
			return fmt.Sprintf("search: %s (limit: %d)", q.Search, q.Limit)
		}
		return fmt.Sprintf("search: %s", q.Search)
	}
	return "unknown"
}

// makeReadFunc creates a function that reads a single object.
// This reads and parses the actual file to get full content.
func makeReadFunc(vaultPath string) func(id string) (interface{}, error) {
	return func(id string) (interface{}, error) {
		// Build file path
		filePath := id
		if !strings.HasSuffix(filePath, ".md") {
			filePath = filePath + ".md"
		}
		fullPath := filepath.Join(vaultPath, filePath)

		// Security: verify path is within vault
		absVault, err := filepath.Abs(vaultPath)
		if err != nil {
			return nil, err
		}
		absFile, err := filepath.Abs(fullPath)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(absFile, absVault+string(filepath.Separator)) && absFile != absVault {
			return nil, fmt.Errorf("path '%s' is outside vault", id)
		}

		// Read and parse file
		content, err := os.ReadFile(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("object not found: %s", id)
			}
			return nil, err
		}

		// Parse the document to get metadata
		doc, err := parser.ParseDocument(string(content), filePath, vaultPath)
		if err != nil {
			// Even on parse error, return raw content
			return map[string]interface{}{
				"id":      id,
				"content": string(content),
			}, nil
		}

		// Build result with parsed info
		result := map[string]interface{}{
			"id":      id,
			"content": string(content),
		}

		// Add type from file-level object if present
		if len(doc.Objects) > 0 {
			result["type"] = doc.Objects[0].ObjectType
			result["fields"] = doc.Objects[0].Fields
		}

		return result, nil
	}
}

// makeQueryFunc creates a function that runs a Raven query.
func makeQueryFunc(vaultPath string) func(queryStr string) (interface{}, error) {
	return func(queryStr string) (interface{}, error) {
		db, _, err := index.OpenWithRebuild(vaultPath)
		if err != nil {
			return nil, err
		}
		defer db.Close()

		// Load vault config for canonical reference resolution settings.
		vaultCfg, _ := config.LoadVaultConfig(vaultPath)
		dailyDir := "daily"
		if vaultCfg != nil && vaultCfg.DailyDirectory != "" {
			dailyDir = vaultCfg.DailyDirectory
		}

		// Parse the query
		q, err := query.Parse(queryStr)
		if err != nil {
			return nil, fmt.Errorf("parse error: %v", err)
		}

		executor := query.NewExecutor(db.DB())
		if res, err := db.Resolver(dailyDir); err == nil {
			executor.SetResolver(res)
		}

		if q.Type == query.QueryTypeObject {
			results, err := executor.ExecuteObjectQuery(q)
			if err != nil {
				return nil, err
			}
			// Convert to generic format
			var items []map[string]interface{}
			for _, r := range results {
				items = append(items, map[string]interface{}{
					"id":     r.ID,
					"type":   r.Type,
					"fields": r.Fields,
				})
			}
			return items, nil
		} else if q.Type == query.QueryTypeTrait {
			results, err := executor.ExecuteTraitQuery(q)
			if err != nil {
				return nil, err
			}
			// Convert to generic format
			var items []map[string]interface{}
			for _, r := range results {
				item := map[string]interface{}{
					"id":        r.ID,
					"trait":     r.TraitType,
					"content":   r.Content,
					"file_path": r.FilePath,
					"line":      r.Line,
				}
				if r.Value != nil {
					item["value"] = *r.Value
				}
				items = append(items, item)
			}
			return items, nil
		}

		return nil, fmt.Errorf("unknown query type")
	}
}

// makeBacklinksFunc creates a function that gets backlinks.
func makeBacklinksFunc(vaultPath string) func(target string) (interface{}, error) {
	return func(target string) (interface{}, error) {
		db, _, err := index.OpenWithRebuild(vaultPath)
		if err != nil {
			return nil, err
		}
		defer db.Close()

		backlinks, err := db.Backlinks(target)
		if err != nil {
			return nil, err
		}

		// Convert to generic format
		var items []map[string]interface{}
		for _, link := range backlinks {
			item := map[string]interface{}{
				"source_id": link.SourceID,
				"file_path": link.FilePath,
			}
			if link.Line != nil {
				item["line"] = *link.Line
			}
			if link.DisplayText != nil {
				item["display_text"] = *link.DisplayText
			}
			items = append(items, item)
		}

		return items, nil
	}
}

// makeSearchFunc creates a function that performs full-text search.
func makeSearchFunc(vaultPath string) func(term string, limit int) (interface{}, error) {
	return func(term string, limit int) (interface{}, error) {
		db, _, err := index.OpenWithRebuild(vaultPath)
		if err != nil {
			return nil, err
		}
		defer db.Close()

		results, err := db.Search(term, limit)
		if err != nil {
			return nil, err
		}

		// Convert to generic format
		var items []map[string]interface{}
		for _, r := range results {
			items = append(items, map[string]interface{}{
				"object_id": r.ObjectID,
				"title":     r.Title,
				"file_path": r.FilePath,
				"snippet":   r.Snippet,
			})
		}

		return items, nil
	}
}

func init() {
	workflowRenderCmd.Flags().StringArrayVar(&workflowInputFlags, "input", nil, "Set input value (can be repeated): --input name=value")

	workflowCmd.AddCommand(workflowListCmd)
	workflowCmd.AddCommand(workflowShowCmd)
	workflowCmd.AddCommand(workflowRenderCmd)
	rootCmd.AddCommand(workflowCmd)
}
