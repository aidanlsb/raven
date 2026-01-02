package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ravenscroftj/raven/internal/index"
	"github.com/spf13/cobra"
)

var tagCmd = &cobra.Command{
	Use:   "tag <name>",
	Short: "Query objects by tag",
	Long: `Lists all objects that have a specific tag.

Examples:
  rvn tag project       # Find all objects tagged #project
  rvn tag important     # Find all objects tagged #important
  rvn tag --list        # List all tags with counts
  rvn tag project --json # JSON output for agents`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		start := time.Now()

		db, err := index.Open(vaultPath)
		if err != nil {
			return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
		}
		defer db.Close()

		listFlag, _ := cmd.Flags().GetBool("list")
		if listFlag {
			return listAllTagsWithJSON(db, start)
		}

		if len(args) == 0 {
			return handleErrorMsg(ErrMissingArgument, "specify a tag name or use --list", "Run 'rvn tag --list' to see all tags")
		}

		tagName := args[0]
		// Strip leading # if present
		tagName = strings.TrimPrefix(tagName, "#")

		return queryTagWithJSON(db, vaultPath, tagName, start)
	},
}

func listAllTagsWithJSON(db *index.Database, start time.Time) error {
	tags, err := db.QueryAllTags()
	if err != nil {
		return handleError(ErrDatabaseError, err, "")
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		items := make([]TagSummary, len(tags))
		for i, t := range tags {
			items[i] = TagSummary{
				Tag:   t.Tag,
				Count: t.Count,
			}
		}
		outputSuccess(map[string]interface{}{
			"tags": items,
		}, &Meta{Count: len(items), QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	if len(tags) == 0 {
		fmt.Println("No tags found in the vault.")
		return nil
	}

	fmt.Printf("Tags (%d total):\n\n", len(tags))
	for _, t := range tags {
		countStr := fmt.Sprintf("%d", t.Count)
		if t.Count == 1 {
			countStr = "1 object"
		} else {
			countStr = fmt.Sprintf("%d objects", t.Count)
		}
		fmt.Printf("  #%-20s %s\n", t.Tag, countStr)
	}

	return nil
}

func queryTagWithJSON(db *index.Database, vaultPath string, tagName string, start time.Time) error {
	results, err := db.QueryTags(tagName)
	if err != nil {
		return handleError(ErrDatabaseError, err, "")
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		items := make([]TagResult, len(results))
		for i, r := range results {
			items[i] = TagResult{
				Tag:      r.Tag,
				ObjectID: r.ObjectID,
				FilePath: r.FilePath,
				Line:     r.Line,
			}
		}
		outputSuccess(map[string]interface{}{
			"tag":   tagName,
			"items": items,
		}, &Meta{Count: len(items), QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Printf("# #%s (%d)\n\n", tagName, len(results))

	if len(results) == 0 {
		fmt.Printf("No objects found with tag #%s.\n", tagName)
		return nil
	}

	// Group by file path for cleaner output
	byFile := make(map[string][]index.TagResult)
	var files []string
	for _, r := range results {
		if _, exists := byFile[r.FilePath]; !exists {
			files = append(files, r.FilePath)
		}
		byFile[r.FilePath] = append(byFile[r.FilePath], r)
	}
	sort.Strings(files)

	for _, file := range files {
		tags := byFile[file]
		fmt.Printf("• %s\n", file)
		for _, t := range tags {
			if t.Line != nil {
				fmt.Printf("    %s (line %d)\n", t.ObjectID, *t.Line)
			} else {
				fmt.Printf("    %s\n", t.ObjectID)
			}
		}
	}

	return nil
}

// completeTagNames provides shell completion for tag names
func completeTagNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	vaultPath := getVaultPath()
	if vaultPath == "" {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer db.Close()

	tags, err := db.QueryAllTags()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for _, t := range tags {
		names = append(names, t.Tag)
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	tagCmd.Flags().BoolP("list", "l", false, "List all tags with counts")
	tagCmd.ValidArgsFunction = completeTagNames

	// Also add shell completion for types/traits from schema
	rootCmd.AddCommand(tagCmd)
}

