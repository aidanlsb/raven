package model

// SearchMatch represents a full-text search result.
type SearchMatch struct {
	// ObjectID is the ID of the object containing the match.
	ObjectID string `json:"object_id"`

	// Title is the title of the matched document.
	Title string `json:"title"`

	// FilePath is the path to the file containing the match.
	FilePath string `json:"file_path"`

	// IsSection is true when the match is a heading-derived section row.
	IsSection bool `json:"is_section"`

	// FileObjectID is the containing file-backed object ID for section matches.
	FileObjectID string `json:"file_object_id,omitempty"`

	// LineStart is the 1-indexed line where a section match starts.
	LineStart int `json:"line_start,omitempty"`

	// LineEnd is the direct section end for section matches.
	LineEnd *int `json:"line_end,omitempty"`

	// SubtreeLineEnd is the full subtree end for section matches.
	SubtreeLineEnd *int `json:"subtree_line_end,omitempty"`

	// Snippet is the matched text with surrounding context.
	// Match boundaries are marked with » and «.
	Snippet string `json:"snippet"`

	// Rank is the FTS5 BM25 ranking score (lower is better match).
	Rank float64 `json:"rank"`
}
