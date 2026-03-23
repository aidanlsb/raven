package commandimpl

import (
	"context"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/docssvc"
	"github.com/aidanlsb/raven/internal/maintsvc"
	"github.com/aidanlsb/raven/internal/versioninfo"
)

// HandleDocs executes the canonical `docs` command.
func HandleDocs(_ context.Context, req commandexec.Request) commandexec.Result {
	sectionInput := strings.TrimSpace(stringArg(req.Args, "section"))
	topicInput := strings.TrimSpace(stringArg(req.Args, "topic"))

	source, err := docssvc.LoadVaultDocsSource(req.VaultPath)
	if err != nil {
		return mapDocsSvcFailure(err, "Run 'rvn docs fetch' to download docs for this vault")
	}

	sections, err := docssvc.ListSectionsFS(source, ".")
	if err != nil {
		return mapDocsSvcFailure(err, "Run 'rvn docs fetch' to refresh docs")
	}

	if sectionInput == "" {
		if topicInput != "" {
			return commandexec.Failure("INVALID_INPUT", "section is required when topic is provided", nil, "Provide section and topic together")
		}
		return commandexec.Success(docsSectionsData(sections), &commandexec.Meta{Count: len(sections)})
	}

	section, ok := docssvc.FindSection(sections, sectionInput)
	if !ok {
		available := make([]string, 0, len(sections))
		for _, item := range sections {
			available = append(available, item.ID)
		}
		sort.Strings(available)
		return commandexec.Failure(
			"INVALID_INPUT",
			"unknown docs section: "+sectionInput,
			map[string]interface{}{"available": available},
			"Run 'rvn docs' to list sections (available: "+strings.Join(available, ", ")+")",
		)
	}

	topics, err := docssvc.ListTopicsFS(source, ".", section.ID)
	if err != nil {
		return mapDocsSvcFailure(err, "Run 'rvn docs fetch' to refresh docs")
	}

	if topicInput == "" {
		return commandexec.Success(map[string]interface{}{
			"section": section.ID,
			"title":   section.Title,
			"topics":  docsTopicItems(topics),
		}, &commandexec.Meta{Count: len(topics)})
	}

	topic, ok := docssvc.FindTopic(topics, topicInput)
	if !ok {
		available := make([]string, 0, len(topics))
		for _, item := range topics {
			available = append(available, item.ID)
		}
		sort.Strings(available)

		suggestion := "Run 'rvn docs " + section.ID + "' to list topics"
		if len(available) > 0 {
			suggestion += " (available: " + strings.Join(available, ", ") + ")"
		}

		return commandexec.Failure(
			"INVALID_INPUT",
			`unknown topic "`+topicInput+`" in section "`+section.ID+`"`,
			map[string]interface{}{"section": section.ID, "available": available},
			suggestion,
		)
	}

	content, err := docssvc.ReadTopicContentFS(source, topic)
	if err != nil {
		return mapDocsSvcFailure(err, "Run 'rvn docs fetch' to refresh docs")
	}

	return commandexec.Success(map[string]interface{}{
		"section": topic.Section,
		"topic":   topic.ID,
		"title":   topic.Title,
		"path":    topic.Path,
		"content": content,
	}, nil)
}

// HandleDocsList executes the canonical `docs list` command.
func HandleDocsList(_ context.Context, req commandexec.Request) commandexec.Result {
	sections, err := docssvc.ListSections(req.VaultPath)
	if err != nil {
		return mapDocsSvcFailure(err, "Run 'rvn docs fetch' to download docs for this vault")
	}
	return commandexec.Success(docsSectionsData(sections), &commandexec.Meta{Count: len(sections)})
}

// HandleDocsSearch executes the canonical `docs search` command.
func HandleDocsSearch(_ context.Context, req commandexec.Request) commandexec.Result {
	query := strings.TrimSpace(stringArg(req.Args, "query"))
	if query == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "specify a search query", nil, "Usage: rvn docs search <query>")
	}

	limit, ok := intArg(req.Args, "limit")
	if !ok {
		limit = 20
	}
	if limit < 1 {
		return commandexec.Failure("INVALID_INPUT", "--limit must be >= 1", nil, "")
	}

	matches, err := docssvc.Search(req.VaultPath, query, strings.TrimSpace(stringArg(req.Args, "section")), limit)
	if err != nil {
		return mapDocsSvcFailure(err, "Run 'rvn docs' to list sections")
	}

	return commandexec.Success(map[string]interface{}{
		"query":   query,
		"count":   len(matches),
		"matches": matches,
	}, &commandexec.Meta{Count: len(matches)})
}

// HandleDocsFetch executes the canonical `docs fetch` command.
func HandleDocsFetch(_ context.Context, req commandexec.Request) commandexec.Result {
	version := versioninfo.Current().Version
	if strings.TrimSpace(req.ExecutablePath) != "" {
		version = maintsvc.CurrentVersionInfoFromExecutable(req.ExecutablePath).Version
	}

	result, err := docssvc.Fetch(docssvc.FetchRequest{
		VaultPath:  req.VaultPath,
		Ref:        strings.TrimSpace(stringArg(req.Args, "ref")),
		SourceBase: strings.TrimSpace(stringArg(req.Args, "source")),
		CLIVersion: version,
	})
	if err != nil {
		return mapDocsSvcFailure(err, "Check your network connection and run 'rvn docs fetch' again")
	}

	return commandexec.Success(map[string]interface{}{
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
}

func docsSectionsData(sections []docssvc.SectionView) map[string]interface{} {
	return map[string]interface{}{
		"sections":       sections,
		"command_docs":   "rvn help <command>",
		"navigation_tip": "rvn docs <section> <topic>",
	}
}

func docsTopicItems(topics []docssvc.TopicRecord) []map[string]interface{} {
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

func mapDocsSvcFailure(err error, fallbackSuggestion string) commandexec.Result {
	svcErr, ok := docssvc.AsError(err)
	if !ok {
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, fallbackSuggestion)
	}

	suggestion := svcErr.Suggestion
	if suggestion == "" {
		suggestion = fallbackSuggestion
	}

	switch svcErr.Code {
	case docssvc.CodeInvalidInput:
		return commandexec.Failure("INVALID_INPUT", svcErr.Message, nil, suggestion)
	case docssvc.CodeNotFound:
		return commandexec.Failure("FILE_NOT_FOUND", svcErr.Message, nil, suggestion)
	case docssvc.CodeFileRead:
		return commandexec.Failure("FILE_READ_ERROR", svcErr.Message, nil, suggestion)
	default:
		return commandexec.Failure("INTERNAL_ERROR", svcErr.Message, nil, suggestion)
	}
}
