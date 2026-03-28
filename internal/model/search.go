package model

// SearchMatch represents a full-text search result.
type SearchMatch struct {
	// ObjectID is the ID of the object containing the match.
	ObjectID string `json:"object_id"`

	// Title is the title of the matched document.
	Title string `json:"title"`

	// FilePath is the path to the file containing the match.
	FilePath string `json:"file_path"`

	// Snippet is the matched text with surrounding context.
	// Match boundaries are marked with » and «.
	Snippet string `json:"snippet"`

	// Rank is the FTS5 BM25 ranking score (lower is better match).
	Rank float64 `json:"rank"`
}
