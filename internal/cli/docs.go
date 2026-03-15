package cli

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/docssvc"
	"github.com/aidanlsb/raven/internal/ui"
)

const (
	docsCommandHint = "For command docs, use: rvn help <command>"
)

var (
	docsSearchLimit   int
	docsSearchSection string
	docsFetchRef      string
	docsFetchSource   string

	docsFZFRun         = runDocsFZF
	docsDisplayContext = ui.NewDisplayContext
	docsMarkdownRender = ui.RenderMarkdown
)

type docsSectionView struct {
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
	Section string `json:"section"`
	Topic   string `json:"topic"`
	Title   string `json:"title"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

type docsTopicRecord struct {
	Section string
	ID      string
	Title   string
	Path    string
	FSPath  string
}

var docsCmd = &cobra.Command{
	Use:   "docs [section] [topic]",
	Short: "Browse long-form Markdown documentation",
	Long: `Browse long-form documentation stored in your vault's .raven/docs cache.

Use this command for guides, references, and design notes.
Run 'rvn docs fetch' to sync or refresh docs content.
When run in a terminal with fzf installed, 'rvn docs' opens an interactive selector.
For command-level usage, use 'rvn help <command>'.

Examples:
  rvn docs
  rvn docs list
  rvn docs <section>
  rvn docs <section> <topic>
  rvn docs fetch
  rvn docs search "saved query"
  rvn docs search refs --section reference`,
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		source, err := loadVaultDocsSource(getVaultPath())
		if err != nil {
			return handleError(ErrFileNotFound, err, "Run 'rvn docs fetch' to download docs for this vault")
		}

		sections, err := listDocsSectionsFS(source, ".")
		if err != nil {
			return handleError(ErrInternal, err, "Run 'rvn docs fetch' to refresh docs")
		}

		if len(args) == 0 {
			if shouldUseDocsFZFNavigator() {
				if err := runDocsFZFNavigator(source, sections); err != nil {
					return handleError(ErrInternal, err, "Run 'rvn docs list' for non-interactive output")
				}
				return nil
			}
			return outputDocsSections(sections)
		}

		section, ok := findDocsSection(sections, args[0])
		if !ok {
			return docsSectionNotFound(args, sections)
		}

		topics, err := listDocsTopicsFS(source, ".", section.ID)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		if len(args) == 1 {
			return outputDocsTopics(section, topics)
		}

		topic, ok := findDocsTopic(topics, args[1])
		if !ok {
			return docsTopicNotFound(section.ID, args[1], topics)
		}

		return outputDocsTopicContent(source, topic)
	},
}

var docsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List docs sections and section commands",
	Long: `List docs sections with explicit section command syntax.

Use this to see exactly which 'rvn docs <section>' commands are available.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		source, err := loadVaultDocsSource(getVaultPath())
		if err != nil {
			return handleError(ErrFileNotFound, err, "Run 'rvn docs fetch' to download docs for this vault")
		}

		sections, err := listDocsSectionsFS(source, ".")
		if err != nil {
			return handleError(ErrInternal, err, "Run 'rvn docs fetch' to refresh docs")
		}
		return outputDocsSections(sections)
	},
}

var docsSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search long-form Markdown documentation",
	Long: `Search long-form documentation in docs/**/*.md.

Examples:
  rvn docs search query
  rvn docs search "saved query" --section reference
  rvn docs search workflow --limit 10`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source, err := loadVaultDocsSource(getVaultPath())
		if err != nil {
			return handleError(ErrFileNotFound, err, "Run 'rvn docs fetch' to download docs for this vault")
		}

		query := strings.TrimSpace(strings.Join(args, " "))
		if query == "" {
			return handleErrorMsg(ErrMissingArgument, "specify a search query", "Usage: rvn docs search <query>")
		}
		if docsSearchLimit < 1 {
			return handleErrorMsg(ErrInvalidInput, "--limit must be >= 1", "")
		}

		matches, err := searchDocsFS(source, ".", query, docsSearchSection, docsSearchLimit)
		if err != nil {
			return handleError(ErrInvalidInput, err, "Run 'rvn docs' to list sections")
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
			fmt.Printf("- %s/%s:%d %s\n", m.Section, m.Topic, m.Line, m.Snippet)
		}
		return nil
	},
}

var docsFetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch docs into .raven/docs for the current vault",
	Long: `Download docs from Raven's source repository and install them under .raven/docs.

This command replaces the local docs cache for the current vault.
Use --ref to fetch a specific branch/tag/commit; default is "main".`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		info := currentVersionInfo()
		result, err := docssvc.Fetch(docssvc.FetchRequest{
			VaultPath:  getVaultPath(),
			Ref:        strings.TrimSpace(docsFetchRef),
			SourceBase: strings.TrimSpace(docsFetchSource),
			CLIVersion: info.Version,
		})
		if err != nil {
			return handleError(ErrInternal, err, "Check your network connection and run 'rvn docs fetch' again")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"path":         result.Path,
				"file_count":   result.FileCount,
				"byte_count":   result.ByteCount,
				"source":       result.Source,
				"ref":          result.Ref,
				"archive_url":  result.ArchiveURL,
				"fetched_at":   result.FetchedAt,
				"cli_version":  result.CLIVersion,
				"manifest_ver": result.ManifestVer,
			}, nil)
			return nil
		}

		fmt.Printf("Fetched docs to %s (%d files, %d bytes)\n", result.Path, result.FileCount, result.ByteCount)
		fmt.Printf("Source: %s (%s)\n", result.Source, result.Ref)
		return nil
	},
}

func outputDocsSections(sections []docsSectionView) error {
	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"sections":       sections,
			"command_docs":   "rvn help <command>",
			"navigation_tip": "rvn docs <section> <topic>",
		}, &Meta{Count: len(sections)})
		return nil
	}

	fmt.Println("Documentation section commands:")
	for _, s := range sections {
		sectionCommand := fmt.Sprintf("rvn docs %s", s.ID)
		fmt.Printf("  %-24s %s (%s)\n", sectionCommand, s.Title, docsTopicCountSummary(s.TopicCount))
	}
	fmt.Println()
	fmt.Println("General docs commands:")
	fmt.Println("  rvn docs list                 List sections and section commands")
	fmt.Println("  rvn docs <section>            List topics in a section")
	fmt.Println("  rvn docs <section> <topic>    Open a docs topic")
	fmt.Println("  rvn docs search <query>       Search docs")
	fmt.Println("  rvn docs fetch                Sync docs into .raven/docs")
	fmt.Println("  rvn help <command>            Command docs")
	return nil
}

func outputDocsTopics(section docsSectionView, topics []docsTopicRecord) error {
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
			"section": section.ID,
			"title":   section.Title,
			"topics":  items,
		}, &Meta{Count: len(items)})
		return nil
	}

	fmt.Printf("Documentation topic commands for %s [%s]:\n", section.Title, section.ID)
	if len(topics) == 0 {
		fmt.Println("  (no topics)")
		fmt.Println()
		fmt.Println("General docs commands:")
		fmt.Printf("  %-48s %s\n", "rvn docs list", "List sections and section commands")
		fmt.Printf("  %-48s %s\n", fmt.Sprintf("rvn docs search <query> --section %s", section.ID), "Search only this section")
		fmt.Printf("  %-48s %s\n", "rvn docs fetch", "Sync docs into .raven/docs")
		return nil
	}
	for _, t := range topics {
		topicCommand := fmt.Sprintf("rvn docs %s %s", section.ID, t.ID)
		fmt.Printf("  %-48s %s\n", topicCommand, t.Title)
	}
	fmt.Println()
	fmt.Println("General docs commands:")
	fmt.Printf("  %-48s %s\n", fmt.Sprintf("rvn docs %s", section.ID), "List topics in this section")
	fmt.Printf("  %-48s %s\n", fmt.Sprintf("rvn docs search <query> --section %s", section.ID), "Search only this section")
	fmt.Printf("  %-48s %s\n", "rvn docs list", "List sections and section commands")
	fmt.Printf("  %-48s %s\n", "rvn docs fetch", "Sync docs into .raven/docs")
	return nil
}

func outputDocsTopicContent(docsFS fs.FS, topic docsTopicRecord) error {
	content, err := fs.ReadFile(docsFS, topic.FSPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"section": topic.Section,
			"topic":   topic.ID,
			"title":   topic.Title,
			"path":    topic.Path,
			"content": string(content),
		}, nil)
		return nil
	}

	renderedContent := string(content)
	display := docsDisplayContext()
	if display.IsTTY {
		if rendered, renderErr := docsMarkdownRender(string(content), display.TermWidth); renderErr == nil {
			renderedContent = rendered
		}
	}

	fmt.Printf("Path: %s\n\n", topic.Path)
	fmt.Print(renderedContent)
	if !strings.HasSuffix(renderedContent, "\n") {
		fmt.Println()
	}
	return nil
}

func shouldUseDocsFZFNavigator() bool {
	return canUseFZFInteractive()
}

func runDocsFZFNavigator(docsFS fs.FS, sections []docsSectionView) error {
	section, ok, err := pickDocsSectionWithFZF(sections)
	if err != nil || !ok {
		return err
	}

	topics, err := listDocsTopicsFS(docsFS, ".", section.ID)
	if err != nil {
		return err
	}

	topic, ok, err := pickDocsTopicWithFZF(section, topics)
	if err != nil || !ok {
		return err
	}

	return outputDocsTopicContent(docsFS, topic)
}

