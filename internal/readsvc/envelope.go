package readsvc

import (
	"encoding/json"
	"fmt"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/querysvc"
)

// SearchMatchItem is a single row in a `search` result. Section-only fields are
// emitted only for section matches, mirroring the historical map-shaped payload
// (present—even when null—for section rows, absent for object rows) so the JSON
// wire contract is unchanged. It carries a custom MarshalJSON to preserve that
// conditional key presence, which struct tags alone cannot express.
type SearchMatchItem struct {
	ObjectID       string
	Title          string
	FilePath       string
	Snippet        string
	Rank           float64
	IsSection      bool
	FileObjectID   string
	LineStart      int
	LineEnd        *int
	DirectLineEnd  *int
	SubtreeLineEnd *int
}

// MarshalJSON reproduces the legacy search-result item shape: object matches
// carry only the core columns, while section matches additionally carry the
// section location fields (even when their line bounds are null).
func (m SearchMatchItem) MarshalJSON() ([]byte, error) {
	out := map[string]interface{}{
		"object_id": m.ObjectID,
		"title":     m.Title,
		"file_path": m.FilePath,
		"snippet":   m.Snippet,
		"rank":      m.Rank,
	}
	if m.IsSection {
		out["is_section"] = true
		out["file_object_id"] = m.FileObjectID
		out["line_start"] = m.LineStart
		out["line_end"] = m.LineEnd
		out["direct_line_end"] = m.DirectLineEnd
		out["subtree_line_end"] = m.SubtreeLineEnd
	}
	return json.Marshal(out)
}

// SearchResult is the success payload for the `search` command.
type SearchResult struct {
	Query string            `json:"query"`
	Items []SearchMatchItem `json:"items"`
	querysvc.Pagination
}

// SearchPayload maps index search hits onto the stable search envelope.
func SearchPayload(query string, matches []model.SearchMatch, total, offset, limit int) SearchResult {
	items := make([]SearchMatchItem, len(matches))
	for i, match := range matches {
		items[i] = SearchMatchItem{
			ObjectID:  match.ObjectID,
			Title:     match.Title,
			FilePath:  match.FilePath,
			Snippet:   match.Snippet,
			Rank:      match.Rank,
			IsSection: match.IsSection,
		}
		if match.IsSection {
			items[i].FileObjectID = match.FileObjectID
			items[i].LineStart = match.LineStart
			items[i].LineEnd = match.LineEnd
			items[i].DirectLineEnd = match.LineEnd
			items[i].SubtreeLineEnd = match.SubtreeLineEnd
		}
	}
	return SearchResult{
		Query:      query,
		Items:      items,
		Pagination: querysvc.NewPagination(total, len(items), offset, limit),
	}
}

// ReadContentResult is the success payload for an enriched (non-raw) `read`.
// References, backlinks, and links are always present so downstream consumers
// can rely on the keys existing even when empty. Links locate [[wikilinks]] in
// Content with UTF-8 byte and UTF-16 code-unit offsets.
type ReadContentResult struct {
	ObjectID   string              `json:"object_id"`
	Path       string              `json:"path"`
	Content    string              `json:"content"`
	LineCount  int                 `json:"line_count"`
	References []ReadReference     `json:"references"`
	Backlinks  []ReadBacklinkGroup `json:"backlinks"`
	Links      []ReadLink          `json:"links"`
}

// ReadRawResult is the success payload for a raw or line-range `read`. The
// range and structured-line fields are omitted for a full-file raw read, and
// the line-range fields are omitted unless an explicit range was requested.
type ReadRawResult struct {
	ObjectID  string     `json:"object_id"`
	Path      string     `json:"path"`
	Content   string     `json:"content"`
	LineCount int        `json:"line_count"`
	StartLine int        `json:"start_line,omitempty"`
	EndLine   int        `json:"end_line,omitempty"`
	Lines     []ReadLine `json:"lines,omitempty"`
}

