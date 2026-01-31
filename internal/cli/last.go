// Package cli implements the command-line interface.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/lastquery"
	"github.com/aidanlsb/raven/internal/lastresults"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/schema"
)

var lastCmd = &cobra.Command{
	Use:   "last [numbers...]",
	Short: "Show or select results from the last retrieval",
	Long: `Show or select results from the most recent retrieval (query, search, backlinks, outlinks).

Without arguments, displays all results from the last retrieval with their numbers.
With number arguments, outputs the selected IDs for piping to other commands.

Number formats:
  1         Single result
  1,3,5     Multiple results (comma-separated)
  1-5       Range of results
  1,3-5,7   Mixed format

Examples:
  rvn last                              # Show all results from last query
  rvn last 1,3                          # Output IDs for results 1 and 3
  rvn last 1-5 | rvn update --stdin value=done   # Pipe to update command
  rvn last | fzf --multi | rvn update --stdin value=done  # Interactive selection

With --apply, applies an operation directly to selected query results:
  rvn last 1,3 --apply "update value=done"
  rvn last 1-5 --apply "set status=archived"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		// Handle --pipe/--no-pipe flags
		if pipeFlag, _ := cmd.Flags().GetBool("pipe"); pipeFlag {
			t := true
			SetPipeFormat(&t)
		} else if noPipeFlag, _ := cmd.Flags().GetBool("no-pipe"); noPipeFlag {
			f := false
			SetPipeFormat(&f)
		}

		// Load the last results
		lr, err := lastresults.Read(vaultPath)
		if err != nil {
			if err == lastresults.ErrNoLastResults {
				return handleErrorMsg(ErrMissingArgument,
					"no results available",
					"Run a query, search, or backlinks command first, then use 'rvn last'")
			}
			return handleError(ErrInternal, err, "")
		}

		// Check for --apply flag
		applyStr, _ := cmd.Flags().GetString("apply")
		confirmApply, _ := cmd.Flags().GetBool("confirm")

		// If no number args, display all results
		if len(args) == 0 {
			return displayLastResults(lr, applyStr, confirmApply, vaultPath)
		}

		// Parse the number arguments
		nums, err := lastquery.ParseNumberArgs(args)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(),
				fmt.Sprintf("Valid range: 1-%d", len(lr.Results)))
		}

		// Get the selected entries
		entries, err := lr.GetByNumbers(nums)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(),
				fmt.Sprintf("Last results returned %d results", len(lr.Results)))
		}

		// If --apply is set, apply the operation
		if applyStr != "" {
			return applyFromLastResults(vaultPath, lr, entries, applyStr, confirmApply)
		}

		// Output selected IDs (for piping)
		if isJSONOutput() {
			items := make([]map[string]interface{}, len(entries))
			for i, e := range entries {
				items[i] = map[string]interface{}{
					"num":      nums[i],
					"id":       e.GetID(),
					"kind":     e.GetKind(),
					"content":  resultContentForOutput(e),
					"location": e.GetLocation(),
				}
			}
			outputSuccess(map[string]interface{}{
				"selected": items,
			}, &Meta{Count: len(items)})
			return nil
		}

		// Plain output: one ID per line for piping
		for _, e := range entries {
			fmt.Println(e.GetID())
		}
		return nil
	},
}

// displayLastResults shows the last results in the appropriate format.
func displayLastResults(lr *lastresults.LastResults, applyStr string, confirm bool, vaultPath string) error {
	// If --apply is set without numbers, apply to all
	if applyStr != "" {
		results, err := lr.DecodeAll()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		return applyFromLastResults(vaultPath, lr, results, applyStr, confirm)
	}

	if isJSONOutput() {
		results, err := lr.DecodeAll()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		items := make([]map[string]interface{}, len(results))
		for i, r := range results {
			items[i] = map[string]interface{}{
				"num":      i + 1,
				"id":       r.GetID(),
				"kind":     r.GetKind(),
				"content":  resultContentForOutput(r),
				"location": r.GetLocation(),
			}
		}
		outputSuccess(map[string]interface{}{
			"source":    lr.Source,
			"query":     lr.Query,
			"target":    lr.Target,
			"timestamp": lr.Timestamp,
			"items":     items,
		}, &Meta{Count: len(items)})
		return nil
	}

	if ShouldUsePipeFormat() {
		return writeLastResultsPipe(lr)
	}

	return renderLastResultsHuman(lr, vaultPath)
}

func renderLastResultsHuman(lr *lastresults.LastResults, vaultPath string) error {
	switch lr.Source {
	case lastresults.SourceQuery:
		return renderLastQueryResults(lr, vaultPath)
	case lastresults.SourceSearch:
		results, err := lr.DecodeSearchMatches()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		printSearchResults(lr.Query, results)
		return nil
	case lastresults.SourceBacklinks:
		results, err := lr.DecodeReferences()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		printBacklinksResults(lr.Target, results)
		return nil
	case lastresults.SourceOutlinks:
		results, err := lr.DecodeReferences()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		printOutlinksResults(lr.Target, results)
		return nil
	default:
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown result source: %s", lr.Source), "")
	}
}

func renderLastQueryResults(lr *lastresults.LastResults, vaultPath string) error {
	kind := ""
	typeName := ""

	if len(lr.Results) > 0 {
		kind = lr.Results[0].Kind
	}

	if lr.Query != "" {
		parsedKind, parsedName, err := queryTypeFromString(lr.Query)
		if err == nil {
			if kind == "" {
				kind = parsedKind
			}
			if typeName == "" {
				typeName = parsedName
			}
		}
	}

	switch kind {
	case "trait":
		traits, err := lr.DecodeTraits()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		if typeName == "" && len(traits) > 0 {
			typeName = traits[0].TraitType
		}
		if typeName == "" {
			typeName = "trait"
		}
		printQueryTraitResults(lr.Query, typeName, traits)
		return nil
	case "object":
		objects, err := lr.DecodeObjects()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		if typeName == "" && len(objects) > 0 {
			typeName = objects[0].Type
		}
		if typeName == "" {
			typeName = "object"
		}
		var sch *schema.Schema
		if loaded, err := schema.Load(vaultPath); err == nil {
			sch = loaded
		}
		printQueryObjectResults(lr.Query, typeName, objects, sch)
		return nil
	default:
		return handleErrorMsg(ErrInvalidInput, "last results are not from a query", "")
	}
}

func writeLastResultsPipe(lr *lastresults.LastResults) error {
	switch lr.Source {
	case lastresults.SourceQuery:
		kind := ""
		if len(lr.Results) > 0 {
			kind = lr.Results[0].Kind
		}
		if kind == "" && lr.Query != "" {
			parsedKind, _, err := queryTypeFromString(lr.Query)
			if err == nil {
				kind = parsedKind
			}
		}

		switch kind {
		case "trait":
			traits, err := lr.DecodeTraits()
			if err != nil {
				return handleError(ErrInternal, err, "")
			}
			WritePipeableList(os.Stdout, pipeItemsForTraitResults(traits))
			return nil
		case "object":
			objects, err := lr.DecodeObjects()
			if err != nil {
				return handleError(ErrInternal, err, "")
			}
			WritePipeableList(os.Stdout, pipeItemsForObjectResults(objects))
			return nil
		default:
			return handleErrorMsg(ErrInvalidInput, "last results are not from a query", "")
		}
	default:
		results, err := lr.DecodeAll()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		pipeItems := make([]PipeableItem, len(results))
		for i, r := range results {
			pipeItems[i] = PipeableItem{
				Num:      i + 1,
				ID:       r.GetID(),
				Content:  TruncateContent(resultContentForOutput(r), 60),
				Location: r.GetLocation(),
			}
		}
		WritePipeableList(os.Stdout, pipeItems)
		return nil
	}
}

func applyFromLastResults(vaultPath string, lr *lastresults.LastResults, results []model.Result, applyStr string, confirm bool) error {
	if lr.Source != lastresults.SourceQuery {
		return handleErrorMsg(ErrInvalidInput,
			"--apply is only supported for query results",
			"Run 'rvn query --apply ...' for query operations")
	}

	queryType, err := queryTypeFromResults(lr, results)
	if err != nil {
		return handleError(ErrInvalidInput, err, "")
	}
	return applyToResults(vaultPath, results, applyStr, confirm, queryType)
}

func applyToResults(vaultPath string, results []model.Result, applyStr string, confirm bool, queryType string) error {
	if len(results) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no entries to apply operation to", "")
	}

	// Parse the apply command
	parts := strings.Fields(applyStr)
	if len(parts) == 0 {
		return handleErrorMsg(ErrInvalidInput, "no apply command specified",
			"Use --apply \"update value=done\" or similar")
	}

	applyCmd := parts[0]
	applyArgs := parts[1:]

	// Collect IDs
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.GetID()
	}

	// Load vault config for operations
	vaultCfg := loadVaultConfigSafe(vaultPath)

	// For trait queries, only 'update' with 'value=' is supported
	var traitValue string
	if queryType == "trait" {
		if applyCmd != "update" {
			return handleErrorMsg(ErrInvalidInput,
				fmt.Sprintf("'%s' is not supported for trait results", applyCmd),
				"For traits, use: --apply \"update value=<new_value>\"")
		}
		var err error
		traitValue, err = parseTraitValueArgs(applyArgs)
		if err != nil {
			return err
		}
	}

	// Dispatch to appropriate bulk operation
	var warnings []Warning
	switch applyCmd {
	case "set":
		return applySetFromQuery(vaultPath, ids, applyArgs, warnings, nil, vaultCfg, confirm)
	case "update":
		if queryType != "trait" {
			return handleErrorMsg(ErrInvalidInput, "update is only supported for trait results",
				"Use: --apply \"set field=value\" for object results")
		}
		return applyUpdateTraitsByID(vaultPath, ids, traitValue, confirm, true, vaultCfg)
	case "delete":
		if queryType == "trait" {
			return handleErrorMsg(ErrInvalidInput, "cannot delete traits directly",
				"Traits are part of their containing file")
		}
		return applyDeleteFromQuery(vaultPath, ids, warnings, vaultCfg, confirm)
	case "move":
		if queryType == "trait" {
			return handleErrorMsg(ErrInvalidInput, "cannot move traits directly",
				"Traits are part of their containing file")
		}
		return applyMoveFromQuery(vaultPath, ids, applyArgs, warnings, vaultCfg, confirm)
	default:
		return handleErrorMsg(ErrInvalidInput,
			fmt.Sprintf("unknown apply command: %s", applyCmd),
			"Supported commands: set, delete, move")
	}
}

func queryTypeFromResults(lr *lastresults.LastResults, results []model.Result) (string, error) {
	if len(results) > 0 {
		kind := results[0].GetKind()
		if kind == "trait" || kind == "object" {
			return kind, nil
		}
	}

	if lr.Query == "" {
		return "", fmt.Errorf("missing query string for last results")
	}

	kind, _, err := queryTypeFromString(lr.Query)
	return kind, err
}

func queryTypeFromString(queryStr string) (string, string, error) {
	q, err := query.Parse(queryStr)
	if err != nil {
		return "", "", err
	}
	if q.Type == query.QueryTypeTrait {
		return "trait", q.TypeName, nil
	}
	return "object", q.TypeName, nil
}

func resultContentForOutput(result model.Result) string {
	switch r := result.(type) {
	case model.Object:
		return filepath.Base(r.ID)
	case *model.Object:
		return filepath.Base(r.ID)
	default:
		return result.GetContent()
	}
}

func init() {
	lastCmd.Flags().String("apply", "", "Apply an operation to selected results (e.g., \"update value=done\")")
	lastCmd.Flags().Bool("confirm", false, "Apply changes (without this flag, shows preview only)")
	lastCmd.Flags().Bool("pipe", false, "Force pipe-friendly output format")
	lastCmd.Flags().Bool("no-pipe", false, "Force human-readable output format")

	rootCmd.AddCommand(lastCmd)
}
