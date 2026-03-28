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
