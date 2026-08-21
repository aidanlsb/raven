package cli

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/querysvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

var queryCmd = &cobra.Command{
	Use:   "query <query-string>",
	Short: "Run a query using the Raven query language",
	Long: `Query items, sections, traits, or outgoing link edges using the Raven query language.

Query roots:
  type:<type> [predicates]    Query items of a type
  section [predicates]        Query heading-derived sections
  trait:<name> [predicates]   Query traits by name
  link [predicates]           Query indexed outgoing Markdown link edges

Predicates for type queries:
  .field==value      Field equals value
  exists(.field)     Field exists (has a value)
  !.field==value     Field does not equal value
  oneof(.field, [...]) Field matches any listed scalar value
  includes(.field, "text") Substring match
  has(trait:...)      Has a directly scoped trait
  has(section...)     Has a directly scoped section
  contains(trait:...) Recursively contains a matching trait
  contains(section...) Recursively contains a matching section
  refs([[target]])      References a specific target
  refs(type:...)      References an item matching nested type query
  links(.ext==pdf)      Contains a matching outgoing file or URL link
  refd([[source]])      Referenced by a specific source
  refd(type:...)      Referenced by an item matching nested type query
  refd(trait:...)       Referenced by a trait matching nested trait query
  content("term")       Full-text search on item content

Predicates for trait queries:
  .value==val      Trait value equals val
  oneof(.value, [...]) Value matches any listed scalar value
  in(type:...)       Direct scope matches nested type query
  in(section...)     Direct scope matches nested section query
  within(type:...)   Any scope matches nested type query
  within(section...) Any scope matches nested section query
  at(trait:...)        Co-located with trait matching nested trait query
  refs([[target]])     Line contains reference to target
  refs(type:...)     Line references an item matching nested type query
  links(.ext==pdf)     Line contains a matching file or URL link
  content("term")      Line content contains term

Predicates for section queries:
  .title==Tasks        Section field equals value
  within(type:...)     Any containing scope matches nested query
  contains(trait:...)  Recursively contains a matching trait
  refs([[target]])     Subtree contains a reference to target
  links(.ext==pdf)     Subtree contains a matching file or URL link

Predicates for link queries:
  .ext==pdf              Link target extension equals value
  .is_image==true        Link is an image
  .scheme==url           Link target scheme is file, url, or other
  .normalized_key=="..." Case-sensitive canonical target equality
  .raw_target=="..."     Case-sensitive authored target equality
  includes(.raw_target, "text") String match on the authored target
  includes(.display, "text") String match on the display or alt text
  within(type:...)       Source file matches a type query
  within(section ...)    Link occurs in a matching section subtree

For refs() and links(), type roots inspect the whole file, section roots inspect
the complete subtree, and trait roots inspect only the trait's source line.
All link fields are present, so exists()/!exists() are invalid; use an empty
comparison such as .ext=="" when appropriate. Link has no in(); use
trait:<name> links(...) rather than link within(trait:<name>).

Boolean operators:
  !pred            NOT
  pred1 pred2      AND (space-separated)
  pred1 | pred2    OR

Saved query inputs must be declared with args: in raven.yaml when using {{args.<name>}}.
You can then pass inputs either by position (following args order) or as key=value pairs.
Saved queries store only their RQL definition, declared args, and description.
Pass execution flags such as --refresh, --limit, --apply, and --confirm each time
you invoke the saved query.

Use --browse to open an interactive Raven picker with filtering, preview, and
editor handoff for the selected result.
Use --no-links to disable clickable terminal hyperlinks in human-readable
results.


Examples:
  rvn query "type:project .status==active"
  rvn query "type:meeting has(trait:due)"
  rvn query "section .title==Tasks"
  rvn query "trait:due .value<today"
  rvn query "link .ext==pdf within(type:project)"
  rvn query "trait:todo content(\"my task\")"
  rvn query "trait:highlight in(type:book .status==reading)"
  rvn query tasks                    # Run saved query
  rvn query project-todos raven      # Positional input (args: [project])
  rvn query project-todos project=projects/raven
  rvn query saved list               # Manage saved queries`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		noLinks, _ := cmd.Flags().GetBool("no-links")
		setHyperlinksDisabled(noLinks)

		if len(args) == 0 {
			return handleErrorMsg(ErrMissingArgument, "specify a query string", "Run 'rvn query saved list' to see saved queries")
		}

		rawQuery := joinQueryArgs(args)

		// Saved-query resolution lives in querysvc so the CLI never reimplements
		// it. Runtime behavior comes only from flags on this invocation.
		rt, err := vaultruntime.New(getVaultPath(), vaultruntime.Options{SkipSchema: true})
		if err != nil {
			return handleCanonicalFailure(commandexec.Failure(
				"CONFIG_INVALID",
				"failed to load vault config",
				nil,
				"Fix raven.yaml and try again",
			))
		}
		defer rt.Close()
		runOpts, err := querysvc.ResolveRunOptions(rt, rawQuery, savedQueryOptionsFromFlags(cmd))
		if err != nil {
			return handleCanonicalFailure(commandexec.FromServiceError(err))
		}

		browse := runOpts.Browse
		SetPipeFormat(runOpts.Pipe)
		if browse {
			if handled, err := validateInteractiveBrowse(true); handled || err != nil {
				return err
			}
			if runOpts.IDs || runOpts.CountOnly || len(runOpts.Apply) > 0 {
				return handleErrorMsg(ErrInvalidInput, "--browse cannot be used with --ids, --count-only, or --apply", "Run the query without browse for machine-readable or bulk modes")
			}
			if runOpts.Pipe != nil && *runOpts.Pipe {
				return handleErrorMsg(ErrInvalidInput, "--browse cannot be used with --pipe", "Use --no-pipe or remove --browse")
			}
		}

		// --apply routes through the canonical bulk apply flow (preview + confirm).
		if len(runOpts.Apply) > 0 {
			return runCanonicalQuery(runOpts.ResolvedQuery, map[string]interface{}{
				"query_string": rawQuery,
				"refresh":      runOpts.Refresh,
				"apply":        runOpts.Apply,
				"confirm":      runOpts.Confirm,
			})
		}

		return runCanonicalQuery(runOpts.ResolvedQuery, map[string]interface{}{
			"query_string": rawQuery,
			"refresh":      runOpts.Refresh,
			"ids":          runOpts.IDs,
			"limit":        runOpts.Limit,
			"offset":       runOpts.Offset,
			"count-only":   runOpts.CountOnly,
			"browse":       browse,
		})
	},
}

