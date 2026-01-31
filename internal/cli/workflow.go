package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/workflow"
)

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage and run workflows",
	Long:  `Workflows are multi-step pipelines with deterministic Raven steps and agent prompt steps.`,
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
			// Prefer showing simplified workflow shape when present.
			if wf.Prompt != "" {
				if len(wf.Context) > 0 {
					out["context"] = wf.Context
				}
				out["prompt"] = wf.Prompt
				if len(wf.Outputs) > 0 {
					out["outputs"] = wf.Outputs
				}
			} else if len(wf.Steps) > 0 {
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

		if wf.Prompt != "" {
			if len(wf.Context) > 0 {
				fmt.Println("\ncontext:")
				keys := make([]string, 0, len(wf.Context))
				for k := range wf.Context {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					item := wf.Context[k]
					if item == nil {
						fmt.Printf("  %s: (nil)\n", k)
						continue
					}
					switch {
					case item.Query != "":
						fmt.Printf("  %s: query: %s\n", k, item.Query)
					case item.Read != "":
						fmt.Printf("  %s: read: %s\n", k, item.Read)
					case item.Backlinks != "":
						fmt.Printf("  %s: backlinks: %s\n", k, item.Backlinks)
					case item.Search != "":
						if item.Limit > 0 {
							fmt.Printf("  %s: search: %s (limit %d)\n", k, item.Search, item.Limit)
						} else {
							fmt.Printf("  %s: search: %s\n", k, item.Search)
						}
					default:
						fmt.Printf("  %s: (empty)\n", k)
					}
				}
			}

			if len(wf.Outputs) > 0 {
				fmt.Println("\noutputs:")
				keys := make([]string, 0, len(wf.Outputs))
				for k := range wf.Outputs {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					out := wf.Outputs[k]
					if out == nil {
						fmt.Printf("  %s: (nil)\n", k)
						continue
					}
					req := ""
					if out.Required {
						req = " (required)"
					}
					fmt.Printf("  %s: %s%s\n", k, out.Type, req)
				}
			}

			fmt.Println("\nprompt:")
			fmt.Println(wf.Prompt)
		} else if len(wf.Steps) > 0 {
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
				case "query":
					fmt.Printf("     rql: %s\n", step.RQL)
				case "read":
					fmt.Printf("     ref: %s\n", step.Ref)
				case "search":
					fmt.Printf("     term: %s\n", step.Term)
					if step.Limit > 0 {
						fmt.Printf("     limit: %d\n", step.Limit)
					}
				case "backlinks":
					fmt.Printf("     target: %s\n", step.Target)
				case "prompt":
					fmt.Printf("     outputs: %d\n", len(step.Outputs))
				}
			}
		}

		return nil
	},
}

var workflowInputFlags []string

var workflowRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Run a workflow until it reaches a prompt step",
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
		runner.ReadFunc = makeReadFunc(vaultPath, vaultCfg)
		runner.QueryFunc = makeQueryFunc(vaultPath)
		runner.BacklinksFunc = makeBacklinksFunc(vaultPath)
		runner.SearchFunc = makeSearchFunc(vaultPath)

		result, err := runner.Run(wf, inputs)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}

		if isJSONOutput() {
			outputSuccess(result, nil)
			return nil
		}

		if result.Next != nil {
			fmt.Println("=== PROMPT ===")
			fmt.Println(result.Next.Prompt)
			fmt.Println()
			fmt.Println("=== OUTPUTS ===")
			outputJSON, _ := json.MarshalIndent(result.Next.Outputs, "", "  ")
			fmt.Println(string(outputJSON))
			return nil
		}

		fmt.Println("Workflow completed (no prompt steps).")
		fmt.Println()
		fmt.Println("=== STEPS ===")
		stepsJSON, _ := json.MarshalIndent(result.Steps, "", "  ")
		fmt.Println(string(stepsJSON))
		return nil
	},
}

// makeReadFunc creates a function that reads a single object.
// This reads and parses the actual file to get full content.
func makeReadFunc(vaultPath string, vaultCfg *config.VaultConfig) func(id string) (interface{}, error) {
	return func(id string) (interface{}, error) {
		// Resolve reference to a vault-relative file path.
		//
		// Backwards compatibility: if the caller already provided an explicit
		// markdown file path (ending in .md), treat it as a relative path.
		// Otherwise, resolve via the vault's canonical reference rules.
		filePath := id
		if !strings.HasSuffix(filePath, ".md") {
			if vaultCfg != nil {
				filePath = vaultCfg.ResolveReferenceToFilePath(id)
			} else {
				filePath = id + ".md"
			}
		}

		fullPath := filepath.Join(vaultPath, filePath)

		// Security: verify path is within vault
		if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
			if errors.Is(err, paths.ErrPathOutsideVault) {
				return nil, fmt.Errorf("path '%s' is outside vault", id)
			}
			return nil, err
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
		doc, parseErr := parser.ParseDocument(string(content), filePath, vaultPath)
		if parseErr != nil {
			// Even on parse error, return raw content
			res := map[string]interface{}{
				"id":          id,
				"content":     string(content),
				"parse_error": parseErr.Error(),
			}
			return res, nil //nolint:nilerr
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
			return nil, fmt.Errorf("parse error: %w", err)
		}

		executor := query.NewExecutor(db.DB())
		if res, err := db.Resolver(index.ResolverOptions{DailyDirectory: dailyDir}); err == nil {
			executor.SetResolver(res)
		}

		if q.Type == query.QueryTypeObject {
			results, err := executor.ExecuteObjectQuery(q)
			if err != nil {
				return nil, err
			}
			// Convert to generic format
			items := make([]map[string]interface{}, 0, len(results))
			for _, r := range results {
				item := map[string]interface{}{
					"id":     r.ID,
					"type":   r.Type,
					"fields": r.Fields,
				}
				items = append(items, item)
			}
			return items, nil
		} else if q.Type == query.QueryTypeTrait {
			results, err := executor.ExecuteTraitQuery(q)
			if err != nil {
				return nil, err
			}
			// Convert to generic format
			items := make([]map[string]interface{}, 0, len(results))
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
		items := make([]map[string]interface{}, 0, len(backlinks))
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
		items := make([]map[string]interface{}, 0, len(results))
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
	workflowRunCmd.Flags().StringArrayVar(&workflowInputFlags, "input", nil, "Set input value (can be repeated): --input name=value")

	workflowCmd.AddCommand(workflowListCmd)
	workflowCmd.AddCommand(workflowShowCmd)
	workflowCmd.AddCommand(workflowRunCmd)
	rootCmd.AddCommand(workflowCmd)
}
