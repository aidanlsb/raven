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

// GetID returns the source ID as the reference identifier.
func (r Reference) GetID() string { return r.SourceID }

// GetKind returns "reference" for reference results.
func (r Reference) GetKind() string { return "reference" }

// GetContent returns a human-readable description for display.
func (r Reference) GetContent() string {
	if r.DisplayText != nil && *r.DisplayText != "" {
		return *r.DisplayText
	}
	return r.TargetRaw
}

// GetLocation returns a short location string (file:line).
func (r Reference) GetLocation() string {
	if r.Line != nil {
		return r.FilePath + ":" + itoa(*r.Line)
	}
	return r.FilePath
}
