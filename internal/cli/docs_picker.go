package cli

import (
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/docssvc"
	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/versioninfo"
)

type docsSectionView = docssvc.SectionView
type docsTopicRecord = docssvc.TopicRecord

func prepareDocsCommand(_ *cobra.Command, args []string) ([]string, bool, error) {
	if len(args) != 0 || !shouldUseDocsPickerNavigator() {
		return args, false, nil
	}

	source, err := loadGlobalDocsSource(getConfigPath())
	if err != nil {
		return nil, false, handleError(ErrFileNotFound, err, "Run 'rvn docs fetch' to download docs")
	}
	outputDocsServiceWarnings(source.Warnings)

	sections, err := listDocsSectionsFS(source.FS, ".")
	if err != nil {
		return nil, false, handleError(ErrInternal, err, "Run 'rvn docs fetch' to refresh docs")
	}
	if err := runDocsPickerNavigator(source.FS, sections); err != nil {
		return nil, false, handleError(ErrInternal, err, "Run 'rvn docs --json' for non-interactive output")
	}
	return nil, true, nil
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

	selected, ok, err := cliSelector.docsSection(items)
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

	title := fmt.Sprintf("Select a topic in %s [%s]", section.Title, section.ID)
	prompt := fmt.Sprintf("docs/%s", section.ID)
	selected, ok, err := cliSelector.docsTopic(items, title, prompt)
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

func outputDocsTopicContent(docsFS fs.FS, topic docsTopicRecord) error {
	content, err := docssvc.ReadTopicContentFS(docsFS, topic)
	if err != nil {
		return handleError(ErrFileRead, err, "")
	}
	return renderDocsTopicContent(topic.Path, content)
}

func loadGlobalDocsSource(configPath string) (*docssvc.GlobalDocsSource, error) {
	return docssvc.LoadGlobalDocsSource(configPath, versioninfo.Current().Version)
}

func listDocsSectionsFS(docsFS fs.FS, docsRoot string) ([]docsSectionView, error) {
	return docssvc.ListSectionsFS(docsFS, docsRoot)
}

func listDocsTopicsFS(docsFS fs.FS, docsRoot, section string) ([]docsTopicRecord, error) {
	return docssvc.ListTopicsFS(docsFS, docsRoot, section)
}

func findDocsSection(sections []docsSectionView, raw string) (docsSectionView, bool) {
	return docssvc.FindSection(sections, raw)
}

func findDocsTopic(topics []docsTopicRecord, raw string) (docsTopicRecord, bool) {
	return docssvc.FindTopic(topics, raw)
}

func outputDocsServiceWarnings(warnings []docssvc.Warning) {
	for _, warning := range warnings {
		fmt.Fprintln(os.Stderr, ui.Warningf("%s: %s", warning.Code, warning.Message))
	}
}
