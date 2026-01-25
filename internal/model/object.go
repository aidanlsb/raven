package model

// Object represents an instance of a typed object in the vault.
// Objects are markdown files with frontmatter that defines their type and fields.
type Object struct {
	// ID uniquely identifies this object.
	// Typically the file path without extension, e.g., "people/alice".
	ID string `json:"id"`

	// Type is the type of this object (e.g., "person", "project").
	Type string `json:"type"`

	// Fields contains the frontmatter field values, parsed from YAML.
	Fields map[string]interface{} `json:"fields,omitempty"`

	// FilePath is the path to the file containing this object,
	// relative to the vault root.
	FilePath string `json:"file_path"`

	// LineStart is the 1-indexed line number where this object starts.
	LineStart int `json:"line_start"`

	// ParentID is the ID of the parent object, if this object is nested.
	// Nil for top-level objects.
	ParentID *string `json:"parent_id,omitempty"`
}

// GetID returns the object's unique identifier.
func (o Object) GetID() string { return o.ID }

// GetKind returns "object" for object results.
func (o Object) GetKind() string { return "object" }

// GetContent returns a human-readable description for display.
// Uses the object ID (typically the file basename).
func (o Object) GetContent() string { return o.ID }

// GetLocation returns a short location string (file:line).
func (o Object) GetLocation() string {
	return o.FilePath + ":" + itoa(o.LineStart)
}
