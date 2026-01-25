// Package model defines canonical types for core Raven concepts.
// These types are the single source of truth used across all layers:
// database, query execution, CLI output, and MCP tools.
package model

// Trait represents an instance of a trait annotation in the vault.
// Examples: @todo, @todo(done), @due(2025-01-25), @highlight
type Trait struct {
	// ID uniquely identifies this trait instance.
	// Format: "file/path.md:trait:N" where N is the trait index in the file.
	ID string `json:"id"`

	// TraitType is the name of the trait (e.g., "todo", "due", "highlight").
	TraitType string `json:"trait_type"`

	// Value is the trait's value, if any. Nil for boolean traits like @highlight.
	Value *string `json:"value,omitempty"`

	// Content is the text content of the line containing this trait,
	// with trait annotations removed.
	Content string `json:"content"`

	// FilePath is the path to the file containing this trait,
	// relative to the vault root.
	FilePath string `json:"file_path"`

	// Line is the 1-indexed line number where this trait appears.
	Line int `json:"line"`

	// ParentObjectID is the ID of the object containing this trait.
	// This is the nearest ancestor object in the document hierarchy.
	ParentObjectID string `json:"parent_object_id"`
}

// GetID returns the trait's unique identifier.
func (t Trait) GetID() string { return t.ID }

// GetKind returns "trait" for trait results.
func (t Trait) GetKind() string { return "trait" }

// GetContent returns a human-readable description for display.
func (t Trait) GetContent() string { return t.Content }

// GetLocation returns a short location string (file:line).
func (t Trait) GetLocation() string {
	return t.FilePath + ":" + itoa(t.Line)
}

// itoa is a simple int to string conversion without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[n:])
}
