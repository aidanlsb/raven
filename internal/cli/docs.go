package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	builtindocs "github.com/aidanlsb/raven/docs"
	"github.com/aidanlsb/raven/internal/ui"
)

const (
	docsCommandHint = "For command docs, use: rvn help <command>"
	docsIndexPath   = "index.yaml"
)

var (
	docsSearchLimit   int
	docsSearchSection string

	docsLookPath         = exec.LookPath
	docsFZFRun           = runDocsFZF
	docsStdinIsTerminal  = func() bool { return isatty.IsTerminal(os.Stdin.Fd()) }
	docsStdoutIsTerminal = func() bool {
		return isatty.IsTerminal(os.Stdout.Fd())
	}
	docsDisplayContext = ui.NewDisplayContext
	docsMarkdownRender = ui.RenderMarkdown
)

type docsFZFRunFunc func(lines []string, prompt, header string) (selectionLine string, selected bool, err error)

type docsSectionView struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	TopicCount int    `json:"topic_count"`
	sortOrder  *int
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
	Section   string
	ID        string
	Title     string
	Path      string
	FSPath    string
	sortOrder *int
}

type docsIndex struct {
	Sections     map[string]docsIndexSection
	SectionOrder map[string]int
}

type docsIndexSection struct {
	Title      string `yaml:"title"`
	Topics     map[string]docsIndexTopicMeta
	TopicOrder map[string]int
}

type docsIndexTopicMeta struct {
	Title string `yaml:"title"`
	Path  string `yaml:"path"`
}

