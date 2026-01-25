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

// GetID returns the object ID as the search match identifier.
func (s SearchMatch) GetID() string { return s.ObjectID }

// GetKind returns "search" for search match results.
func (s SearchMatch) GetKind() string { return "search" }

// GetContent returns the snippet for display.
func (s SearchMatch) GetContent() string { return s.Snippet }

// GetLocation returns the file path as location.
func (s SearchMatch) GetLocation() string { return s.FilePath }
