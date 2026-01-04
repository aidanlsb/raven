// Package commands provides a central registry of Raven CLI commands.
// This registry is the single source of truth for command metadata,
// used by both the CLI and MCP server.
package commands

// Meta defines metadata for a CLI command that can be used to
// generate both Cobra commands and MCP tool schemas.
type Meta struct {
	Name        string     // Command name (e.g., "trait", "add", "new")
	Description string     // Short description
	LongDesc    string     // Long description (for --help)
	Args        []ArgMeta  // Positional arguments
	Flags       []FlagMeta // Command flags
	Examples    []string   // Usage examples
	UseCases    []string   // Agent use cases (for MCP hints)
}

// ArgMeta defines a positional argument.
type ArgMeta struct {
	Name        string   // Argument name
	Description string   // Description
	Required    bool     // Is this argument required?
	Completions []string // Static completions (if any)
	DynamicComp string   // Dynamic completion type: "types", "traits", "files"
}

// FlagMeta defines a command flag.
type FlagMeta struct {
	Name        string   // Flag name (e.g., "value", "to")
	Short       string   // Short flag (e.g., "v" for -v)
	Description string   // Description
	Type        FlagType // Type of flag
	Default     string   // Default value
	Examples    []string // Example values
}

// FlagType represents the type of a flag.
type FlagType string

const (
	FlagTypeString      FlagType = "string"
	FlagTypeBool        FlagType = "bool"
	FlagTypeInt         FlagType = "int"
	FlagTypeKeyValue    FlagType = "key=value"    // For --field name=value
	FlagTypeStringSlice FlagType = "stringSlice"  // For repeatable string flags
)

