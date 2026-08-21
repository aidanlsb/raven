package model

// Reference represents a wikilink reference from one location to another.
// This is used for both backlinks (who references X?) and outlinks (what does X reference?).
// Parse/index paths also use this as the canonical in-document reference type.
type Reference struct {
	// SourceID is the ID of the file object or section scope containing the reference.
	SourceID string `json:"source_id"`

	// SourceType is the Raven type of the source scope (for example, "page",
	// "project", or "section"). It may be empty when type information is unavailable.
	SourceType string `json:"source_type"`

	// TargetRaw is the raw target as written in the wikilink.
	TargetRaw string `json:"target_raw"`

	// FilePath is the path to the file containing this reference.
	FilePath string `json:"file_path"`

	// Line is the 1-indexed line number where this reference appears.
	// May be nil if the reference is in frontmatter.
	Line *int `json:"line,omitempty"`

	// PositionStart is the 0-indexed byte offset of the wikilink start within its line.
	// May be nil when position data is unavailable.
	PositionStart *int `json:"position_start,omitempty"`

	// PositionEnd is the 0-indexed byte offset just past the wikilink end within its line.
	// May be nil when position data is unavailable.
	PositionEnd *int `json:"position_end,omitempty"`

	// DisplayText is the display text of the wikilink, if different from target.
	DisplayText *string `json:"display_text,omitempty"`
}

// FieldReference represents a reference stored in a schema-typed ref/ref[]
// frontmatter field.
type FieldReference struct {
	SourceID  string `json:"source_id"`
	FieldName string `json:"field_name"`
	TargetRaw string `json:"target_raw"`
	FilePath  string `json:"file_path"`
	Line      *int   `json:"line,omitempty"`
}

// IntPtr returns a pointer to v. Useful when constructing References.
func IntPtr(v int) *int {
	return &v
}

// NewInlineReference builds a parse-time reference with line and optional span.
func NewInlineReference(sourceID, targetRaw string, displayText *string, line, start, end int) *Reference {
	ref := &Reference{
		SourceID:    sourceID,
		TargetRaw:   targetRaw,
		DisplayText: displayText,
		Line:        IntPtr(line),
	}
	if start != 0 || end != 0 {
		ref.PositionStart = IntPtr(start)
		ref.PositionEnd = IntPtr(end)
	}
	return ref
}

// LineOrZero returns the 1-indexed line number, or 0 when unset.
func (r *Reference) LineOrZero() int {
	if r == nil || r.Line == nil {
		return 0
	}
	return *r.Line
}

// PositionStartOrZero returns the start offset, or 0 when unset.
func (r *Reference) PositionStartOrZero() int {
	if r == nil || r.PositionStart == nil {
		return 0
	}
	return *r.PositionStart
}

// PositionEndOrZero returns the end offset, or 0 when unset.
func (r *Reference) PositionEndOrZero() int {
	if r == nil || r.PositionEnd == nil {
		return 0
	}
	return *r.PositionEnd
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
