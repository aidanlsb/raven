// Package model defines canonical types for core Raven concepts.
// These types are the single source of truth used across all layers:
// database, query execution, CLI output, and MCP tools.
package model

import (
	"encoding/json"

	"github.com/aidanlsb/raven/internal/fieldvalue"
)

// Trait represents an instance of a trait annotation in the vault.
// Examples: @todo, @todo(done), @due(2025-01-25), @highlight
type Trait struct {
	// ID uniquely identifies this trait instance.
	// Format: "file/path.md:trait:N" where N is the trait index in the file.
	// Empty at parse time; assigned when indexing.
	ID string `json:"id"`

	// TraitType is the name of the trait (e.g., "todo", "due", "highlight").
	TraitType string `json:"trait_type"`

	// Value is the trait's typed value, if any. Nil for boolean traits like @highlight.
	// Query/CLI wire output should use IndexValueString() to preserve the string contract.
	Value *fieldvalue.FieldValue `json:"-"`

	// Content is the text content of the line containing this trait,
	// with trait annotations removed.
	Content string `json:"content"`

	// FilePath is the path to the file containing this trait,
	// relative to the vault root.
	FilePath string `json:"file_path"`

	// Line is the 1-indexed line number where this trait appears.
	Line int `json:"line"`

	// ParentScopeID is the ID of the file object or section directly containing
	// this trait.
	ParentScopeID string `json:"-"`

	// PositionStart is the byte offset of the annotation start within its line.
	// Parse/edit metadata only; omitted from query JSON.
	PositionStart int `json:"-"`

	// PositionEnd is the byte offset just past the annotation end within its line.
	PositionEnd int `json:"-"`
}

// HasValue reports whether this trait has a non-null value.
func (t *Trait) HasValue() bool {
	return t != nil && t.Value != nil && !t.Value.IsNull()
}

// ValueString returns a display string for the trait value, or "" if none.
func (t *Trait) ValueString() string {
	if !t.HasValue() {
		return ""
	}
	return fieldvalue.FormatLiteral(*t.Value)
}

// IndexValueString returns the index/wire string form of the trait value.
// Array values are JSON-encoded. Returns nil when there is no value.
func (t *Trait) IndexValueString() *string {
	if t == nil || t.Value == nil {
		return nil
	}
	s := fieldvalue.TraitIndexString(*t.Value)
	if s == "" {
		return nil
	}
	return &s
}

// SetIndexValueString sets Value from a stored index/wire string.
func (t *Trait) SetIndexValueString(s *string) {
	if t == nil {
		return
	}
	if s == nil {
		t.Value = nil
		return
	}
	fv := fieldvalue.String(*s)
	t.Value = &fv
}

// MarshalJSON preserves the historical trait query/CLI envelope where value is a string.
func (t Trait) MarshalJSON() ([]byte, error) {
	type wire struct {
		ID             string  `json:"id"`
		TraitType      string  `json:"trait_type"`
		Value          *string `json:"value,omitempty"`
		Content        string  `json:"content"`
		FilePath       string  `json:"file_path"`
		Line           int     `json:"line"`
		ParentObjectID string  `json:"parent_object_id"`
	}
	return json.Marshal(wire{
		ID:             t.ID,
		TraitType:      t.TraitType,
		Value:          t.IndexValueString(),
		Content:        t.Content,
		FilePath:       t.FilePath,
		Line:           t.Line,
		ParentObjectID: t.ParentScopeID,
	})
}