func pickDocsSectionWithFZF(sections []docsSectionView) (docsSectionView, bool, error) {
	lines := make([]string, 0, len(sections))
	for _, section := range sections {
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s", section.ID, section.Title, docsTopicCountSummary(section.TopicCount)))
	}

	selectedLine, selected, err := docsFZFRun(lines, "docs/section> ", "Select a docs section (Esc to cancel)")
	if err != nil {
		return docsSectionView{}, false, err
	}
	if !selected {
		return docsSectionView{}, false, nil
	}

	sectionID := docsFZFSelectionID(selectedLine)
	section, ok := findDocsSection(sections, sectionID)
	if !ok {
		return docsSectionView{}, false, fmt.Errorf("selected unknown docs section %q", sectionID)
	}
	return section, true, nil
}

func pickDocsTopicWithFZF(section docsSectionView, topics []docsTopicRecord) (docsTopicRecord, bool, error) {
	lines := make([]string, 0, len(topics))
	for _, topic := range topics {
		lines = append(lines, fmt.Sprintf("%s\t%s", topic.ID, topic.Title))
	}

	prompt := fmt.Sprintf("docs/%s> ", section.ID)
	header := fmt.Sprintf("Select a topic in %s [%s] (Esc to cancel)", section.Title, section.ID)
	selectedLine, selected, err := docsFZFRun(lines, prompt, header)
	if err != nil {
		return docsTopicRecord{}, false, err
	}
	if !selected {
		return docsTopicRecord{}, false, nil
	}

	topicID := docsFZFSelectionID(selectedLine)
	topic, ok := findDocsTopic(topics, topicID)
	if !ok {
		return docsTopicRecord{}, false, fmt.Errorf("selected unknown docs topic %q in section %q", topicID, section.ID)
	}
	return topic, true, nil
}

func docsFZFSelectionID(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	parts := strings.SplitN(line, "\t", 2)
	return strings.TrimSpace(parts[0])
}

func runDocsFZF(lines []string, prompt, header string) (string, bool, error) {
	return runFZFPicker(lines, fzfPickerOptions{
		Prompt:    prompt,
		Header:    header,
		Delimiter: "\t",
		WithNth:   "2..",
	})
}

func docsSectionNotFound(args []string, sections []docsSectionView) error {
	if cmdPath, ok := resolveCLICommandPath(args); ok {
		return handleErrorMsg(
			ErrInvalidInput,
			fmt.Sprintf("%q is a CLI command, not a docs section", cmdPath),
			fmt.Sprintf("Use 'rvn help %s' for command documentation", cmdPath),
		)
	}

	if isCommandSectionAlias(args[0]) {
		return handleErrorMsg(
			ErrInvalidInput,
			"command docs are not part of 'rvn docs'",
			docsCommandHint,
		)
	}

	available := make([]string, 0, len(sections))
	for _, s := range sections {
		available = append(available, s.ID)
	}
	sort.Strings(available)

	return handleErrorMsg(
		ErrInvalidInput,
		fmt.Sprintf("unknown docs section: %s", args[0]),
		fmt.Sprintf("Run 'rvn docs' to list sections (available: %s)", strings.Join(available, ", ")),
	)
}

func docsTopicNotFound(sectionID, topicInput string, topics []docsTopicRecord) error {
	available := make([]string, 0, len(topics))
	for _, t := range topics {
		available = append(available, t.ID)
	}
	sort.Strings(available)

	suggestion := fmt.Sprintf("Run 'rvn docs %s' to list topics", sectionID)
	if len(available) > 0 {
		suggestion = fmt.Sprintf("%s (available: %s)", suggestion, strings.Join(available, ", "))
	}

	return handleErrorMsg(
		ErrInvalidInput,
		fmt.Sprintf("unknown topic %q in section %q", topicInput, sectionID),
		suggestion,
	)
}

func listDocsSections(docsRoot string) ([]docsSectionView, error) {
	sections, err := docssvc.ListSectionsFromRoot(docsRoot)
	if err != nil {
		return nil, err
	}
	return docsSectionsFromService(sections), nil
}

func loadVaultDocsSource(vaultPath string) (fs.FS, error) {
	return docssvc.LoadVaultDocsSource(vaultPath)
}

func listDocsSectionsFS(docsFS fs.FS, docsRoot string) ([]docsSectionView, error) {
	sections, err := docssvc.ListSectionsFS(docsFS, docsRoot)
	if err != nil {
		return nil, err
	}
	return docsSectionsFromService(sections), nil
}

