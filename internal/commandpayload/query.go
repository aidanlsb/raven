// Package commandpayload defines shared, typed result payloads that command
// handlers emit as commandexec.Result.Data. Keeping these structs in a neutral
// package (rather than inside a CLI- or MCP-specific package) lets the
// in-process CLI render path type-assert them directly instead of rehydrating
// generic map[string]interface{} envelopes, while the JSON wire shape stays
// stable via explicit struct tags.
package commandpayload

import "github.com/aidanlsb/raven/internal/fieldvalue"

// Pagination holds the shared paging affordances emitted alongside query
// result windows. HasMore is always present so agents can loop without
// guessing; NextOffset is a forward cursor emitted only when more results
// remain.
type Pagination struct {
	Total      int  `json:"total"`
	Returned   int  `json:"returned"`
	Offset     int  `json:"offset"`
	Limit      int  `json:"limit"`
	HasMore    bool `json:"has_more"`
	NextOffset *int `json:"next_offset,omitempty"`
}

// ObjectItem is a single row in a `type:` query result.
type ObjectItem struct {
	Num      int                              `json:"num"`
	ID       string                           `json:"id"`
	Type     string                           `json:"type"`
	Fields   map[string]fieldvalue.FieldValue `json:"fields"`
	FilePath string                           `json:"file_path"`
	Line     int                              `json:"line"`
}

// TraitItem is a single row in a `trait:` query result.
type TraitItem struct {
	Num       int     `json:"num"`
	ID        string  `json:"id"`
	TraitType string  `json:"trait_type"`
	Value     *string `json:"value"`
	Content   string  `json:"content"`
	FilePath  string  `json:"file_path"`
	Line      int     `json:"line"`
	ScopeID   string  `json:"object_id"` // JSON name retained for compatibility.
}

// SectionItem is a single row in a `section` query result.
type SectionItem struct {
	Num             int     `json:"num"`
	ID              string  `json:"id"`
	FileObjectID    string  `json:"file_object_id"`
	FilePath        string  `json:"file_path"`
	Slug            string  `json:"slug"`
	Title           string  `json:"title"`
	Level           int     `json:"level"`
	LineStart       int     `json:"line_start"`
	LineEnd         *int    `json:"line_end"`
	DirectLineEnd   *int    `json:"direct_line_end"`
	SubtreeLineEnd  *int    `json:"subtree_line_end"`
	ParentSectionID *string `json:"parent_section_id"`
}

// LinkItem is one outgoing Markdown link edge returned by a `link` query.
type LinkItem struct {
	Num           int    `json:"num"`
	SourceID      string `json:"source_id"`
	SourceType    string `json:"source_type"`
	FilePath      string `json:"file_path"`
	Line          int    `json:"line"`
	PositionStart int    `json:"position_start"`
	PositionEnd   int    `json:"position_end"`
	RawTarget     string `json:"raw_target"`
	Display       string `json:"display"`
	IsImage       bool   `json:"is_image"`
	Scheme        string `json:"scheme"`
	Ext           string `json:"ext"`
	NormalizedKey string `json:"normalized_key"`
}

// QueryObjectResult is the success payload for a `type:` query.
type QueryObjectResult struct {
	QueryKind  string       `json:"query_kind"`
	Type       string       `json:"type,omitempty"`
	SavedQuery string       `json:"saved_query,omitempty"`
	Items      []ObjectItem `json:"items"`
	Pagination
}

// QueryTraitResult is the success payload for a `trait:` query.
type QueryTraitResult struct {
	QueryKind  string      `json:"query_kind"`
	Trait      string      `json:"trait,omitempty"`
	SavedQuery string      `json:"saved_query,omitempty"`
	Items      []TraitItem `json:"items"`
	Pagination
}

// QuerySectionResult is the success payload for a `section` query.
type QuerySectionResult struct {
	QueryKind  string        `json:"query_kind"`
	SavedQuery string        `json:"saved_query,omitempty"`
	Items      []SectionItem `json:"items"`
	Pagination
}

// QueryLinkResult is the success payload for a `link` query.
type QueryLinkResult struct {
	QueryKind  string     `json:"query_kind"`
	SavedQuery string     `json:"saved_query,omitempty"`
	Items      []LinkItem `json:"items"`
	Pagination
}

// QueryIDsResult is the success payload for any query run with --ids.
type QueryIDsResult struct {
	IDs []string `json:"ids"`
	Pagination
}

// QueryCountResult is the success payload for any query run with --count-only.
// Type and Trait are mutually exclusive discriminators; section and link
// count responses omit both.
type QueryCountResult struct {
	QueryKind string `json:"query_kind"`
	Type      string `json:"type,omitempty"`
	Trait     string `json:"trait,omitempty"`
	Total     int    `json:"total"`
}
