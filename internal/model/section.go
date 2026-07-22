package model

// Section represents a markdown heading-derived region inside a file-backed object.
type Section struct {
	ID              string  `json:"id"`
	FileObjectID    string  `json:"file_object_id"`
	FilePath        string  `json:"file_path"`
	Slug            string  `json:"slug"`
	Title           string  `json:"title"`
	Level           int     `json:"level"`
	LineStart       int     `json:"line_start"`
	LineEnd         *int    `json:"line_end,omitempty"`
	SubtreeLineEnd  *int    `json:"subtree_line_end,omitempty"`
	ParentSectionID *string `json:"parent_section_id,omitempty"`
}

// ParentScopeID returns the section's direct parent scope. Top-level sections
// are directly contained by their file object.
func (s *Section) ParentScopeID() string {
	if s == nil {
		return ""
	}
	if s.ParentSectionID != nil {
		return *s.ParentSectionID
	}
	return s.FileObjectID
}