func listDocsTopics(docsRoot, section string) ([]docsTopicRecord, error) {
	topics, err := docssvc.ListTopicsFromRoot(docsRoot, section)
	if err != nil {
		return nil, err
	}
	return docsTopicsFromService(topics), nil
}

func listDocsTopicsFS(docsFS fs.FS, docsRoot, section string) ([]docsTopicRecord, error) {
	topics, err := docssvc.ListTopicsFS(docsFS, docsRoot, section)
	if err != nil {
		return nil, err
	}
	return docsTopicsFromService(topics), nil
}

func findDocsSection(sections []docsSectionView, raw string) (docsSectionView, bool) {
	found, ok := docssvc.FindSection(docsSectionsToService(sections), raw)
	if !ok {
		return docsSectionView{}, false
	}
	return docsSectionView{
		ID:         found.ID,
		Title:      found.Title,
		TopicCount: found.TopicCount,
	}, true
}

func findDocsTopic(topics []docsTopicRecord, raw string) (docsTopicRecord, bool) {
	found, ok := docssvc.FindTopic(docsTopicsToService(topics), raw)
	if !ok {
		return docsTopicRecord{}, false
	}
	return docsTopicRecord{
		Section: found.Section,
		ID:      found.ID,
		Title:   found.Title,
		Path:    found.Path,
		FSPath:  found.FSPath,
	}, true
}

func searchDocsFS(docsFS fs.FS, docsRoot, query, sectionFilter string, limit int) ([]docsSearchMatchView, error) {
	matches, err := docssvc.SearchFS(docsFS, docsRoot, query, sectionFilter, limit)
	if err != nil {
		return nil, err
	}
	return docsMatchesFromService(matches), nil
}

func normalizeDocsPathSlug(input string) string {
	return docssvc.NormalizePathSlug(input)
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

func docsTopicCountSummary(topicCount int) string {
	if topicCount == 1 {
		return "1 topic"
	}
	return fmt.Sprintf("%d topics", topicCount)
}

func isCommandSectionAlias(raw string) bool {
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

func docsSectionsToService(in []docsSectionView) []docssvc.SectionView {
	out := make([]docssvc.SectionView, 0, len(in))
	for _, section := range in {
		out = append(out, docssvc.SectionView{
			ID:         section.ID,
			Title:      section.Title,
			TopicCount: section.TopicCount,
		})
	}
	return out
}

func docsSectionsFromService(in []docssvc.SectionView) []docsSectionView {
	out := make([]docsSectionView, 0, len(in))
	for _, section := range in {
		out = append(out, docsSectionView{
			ID:         section.ID,
			Title:      section.Title,
			TopicCount: section.TopicCount,
		})
	}
	return out
}

func docsTopicsToService(in []docsTopicRecord) []docssvc.TopicRecord {
	out := make([]docssvc.TopicRecord, 0, len(in))
	for _, topic := range in {
		out = append(out, docssvc.TopicRecord{
			Section: topic.Section,
			ID:      topic.ID,
			Title:   topic.Title,
			Path:    topic.Path,
			FSPath:  topic.FSPath,
		})
	}
	return out
}

func docsTopicsFromService(in []docssvc.TopicRecord) []docsTopicRecord {
	out := make([]docsTopicRecord, 0, len(in))
	for _, topic := range in {
		out = append(out, docsTopicRecord{
			Section: topic.Section,
			ID:      topic.ID,
			Title:   topic.Title,
			Path:    topic.Path,
			FSPath:  topic.FSPath,
		})
	}
	return out
}

func docsMatchesFromService(in []docssvc.SearchMatchView) []docsSearchMatchView {
	out := make([]docsSearchMatchView, 0, len(in))
	for _, match := range in {
		out = append(out, docsSearchMatchView{
			Section: match.Section,
			Topic:   match.Topic,
			Title:   match.Title,
			Path:    match.Path,
			Line:    match.Line,
			Snippet: match.Snippet,
		})
	}
	return out
}

func init() {
	docsSearchCmd.Flags().IntVarP(&docsSearchLimit, "limit", "n", 20, "Maximum number of matches")
	docsSearchCmd.Flags().StringVarP(&docsSearchSection, "section", "s", "", "Filter search to a docs section")
	docsFetchCmd.Flags().StringVar(&docsFetchRef, "ref", "", "Git ref for docs archive (branch, tag, or commit)")
	docsFetchCmd.Flags().StringVar(&docsFetchSource, "source", "", "Override docs archive base URL")

	docsCmd.AddCommand(docsListCmd)
	docsCmd.AddCommand(docsSearchCmd)
	docsCmd.AddCommand(docsFetchCmd)
	rootCmd.AddCommand(docsCmd)
}