var docsCmd = &cobra.Command{
	Use:   "docs [section] [topic]",
	Short: "Browse long-form Markdown documentation",
	Long: `Browse long-form documentation bundled into the rvn binary.

Use this command for guides, references, and design notes.
When run in a terminal with fzf installed, 'rvn docs' opens an interactive selector.
For command-level usage, use 'rvn help <command>'.

Examples:
  rvn docs
  rvn docs list
  rvn docs <section>
  rvn docs <section> <topic>
  rvn docs search "saved query"
  rvn docs search refs --section reference`,
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sections, err := listBundledDocsSections()
		if err != nil {
			return handleError(ErrInternal, err, "Rebuild rvn so bundled docs are available")
		}

		if len(args) == 0 {
			if shouldUseDocsFZFNavigator() {
				if err := runDocsFZFNavigator(sections); err != nil {
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

		topics, err := listDocsTopicsFS(builtindocs.FS, ".", section.ID)
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

		return outputDocsTopicContent(topic)
	},
}

var docsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List docs sections and section commands",
	Long: `List docs sections with explicit section command syntax.

Use this to see exactly which 'rvn docs <section>' commands are available.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		sections, err := listBundledDocsSections()
		if err != nil {
			return handleError(ErrInternal, err, "Rebuild rvn so bundled docs are available")
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
		query := strings.TrimSpace(strings.Join(args, " "))
		if query == "" {
			return handleErrorMsg(ErrMissingArgument, "specify a search query", "Usage: rvn docs search <query>")
		}
		if docsSearchLimit < 1 {
			return handleErrorMsg(ErrInvalidInput, "--limit must be >= 1", "")
		}

		matches, err := searchDocsFS(builtindocs.FS, ".", query, docsSearchSection, docsSearchLimit)
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
	return nil
}

func outputDocsTopicContent(topic docsTopicRecord) error {
	content, err := fs.ReadFile(builtindocs.FS, topic.FSPath)
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
	if isJSONOutput() {
		return false
	}
	if !docsStdinIsTerminal() || !docsStdoutIsTerminal() {
		return false
	}
	_, err := docsLookPath("fzf")
	return err == nil
}

func runDocsFZFNavigator(sections []docsSectionView) error {
	section, ok, err := pickDocsSectionWithFZF(sections)
	if err != nil || !ok {
		return err
	}

	topics, err := listDocsTopicsFS(builtindocs.FS, ".", section.ID)
	if err != nil {
		return err
	}

	topic, ok, err := pickDocsTopicWithFZF(section, topics)
	if err != nil || !ok {
		return err
	}

	return outputDocsTopicContent(topic)
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
	if len(lines) == 0 {
		return "", false, nil
	}

	args := []string{
		"--layout=reverse",
		"--height=80%",
		"--border",
		"--prompt", prompt,
		"--delimiter", "\t",
		"--with-nth", "2..",
		"--select-1",
		"--exit-0",
	}
	if strings.TrimSpace(header) != "" {
		args = append(args, "--header", header)
	}

	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if code := exitErr.ExitCode(); code == 1 || code == 130 {
				return "", false, nil
			}
		}
		return "", false, fmt.Errorf("run fzf selector: %w", err)
	}

	selection := strings.TrimSpace(stdout.String())
	if selection == "" {
		return "", false, nil
	}
	return selection, true, nil
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
	return listDocsSectionsFS(os.DirFS(docsRoot), ".")
}

func listBundledDocsSections() ([]docsSectionView, error) {
	return listDocsSectionsFS(builtindocs.FS, ".")
}

func listDocsSectionsFS(docsFS fs.FS, docsRoot string) ([]docsSectionView, error) {
	index, err := loadDocsIndexFS(docsFS, docsRoot)
	if err != nil {
		return nil, err
	}
	if len(index.Sections) == 0 {
		return nil, fmt.Errorf("docs index has no sections")
	}

	sections := make([]docsSectionView, 0, len(index.Sections))
	for sectionID, meta := range index.Sections {
		topics, err := listDocsTopicsWithIndexFS(docsFS, docsRoot, sectionID, index)
		if err != nil {
			return nil, err
		}
		title := titleFromSlug(sectionID)
		if override := strings.TrimSpace(meta.Title); override != "" {
			title = override
		}
		sections = append(sections, docsSectionView{
			ID:         sectionID,
			Title:      title,
			TopicCount: len(topics),
			sortOrder:  docsSortOrder(index.SectionOrder, sectionID),
		})
	}

	sort.Slice(sections, func(i, j int) bool {
		return docsSortLess(sections[i].sortOrder, sections[j].sortOrder, sections[i].ID, sections[j].ID)
	})

	return sections, nil
}

func listDocsTopics(docsRoot, section string) ([]docsTopicRecord, error) {
	return listDocsTopicsFS(os.DirFS(docsRoot), ".", section)
}

func listDocsTopicsFS(docsFS fs.FS, docsRoot, section string) ([]docsTopicRecord, error) {
	index, err := loadDocsIndexFS(docsFS, docsRoot)
	if err != nil {
		return nil, err
	}
	return listDocsTopicsWithIndexFS(docsFS, docsRoot, section, index)
}

func listDocsTopicsWithIndexFS(docsFS fs.FS, docsRoot, section string, index docsIndex) ([]docsTopicRecord, error) {
	sectionMeta, ok := index.Sections[section]
	if !ok {
		return nil, fmt.Errorf("section %q is not declared in docs index", section)
	}

	sectionPath := path.Join(docsRoot, section)
	info, err := fs.Stat(docsFS, sectionPath)
	if err != nil {
		return nil, fmt.Errorf("section %q not found: %w", section, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("section path %q is not a directory", sectionPath)
	}

	records := make([]docsTopicRecord, 0, len(sectionMeta.Topics))
	seenPaths := make(map[string]string)

	for topicID, meta := range sectionMeta.Topics {
		normalizedID := normalizeDocsPathSlug(topicID)
		if normalizedID == "" || normalizedID != topicID {
			return nil, fmt.Errorf("docs index topic id %q in section %q must use normalized slug format", topicID, section)
		}

		relPath, fsPath, err := resolveDocsTopicPath(sectionPath, meta.Path)
		if err != nil {
			return nil, fmt.Errorf("docs index topic %q in section %q: %w", topicID, section, err)
		}
		if previousID, exists := seenPaths[relPath]; exists {
			return nil, fmt.Errorf("docs index section %q maps duplicate path %q to topics %q and %q", section, relPath, previousID, topicID)
		}
		seenPaths[relPath] = topicID

		fileInfo, err := fs.Stat(docsFS, fsPath)
		if err != nil {
			return nil, fmt.Errorf("docs index topic %q in section %q points to missing file %q: %w", topicID, section, relPath, err)
		}
		if fileInfo.IsDir() {
			return nil, fmt.Errorf("docs index topic %q in section %q path %q is a directory", topicID, section, relPath)
		}

		title := extractDocsTitleFS(docsFS, fsPath, topicID)
		if override := strings.TrimSpace(meta.Title); override != "" {
			title = override
		}
		records = append(records, docsTopicRecord{
			Section:   section,
			ID:        topicID,
			Title:     title,
			Path:      path.Join("docs", section, relPath),
			FSPath:    fsPath,
			sortOrder: docsSortOrder(sectionMeta.TopicOrder, topicID),
		})
	}

	sort.Slice(records, func(i, j int) bool {
		return docsSortLess(records[i].sortOrder, records[j].sortOrder, records[i].ID, records[j].ID)
	})
	return records, nil
}

func loadDocsIndexFS(docsFS fs.FS, docsRoot string) (docsIndex, error) {
	index := docsIndex{
		Sections:     make(map[string]docsIndexSection),
		SectionOrder: make(map[string]int),
	}
	raw, err := fs.ReadFile(docsFS, path.Join(docsRoot, docsIndexPath))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return docsIndex{}, fmt.Errorf("docs index not found at %s", path.Join(docsRoot, docsIndexPath))
		}
		return docsIndex{}, fmt.Errorf("read docs index: %w", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return docsIndex{}, fmt.Errorf("parse docs index: %w", err)
	}
	if len(root.Content) == 0 {
		return index, nil
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return docsIndex{}, fmt.Errorf("parse docs index: top-level YAML must be a mapping")
	}
	for i := 0; i+1 < len(top.Content); i += 2 {
		key := strings.TrimSpace(top.Content[i].Value)
		value := top.Content[i+1]
		switch key {
		case "sections":
			if err := decodeDocsSectionsNode(value, &index); err != nil {
				return docsIndex{}, fmt.Errorf("parse docs index sections: %w", err)
			}
		default:
			return docsIndex{}, fmt.Errorf("parse docs index: unknown top-level field %q", key)
		}
	}

	return index, nil
}

func resolveDocsTopicPath(sectionPath, rawPath string) (string, string, error) {
	relPath := strings.ReplaceAll(strings.TrimSpace(rawPath), "\\", "/")
	if relPath == "" {
		return "", "", fmt.Errorf("missing required field \"path\"")
	}
	cleanPath := path.Clean(relPath)
	if cleanPath == "." || cleanPath == "/" {
		return "", "", fmt.Errorf("invalid topic path %q", relPath)
	}
	if strings.HasPrefix(cleanPath, "/") || cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
		return "", "", fmt.Errorf("topic path %q must be relative to the section directory", relPath)
	}
	if strings.ToLower(filepath.Ext(cleanPath)) != ".md" {
		return "", "", fmt.Errorf("topic path %q must end with .md", relPath)
	}

	segments := strings.Split(cleanPath, "/")
	for _, segment := range segments {
		if !isPublicDocsName(segment) {
			return "", "", fmt.Errorf("topic path %q includes hidden/private segment %q", relPath, segment)
		}
	}

	return cleanPath, path.Join(sectionPath, cleanPath), nil
}

func decodeDocsSectionsNode(node *yaml.Node, index *docsIndex) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("sections must be a mapping")
	}

	position := 0
	for i := 0; i+1 < len(node.Content); i += 2 {
		sectionID := strings.TrimSpace(node.Content[i].Value)
		if sectionID == "" {
			return fmt.Errorf("sections contains an empty section key")
		}
		if normalizeDocsSegment(sectionID) != sectionID {
			return fmt.Errorf("section id %q must use normalized slug format", sectionID)
		}
		if _, exists := index.Sections[sectionID]; exists {
			return fmt.Errorf("duplicate section %q", sectionID)
		}

		sectionNode := node.Content[i+1]
		if sectionNode.Kind != yaml.MappingNode {
			return fmt.Errorf("section %q must be a mapping", sectionID)
		}

		section := docsIndexSection{
			Topics:     make(map[string]docsIndexTopicMeta),
			TopicOrder: make(map[string]int),
		}
		hasTopics := false
		for j := 0; j+1 < len(sectionNode.Content); j += 2 {
			field := strings.TrimSpace(sectionNode.Content[j].Value)
			value := sectionNode.Content[j+1]
			switch field {
			case "title":
				var title string
				if err := value.Decode(&title); err != nil {
					return fmt.Errorf("section %q field %q: %w", sectionID, field, err)
				}
				section.Title = strings.TrimSpace(title)
			case "topics":
				if err := decodeDocsTopicsNode(value, sectionID, &section); err != nil {
					return err
				}
				hasTopics = true
			default:
				return fmt.Errorf("section %q has unknown field %q", sectionID, field)
			}
		}
		if !hasTopics {
			return fmt.Errorf("section %q is missing required field \"topics\"", sectionID)
		}

		index.Sections[sectionID] = section
		index.SectionOrder[sectionID] = position
		position++
	}
	return nil
}

func decodeDocsTopicsNode(node *yaml.Node, sectionID string, section *docsIndexSection) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("section %q topics must be a mapping", sectionID)
	}

	position := 0
	for i := 0; i+1 < len(node.Content); i += 2 {
		topicID := strings.TrimSpace(node.Content[i].Value)
		if topicID == "" {
			return fmt.Errorf("section %q topics contains an empty topic key", sectionID)
		}
		if normalizeDocsPathSlug(topicID) != topicID {
			return fmt.Errorf("topic id %q in section %q must use normalized slug format", topicID, sectionID)
		}
		if _, exists := section.Topics[topicID]; exists {
			return fmt.Errorf("duplicate topic %q in section %q", topicID, sectionID)
		}

		var meta docsIndexTopicMeta
		if err := node.Content[i+1].Decode(&meta); err != nil {
			return fmt.Errorf("topic %q metadata in section %q: %w", topicID, sectionID, err)
		}
		meta.Title = strings.TrimSpace(meta.Title)
		meta.Path = strings.TrimSpace(meta.Path)
		if meta.Path == "" {
			return fmt.Errorf("topic %q in section %q is missing required field \"path\"", topicID, sectionID)
		}

		section.Topics[topicID] = meta
		section.TopicOrder[topicID] = position
		position++
	}

	if len(section.Topics) == 0 {
		return fmt.Errorf("section %q has an empty topics mapping", sectionID)
	}
	return nil
}

func docsSortOrder(orderByID map[string]int, id string) *int {
	order, ok := orderByID[id]
	if !ok {
		return nil
	}
	out := order
	return &out
}

func docsSortLess(orderA, orderB *int, idA, idB string) bool {
	if orderA == nil && orderB == nil {
		return idA < idB
	}
	if orderA == nil {
		return false
	}
	if orderB == nil {
		return true
	}
	if *orderA != *orderB {
		return *orderA < *orderB
	}
	return idA < idB
}

func findDocsSection(sections []docsSectionView, raw string) (docsSectionView, bool) {
	needle := normalizeDocsSegment(raw)
	for _, section := range sections {
		if normalizeDocsSegment(section.ID) == needle {
			return section, true
		}
	}
	return docsSectionView{}, false
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

func searchDocs(docsRoot, query, sectionFilter string, limit int) ([]docsSearchMatchView, error) {
	return searchDocsFS(os.DirFS(docsRoot), ".", query, sectionFilter, limit)
}

func searchDocsFS(docsFS fs.FS, docsRoot, query, sectionFilter string, limit int) ([]docsSearchMatchView, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}
	if limit < 1 {
		return nil, fmt.Errorf("limit must be >= 1")
	}

	sections, err := listDocsSectionsFS(docsFS, docsRoot)
	if err != nil {
		return nil, err
	}

	selected := make([]docsSectionView, 0)
	if strings.TrimSpace(sectionFilter) == "" {
		selected = sections
	} else {
		section, ok := findDocsSection(sections, sectionFilter)
		if !ok {
			return nil, fmt.Errorf("unknown section: %s", sectionFilter)
		}
		selected = append(selected, section)
	}

	queryLower := strings.ToLower(query)
	matches := make([]docsSearchMatchView, 0, limit)

	for _, section := range selected {
		topics, err := listDocsTopicsFS(docsFS, docsRoot, section.ID)
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
					Section: section.ID,
					Topic:   topic.ID,
					Title:   topic.Title,
					Path:    topic.Path,
					Line:    i + 1,
					Snippet: shortenDocsSnippet(line, queryLower),
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

func init() {
	docsSearchCmd.Flags().IntVarP(&docsSearchLimit, "limit", "n", 20, "Maximum number of matches")
	docsSearchCmd.Flags().StringVarP(&docsSearchSection, "section", "s", "", "Filter search to a docs section")

	docsCmd.AddCommand(docsListCmd)
	docsCmd.AddCommand(docsSearchCmd)
	rootCmd.AddCommand(docsCmd)
}