// Registry holds all registered commands.
var Registry = map[string]Meta{
	"new": {
		Name:        "new",
		Description: "Create a new typed object",
		LongDesc: `Creates a new note with the specified type.

The type is required. If title is not provided, you will be prompted for it.
Required fields (as defined in schema.yaml) will be prompted for interactively,
or can be provided via --field flags.

For agents: If required fields are missing, returns error with details. 
Ask user for values, then retry with --field flags.`,
		Args: []ArgMeta{
			{Name: "type", Description: "Object type (e.g., person, project)", Required: true, DynamicComp: "types"},
			{Name: "title", Description: "Title/name for the object", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "field", Description: "Set field value (repeatable)", Type: FlagTypeKeyValue, Examples: []string{"name=Freya", "email=a@b.com"}},
		},
		Examples: []string{
			"rvn new person \"Freya\" --json",
			"rvn new person \"Freya\" --field name=\"Freya\" --json",
			"rvn new project \"Website Redesign\" --json",
		},
		UseCases: []string{
			"Create a new person entry",
			"Create a new project file",
			"Create any typed object defined in schema",
		},
	},
	"add": {
		Name:        "add",
		Description: "Append content to existing file or daily note",
		LongDesc: `Quickly capture a thought, task, or note.

By default, appends to today's daily note. 
Only works on files that already exist (daily notes are auto-created).
Timestamps are OFF by default; use --timestamp to include the current time.
For creating NEW typed objects, use 'rvn new' instead.`,
		Args: []ArgMeta{
			{Name: "text", Description: "Text to add (can include @traits and [[refs]])", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "to", Description: "Target EXISTING file path (must exist)", Type: FlagTypeString, Examples: []string{"projects/website.md", "inbox.md"}},
			{Name: "timestamp", Description: "Prefix with current time (HH:MM)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn add \"Quick thought\" --json",
			"rvn add \"@priority(high) Urgent task\" --json",
			"rvn add \"Note\" --to projects/website.md --json",
			"rvn add \"Call Odin\" --timestamp --json",
		},
		UseCases: []string{
			"Quick capture to daily note",
			"Add tasks to existing project files",
			"Append notes to existing documents",
			"Log timestamped events with --timestamp",
		},
	},
	"delete": {
		Name:        "delete",
		Description: "Delete an object from the vault",
		LongDesc: `Delete a file/object from the vault.

By default, files are moved to a trash directory (.trash/).
Warns about backlinks (objects that reference the deleted item).`,
		Args: []ArgMeta{
			{Name: "object_id", Description: "Object ID to delete (e.g., people/freya)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "force", Description: "Skip confirmation prompt", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn delete people/freya --json",
			"rvn delete projects/old --force --json",
		},
	},
	"move": {
		Name:        "move",
		Description: "Move or rename an object within the vault",
		LongDesc: `Move or rename a file/object within the vault.

SECURITY: Both source and destination must be within the vault.
Files cannot be moved outside the vault, and external files cannot be moved in.

This command:
- Validates paths are within the vault
- Updates all references to the moved file (--update-refs, default: true)
- Warns if moving to a type's default directory with mismatched type
- Creates destination directories if needed

If moving a file to a type's default directory (e.g., people/) but the file
has a different type, returns a warning with needs_confirm=true. The agent
should ask the user how to proceed.`,
		Args: []ArgMeta{
			{Name: "source", Description: "Source file path (e.g., inbox/note.md or people/loki)", Required: true},
			{Name: "destination", Description: "Destination path (e.g., people/loki-archived or archive/note.md)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "force", Description: "Skip confirmation prompts", Type: FlagTypeBool},
			{Name: "update-refs", Description: "Update references to moved file (default: true)", Type: FlagTypeBool, Default: "true"},
			{Name: "skip-type-check", Description: "Skip type-directory mismatch warning", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn move people/loki people/loki-archived --json",
			"rvn move inbox/task.md projects/website/task.md --json",
			"rvn move drafts/person.md people/freya.md --update-refs --json",
		},
		UseCases: []string{
			"Rename a file in place",
			"Move file to different directory",
			"Reorganize vault structure",
			"Archive old content",
		},
	},
	"query": {
		Name:        "query",
		Description: "Run a query using the Raven query language",
		Args: []ArgMeta{
			{Name: "query_string", Description: "Query string (e.g., 'object:project .status:active' or saved query name)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "list", Description: "List available saved queries", Type: FlagTypeBool},
			{Name: "refresh", Description: "Refresh stale files before query (auto-reindex changed files)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn query 'object:project .status:active' --json",
			"rvn query 'object:meeting has:due' --json",
			"rvn query 'object:meeting refs:[[people/freya]]' --json",
			"rvn query 'trait:due value:past' --json",
			"rvn query 'trait:highlight on:{object:book .status:reading}' --json",
			"rvn query tasks --json",
			"rvn query --list --json",
			"rvn query 'object:project' --refresh --json",
		},
	},
	"query_add": {
		Name:        "query_add",
		Description: "Add a saved query to raven.yaml",
		Args: []ArgMeta{
			{Name: "name", Description: "Name for the new query", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "traits", Description: "Traits to query (comma-separated)", Type: FlagTypeStringSlice},
			{Name: "types", Description: "Types to query (comma-separated)", Type: FlagTypeStringSlice},
			{Name: "filter", Description: "Filter in key=value format (repeatable)", Type: FlagTypeStringSlice},
			{Name: "description", Description: "Human-readable description", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn query add overdue --traits due --filter due=past --json",
			"rvn query add my-tasks --traits due,status --filter status=todo --json",
			"rvn query add people --types person --json",
		},
	},
	"query_remove": {
		Name:        "query_remove",
		Description: "Remove a saved query from raven.yaml",
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the query to remove", Required: true, DynamicComp: "queries"},
		},
		Examples: []string{
			"rvn query remove overdue --json",
		},
	},
	"backlinks": {
		Name:        "backlinks",
		Description: "Find objects that reference a target",
		Args: []ArgMeta{
			{Name: "target", Description: "Target object ID (e.g., people/freya)", Required: true},
		},
		Examples: []string{
			"rvn backlinks people/freya --json",
		},
	},
	"date": {
		Name:        "date",
		Description: "Date hub - all activity for a date",
		Args: []ArgMeta{
			{Name: "date", Description: "Date (today, yesterday, YYYY-MM-DD)", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "edit", Description: "Open the daily note in editor", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn date today --json",
			"rvn date 2025-02-01 --json",
		},
	},
	"read": {
		Name:        "read",
		Description: "Read raw file content",
		Args: []ArgMeta{
			{Name: "path", Description: "File path relative to vault", Required: true},
		},
		Examples: []string{
			"rvn read daily/2025-02-01.md --json",
			"rvn read people/freya.md --json",
		},
	},
	"stats": {
		Name:        "stats",
		Description: "Show vault statistics",
		Examples: []string{
			"rvn stats --json",
		},
	},
	"reindex": {
		Name:        "reindex",
		Description: "Rebuild the SQLite index from all vault files",
		LongDesc: `Parses all markdown files in the vault and rebuilds the SQLite index.

Use this after:
- Bulk file operations outside of Raven
- Schema changes that affect indexing
- Recovering from index corruption

By default, reindexes all files. Use --smart for incremental mode that only
reindexes files that have changed since the last index.`,
		Examples: []string{
			"rvn reindex",
			"rvn reindex --smart",
			"rvn reindex --smart --dry-run",
		},
		Flags: []FlagMeta{
			{Name: "smart", Description: "Only reindex files that have changed since last index", Type: FlagTypeBool},
			{Name: "dry-run", Description: "Show what would be reindexed without doing it", Type: FlagTypeBool},
		},
	},
	"check": {
		Name:        "check",
		Description: "Validate vault against schema",
		LongDesc: `Validates all files in the vault against the schema.

Returns structured issues with:
- issue_type: unknown_type, missing_reference, undefined_trait, unknown_frontmatter_key, etc.
- fix_command: Suggested CLI command to fix the issue
- fix_hint: Human-readable explanation of how to fix

The summary groups issues by type with counts and top values, making it easy to prioritize fixes.

For agents: Use this tool to discover issues, then use the fix_command suggestions to resolve them.
Ask the user for clarification when needed (e.g., which type to use for missing references).`,
		Flags: []FlagMeta{
			{Name: "create-missing", Description: "Interactively create missing pages (CLI only, not for agents)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn check --json",
		},
	},
	"schema": {
		Name:        "schema",
		Description: "Introspect the schema",
		Args: []ArgMeta{
			{Name: "subcommand", Description: "types, traits, commands, type <name>, trait <name>", Required: false},
		},
		Examples: []string{
			"rvn schema --json",
			"rvn schema types --json",
			"rvn schema commands --json",
		},
	},
	"schema_add_type": {
		Name:        "schema add type",
		Description: "Add a new type to the schema",
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the new type", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "default-path", Description: "Default directory for files of this type", Type: FlagTypeString, Examples: []string{"people/", "projects/"}},
		},
		Examples: []string{
			"rvn schema add type event --default-path events/ --json",
		},
	},
	"schema_add_trait": {
		Name:        "schema add trait",
		Description: "Add a new trait to the schema",
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the new trait", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "type", Description: "Trait type (string, date, enum, bool)", Type: FlagTypeString, Default: "string"},
			{Name: "values", Description: "Enum values (comma-separated)", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema add trait priority --type enum --values high,medium,low --json",
		},
	},
	"schema_add_field": {
		Name:        "schema add field",
		Description: "Add a field to an existing type",
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type to add field to", Required: true},
			{Name: "field_name", Description: "Name of the new field", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "type", Description: "Field type (string, date, enum, ref, bool)", Type: FlagTypeString, Default: "string"},
			{Name: "required", Description: "Mark field as required", Type: FlagTypeBool},
			{Name: "target", Description: "Target type for ref fields", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema add field person email --type string --required --json",
		},
	},
	"schema_validate": {
		Name:        "schema validate",
		Description: "Validate the schema for correctness",
		Examples: []string{
			"rvn schema validate --json",
		},
	},
	"schema_update_type": {
		Name:        "schema update type",
		Description: "Update an existing type in the schema",
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the type to update", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "default-path", Description: "Update default directory for files", Type: FlagTypeString},
			{Name: "add-trait", Description: "Add a trait to this type", Type: FlagTypeString},
			{Name: "remove-trait", Description: "Remove a trait from this type", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema update type person --default-path people/ --json",
			"rvn schema update type meeting --add-trait due --json",
		},
	},
	"schema_update_trait": {
		Name:        "schema update trait",
		Description: "Update an existing trait in the schema",
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the trait to update", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "type", Description: "Update trait type", Type: FlagTypeString},
			{Name: "values", Description: "Update enum values (comma-separated)", Type: FlagTypeString},
			{Name: "default", Description: "Update default value", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema update trait priority --values critical,high,medium,low --json",
		},
	},
	"schema_update_field": {
		Name:        "schema update field",
		Description: "Update a field on an existing type",
		LongDesc: `Update an existing field's properties.

Note: Making a field required will be blocked if any objects lack that field.
Add the field to all objects first, then make it required.`,
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type containing the field", Required: true},
			{Name: "field_name", Description: "Field to update", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "type", Description: "Update field type", Type: FlagTypeString},
			{Name: "required", Description: "Update required status (true/false)", Type: FlagTypeString},
			{Name: "default", Description: "Update default value", Type: FlagTypeString},
			{Name: "target", Description: "Update target type for ref fields", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema update field person email --required=true --json",
			"rvn schema update field project status --default=active --json",
		},
	},
	"schema_remove_type": {
		Name:        "schema remove type",
		Description: "Remove a type from the schema",
		LongDesc: `Remove a type definition from schema.yaml.

Existing files of this type will become 'page' type (fallback).
Use --force to skip confirmation prompt.`,
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the type to remove", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "force", Description: "Skip confirmation prompt", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn schema remove type event --json",
			"rvn schema remove type legacy --force --json",
		},
	},
	"schema_remove_trait": {
		Name:        "schema remove trait",
		Description: "Remove a trait from the schema",
		LongDesc: `Remove a trait definition from schema.yaml.

Existing @trait instances will remain in files but no longer be indexed.
Use --force to skip confirmation prompt.`,
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the trait to remove", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "force", Description: "Skip confirmation prompt", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn schema remove trait priority --json",
		},
	},
	"schema_remove_field": {
		Name:        "schema remove field",
		Description: "Remove a field from a type",
		LongDesc: `Remove a field from a type definition.

If the field is required, removal will be blocked until you make it optional first.
Existing field values will remain in files but no longer be validated.`,
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type containing the field", Required: true},
			{Name: "field_name", Description: "Field to remove", Required: true},
		},
		Examples: []string{
			"rvn schema remove field person nickname --json",
		},
	},
	"set": {
		Name:        "set",
		Description: "Set frontmatter fields on an object",
		LongDesc: `Set one or more frontmatter fields on an existing object.

The object ID can be a full path (e.g., "people/freya") or a short reference
that uniquely identifies an object. Field values are validated against the
schema if the object has a known type.

Use this to update existing objects' metadata without manually editing files.`,
		Args: []ArgMeta{
			{Name: "object_id", Description: "Object to update (e.g., people/freya)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "fields", Description: "Fields to update (object with key-value pairs)", Type: FlagTypeKeyValue, Examples: []string{`{"email": "freya@asgard.realm"}`, `{"status": "active", "priority": "high"}`}},
		},
		Examples: []string{
			"rvn set people/freya email=freya@asgard.realm --json",
			"rvn set people/freya name=\"Freya\" status=active --json",
			"rvn set projects/website priority=high --json",
		},
		UseCases: []string{
			"Update a person's email or status",
			"Change project priority or status",
			"Set task due dates or assignments",
			"Modify any frontmatter field on an object",
		},
	},
	"edit": {
		Name:        "edit",
		Description: "Surgical text replacement in vault files",
		LongDesc: `Replace a unique string in a vault file with another string.

The string to replace must appear exactly once in the file to prevent 
ambiguous edits. By default, returns a preview requiring confirmation.
Call with --confirm to apply the edit.

Whitespace matters—old_str must match exactly including indentation.
For multi-line replacements, include newlines in both old_str and new_str.`,
		Args: []ArgMeta{
			{Name: "path", Description: "File path relative to vault root", Required: true},
			{Name: "old_str", Description: "String to replace (must be unique in file)", Required: true},
			{Name: "new_str", Description: "Replacement string (can be empty to delete)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "confirm", Description: "Apply the edit (default: preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			`rvn edit "daily/2025-12-27.md" "- Churn analysis" "- [[churn-analysis|Churn analysis]]" --json`,
			`rvn edit "pages/notes.md" "reccommendation" "recommendation" --confirm --json`,
			`rvn edit "daily/2026-01-02.md" "- old task" "" --confirm --json`,
		},
		UseCases: []string{
			"Add wiki links to existing text",
			"Fix typos in notes",
			"Add traits to existing lines",
			"Delete specific content",
			"Make precise edits without overwriting files",
		},
	},
	"search": {
		Name:        "search",
		Description: "Full-text search across all vault content",
		LongDesc: `Search for content across all files in the vault.

Uses full-text search with relevance ranking. Supports:
  - Simple words: "meeting notes" (finds pages containing both words)
  - Phrases: '"team meeting"' (exact phrase match)
  - Prefix matching: "meet*" (matches meeting, meetings, etc.)
  - Boolean: "meeting AND notes", "meeting OR notes", "meeting NOT private"

Results are ranked by relevance with snippets showing matched content.
Use --type to filter results to specific object types.`,
		Args: []ArgMeta{
			{Name: "query", Description: "Search query (words, phrases, or boolean expressions)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "limit", Short: "n", Description: "Maximum number of results (default: 20)", Type: FlagTypeInt, Default: "20"},
			{Name: "type", Short: "t", Description: "Filter by object type", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn search \"meeting notes\" --json",
			"rvn search \"project*\" --type project --json",
			"rvn search '\"world tree\"' --limit 5 --json",
			"rvn search \"freya OR thor\" --json",
		},
		UseCases: []string{
			"Find pages mentioning specific topics",
			"Search for content across the entire vault",
			"Locate pages by partial matches",
			"Find all mentions of a person or concept",
		},
	},
}
