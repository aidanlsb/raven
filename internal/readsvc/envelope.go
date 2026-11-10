package readsvc

import (
	"encoding/json"

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