// ReadSectionsResult is the success payload for `read --sections`.
type ReadSectionsResult struct {
	ObjectID string            `json:"object_id"`
	Path     string            `json:"path"`
	Sections []ReadSectionItem `json:"sections"`
}

// ReadSectionItem is one heading-derived section in a `read --sections`
// outline. Line bounds and the parent pointer are omitted when unset, matching
// the historical map payload.
type ReadSectionItem struct {
	ID              string  `json:"id"`
	Slug            string  `json:"slug"`
	Title           string  `json:"title"`
	Level           int     `json:"level"`
	LineStart       int     `json:"line_start"`
	LineEnd         *int    `json:"line_end,omitempty"`
	SubtreeLineEnd  *int    `json:"subtree_line_end,omitempty"`
	ParentSectionID *string `json:"parent_section_id,omitempty"`
}

// ContentPayload is the enriched read envelope. Links is never nil.
func (r *ReadResult) ContentPayload() ReadContentResult {
	links := r.Links
	if links == nil {
		links = []ReadLink{}
	}
	return ReadContentResult{
		ObjectID:   r.ObjectID,
		Path:       r.Path,
		Content:    r.Content,
		LineCount:  r.LineCount,
		References: r.References,
		Backlinks:  r.Backlinks,
		Links:      links,
	}
}

// RawPayload is the raw/line-range read envelope.
func (r *ReadResult) RawPayload() ReadRawResult {
	payload := ReadRawResult{
		ObjectID:  r.ObjectID,
		Path:      r.Path,
		Content:   r.Content,
		LineCount: r.LineCount,
	}
	if r.StartLine > 0 {
		payload.StartLine = r.StartLine
		payload.EndLine = r.EndLine
	}
	if len(r.Lines) > 0 {
		payload.Lines = r.Lines
	}
	return payload
}

// SectionsPayload maps a section outline onto the stable read --sections envelope.
func (r *SectionsResult) SectionsPayload() ReadSectionsResult {
	sections := make([]ReadSectionItem, 0, len(r.Sections))
	for _, section := range r.Sections {
		sections = append(sections, ReadSectionItem{
			ID:              section.ID,
			Slug:            section.Slug,
			Title:           section.Title,
			Level:           section.Level,
			LineStart:       section.LineStart,
			LineEnd:         section.LineEnd,
			SubtreeLineEnd:  section.SubtreeLineEnd,
			ParentSectionID: section.ParentSectionID,
		})
	}
	return ReadSectionsResult{ObjectID: r.ObjectID, Path: r.Path, Sections: sections}
}

// ObjectsResult is the success payload for `objects`.
type ObjectsResult struct {
	Items []ObjectSummary `json:"items"`
	querysvc.Pagination
}

// ObjectsPayload maps a page of object summaries onto the stable objects envelope.
func (r *ListObjectsResult) ObjectsPayload() ObjectsResult {
	return ObjectsResult{
		Items:      r.Items,
		Pagination: querysvc.NewPagination(r.Total, len(r.Items), r.Offset, r.Limit),
	}
}

// BacklinksPayload is the success payload for a single-target `backlinks`.
type BacklinksPayload struct {
	Items  []model.Reference `json:"items"`
	Target string            `json:"target"`
}

// BacklinksStdinPayload is the success payload for `backlinks --stdin`.
type BacklinksStdinPayload struct {
	Errors        []model.ReferenceInputError `json:"errors"`
	ItemsByTarget []model.BacklinksGroup      `json:"items_by_target"`
	Resolved      int                         `json:"resolved"`
	Stdin         bool                        `json:"stdin"`
	TotalInputs   int                         `json:"total_inputs"`
}

func (r BacklinksResult) Payload() BacklinksPayload {
	group := r.Groups[0]
	return BacklinksPayload{Items: referenceItems(group.Items), Target: group.Target}
}

func (r BacklinksResult) StdinPayload(totalInputs int, errors []model.ReferenceInputError) BacklinksStdinPayload {
	return BacklinksStdinPayload{
		Errors:        errors,
		ItemsByTarget: r.Groups,
		Resolved:      len(r.Groups),
		Stdin:         true,
		TotalInputs:   totalInputs,
	}
}

