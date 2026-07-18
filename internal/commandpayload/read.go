package commandpayload

import (
	"encoding/json"

	"github.com/aidanlsb/raven/internal/readsvc"
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
	Query   string            `json:"query"`
	Results []SearchMatchItem `json:"results"`
}

// ReadContentResult is the success payload for an enriched (non-raw) `read`.
// References and backlinks are always present so downstream consumers can rely
// on the keys existing even when empty.
type ReadContentResult struct {
	ObjectID   string                      `json:"object_id"`
	Path       string                      `json:"path"`
	Content    string                      `json:"content"`
	LineCount  int                         `json:"line_count"`
	References []readsvc.ReadReference     `json:"references"`
	Backlinks  []readsvc.ReadBacklinkGroup `json:"backlinks"`
}

// ReadRawResult is the success payload for a raw or line-range `read`. The
// range and structured-line fields are omitted for a full-file raw read, and
// the line-range fields are omitted unless an explicit range was requested.
type ReadRawResult struct {
	ObjectID  string             `json:"object_id"`
	Path      string             `json:"path"`
	Content   string             `json:"content"`
	LineCount int                `json:"line_count"`
	StartLine int                `json:"start_line,omitempty"`
	EndLine   int                `json:"end_line,omitempty"`
	Lines     []readsvc.ReadLine `json:"lines,omitempty"`
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
