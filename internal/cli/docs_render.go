package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/docssvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	docsDisplayContext = ui.NewDisplayContext
	docsMarkdownRender = ui.RenderMarkdown
)

func renderDocsCommand(_ *cobra.Command, result commandexec.Result) error {
	outputCanonicalDocsWarnings(result.Warnings)
	data := canonicalDataMap(result)
	if _, ok := data["content"]; ok {
		return renderDocsTopicContent(stringValue(data["path"]), stringValue(data["content"]))
	}
	if _, ok := data["topics"]; ok {
		section := docsSectionView{
			ID:    stringValue(data["section"]),
			Title: stringValue(data["title"]),
		}
		return outputDocsTopics(section, docsTopicsFromCanonical(data["topics"], section.ID))
	}
	return outputDocsSections(docsSectionsFromCanonical(data["sections"]))
}

func renderDocsList(_ *cobra.Command, result commandexec.Result) error {
	outputCanonicalDocsWarnings(result.Warnings)
	return outputDocsSections(docsSectionsFromCanonical(canonicalDataMap(result)["sections"]))
}

func renderDocsFetch(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Println(ui.Checkf(
		"Fetched docs to %s (%d files, %d bytes)",
		ui.FilePath(stringValue(data["path"])),
		intValue(data["file_count"]),
		int64Value(data["byte_count"]),
	))
	fmt.Printf("%s %s %s\n", ui.Hint("Source:"), stringValue(data["source"]), ui.Hint("("+stringValue(data["ref"])+")"))
	return nil
}

func renderDocsSearch(_ *cobra.Command, result commandexec.Result) error {
	outputCanonicalDocsWarnings(result.Warnings)
	data := canonicalDataMap(result)
	matches := docsSearchMatchesFromCanonical(data["items"])
	if len(matches) == 0 {
		fmt.Println(ui.Starf("No docs matched %q.", stringValue(data["query"])))
		return nil
	}

	fmt.Printf("%s\n", ui.SectionHeader(fmt.Sprintf("Matches for %q (%d)", stringValue(data["query"]), len(matches))))
	for _, match := range matches {
		fmt.Println(ui.Bullet(fmt.Sprintf("%s/%s:%d %s", match.Section, match.Topic, match.Line, match.Snippet)))
	}
	if boolValue(data["has_more"]) {
		nextOffset := intValue(data["offset"]) + intValue(data["returned"])
		fmt.Println()
		fmt.Println(ui.Hint(fmt.Sprintf("More matches available. Continue with --offset %d.", nextOffset)))
	}
	return nil
}

func outputDocsSections(sections []docsSectionView) error {
	fmt.Println(ui.SectionHeader("Documentation section commands"))
	for _, section := range sections {
		sectionCommand := fmt.Sprintf("rvn docs %s", section.ID)
		fmt.Println(ui.Bullet(fmt.Sprintf(
			"%s %s %s",
			ui.Bold.Render(sectionCommand),
			section.Title,
			ui.Hint("("+docsTopicCountSummary(section.TopicCount)+")"),
		)))
	}
	fmt.Println()
	printDocsGeneralCommands("", false)
	return nil
}

func outputDocsTopics(section docsSectionView, topics []docsTopicRecord) error {
	fmt.Println(ui.SectionHeader(fmt.Sprintf("Documentation topic commands for %s [%s]", section.Title, section.ID)))
	if len(topics) == 0 {
		fmt.Println(ui.Bullet(ui.Hint("(no topics)")))
		fmt.Println()
		printDocsGeneralCommands(section.ID, false)
		return nil
	}
	for _, topic := range topics {
		topicCommand := fmt.Sprintf("rvn docs %s %s", section.ID, topic.ID)
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render(topicCommand), ui.Hint(topic.Title))))
	}
	fmt.Println()
	printDocsGeneralCommands(section.ID, true)
	return nil
}

func printDocsGeneralCommands(sectionID string, includeSectionCommand bool) {
	fmt.Println(ui.SectionHeader("General docs commands"))
	if sectionID == "" {
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs list"), ui.Hint("List sections and section commands"))))
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs <section>"), ui.Hint("List topics in a section"))))
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs <section> <topic>"), ui.Hint("Open a docs topic"))))
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs search <query>"), ui.Hint("Search docs"))))
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs fetch"), ui.Hint("Sync global docs"))))
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn help <command>"), ui.Hint("Command docs"))))
		return
	}

	if !includeSectionCommand {
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs list"), ui.Hint("List sections and section commands"))))
		fmt.Println(ui.Bullet(fmt.Sprintf(
			"%s %s",
			ui.Bold.Render(fmt.Sprintf("rvn docs search <query> --section %s", sectionID)),
			ui.Hint("Search only this section"),
		)))
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs fetch"), ui.Hint("Sync global docs"))))
		return
	}

	fmt.Println(ui.Bullet(fmt.Sprintf(
		"%s %s",
		ui.Bold.Render(fmt.Sprintf("rvn docs %s", sectionID)),
		ui.Hint("List topics in this section"),
	)))
	fmt.Println(ui.Bullet(fmt.Sprintf(
		"%s %s",
		ui.Bold.Render(fmt.Sprintf("rvn docs search <query> --section %s", sectionID)),
		ui.Hint("Search only this section"),
	)))
	fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs list"), ui.Hint("List sections and section commands"))))
	fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render("rvn docs fetch"), ui.Hint("Sync global docs"))))
}

func renderDocsTopicContent(topicPath, content string) error {
	renderedContent := content
	display := docsDisplayContext()
	if display.IsTTY {
		if rendered, err := docsMarkdownRender(content, display.TermWidth); err == nil {
			renderedContent = rendered
		}
	}

	fmt.Printf("%s %s\n\n", ui.Hint("Path:"), ui.FilePath(topicPath))
	fmt.Print(renderedContent)
	if !strings.HasSuffix(renderedContent, "\n") {
		fmt.Println()
	}
	return nil
}

func docsSectionsFromCanonical(raw interface{}) []docsSectionView {
	sections, _ := raw.([]docssvc.SectionView)
	return sections
}

func docsTopicsFromCanonical(raw interface{}, sectionID string) []docsTopicRecord {
	items, _ := raw.([]map[string]interface{})
	topics := make([]docsTopicRecord, 0, len(items))
	for _, item := range items {
		topics = append(topics, docsTopicRecord{
			Section: sectionID,
			ID:      stringValue(item["id"]),
			Title:   stringValue(item["title"]),
			Path:    stringValue(item["path"]),
		})
	}
	return topics
}

func docsSearchMatchesFromCanonical(raw interface{}) []docssvc.SearchMatchView {
	matches, _ := raw.([]docssvc.SearchMatchView)
	return matches
}

func outputCanonicalDocsWarnings(warnings []commandexec.Warning) {
	for _, warning := range warnings {
		fmt.Fprintln(os.Stderr, ui.Warningf("%s: %s", warning.Code, warning.Message))
	}
}

func docsTopicCountSummary(topicCount int) string {
	if topicCount == 1 {
		return "1 topic"
	}
	return fmt.Sprintf("%d topics", topicCount)
}
