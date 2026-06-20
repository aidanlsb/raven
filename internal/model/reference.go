package model

// Reference represents a wikilink reference from one location to another.
// This is used for both backlinks (who references X?) and outlinks (what does X reference?).
type Reference struct {
	// SourceID is the ID of the object or trait containing the reference.
	SourceID string `json:"source_id"`

	// SourceType indicates whether the source is an "object" or "trait".
	SourceType string `json:"source_type"`

	// TargetRaw is the raw target as written in the wikilink.
	TargetRaw string `json:"target_raw"`

	// FilePath is the path to the file containing this reference.
	FilePath string `json:"file_path"`

	// Line is the 1-indexed line number where this reference appears.
	// May be nil if the reference is in frontmatter.
	Line *int `json:"line,omitempty"`

	// DisplayText is the display text of the wikilink, if different from target.
	DisplayText *string `json:"display_text,omitempty"`
}

// ReferenceInputError describes a non-fatal error for one input in a bulk
// reference traversal.
type ReferenceInputError struct {
	Input      string      `json:"input"`
	Code       string      `json:"code"`
	Message    string      `json:"message"`
	Suggestion string      `json:"suggestion,omitempty"`
	Details    interface{} `json:"details,omitempty"`
}

// BacklinksGroup contains backlinks for one requested target.
type BacklinksGroup struct {
	Input  string      `json:"input"`
	Target string      `json:"target"`
	Items  []Reference `json:"items"`
	Count  int         `json:"count"`
}

// OutlinksGroup contains outlinks for one requested source.
type OutlinksGroup struct {
	Input  string      `json:"input"`
	Source string      `json:"source"`
	Items  []Reference `json:"items"`
	Count  int         `json:"count"`
}