// OutlinksPayload is the success payload for a single-target `outlinks`.
type OutlinksPayload struct {
	Items  []model.Reference `json:"items"`
	Source string            `json:"source"`
}

// OutlinksStdinPayload is the success payload for `outlinks --stdin`.
type OutlinksStdinPayload struct {
	Errors        []model.ReferenceInputError `json:"errors"`
	ItemsBySource []model.OutlinksGroup       `json:"items_by_source"`
	Resolved      int                         `json:"resolved"`
	Stdin         bool                        `json:"stdin"`
	TotalInputs   int                         `json:"total_inputs"`
}

func (r OutlinksResult) Payload() OutlinksPayload {
	group := r.Groups[0]
	return OutlinksPayload{Items: referenceItems(group.Items), Source: group.Source}
}

func (r OutlinksResult) StdinPayload(totalInputs int, errors []model.ReferenceInputError) OutlinksStdinPayload {
	return OutlinksStdinPayload{
		Errors:        errors,
		ItemsBySource: r.Groups,
		Resolved:      len(r.Groups),
		Stdin:         true,
		TotalInputs:   totalInputs,
	}
}

func referenceItems(references []model.Reference) []model.Reference {
	items := make([]model.Reference, len(references))
	copy(items, references)
	return items
}

// ResolveMatch is one candidate in an ambiguous `resolve` result.
type ResolveMatch struct {
	MatchSource string `json:"match_source,omitempty"`
	ObjectID    string `json:"object_id"`
}

// ResolveMissPayload is the success payload when `resolve` finds no target.
type ResolveMissPayload struct {
	Reference string `json:"reference"`
	Resolved  bool   `json:"resolved"`
}

// ResolveAmbiguousPayload is the success payload when `resolve` matches more
// than one object.
type ResolveAmbiguousPayload struct {
	Ambiguous bool           `json:"ambiguous"`
	Items     []ResolveMatch `json:"items"`
	Reference string         `json:"reference"`
	Resolved  bool           `json:"resolved"`
}

// ResolveHitPayload is the success payload for a unique `resolve`.
type ResolveHitPayload struct {
	DirectLineEnd  *int   `json:"direct_line_end,omitempty"`
	FileObjectID   string `json:"file_object_id,omitempty"`
	FilePath       string `json:"file_path"`
	IsSection      bool   `json:"is_section"`
	LineEnd        *int   `json:"line_end,omitempty"`
	LineStart      int    `json:"line_start,omitempty"`
	MatchSource    string `json:"match_source,omitempty"`
	ObjectID       string `json:"object_id"`
	Resolved       bool   `json:"resolved"`
	SubtreeLineEnd *int   `json:"subtree_line_end,omitempty"`
	Type           string `json:"type,omitempty"`
}

func ResolveMiss(reference string) ResolveMissPayload {
	return ResolveMissPayload{Reference: reference, Resolved: false}
}

func (r *ResolveResult) AmbiguousPayload(reference string) ResolveAmbiguousPayload {
	matches := make([]ResolveMatch, 0, len(r.Matches))
	for _, match := range r.Matches {
		entry := ResolveMatch{ObjectID: match}
		if r.MatchSources != nil {
			entry.MatchSource = r.MatchSources[match]
		}
		matches = append(matches, entry)
	}
	return ResolveAmbiguousPayload{
		Ambiguous: true,
		Items:     matches,
		Reference: reference,
		Resolved:  false,
	}
}

func (r *ResolveResult) HitPayload(filePath string) ResolveHitPayload {
	resolved := r.Resolved
	payload := ResolveHitPayload{
		FilePath:    filePath,
		IsSection:   resolved.IsSection,
		MatchSource: resolved.MatchSource,
		ObjectID:    resolved.ObjectID,
		Resolved:    true,
		Type:        r.ObjectType,
	}
	if resolved.IsSection {
		payload.FileObjectID = resolved.FileObjectID
		if resolved.LineStart > 0 {
			payload.LineStart = resolved.LineStart
		}
		if resolved.LineEnd != nil {
			payload.LineEnd = resolved.LineEnd
			payload.DirectLineEnd = resolved.LineEnd
		}
		payload.SubtreeLineEnd = resolved.SubtreeLineEnd
	}
	return payload
}

