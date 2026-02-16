package cli

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
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
	AbsPath  string
}

var docsCmd = &cobra.Command{
	Use:   "docs [category] [topic]",
	Short: "Browse long-form Markdown documentation",
	Long: `Browse long-form documentation from the repository docs/ directory.

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
		docsRoot, err := discoverDocsRoot()
		if err != nil {
			return handleErrorMsg(
				ErrFileNotFound,
				"docs directory not found",
				"Run from the Raven repository (or a subdirectory) that contains ./docs",
			)
		}

		categories, err := listDocsCategories(docsRoot)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		if len(args) == 0 {
			return outputDocsCategories(categories)
		}

		category, ok := findDocsCategory(categories, args[0])
		if !ok {
			return docsCategoryNotFound(args, categories)
		}

		topics, err := listDocsTopics(docsRoot, category.ID)
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

		content, err := os.ReadFile(topic.AbsPath)
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

		docsRoot, err := discoverDocsRoot()
		if err != nil {
			return handleErrorMsg(
				ErrFileNotFound,
				"docs directory not found",
				"Run from the Raven repository (or a subdirectory) that contains ./docs",
			)
		}

		matches, err := searchDocs(docsRoot, query, docsSearchCategory, docsSearchLimit)
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

func discoverDocsRoot() (string, error) {
	if cwd, err := os.Getwd(); err == nil {
		if p, ok := findDocsRootFrom(cwd); ok {
			return p, nil
		}
	}

	if exe, err := os.Executable(); err == nil {
		if p, ok := findDocsRootFrom(filepath.Dir(exe)); ok {
			return p, nil
		}
	}

	return "", fmt.Errorf("docs root not found")
}

func findDocsRootFrom(start string) (string, bool) {
	dir := start
	for {
		candidate := filepath.Join(dir, "docs")
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return candidate, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func listDocsCategories(docsRoot string) ([]docsCategoryView, error) {
	entries, err := os.ReadDir(docsRoot)
	if err != nil {
		return nil, fmt.Errorf("read docs root: %w", err)
	}

	categories := make([]docsCategoryView, 0)
	for _, entry := range entries {
		if !entry.IsDir() || !isPublicDocsName(entry.Name()) {
			continue
		}

		topics, err := listDocsTopics(docsRoot, entry.Name())
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
	categoryPath := filepath.Join(docsRoot, category)
	info, err := os.Stat(categoryPath)
	if err != nil {
		return nil, fmt.Errorf("category %q not found: %w", category, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("category path %q is not a directory", categoryPath)
	}

	records := make([]docsTopicRecord, 0)
	seen := make(map[string]string)

	err = filepath.WalkDir(categoryPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		name := d.Name()
		if d.IsDir() {
			if path == categoryPath {
				return nil
			}
			if !isPublicDocsName(name) {
				return filepath.SkipDir
			}
			return nil
		}

		if !isPublicDocsName(name) {
			return nil
		}
		if strings.ToLower(filepath.Ext(name)) != ".md" {
			return nil
		}

		rel, err := filepath.Rel(categoryPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

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
			Title:    extractDocsTitle(path, id),
			Path:     filepath.ToSlash(filepath.Join("docs", category, rel)),
			AbsPath:  path,
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
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}
	if limit < 1 {
		return nil, fmt.Errorf("limit must be >= 1")
	}

	categories, err := listDocsCategories(docsRoot)
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
		topics, err := listDocsTopics(docsRoot, category.ID)
		if err != nil {
			return nil, err
		}

		for _, topic := range topics {
			content, err := os.ReadFile(topic.AbsPath)
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

func extractDocsTitle(absPath, fallbackSlug string) string {
	f, err := os.Open(absPath)
	if err != nil {
		return titleFromSlug(filepath.Base(fallbackSlug))
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

	return titleFromSlug(filepath.Base(fallbackSlug))
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
