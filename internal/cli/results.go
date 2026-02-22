// Package cli implements the command-line interface.
// This file defines shared JSON result types for consistent CLI output.
package cli

import "github.com/aidanlsb/raven/internal/model"

// =============================================================================
// Core Result Types - Used across multiple commands
// =============================================================================

// ObjectResult is now defined in model.Object.
// Use model.Object directly for object instances.

// TraitResult is now defined in model.Trait.
// This type alias is kept for reference in documentation.
// Use model.Trait directly for trait instances.

// BacklinkResult is now defined in model.Reference.
// Use model.Reference directly for reference/backlink instances.

// =============================================================================
// Summary Types - For --list style commands
// =============================================================================

// TypeSummary represents a type with its count.
// Used by: type --list
type TypeSummary struct {
	Name    string `json:"name"`
	Count   int    `json:"count"`
	Builtin bool   `json:"builtin"`
}

// =============================================================================
// Action Result Types - For mutation commands
// =============================================================================

// CreateResult represents the result of creating an object.
// Used by: new
type CreateResult struct {
	File  string `json:"file"`
	Type  string `json:"type"`
	Title string `json:"title"`
	ID    string `json:"id"`
}

// CaptureResult represents the result of quick capture.
// Used by: add
type CaptureResult struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// DeleteResult represents the result of deleting an object.
// Used by: delete
type DeleteResult struct {
	Deleted   string `json:"deleted"`
	Behavior  string `json:"behavior"`
	TrashPath string `json:"trash_path,omitempty"`
}

// FileResult represents raw file content.
// Used by: read
type FileResult struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	LineCount int    `json:"line_count"`

	// Optional range output (1-indexed, inclusive). When omitted, Content is the full file.
	StartLine int `json:"start_line,omitempty"`
	EndLine   int `json:"end_line,omitempty"`

	// Optional structured lines for copy-paste-safe editing.
	Lines []FileLine `json:"lines,omitempty"`
}

// FileLine represents one line of file content (without trailing '\n').
type FileLine struct {
	Num  int    `json:"num"`
	Text string `json:"text"`
}

// =============================================================================
// Stats Types
// =============================================================================

// StatsResult represents vault statistics.
// Used by: stats
type StatsResult struct {
	FileCount   int `json:"file_count"`
	ObjectCount int `json:"object_count"`
	TraitCount  int `json:"trait_count"`
	RefCount    int `json:"ref_count"`
}

// =============================================================================
// Query Types
// =============================================================================

// SavedQueryInfo represents a saved query definition.
// Used by: query --list
type SavedQueryInfo struct {
	Name        string   `json:"name"`
	Query       string   `json:"query"`
	Args        []string `json:"args,omitempty"`
	Description string   `json:"description,omitempty"`
}

// QueryResult represents results from running a saved query.
// Used by: query <name>
type QueryResult struct {
	QueryName string            `json:"query_name"`
	Types     []TypeQueryResult `json:"types,omitempty"`
	Traits    []model.Trait     `json:"traits,omitempty"`
}

// TypeQueryResult represents objects of a type in query results.
type TypeQueryResult struct {
	Type  string         `json:"type"`
	Items []model.Object `json:"items"`
}

// =============================================================================
// Schema Types - For schema introspection
// =============================================================================

// SchemaResult represents the full schema dump.
type SchemaResult struct {
	Version   int                       `json:"version"`
	Types     map[string]TypeSchema     `json:"types"`
	Traits    map[string]TraitSchema    `json:"traits"`
	Templates map[string]TemplateSchema `json:"templates,omitempty"`
	Queries   map[string]SavedQueryInfo `json:"queries,omitempty"`
}

// TypeSchema represents a type definition.
type TypeSchema struct {
	Name            string                 `json:"name"`
	Builtin         bool                   `json:"builtin"`
	DefaultPath     string                 `json:"default_path,omitempty"`
	Description     string                 `json:"description,omitempty"`
	NameField       string                 `json:"name_field,omitempty"`
	Template        string                 `json:"template,omitempty"`
	Templates       []string               `json:"templates,omitempty"`
	DefaultTemplate string                 `json:"default_template,omitempty"`
	Fields          map[string]FieldSchema `json:"fields,omitempty"`
}

// TemplateSchema represents a schema-level template definition.
type TemplateSchema struct {
	ID          string `json:"id"`
	File        string `json:"file"`
	Description string `json:"description,omitempty"`
}

// FieldSchema represents a field definition.
type FieldSchema struct {
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	Values      []string `json:"values,omitempty"`
	Target      string   `json:"target,omitempty"`
	Description string   `json:"description,omitempty"`
}

// TraitSchema represents a trait definition.
type TraitSchema struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Values  []string `json:"values,omitempty"`
	Default string   `json:"default,omitempty"`
}

// CommandSchema describes a command for agent discovery.
type CommandSchema struct {
	Description   string                `json:"description"`
	DefaultTarget string                `json:"default_target,omitempty"`
	Args          []string              `json:"args,omitempty"`
	Flags         map[string]FlagSchema `json:"flags,omitempty"`
	Examples      []string              `json:"examples,omitempty"`
	UseCases      []string              `json:"use_cases,omitempty"`
}

// FlagSchema describes a command flag.
type FlagSchema struct {
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description"`
	Examples    []string `json:"examples,omitempty"`
}
