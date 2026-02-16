package cli

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	builtindocs "github.com/aidanlsb/raven/docs"
)

const (
	docsCommandHint = "For command docs, use: rvn help <command>"
)

var (
	docsSearchLimit    int
	docsSearchCategory string
)

type docsCategoryView struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	TopicCount int    `json:"topic_count"`
}

type docsTopicView struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Path  string `json:"path"`
}

type docsSearchMatchView struct {
	Category string `json:"category"`
	Topic    string `json:"topic"`
	Title    string `json:"title"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Snippet  string `json:"snippet"`
}

type docsTopicRecord struct {
	Category string
	ID       string
	Title    string
	Path     string
	FSPath   string
}

var docsCmd = &cobra.Command{
	Use:   "docs [category] [topic]",
	Short: "Browse long-form Markdown documentation",
	Long: `Browse long-form documentation bundled into the rvn binary.

Use this command for guides, references, and design notes.
For command-level usage, use 'rvn help <command>'.

Examples:
  rvn docs
  rvn docs guide
  rvn docs reference query-language
  rvn docs search "saved query"
  rvn docs search refs --category reference`,
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		categories, err := listDocsCategoriesFS(builtindocs.FS, ".")
		if err != nil {
			return handleError(ErrInternal, err, "Rebuild rvn so bundled docs are available")
		}

		if len(args) == 0 {
			return outputDocsCategories(categories)
		}

		category, ok := findDocsCategory(categories, args[0])
		if !ok {
			return docsCategoryNotFound(args, categories)
		}

		topics, err := listDocsTopicsFS(builtindocs.FS, ".", category.ID)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		if len(args) == 1 {
			return outputDocsTopics(category, topics)
		}

		topic, ok := findDocsTopic(topics, args[1])
		if !ok {
			return docsTopicNotFound(category.ID, args[1], topics)
		}

		content, err := fs.ReadFile(builtindocs.FS, topic.FSPath)
		if err != nil {
			return handleError(ErrFileReadError, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"category": category.ID,
				"topic":    topic.ID,
				"title":    topic.Title,
				"path":     topic.Path,
				"content":  string(content),
			}, nil)
			return nil
		}

		fmt.Printf("Path: %s\n\n", topic.Path)
		fmt.Print(string(content))
		if len(content) > 0 && content[len(content)-1] != '\n' {
			fmt.Println()
		}
		return nil
	},
}

var docsSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search long-form Markdown documentation",
	Long: `Search long-form documentation in docs/**/*.md.

Examples:
  rvn docs search query
  rvn docs search "saved query" --category reference
  rvn docs search workflow --limit 10`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.TrimSpace(strings.Join(args, " "))
		if query == "" {
			return handleErrorMsg(ErrMissingArgument, "specify a search query", "Usage: rvn docs search <query>")
		}
		if docsSearchLimit < 1 {
			return handleErrorMsg(ErrInvalidInput, "--limit must be >= 1", "")
		}

		matches, err := searchDocsFS(builtindocs.FS, ".", query, docsSearchCategory, docsSearchLimit)
		if err != nil {
			return handleError(ErrInvalidInput, err, "Run 'rvn docs' to list categories")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"query":   query,
				"count":   len(matches),
				"matches": matches,
			}, &Meta{Count: len(matches)})
			return nil
		}

		if len(matches) == 0 {
			fmt.Printf("No docs matched %q.\n", query)
			return nil
		}

		fmt.Printf("Matches for %q (%d):\n", query, len(matches))
		for _, m := range matches {
			fmt.Printf("- %s/%s:%d %s\n", m.Category, m.Topic, m.Line, m.Snippet)
		}
		return nil
	},
}

func outputDocsCategories(categories []docsCategoryView) error {
	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"categories":     categories,
			"command_docs":   "rvn help <command>",
			"navigation_tip": "rvn docs <category> <topic>",
		}, &Meta{Count: len(categories)})
		return nil
	}

	fmt.Println("Documentation categories:")
	for _, c := range categories {
		fmt.Printf("  %-12s %d topic(s)\n", c.ID, c.TopicCount)
	}
	fmt.Println()
	fmt.Println(docsCommandHint)
	fmt.Println("Open a category: rvn docs <category>")
	fmt.Println("Open a topic:    rvn docs <category> <topic>")
	fmt.Println("Search docs:     rvn docs search <query>")
	return nil
}

func outputDocsTopics(category docsCategoryView, topics []docsTopicRecord) error {
	if isJSONOutput() {
		items := make([]docsTopicView, 0, len(topics))
		for _, t := range topics {
			items = append(items, docsTopicView{
				ID:    t.ID,
				Title: t.Title,
				Path:  t.Path,
			})
		}
		outputSuccess(map[string]interface{}{
			"category": category.ID,
			"title":    category.Title,
			"topics":   items,
		}, &Meta{Count: len(items)})
		return nil
	}

	fmt.Printf("%s topics:\n", category.Title)
	if len(topics) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	for _, t := range topics {
		fmt.Printf("  %-24s %s\n", t.ID, t.Title)
	}
	return nil
}

func docsCategoryNotFound(args []string, categories []docsCategoryView) error {
	if cmdPath, ok := resolveCLICommandPath(args); ok {
		return handleErrorMsg(
			ErrInvalidInput,
			fmt.Sprintf("%q is a CLI command, not a docs category", cmdPath),
			fmt.Sprintf("Use 'rvn help %s' for command documentation", cmdPath),
		)
	}

	if isCommandCategoryAlias(args[0]) {
		return handleErrorMsg(
			ErrInvalidInput,
			"command docs are not part of 'rvn docs'",
			docsCommandHint,
		)
	}

	available := make([]string, 0, len(categories))
	for _, c := range categories {
		available = append(available, c.ID)
	}
	sort.Strings(available)

	return handleErrorMsg(
		ErrInvalidInput,
		fmt.Sprintf("unknown docs category: %s", args[0]),
		fmt.Sprintf("Run 'rvn docs' to list categories (available: %s)", strings.Join(available, ", ")),
	)
}

func docsTopicNotFound(categoryID, topicInput string, topics []docsTopicRecord) error {
	available := make([]string, 0, len(topics))
	for _, t := range topics {
		available = append(available, t.ID)
	}
	sort.Strings(available)

	suggestion := fmt.Sprintf("Run 'rvn docs %s' to list topics", categoryID)
	if len(available) > 0 {
		suggestion = fmt.Sprintf("%s (available: %s)", suggestion, strings.Join(available, ", "))
	}

	return handleErrorMsg(
		ErrInvalidInput,
		fmt.Sprintf("unknown topic %q in category %q", topicInput, categoryID),
		suggestion,
	)
}

func listDocsCategories(docsRoot string) ([]docsCategoryView, error) {
	return listDocsCategoriesFS(os.DirFS(docsRoot), ".")
}

func listDocsCategoriesFS(docsFS fs.FS, docsRoot string) ([]docsCategoryView, error) {
	entries, err := fs.ReadDir(docsFS, docsRoot)
	if err != nil {
		return nil, fmt.Errorf("read docs root: %w", err)
	}

	categories := make([]docsCategoryView, 0)
	for _, entry := range entries {
		if !entry.IsDir() || !isPublicDocsName(entry.Name()) {
			continue
		}

		topics, err := listDocsTopicsFS(docsFS, docsRoot, entry.Name())
		if err != nil {
			return nil, err
		}
		categories = append(categories, docsCategoryView{
			ID:         entry.Name(),
			Title:      titleFromSlug(entry.Name()),
			TopicCount: len(topics),
		})
	}

	sort.Slice(categories, func(i, j int) bool {
		return categories[i].ID < categories[j].ID
	})

	return categories, nil
}

func listDocsTopics(docsRoot, category string) ([]docsTopicRecord, error) {
	return listDocsTopicsFS(os.DirFS(docsRoot), ".", category)
}

func listDocsTopicsFS(docsFS fs.FS, docsRoot, category string) ([]docsTopicRecord, error) {
	categoryPath := path.Join(docsRoot, category)
	info, err := fs.Stat(docsFS, categoryPath)
	if err != nil {
		return nil, fmt.Errorf("category %q not found: %w", category, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("category path %q is not a directory", categoryPath)
	}

	records := make([]docsTopicRecord, 0)
	seen := make(map[string]string)

	err = fs.WalkDir(docsFS, categoryPath, func(entryPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		name := d.Name()
		if d.IsDir() {
			if entryPath == categoryPath {
				return nil
			}
			if !isPublicDocsName(name) {
				return fs.SkipDir
			}
			return nil
		}

		if !isPublicDocsName(name) {
			return nil
		}
		if strings.ToLower(filepath.Ext(name)) != ".md" {
			return nil
		}

		rel := strings.TrimPrefix(entryPath, categoryPath+"/")
		if rel == entryPath {
			return fmt.Errorf("unexpected docs topic path %q for category %q", entryPath, category)
		}

		id := normalizeDocsPathSlug(strings.TrimSuffix(rel, filepath.Ext(rel)))
		if id == "" {
			return nil
		}
		if prev, exists := seen[id]; exists {
			return fmt.Errorf("duplicate topic slug %q in category %q (%s and %s)", id, category, prev, rel)
		}
		seen[id] = rel

		records = append(records, docsTopicRecord{
			Category: category,
			ID:       id,
			Title:    extractDocsTitleFS(docsFS, entryPath, id),
			Path:     path.Join("docs", category, rel),
			FSPath:   entryPath,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].ID < records[j].ID
	})
	return records, nil
}

func findDocsCategory(categories []docsCategoryView, raw string) (docsCategoryView, bool) {
	needle := normalizeDocsSegment(raw)
	for _, c := range categories {
		if normalizeDocsSegment(c.ID) == needle {
			return c, true
		}
	}
	return docsCategoryView{}, false
}

func findDocsTopic(topics []docsTopicRecord, raw string) (docsTopicRecord, bool) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(raw, ".md"))
	needle := normalizeDocsPathSlug(trimmed)
	for _, topic := range topics {
		if topic.ID == needle {
			return topic, true
		}
	}
	return docsTopicRecord{}, false
}

func searchDocs(docsRoot, query, categoryFilter string, limit int) ([]docsSearchMatchView, error) {
	return searchDocsFS(os.DirFS(docsRoot), ".", query, categoryFilter, limit)
}

func searchDocsFS(docsFS fs.FS, docsRoot, query, categoryFilter string, limit int) ([]docsSearchMatchView, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}
	if limit < 1 {
		return nil, fmt.Errorf("limit must be >= 1")
	}

	categories, err := listDocsCategoriesFS(docsFS, docsRoot)
	if err != nil {
		return nil, err
	}

	selected := make([]docsCategoryView, 0)
	if strings.TrimSpace(categoryFilter) == "" {
		selected = categories
	} else {
		category, ok := findDocsCategory(categories, categoryFilter)
		if !ok {
			return nil, fmt.Errorf("unknown category: %s", categoryFilter)
		}
		selected = append(selected, category)
	}

	queryLower := strings.ToLower(query)
	matches := make([]docsSearchMatchView, 0, limit)

	for _, category := range selected {
		topics, err := listDocsTopicsFS(docsFS, docsRoot, category.ID)
		if err != nil {
			return nil, err
		}

		for _, topic := range topics {
			content, err := fs.ReadFile(docsFS, topic.FSPath)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", topic.Path, err)
			}

			lines := strings.Split(string(content), "\n")
			for i, line := range lines {
				if !strings.Contains(strings.ToLower(line), queryLower) {
					continue
				}

				matches = append(matches, docsSearchMatchView{
					Category: category.ID,
					Topic:    topic.ID,
					Title:    topic.Title,
					Path:     topic.Path,
					Line:     i + 1,
					Snippet:  shortenDocsSnippet(line, queryLower),
				})
				if len(matches) >= limit {
					return matches, nil
				}
			}
		}
	}

	return matches, nil
}

func shortenDocsSnippet(line, queryLower string) string {
	const maxLen = 160
	snippet := strings.TrimSpace(line)
	if snippet == "" {
		return "(blank line)"
	}
	if len(snippet) <= maxLen {
		return snippet
	}

	idx := strings.Index(strings.ToLower(snippet), queryLower)
	if idx < 0 {
		return snippet[:maxLen-1] + "..."
	}

	start := idx - 50
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(snippet) {
		end = len(snippet)
	}
	out := snippet[start:end]
	if start > 0 {
		out = "..." + out
	}
	if end < len(snippet) {
		out += "..."
	}
	return out
}

func extractDocsTitleFS(docsFS fs.FS, docsPath, fallbackSlug string) string {
	f, err := docsFS.Open(docsPath)
	if err != nil {
		return titleFromSlug(path.Base(fallbackSlug))
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# ") {
			title := strings.TrimSpace(strings.TrimPrefix(line, "# "))
			if title != "" {
				return title
			}
		}
	}

	return titleFromSlug(path.Base(fallbackSlug))
}

func isPublicDocsName(name string) bool {
	if name == "" {
		return false
	}
	return !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "_")
}

func normalizeDocsPathSlug(input string) string {
	input = strings.ReplaceAll(strings.TrimSpace(input), "\\", "/")
	if input == "" {
		return ""
	}

	parts := strings.Split(input, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		norm := normalizeDocsSegment(part)
		if norm == "" {
			continue
		}
		out = append(out, norm)
	}
	return strings.Join(out, "/")
}

func normalizeDocsSegment(input string) string {
	s := strings.ToLower(strings.TrimSpace(input))
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

func titleFromSlug(slug string) string {
	parts := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_'
	})
	if len(parts) == 0 {
		return slug
	}
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func isCommandCategoryAlias(raw string) bool {
	normalized := normalizeDocsSegment(raw)
	return normalized == "command" || normalized == "commands"
}

func resolveCLICommandPath(args []string) (string, bool) {
	for i := len(args); i >= 1; i-- {
		path := strings.Join(args[:i], " ")
		cmd, ok := findCommandByPathRuntime(rootCmd, path)
		if !ok {
			continue
		}
		// Don't redirect docs->docs.
		if cmd.Name() == "docs" {
			continue
		}
		return path, true
	}
	return "", false
}

func findCommandByPathRuntime(root *cobra.Command, path string) (*cobra.Command, bool) {
	parts := strings.Fields(path)
	if len(parts) == 0 {
		return nil, false
	}

	cur := root
	for _, part := range parts {
		var next *cobra.Command
		for _, child := range cur.Commands() {
			if child.Name() == part {
				next = child
				break
			}
		}
		if next == nil {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func init() {
	docsSearchCmd.Flags().IntVarP(&docsSearchLimit, "limit", "n", 20, "Maximum number of matches")
	docsSearchCmd.Flags().StringVarP(&docsSearchCategory, "category", "c", "", "Filter search to a docs category")

	docsCmd.AddCommand(docsSearchCmd)
	rootCmd.AddCommand(docsCmd)
}
