package commandimpl

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/docssvc"
	"github.com/aidanlsb/raven/internal/versioninfo"
)

// HandleDocs executes the canonical `docs` command.
func HandleDocs(_ context.Context, req commandexec.Request) commandexec.Result {
	sectionInput := strings.TrimSpace(stringArg(req.Args, "section"))
	topicInput := strings.TrimSpace(stringArg(req.Args, "topic"))

	source, err := docssvc.LoadGlobalDocsSource(req.ConfigPath, docsCLIVersion(req))
	if err != nil {
		return commandexec.FromServiceErrorWithFallback(err, "Run 'rvn docs fetch' to download docs")
	}
	warnings := source.Warnings

	sections, err := docssvc.ListSectionsFS(source.FS, ".")
	if err != nil {
		return commandexec.FromServiceErrorWithFallback(err, "Run 'rvn docs fetch' to refresh docs")
	}

	if sectionInput == "" {
		if topicInput != "" {
			return commandexec.Failure("INVALID_INPUT", "section is required when topic is provided", nil, "Provide section and topic together")
		}
		return commandexec.SuccessWithWarnings(docssvc.SectionsEnvelope(sections), warnings, &commandexec.Meta{Count: len(sections)})
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

	topics, err := docssvc.ListTopicsFS(source.FS, ".", section.ID)
	if err != nil {
		return commandexec.FromServiceErrorWithFallback(err, "Run 'rvn docs fetch' to refresh docs")
	}

	if topicInput == "" {
		return commandexec.SuccessWithWarnings(docssvc.TopicListEnvelope(section, topics), warnings, &commandexec.Meta{Count: len(topics)})
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

	content, err := docssvc.ReadTopicContentFS(source.FS, topic)
	if err != nil {
		return commandexec.FromServiceErrorWithFallback(err, "Run 'rvn docs fetch' to refresh docs")
	}

	return commandexec.SuccessWithWarnings(docssvc.TopicContentEnvelope(topic, content), warnings, nil)
}

// HandleDocsSearch executes the canonical `docs search` command.
func HandleDocsSearch(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()
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
	offset, _ := intArg(req.Args, "offset")
	if offset < 0 {
		return commandexec.Failure("INVALID_INPUT", "--offset must be >= 0", nil, "Use --offset 0 for no offset")
	}

	result, warnings, err := docssvc.Search(req.ConfigPath, docsCLIVersion(req), query, strings.TrimSpace(stringArg(req.Args, "section")), limit, offset)
	if err != nil {
		return commandexec.FromServiceErrorWithFallback(err, "Run 'rvn docs' to list sections")
	}

	return commandexec.SuccessWithWarnings(result.Payload(query), warnings, &commandexec.Meta{Count: result.Returned, QueryTimeMs: time.Since(start).Milliseconds()})
}

// HandleDocsFetch executes the canonical `docs fetch` command.
func HandleDocsFetch(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := docssvc.Fetch(docssvc.FetchRequest{
		ConfigPath: req.ConfigPath,
		Ref:        strings.TrimSpace(stringArg(req.Args, "ref")),
		SourceBase: strings.TrimSpace(stringArg(req.Args, "source")),
		CLIVersion: docsCLIVersion(req),
	})
	if err != nil {
		return commandexec.FromServiceErrorWithFallback(err, "Check your network connection and run 'rvn docs fetch' again")
	}

	return commandexec.Success(result.Payload(), nil)
}

func docsCLIVersion(req commandexec.Request) string {
	version := versioninfo.Current().Version
	if strings.TrimSpace(req.ExecutablePath) != "" {
		version = versioninfo.CurrentVersionInfoFromExecutable(req.ExecutablePath).Version
	}
	return version
}
