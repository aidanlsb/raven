package docssvc

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/docsync"
)

const docsIndexPath = "index.yaml"

type Code string

const (
	CodeInvalidInput Code = "INVALID_INPUT"
	CodeNotFound     Code = "NOT_FOUND"
	CodeFileRead     Code = "FILE_READ_ERROR"
	CodeFetchFailed  Code = "FETCH_FAILED"
	CodeInternal     Code = "INTERNAL_ERROR"
)

type Error struct {
	Code       Code
	Message    string
	Suggestion string
	Err        error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newError(code Code, message, suggestion string, err error) *Error {
	return &Error{Code: code, Message: message, Suggestion: suggestion, Err: err}
}

func AsError(err error) (*Error, bool) {
	var svcErr *Error
	if errors.As(err, &svcErr) {
		return svcErr, true
	}
	return nil, false
}

type SectionView struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	TopicCount int    `json:"topic_count"`
	sortOrder  *int
}

type TopicRecord struct {
	Section   string
	ID        string
	Title     string
	Path      string
	FSPath    string
	sortOrder *int
}

type SearchMatchView struct {
	Section string `json:"section"`
	Topic   string `json:"topic"`
	Title   string `json:"title"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

type FetchRequest struct {
	VaultPath  string
	Ref        string
	SourceBase string
	CLIVersion string
}

type FetchResult struct {
	Path        string `json:"path"`
	FileCount   int    `json:"file_count"`
	ByteCount   int64  `json:"byte_count"`
	Source      string `json:"source"`
	Ref         string `json:"ref"`
	ArchiveURL  string `json:"archive_url"`
	FetchedAt   string `json:"fetched_at"`
	CLIVersion  string `json:"cli_version"`
	ManifestVer int    `json:"manifest_ver"`
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

func LoadVaultDocsSource(vaultPath string) (fs.FS, error) {
	if strings.TrimSpace(vaultPath) == "" {
		return nil, newError(CodeInvalidInput, "no vault resolved for docs command", "", nil)
	}

	docsFS, err := docsync.OpenFS(vaultPath)
	if err != nil {
		if errors.Is(err, docsync.ErrDocsNotFetched) {
			return nil, newError(CodeNotFound, "docs are not available for this vault", "Run 'rvn docs fetch' to download docs for this vault", err)
		}
		return nil, newError(CodeFileRead, "failed to open docs cache", "", err)
	}
	return docsFS, nil
}

func ListSections(vaultPath string) ([]SectionView, error) {
	source, err := LoadVaultDocsSource(vaultPath)
	if err != nil {
		return nil, err
	}
	return ListSectionsFS(source, ".")
}

func ListSectionsFS(docsFS fs.FS, docsRoot string) ([]SectionView, error) {
	sections, err := listSectionsFS(docsFS, docsRoot)
	if err != nil {
		if strings.Contains(err.Error(), "docs index not found") {
			return nil, newError(CodeNotFound, err.Error(), "Run 'rvn docs fetch' to refresh docs", err)
		}
		return nil, newError(CodeInternal, err.Error(), "", err)
	}
	return sections, nil
}

func ListTopics(vaultPath, section string) ([]TopicRecord, error) {
	source, err := LoadVaultDocsSource(vaultPath)
	if err != nil {
		return nil, err
	}
	return ListTopicsFS(source, ".", section)
}

func ListTopicsFS(docsFS fs.FS, docsRoot, section string) ([]TopicRecord, error) {
	topics, err := listTopicsFS(docsFS, docsRoot, section)
	if err != nil {
		if strings.Contains(err.Error(), "is not declared in docs index") {
			return nil, newError(CodeInvalidInput, err.Error(), "Run 'rvn docs' to list sections", err)
		}
		if strings.Contains(err.Error(), "docs index not found") {
			return nil, newError(CodeNotFound, err.Error(), "Run 'rvn docs fetch' to refresh docs", err)
		}
		return nil, newError(CodeInternal, err.Error(), "", err)
	}
	return topics, nil
}

func ReadTopicContentFS(docsFS fs.FS, topic TopicRecord) (string, error) {
	content, err := fs.ReadFile(docsFS, topic.FSPath)
	if err != nil {
		return "", newError(CodeFileRead, "failed to read docs topic", "", err)
	}
	return string(content), nil
}

func Search(vaultPath, query, sectionFilter string, limit int) ([]SearchMatchView, error) {
	source, err := LoadVaultDocsSource(vaultPath)
	if err != nil {
		return nil, err
	}
	return SearchFS(source, ".", query, sectionFilter, limit)
}

func SearchFS(docsFS fs.FS, docsRoot, query, sectionFilter string, limit int) ([]SearchMatchView, error) {
	matches, err := searchFS(docsFS, docsRoot, query, sectionFilter, limit)
	if err != nil {
		message := err.Error()
		suggestion := ""
		code := CodeInternal
		switch {
		case strings.Contains(message, "empty query"), strings.Contains(message, "limit must be >= 1"), strings.Contains(message, "unknown section"):
			code = CodeInvalidInput
			suggestion = "Run 'rvn docs' to list sections"
		case strings.Contains(message, "docs index not found"):
			code = CodeNotFound
			suggestion = "Run 'rvn docs fetch' to refresh docs"
		case strings.HasPrefix(message, "read "):
			code = CodeFileRead
		}
		return nil, newError(code, message, suggestion, err)
	}
	return matches, nil
}

func Fetch(req FetchRequest) (*FetchResult, error) {
	result, err := docsync.Fetch(docsync.FetchOptions{
		VaultPath:     strings.TrimSpace(req.VaultPath),
		Ref:           strings.TrimSpace(req.Ref),
		SourceBaseURL: strings.TrimSpace(req.SourceBase),
		CLIVersion:    strings.TrimSpace(req.CLIVersion),
		HTTPClient:    &http.Client{Timeout: 60 * time.Second},
	})
	if err != nil {
		return nil, newError(CodeFetchFailed, "failed to fetch docs", "Check your network connection and run 'rvn docs fetch' again", err)
	}

	relPath, relErr := filepath.Rel(req.VaultPath, result.DocsPath)
	if relErr != nil {
		relPath = docsync.StoreRelPath
	}
	relPath = filepath.ToSlash(relPath)

	return &FetchResult{
		Path:        relPath,
		FileCount:   result.FileCount,
		ByteCount:   result.ByteCount,
		Source:      result.Manifest.SourceBaseURL,
		Ref:         result.Manifest.Ref,
		ArchiveURL:  result.Manifest.ArchiveURL,
		FetchedAt:   result.Manifest.FetchedAt,
		CLIVersion:  result.Manifest.CLIVersion,
		ManifestVer: result.Manifest.SchemaVersion,
	}, nil
}

func FindSection(sections []SectionView, raw string) (SectionView, bool) {
	needle := normalizeDocsSegment(raw)
	for _, section := range sections {
		if normalizeDocsSegment(section.ID) == needle {
			return section, true
		}
	}
	return SectionView{}, false
}

func FindTopic(topics []TopicRecord, raw string) (TopicRecord, bool) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(raw, ".md"))
	needle := normalizeDocsPathSlug(trimmed)
	for _, topic := range topics {
		if topic.ID == needle {
			return topic, true
		}
	}
	return TopicRecord{}, false
}

func NormalizePathSlug(input string) string {
	return normalizeDocsPathSlug(input)
}

func listSectionsFS(docsFS fs.FS, docsRoot string) ([]SectionView, error) {
	index, err := loadDocsIndexFS(docsFS, docsRoot)
	if err != nil {
		return nil, err
	}
	if len(index.Sections) == 0 {
		return nil, fmt.Errorf("docs index has no sections")
	}

	sections := make([]SectionView, 0, len(index.Sections))
	for sectionID, meta := range index.Sections {
		topics, err := listTopicsWithIndexFS(docsFS, docsRoot, sectionID, index)
		if err != nil {
			return nil, err
		}
		title := titleFromSlug(sectionID)
		if override := strings.TrimSpace(meta.Title); override != "" {
			title = override
		}
		sections = append(sections, SectionView{
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

func listTopicsFS(docsFS fs.FS, docsRoot, section string) ([]TopicRecord, error) {
	index, err := loadDocsIndexFS(docsFS, docsRoot)
	if err != nil {
		return nil, err
	}
	return listTopicsWithIndexFS(docsFS, docsRoot, section, index)
}

func listTopicsWithIndexFS(docsFS fs.FS, docsRoot, section string, index docsIndex) ([]TopicRecord, error) {
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

	records := make([]TopicRecord, 0, len(sectionMeta.Topics))
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
		records = append(records, TopicRecord{
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

func searchFS(docsFS fs.FS, docsRoot, query, sectionFilter string, limit int) ([]SearchMatchView, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}
	if limit < 1 {
		return nil, fmt.Errorf("limit must be >= 1")
	}

	sections, err := listSectionsFS(docsFS, docsRoot)
	if err != nil {
		return nil, err
	}

	selected := make([]SectionView, 0)
	if strings.TrimSpace(sectionFilter) == "" {
		selected = sections
	} else {
		section, ok := FindSection(sections, sectionFilter)
		if !ok {
			return nil, fmt.Errorf("unknown section: %s", sectionFilter)
		}
		selected = append(selected, section)
	}

	queryLower := strings.ToLower(query)
	matches := make([]SearchMatchView, 0, limit)

	for _, section := range selected {
		topics, err := listTopicsFS(docsFS, docsRoot, section.ID)
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

				matches = append(matches, SearchMatchView{
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

func ListSectionsFromRoot(docsRoot string) ([]SectionView, error) {
	sections, err := ListSectionsFS(os.DirFS(docsRoot), ".")
	if err != nil {
		return nil, err
	}
	return sections, nil
}

func ListTopicsFromRoot(docsRoot, section string) ([]TopicRecord, error) {
	topics, err := ListTopicsFS(os.DirFS(docsRoot), ".", section)
	if err != nil {
		return nil, err
	}
	return topics, nil
}