// runCanonicalQuery executes the query through the canonical command path and
// renders the result. All query semantics (parsing, saved-query resolution,
// execution, apply) live in commandimpl; this wrapper only decides between JSON,
// bulk-apply, and human rendering. queryStr is the resolved query string, used
// only for human-readable labels and empty-result messages.
func runCanonicalQuery(queryStr string, args map[string]interface{}) error {
	return renderCanonicalQueryResult(queryStr, args, executeCanonicalQuery(args))
}

func renderCanonicalQueryResult(queryStr string, args map[string]interface{}, result commandexec.Result) error {
	if hasQueryApply(args) {
		return renderCanonicalQueryApplyResult(args, result)
	}
	if !result.OK {
		if result.Error == nil {
			if isJSONOutput() {
				return outputCanonicalResultJSON(result)
			}
			return handleErrorMsg(ErrInternal, "command execution failed", "")
		}
		return handleCanonicalFailure(result)
	}

	if isJSONOutput() {
		return outputCanonicalResultJSON(result)
	}

	return renderCanonicalQueryHuman(queryStr, result.Data, boolValue(args["browse"]))
}

func executeCanonicalQuery(args map[string]interface{}) commandexec.Result {
	return app.CommandInvoker().Execute(context.Background(), commandexec.Request{
		CommandID: "query",
		VaultPath: getVaultPath(),
		Caller:    commandexec.CallerCLI,
		Args:      args,
	})
}