// OpenHitPayload is the success payload for a single-target `open`.
type OpenHitPayload struct {
	Editor       string `json:"editor"`
	File         string `json:"file"`
	FileObjectID string `json:"file_object_id,omitempty"`
	IsSection    bool   `json:"is_section,omitempty"`
	LineStart    int    `json:"line_start,omitempty"`
	ObjectID     string `json:"object_id"`
	Opened       bool   `json:"opened"`
}

// OpenBulkPayload is the success payload for `open --stdin`.
type OpenBulkPayload struct {
	Editor  string       `json:"editor"`
	Errors  []string     `json:"errors"`
	Files   []string     `json:"files"`
	Opened  bool         `json:"opened"`
	Targets []OpenTarget `json:"targets"`
}

func (t *OpenTarget) Payload(opened bool, editor string) OpenHitPayload {
	payload := OpenHitPayload{
		Editor:   editor,
		File:     t.RelativePath,
		ObjectID: t.ObjectID,
		Opened:   opened,
	}
	if t.IsSection {
		payload.IsSection = true
		payload.FileObjectID = t.FileObjectID
		if t.LineStart > 0 {
			payload.LineStart = t.LineStart
		}
	}
	return payload
}

func (r OpenResult) Payload() OpenBulkPayload {
	files := make([]string, 0, len(r.Targets))
	for _, target := range r.Targets {
		files = append(files, target.RelativePath)
	}
	errs := make([]string, 0, len(r.Failures))
	for _, failure := range r.Failures {
		errs = append(errs, fmt.Sprintf("%s: %s", failure.Reference, failure.Message))
	}
	return OpenBulkPayload{
		Editor:  r.Editor,
		Errors:  errs,
		Files:   files,
		Opened:  r.Opened,
		Targets: r.Targets,
	}
}

// DatePayload is the success payload for `date`.
type DatePayload struct {
	Backlinks   []model.Reference `json:"backlinks"`
	DailyExists bool              `json:"daily_exists"`
	DailyNote   *model.Object     `json:"daily_note,omitempty"`
	DailyNoteID string            `json:"daily_note_id"`
	DailyPath   string            `json:"daily_path"`
	Date        string            `json:"date"`
	DayOfWeek   string            `json:"day_of_week"`
	Items       []DateAssociation `json:"items"`
}

func (r *DateResult) Payload() DatePayload {
	return DatePayload{
		Backlinks:   r.Backlinks,
		DailyExists: r.DailyExists,
		DailyNote:   r.DailyNote,
		DailyNoteID: r.DailyNoteID,
		DailyPath:   r.DailyPath,
		Date:        r.Date,
		DayOfWeek:   r.DayOfWeek,
		Items:       r.Items,
	}
}

// StatsRevision is the index identity object inside `vault stats`.
type StatsRevision struct {
	Counter    int64  `json:"counter"`
	Generation string `json:"generation"`
}

// StatsPayload is the success payload for `vault stats`.
type StatsPayload struct {
	FileCount   int            `json:"file_count"`
	ObjectCount int            `json:"object_count"`
	RefCount    int            `json:"ref_count"`
	Revision    *StatsRevision `json:"revision,omitempty"`
	TraitCount  int            `json:"trait_count"`
}

func (r *StatsResult) Payload() StatsPayload {
	payload := StatsPayload{
		FileCount:   r.FileCount,
		ObjectCount: r.ObjectCount,
		RefCount:    r.RefCount,
		TraitCount:  r.TraitCount,
	}
	if r.Revision.Generation != "" {
		payload.Revision = &StatsRevision{Counter: r.Revision.Counter, Generation: r.Revision.Generation}
	}
	return payload
}
