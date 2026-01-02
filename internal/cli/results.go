// Package cli implements the command-line interface.
// This file defines shared JSON result types for consistent CLI output.
package cli

// =============================================================================
// Core Result Types - Used across multiple commands
// =============================================================================

// ObjectResult represents an object in query results.
// Used by: type, query, backlinks
type ObjectResult struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	FilePath  string                 `json:"file_path"`
	LineStart int                    `json:"line_start"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// TraitResult represents a trait in query results.
// Used by: trait, query
type TraitResult struct {
	ID          string  `json:"id"`
	TraitType   string  `json:"trait_type"`
	Value       *string `json:"value,omitempty"`
	Content     string  `json:"content"`
	ContentText string  `json:"content_text"`
	ObjectID    string  `json:"object_id"`
	FilePath    string  `json:"file_path"`
	Line        int     `json:"line"`
}

// BacklinkResult represents a backlink.
// Used by: backlinks
type BacklinkResult struct {
	SourceID    string  `json:"source_id"`
	FilePath    string  `json:"file_path"`
	Line        *int    `json:"line,omitempty"`
	DisplayText *string `json:"display_text,omitempty"`
}

// TagResult represents a tag query result.
// Used by: tag
type TagResult struct {
	Tag      string `json:"tag"`
	ObjectID string `json:"object_id"`
	FilePath string `json:"file_path"`
	Line     *int   `json:"line,omitempty"`
}

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

// TagSummary represents a tag with its count.
// Used by: tag --list
type TagSummary struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
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
	TagCount    int `json:"tag_count,omitempty"`
}

// =============================================================================
// Query Types
// =============================================================================

// SavedQueryInfo represents a saved query definition.
// Used by: query --list
type SavedQueryInfo struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Types       []string          `json:"types,omitempty"`
	Traits      []string          `json:"traits,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Filters     map[string]string `json:"filters,omitempty"`
}

// QueryResult represents results from running a saved query.
// Used by: query <name>
type QueryResult struct {
	QueryName string              `json:"query_name"`
	Types     []TypeQueryResult   `json:"types,omitempty"`
	Traits    []TraitResult       `json:"traits,omitempty"`
	Tags      []TagQueryResult    `json:"tags,omitempty"`
}

// TypeQueryResult represents objects of a type in query results.
type TypeQueryResult struct {
	Type  string         `json:"type"`
	Items []ObjectResult `json:"items"`
}

// TagQueryResult represents tag results in a query.
type TagQueryResult struct {
	Tags  []string `json:"tags"`
	Items []string `json:"items"` // Object IDs
}

// =============================================================================
// Schema Types - For schema introspection
// =============================================================================

// SchemaResult represents the full schema dump.
type SchemaResult struct {
	Version int                      `json:"version"`
	Types   map[string]TypeSchema    `json:"types"`
	Traits  map[string]TraitSchema   `json:"traits"`
	Queries map[string]SavedQueryInfo `json:"queries,omitempty"`
}

// TypeSchema represents a type definition.
type TypeSchema struct {
	Name           string                 `json:"name"`
	Builtin        bool                   `json:"builtin"`
	DefaultPath    string                 `json:"default_path,omitempty"`
	Fields         map[string]FieldSchema `json:"fields,omitempty"`
	Traits         []string               `json:"traits,omitempty"`
	RequiredTraits []string               `json:"required_traits,omitempty"`
}

// FieldSchema represents a field definition.
type FieldSchema struct {
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Default  string   `json:"default,omitempty"`
	Values   []string `json:"values,omitempty"`
	Target   string   `json:"target,omitempty"`
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
	Description   string            `json:"description"`
	DefaultTarget string            `json:"default_target,omitempty"`
	Args          []string          `json:"args,omitempty"`
	Flags         map[string]FlagSchema `json:"flags,omitempty"`
	Examples      []string          `json:"examples,omitempty"`
	UseCases      []string          `json:"use_cases,omitempty"`
}

// FlagSchema describes a command flag.
type FlagSchema struct {
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description"`
	Examples    []string `json:"examples,omitempty"`
}
