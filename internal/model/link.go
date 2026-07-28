package model

// Link is one outgoing Markdown link edge to a non-Raven target.
type Link struct {
	// SourceID is the file object containing the link.
	SourceID string `json:"source_id"`

	// SourceType is the Raven type of SourceID.
	SourceType string `json:"source_type"`

	// FilePath is the vault-relative Markdown file containing the link.
	FilePath string `json:"file_path"`

	// Line is the 1-indexed source line.
	Line int `json:"line"`

	// PositionStart and PositionEnd are 0-indexed byte offsets on Line. End is
	// exclusive and encompasses the complete Markdown link or image syntax.
	PositionStart int `json:"position_start"`
	PositionEnd   int `json:"position_end"`

	// RawTarget is the destination exactly as authored inside the Markdown link.
	RawTarget string `json:"raw_target"`

	// Display is the rendered link label or image alt text.
	Display string `json:"display"`

	IsImage bool `json:"is_image"`

	// Scheme is one of file, url, or other.
	Scheme string `json:"scheme"`

	// Ext is the lowercased file extension without a leading dot.
	Ext string `json:"ext"`

	// NormalizedKey is the canonical target identity used for matching.
	NormalizedKey string `json:"normalized_key"`
}
