package mcp

import (
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/docssvc"
	"github.com/aidanlsb/raven/internal/maintsvc"
)

func mapDirectDocsSvcError(err error, fallbackSuggestion string) (string, bool) {
	svcErr, ok := docssvc.AsError(err)
	if !ok {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), fallbackSuggestion, nil), true
	}

	suggestion := svcErr.Suggestion
	if suggestion == "" {
		suggestion = fallbackSuggestion
	}

	switch svcErr.Code {
	case docssvc.CodeInvalidInput:
		return errorEnvelope("INVALID_INPUT", svcErr.Message, suggestion, nil), true
	case docssvc.CodeNotFound:
		return errorEnvelope("FILE_NOT_FOUND", svcErr.Message, suggestion, nil), true
	case docssvc.CodeFileRead:
		return errorEnvelope("FILE_READ_ERROR", svcErr.Message, suggestion, nil), true
	case docssvc.CodeFetchFailed:
		return errorEnvelope("INTERNAL_ERROR", svcErr.Message, suggestion, nil), true
	default:
		return errorEnvelope("INTERNAL_ERROR", svcErr.Message, suggestion, nil), true
	}
}

func directDocsSectionsData(sections []docssvc.SectionView) map[string]interface{} {
	return map[string]interface{}{
		"sections":       sections,
		"command_docs":   "rvn help <command>",
		"navigation_tip": "rvn docs <section> <topic>",
	}
}

func directDocsTopicItems(topics []docssvc.TopicRecord) []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(topics))
	for _, topic := range topics {
		items = append(items, map[string]interface{}{
			"id":    topic.ID,
			"title": topic.Title,
			"path":  topic.Path,
		})
	}
	return items
}

func directDocsSectionNotFound(sectionInput string, sections []docssvc.SectionView) (string, bool) {
	available := make([]string, 0, len(sections))
	for _, section := range sections {
		available = append(available, section.ID)
	}
	sort.Strings(available)

	return errorEnvelope(
		"INVALID_INPUT",
		"unknown docs section: "+sectionInput,
		"Run 'rvn docs' to list sections (available: "+strings.Join(available, ", ")+")",
		nil,
	), true
}

func directDocsTopicNotFound(sectionID, topicInput string, topics []docssvc.TopicRecord) (string, bool) {
	available := make([]string, 0, len(topics))
	for _, topic := range topics {
		available = append(available, topic.ID)
	}
	sort.Strings(available)

	suggestion := "Run 'rvn docs " + sectionID + "' to list topics"
	if len(available) > 0 {
		suggestion += " (available: " + strings.Join(available, ", ") + ")"
	}

	return errorEnvelope(
		"INVALID_INPUT",
		"unknown topic \""+topicInput+"\" in section \""+sectionID+"\"",
		suggestion,
		nil,
	), true
}

func (s *Server) callDirectDocs(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)
	sectionInput := strings.TrimSpace(toString(normalized["section"]))
	topicInput := strings.TrimSpace(toString(normalized["topic"]))

	source, sourceErr := docssvc.LoadVaultDocsSource(vaultPath)
	if sourceErr != nil {
		return mapDirectDocsSvcError(sourceErr, "Run 'rvn docs fetch' to download docs for this vault")
	}

	sections, sectionsErr := docssvc.ListSectionsFS(source, ".")
	if sectionsErr != nil {
		return mapDirectDocsSvcError(sectionsErr, "Run 'rvn docs fetch' to refresh docs")
	}

	if sectionInput == "" {
		if topicInput != "" {
			return errorEnvelope("INVALID_INPUT", "section is required when topic is provided", "Provide section and topic together", nil), true
		}
		return successEnvelope(directDocsSectionsData(sections), nil), false
	}

	section, ok := docssvc.FindSection(sections, sectionInput)
	if !ok {
		return directDocsSectionNotFound(sectionInput, sections)
	}

	topics, topicsErr := docssvc.ListTopicsFS(source, ".", section.ID)
	if topicsErr != nil {
		return mapDirectDocsSvcError(topicsErr, "Run 'rvn docs fetch' to refresh docs")
	}

	if topicInput == "" {
		return successEnvelope(map[string]interface{}{
			"section": section.ID,
			"title":   section.Title,
			"topics":  directDocsTopicItems(topics),
		}, nil), false
	}

	topic, ok := docssvc.FindTopic(topics, topicInput)
	if !ok {
		return directDocsTopicNotFound(section.ID, topicInput, topics)
	}

	content, readErr := docssvc.ReadTopicContentFS(source, topic)
	if readErr != nil {
		return mapDirectDocsSvcError(readErr, "Run 'rvn docs fetch' to refresh docs")
	}

	return successEnvelope(map[string]interface{}{
		"section": topic.Section,
		"topic":   topic.ID,
		"title":   topic.Title,
		"path":    topic.Path,
		"content": content,
	}, nil), false
}

func (s *Server) callDirectDocsList(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}

	sections, svcErr := docssvc.ListSections(vaultPath)
	if svcErr != nil {
		return mapDirectDocsSvcError(svcErr, "Run 'rvn docs fetch' to download docs for this vault")
	}
	return successEnvelope(directDocsSectionsData(sections), nil), false
}

func (s *Server) callDirectDocsSearch(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)
	query := strings.TrimSpace(toString(normalized["query"]))
	if query == "" {
		return errorEnvelope("MISSING_ARGUMENT", "specify a search query", "Usage: rvn docs search <query>", nil), true
	}

	limit := intValueDefault(normalized["limit"], 20)
	if limit < 1 {
		return errorEnvelope("INVALID_INPUT", "--limit must be >= 1", "", nil), true
	}

	matches, svcErr := docssvc.Search(vaultPath, query, strings.TrimSpace(toString(normalized["section"])), limit)
	if svcErr != nil {
		return mapDirectDocsSvcError(svcErr, "Run 'rvn docs' to list sections")
	}

	return successEnvelope(map[string]interface{}{
		"query":   query,
		"count":   len(matches),
		"matches": matches,
	}, nil), false
}

func (s *Server) callDirectDocsFetch(args map[string]interface{}) (string, bool) {
	vaultPath, err := s.resolveVaultPath()
	if err != nil {
		return errorEnvelope("VAULT_RESOLUTION_FAILED", "failed to resolve active vault", err.Error(), nil), true
	}
	normalized := normalizeArgs(args)
	result, svcErr := docssvc.Fetch(docssvc.FetchRequest{
		VaultPath:  vaultPath,
		Ref:        strings.TrimSpace(toString(normalized["ref"])),
		SourceBase: strings.TrimSpace(toString(normalized["source"])),
		CLIVersion: maintsvc.CurrentVersionInfoFromExecutable(s.executable).Version,
	})
	if svcErr != nil {
		return mapDirectDocsSvcError(svcErr, "Check your network connection and run 'rvn docs fetch' again")
	}

	return successEnvelope(map[string]interface{}{
		"path":         result.Path,
		"file_count":   result.FileCount,
		"byte_count":   result.ByteCount,
		"source":       result.Source,
		"ref":          result.Ref,
		"archive_url":  result.ArchiveURL,
		"fetched_at":   result.FetchedAt,
		"cli_version":  result.CLIVersion,
		"manifest_ver": result.ManifestVer,
	}, nil), false
}