func renderCanonicalQueryApplyResult(args map[string]interface{}, result commandexec.Result) error {
	if !result.OK {
		if isJSONOutput() {
			return outputJSON(result)
		}
		if result.Error != nil {
			return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
		}
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	if err := renderCanonicalBulkResult(result); err != nil {
		return err
	}

	if !isJSONOutput() && !boolValue(args["confirm"]) && promptForConfirm("Apply changes?") {
		confirmedArgs := copyArgsMap(args)
		confirmedArgs["confirm"] = true
		applyResult := executeCanonicalQuery(confirmedArgs)
		if !applyResult.OK {
			if applyResult.Error != nil {
				return handleErrorWithDetails(applyResult.Error.Code, applyResult.Error.Message, applyResult.Error.Suggestion, applyResult.Error.Details)
			}
			return handleErrorMsg(ErrInternal, "command execution failed", "")
		}
		return renderCanonicalBulkResult(applyResult)
	}

	return nil
}

func hasQueryApply(args map[string]interface{}) bool {
	return len(stringSliceFromAny(args["apply"])) > 0
}

func copyArgsMap(args map[string]interface{}) map[string]interface{} {
	if args == nil {
		return nil
	}
	out := make(map[string]interface{}, len(args))
	for key, value := range args {
		out[key] = value
	}
	return out
}

// joinQueryArgs joins command-line arguments into a single query string.
func joinQueryArgs(args []string) string {
	if len(args) == 1 {
		return args[0]
	}

	return strings.Join(args, " ")
}

// savedQueryOptionsFromFlags builds invocation-scoped query options from only
// the flags the user explicitly set on cmd.
func savedQueryOptionsFromFlags(cmd *cobra.Command) *querysvc.RunOptionOverrides {
	options := &querysvc.RunOptionOverrides{}
	if cmd.Flags().Changed("refresh") {
		value, _ := cmd.Flags().GetBool("refresh")
		options.Refresh = &value
	}
	if cmd.Flags().Changed("ids") {
		value, _ := cmd.Flags().GetBool("ids")
		options.IDs = &value
	}
	if cmd.Flags().Changed("limit") {
		value, _ := cmd.Flags().GetInt("limit")
		options.Limit = &value
	}
	if cmd.Flags().Changed("offset") {
		value, _ := cmd.Flags().GetInt("offset")
		options.Offset = &value
	}
	if cmd.Flags().Changed("count-only") {
		value, _ := cmd.Flags().GetBool("count-only")
		options.CountOnly = &value
	}
	if cmd.Flags().Changed("apply") {
		value, _ := cmd.Flags().GetStringArray("apply")
		options.Apply = append([]string(nil), value...)
	}
	if cmd.Flags().Changed("confirm") {
		value, _ := cmd.Flags().GetBool("confirm")
		options.Confirm = &value
	}
	if cmd.Flags().Changed("pipe") {
		value, _ := cmd.Flags().GetBool("pipe")
		options.Pipe = &value
	} else if cmd.Flags().Changed("no-pipe") {
		noPipe, _ := cmd.Flags().GetBool("no-pipe")
		if noPipe {
			value := false
			options.Pipe = &value
		}
	}
	if cmd.Flags().Changed("browse") {
		value, _ := cmd.Flags().GetBool("browse")
		options.Browse = &value
	}
	if options.IsEmpty() {
		return nil
	}
	return options
}

func init() {
	queryCmd.Flags().Bool("refresh", false, "Refresh stale files before query")
	queryCmd.Flags().Bool("ids", false, "Output only result IDs; link queries output source IDs, one per edge")
	queryCmd.Flags().Int("limit", 0, "Maximum number of query results to return (0 means no limit)")
	queryCmd.Flags().Int("offset", 0, "Zero-based offset for query results")
	queryCmd.Flags().Bool("count-only", false, "Return only the total count of matches (no items or IDs)")
	queryCmd.Flags().StringArray("apply", nil, "Apply a bulk operation to query results (format: command args...)")
	queryCmd.Flags().Bool("confirm", false, "Apply changes (without this flag, shows preview only)")
	queryCmd.Flags().Bool("pipe", false, "Force pipe-friendly output for shell pipelines (jq, head, sort)")
	queryCmd.Flags().Bool("no-pipe", false, "Force human-readable output format")
	queryCmd.Flags().Bool("browse", false, "Interactively browse query results in Raven's picker and open the selected result")
	queryCmd.Flags().Bool("no-links", false, "Disable clickable hyperlinks in terminal output")

	querySavedCmd.AddCommand(querySavedListCmd)
	querySavedCmd.AddCommand(querySavedGetCmd)
	querySavedCmd.AddCommand(querySavedSetCmd)
	querySavedCmd.AddCommand(querySavedRemoveCmd)
	queryCmd.AddCommand(querySavedCmd)
	rootCmd.AddCommand(queryCmd)
}
