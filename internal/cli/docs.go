package cli

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/docssvc"
	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/ui"
)

const (
	docsCommandHint = "For command docs, use: rvn help <command>"
)

var (
	docsRunPicker      = picker.Run
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
	Long: `Browse long-form documentation stored in Raven's global docs directory.

Use this command for guides, references, and design notes.
Run 'rvn docs fetch' to sync or refresh docs content.
When run in an interactive terminal, 'rvn docs' opens Raven's picker.
In the picker, use l to move forward into a section/topic and h to go back.
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
		if len(args) == 0 {
			if !isJSONOutput() && shouldUseDocsPickerNavigator() {
				source, err := loadGlobalDocsSource(getConfigPath())
				if err != nil {
					return handleError(ErrFileNotFound, err, "Run 'rvn docs fetch' to download docs")
				}

				sections, err := listDocsSectionsFS(source, ".")
				if err != nil {
					return handleError(ErrInternal, err, "Run 'rvn docs fetch' to refresh docs")
				}

				if err := runDocsPickerNavigator(source, sections); err != nil {
					return handleError(ErrInternal, err, "Run 'rvn docs list' for non-interactive output")
				}
				return nil
			}
		}

		argsMap := map[string]interface{}{}
		if len(args) > 0 {
			argsMap["section"] = args[0]
		}
		if len(args) > 1 {
			argsMap["topic"] = args[1]
		}

		result := executeCanonicalCommand("docs", "", argsMap)
		if !result.OK {
			return handleCanonicalDocsFailure(result, args)
		}

		if isJSONOutput() {
			outputJSON(result)
			return nil
		}

		data := canonicalDataMap(result)
		switch len(args) {
		case 0:
			return outputDocsSections(docsSectionsFromCanonical(data["sections"]))
		case 1:
			section := docsSectionView{
				ID:    stringValue(data["section"]),
				Title: stringValue(data["title"]),
			}
			return outputDocsTopics(section, docsTopicsFromCanonical(data["topics"], section.ID))
		default:
			return outputDocsTopicContentData(data)
		}
	},
}

var docsListCmd = newCanonicalLeafCommand("docs_list", canonicalLeafOptions{
	HandleError: handleCanonicalDocsLeafFailure,
	RenderHuman: renderDocsList,
})

var docsSearchCmd = newCanonicalLeafCommand("docs_search", canonicalLeafOptions{
	Args:        cobra.MinimumNArgs(1),
	BuildArgs:   buildDocsSearchArgs,
	HandleError: handleCanonicalDocsLeafFailure,
	RenderHuman: renderDocsSearch,
})

var docsFetchCmd = newCanonicalLeafCommand("docs_fetch", canonicalLeafOptions{
	HandleError: handleCanonicalDocsLeafFailure,
	RenderHuman: renderDocsFetch,
})

func handleCanonicalDocsLeafFailure(result commandexec.Result) error {
	return handleCanonicalDocsFailure(result, nil)
}

func renderDocsList(_ *cobra.Command, result commandexec.Result) error {
	return outputDocsSections(docsSectionsFromCanonical(canonicalDataMap(result)["sections"]))
}

func renderDocsFetch(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Println(ui.Checkf("Fetched docs to %s (%d files, %d bytes)", ui.FilePath(stringValue(data["path"])), intValue(data["file_count"]), int64Value(data["byte_count"])))
	fmt.Printf("%s %s %s\n", ui.Hint("Source:"), stringValue(data["source"]), ui.Hint("("+stringValue(data["ref"])+")"))
	return nil
}

func buildDocsSearchArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	query := strings.TrimSpace(strings.Join(args, " "))
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	section, _ := cmd.Flags().GetString("section")
	return map[string]interface{}{
		"query":   query,
		"section": section,
		"limit":   limit,
		"offset":  offset,
	}, nil
}

func renderDocsSearch(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	matches := docsSearchMatchesFromCanonical(data["matches"])
	if len(matches) == 0 {
		fmt.Println(ui.Starf("No docs matched %q.", stringValue(data["query"])))
		return nil
	}

	fmt.Printf("%s\n", ui.SectionHeader(fmt.Sprintf("Matches for %q (%d)", stringValue(data["query"]), len(matches))))
	for _, m := range matches {
		fmt.Println(ui.Bullet(fmt.Sprintf("%s/%s:%d %s", m.Section, m.Topic, m.Line, m.Snippet)))
	}
	if boolValue(data["has_more"]) {
		nextOffset := intValue(data["offset"]) + intValue(data["returned"])
		fmt.Println()
		fmt.Println(ui.Hint(fmt.Sprintf("More matches available. Continue with --offset %d.", nextOffset)))
	}
	return nil
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

	fmt.Println(ui.SectionHeader("Documentation section commands"))
	for _, s := range sections {
		sectionCommand := fmt.Sprintf("rvn docs %s", s.ID)
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s %s", ui.Bold.Render(sectionCommand), s.Title, ui.Hint("("+docsTopicCountSummary(s.TopicCount)+")"))))
	}
	fmt.Println()
	fmt.Println(ui.SectionHeader("General docs commands"))
	fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs list"), ui.Hint("List sections and section commands"))))
	fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs <section>"), ui.Hint("List topics in a section"))))
	fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs <section> <topic>"), ui.Hint("Open a docs topic"))))
	fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs search <query>"), ui.Hint("Search docs"))))
	fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs fetch"), ui.Hint("Sync global docs"))))
	fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn help <command>"), ui.Hint("Command docs"))))
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

	fmt.Println(ui.SectionHeader(fmt.Sprintf("Documentation topic commands for %s [%s]", section.Title, section.ID)))
	if len(topics) == 0 {
		fmt.Println(ui.Bullet(ui.Hint("(no topics)")))
		fmt.Println()
		fmt.Println(ui.SectionHeader("General docs commands"))
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs list"), ui.Hint("List sections and section commands"))))
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render(fmt.Sprintf("rvn docs search <query> --section %s", section.ID)), ui.Hint("Search only this section"))))
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs fetch"), ui.Hint("Sync global docs"))))
		return nil
	}
	for _, t := range topics {
		topicCommand := fmt.Sprintf("rvn docs %s %s", section.ID, t.ID)
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render(topicCommand), ui.Hint(t.Title))))
	}
	fmt.Println()
	fmt.Println(ui.SectionHeader("General docs commands"))
	fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render(fmt.Sprintf("rvn docs %s", section.ID)), ui.Hint("List topics in this section"))))
	fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render(fmt.Sprintf("rvn docs search <query> --section %s", section.ID)), ui.Hint("Search only this section"))))
	fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs list"), ui.Hint("List sections and section commands"))))
	fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs fetch"), ui.Hint("Sync global docs"))))
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

	fmt.Printf("%s %s\n\n", ui.Hint("Path:"), ui.FilePath(topic.Path))
	fmt.Print(renderedContent)
	if !strings.HasSuffix(renderedContent, "\n") {
		fmt.Println()
	}
	return nil
}

func outputDocsTopicContentData(data map[string]interface{}) error {
	if isJSONOutput() {
		outputSuccess(data, nil)
		return nil
	}

	renderedContent := stringValue(data["content"])
	display := docsDisplayContext()
	if display.IsTTY {
		if rendered, renderErr := docsMarkdownRender(renderedContent, display.TermWidth); renderErr == nil {
			renderedContent = rendered
		}
	}

	fmt.Printf("%s %s\n\n", ui.Hint("Path:"), ui.FilePath(stringValue(data["path"])))
	fmt.Print(renderedContent)
	if !strings.HasSuffix(renderedContent, "\n") {
		fmt.Println()
	}
	return nil
}

func shouldUseDocsPickerNavigator() bool {
	return canUseRavenInteractive()
}

func runDocsPickerNavigator(docsFS fs.FS, sections []docsSectionView) error {
	for {
		section, ok, err := pickDocsSection(sections)
		if err != nil || !ok {
			return err
		}

		topics, err := listDocsTopicsFS(docsFS, ".", section.ID)
		if err != nil {
			return err
		}

		for {
			topic, action, ok, err := pickDocsTopic(section, topics)
			if err != nil || !ok {
				return err
			}
			if action == picker.ActionBack {
				break
			}

			return outputDocsTopicContent(docsFS, topic)
		}
	}
}

// docsNavigationItem builds a picker item for navigating the docs tree. The id
// is the navigation key returned on selection (a section or topic ID); title,
// location, and columns are display-only. Matching covers every column.
func docsNavigationItem(id, title, location string, columns []string) picker.Item {
	return picker.Item{
		ID:         id,
		Label:      title,
		Detail:     id,
		Location:   location,
		Columns:    columns,
		SearchText: browseSearchText(columns...),
	}
}

func pickDocsSection(sections []docsSectionView) (docsSectionView, bool, error) {
	items := make([]picker.Item, 0, len(sections))
	for _, section := range sections {
		topicCount := docsTopicCountSummary(section.TopicCount)
		items = append(items, docsNavigationItem(section.ID, section.Title, "", []string{section.ID, section.Title, topicCount}))
	}

	selected, ok, err := docsRunPicker(items, picker.Options{
		Title:        "Select a docs section",
		Prompt:       "docs/section",
		Headers:      []string{"#", "section", "title", "topics"},
		Columns:      ui.SearchLayout(),
		AllowForward: true,
		Shortcuts: []picker.ShortcutTip{
			{Key: "j/k", Description: "move"},
			{Key: "l", Description: "topics"},
			{Key: "enter", Description: "topics"},
			{Key: "/ or i", Description: "filter"},
			{Key: "q", Description: "cancel"},
		},
	})
	if err != nil {
		return docsSectionView{}, false, err
	}
	if !ok {
		return docsSectionView{}, false, nil
	}

	sectionID := strings.TrimSpace(selected.Item.ID)
	section, ok := findDocsSection(sections, sectionID)
	if !ok {
		return docsSectionView{}, false, fmt.Errorf("selected unknown docs section %q", sectionID)
	}
	return section, true, nil
}

func pickDocsTopic(section docsSectionView, topics []docsTopicRecord) (docsTopicRecord, picker.Action, bool, error) {
	items := make([]picker.Item, 0, len(topics))
	for _, topic := range topics {
		items = append(items, docsNavigationItem(topic.ID, topic.Title, topic.Path, []string{topic.ID, topic.Title, topic.Path}))
	}

	prompt := fmt.Sprintf("docs/%s> ", section.ID)
	selected, ok, err := docsRunPicker(items, picker.Options{
		Title:        fmt.Sprintf("Select a topic in %s [%s]", section.Title, section.ID),
		Prompt:       strings.TrimSuffix(prompt, "> "),
		Headers:      []string{"#", "topic", "title", "path"},
		Columns:      ui.SearchLayout(),
		AllowForward: true,
		AllowBack:    true,
		Shortcuts: []picker.ShortcutTip{
			{Key: "j/k", Description: "move"},
			{Key: "h", Description: "sections"},
			{Key: "l", Description: "open"},
			{Key: "enter", Description: "open"},
			{Key: "/ or i", Description: "filter"},
			{Key: "q", Description: "cancel"},
		},
	})
	if err != nil {
		return docsTopicRecord{}, "", false, err
	}
	if !ok {
		return docsTopicRecord{}, "", false, nil
	}
	if selected.Action == picker.ActionBack {
		return docsTopicRecord{}, picker.ActionBack, true, nil
	}

	topicID := strings.TrimSpace(selected.Item.ID)
	topic, ok := findDocsTopic(topics, topicID)
	if !ok {
		return docsTopicRecord{}, "", false, fmt.Errorf("selected unknown docs topic %q in section %q", topicID, section.ID)
	}
	return topic, selected.Action, true, nil
}

func handleCanonicalDocsFailure(result commandexec.Result, args []string) error {
	if result.Error == nil {
		if isJSONOutput() {
			outputJSON(result)
			return nil
		}
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	message := result.Error.Message
	suggestion := result.Error.Suggestion
	if len(args) > 0 && result.Error.Code == ErrInvalidInput && strings.HasPrefix(result.Error.Message, "unknown docs section: ") {
		if cmdPath, ok := resolveCLICommandPath(args); ok {
			message = fmt.Sprintf("%q is a CLI command, not a docs section", cmdPath)
			suggestion = fmt.Sprintf("Use 'rvn help %s' for command documentation", cmdPath)
		} else if isCommandSectionAlias(args[0]) {
			message = "command docs are not part of 'rvn docs'"
			suggestion = docsCommandHint
		}
	}

	if isJSONOutput() {
		result.Error.Message = message
		result.Error.Suggestion = suggestion
		outputJSON(result)
		return nil
	}

	return handleErrorWithDetails(result.Error.Code, message, suggestion, result.Error.Details)
}

func docsSectionsFromCanonical(raw interface{}) []docsSectionView {
	switch sections := raw.(type) {
	case []docssvc.SectionView:
		out := make([]docsSectionView, 0, len(sections))
		for _, section := range sections {
			out = append(out, docsSectionView{
				ID:         section.ID,
				Title:      section.Title,
				TopicCount: section.TopicCount,
			})
		}
		return out
	case []docsSectionView:
		return sections
	default:
		return nil
	}
}

func docsTopicsFromCanonical(raw interface{}, sectionID string) []docsTopicRecord {
	items, _ := raw.([]map[string]interface{})
	out := make([]docsTopicRecord, 0, len(items))
	for _, item := range items {
		out = append(out, docsTopicRecord{
			Section: sectionID,
			ID:      stringValue(item["id"]),
			Title:   stringValue(item["title"]),
			Path:    stringValue(item["path"]),
		})
	}
	return out
}

func docsSearchMatchesFromCanonical(raw interface{}) []docssvc.SearchMatchView {
	switch matches := raw.(type) {
	case []docssvc.SearchMatchView:
		return matches
	default:
		return nil
	}
}

func int64Value(raw interface{}) int64 {
	switch value := raw.(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func loadGlobalDocsSource(configPath string) (fs.FS, error) {
	return docssvc.LoadGlobalDocsSource(configPath)
}

func listDocsSectionsFS(docsFS fs.FS, docsRoot string) ([]docsSectionView, error) {
	sections, err := docssvc.ListSectionsFS(docsFS, docsRoot)
	if err != nil {
		return nil, err
	}
	return docsSectionsFromService(sections), nil
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

func init() {
	docsCmd.AddCommand(docsListCmd)
	docsCmd.AddCommand(docsSearchCmd)
	docsCmd.AddCommand(docsFetchCmd)
	rootCmd.AddCommand(docsCmd)
}
