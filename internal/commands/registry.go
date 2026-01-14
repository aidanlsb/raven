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
	FlagTypeKeyValue    FlagType = "key=value"   // For repeatable flags: --field name=value, --input name=value
	FlagTypePosKeyValue FlagType = "pos-key=value" // For positional key=value args (e.g., `set <id> field=value...`)
	FlagTypeStringSlice FlagType = "stringSlice" // For repeatable string flags
)

// Registry holds all registered commands.
var Registry = map[string]Meta{
	"new": {
		Name:        "new",
		Description: "Create a new typed object",
		LongDesc: `Creates a new note with the specified type.

⚠️ IMPORTANT FOR AGENTS: ALWAYS use this command to create new vault objects instead
of writing files directly with 'echo', 'touch', or file writing tools. The raven_new
command applies templates, validates against the schema, and ensures proper indexing.

The type is required. If title is not provided, you will be prompted for it.
Required fields (as defined in schema.yaml) will be prompted for interactively,
or can be provided via --field flags.

If the type has a name_field configured (e.g., name_field: name), the title
argument automatically populates that field. This means for a person type with
name_field: name, you can just call: rvn new person "Freya" --json
and the name field will be set to "Freya" automatically.

For agents/MCP: Raven runs non-interactively with --json, so title must be provided.
For agents: If required fields are missing, returns error with details including
a retry_with template. Check if the type has name_field set (via raven_schema type <name>)
to understand which fields are auto-populated.`,
		Args: []ArgMeta{
			{Name: "type", Description: "Object type (e.g., person, project)", Required: true, DynamicComp: "types"},
			// NOTE: Title is optional in interactive CLI mode, but required in --json (MCP) mode.
			{Name: "title", Description: "Title/name for the object (auto-populates name_field if configured)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "field", Description: "Set field value (repeatable) (key-value object)", Type: FlagTypeKeyValue, Examples: []string{`{"name": "Freya", "email": "a@b.com"}`}},
		},
		Examples: []string{
			"rvn new person \"Freya\" --json",
			"rvn new project \"Website Redesign\" --json",
			"rvn new book \"The Prose Edda\" --field author=people/snorri --json",
		},
		UseCases: []string{
			"Create a new typed object (NEVER write vault files directly)",
			"Create a new person entry with schema validation",
			"Create a new project file with template applied",
		},
	},
	"add": {
		Name:        "add",
		Description: "Append content to existing file or daily note",
		LongDesc: `Quickly capture a thought, task, or note.

By default, appends to today's daily note. 
Only works on files that already exist (daily notes are auto-created).
Timestamps are OFF by default; use --timestamp to include the current time.
For creating NEW typed objects, use 'rvn new' instead.

Bulk operations:
Use --stdin to read object IDs from stdin (one per line).
IMPORTANT: Bulk operations return preview by default. Changes are NOT applied unless confirm=true.`,
		Args: []ArgMeta{
			{Name: "text", Description: "Text to add (can include @traits and [[refs]])", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "to", Description: "Target EXISTING file path (must exist)", Type: FlagTypeString, Examples: []string{"projects/website.md", "inbox.md"}},
			{Name: "timestamp", Description: "Prefix with current time (HH:MM)", Type: FlagTypeBool},
			{Name: "stdin", Description: "Read object IDs from stdin for bulk operations", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply bulk changes (without this flag, shows preview only)", Type: FlagTypeBool},
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

⚠️ IMPORTANT FOR AGENTS: ALWAYS use this command instead of shell commands like 'rm'.
Using 'rm' directly will NOT warn about backlinks (other files that reference this one),
potentially creating broken links throughout the vault. The raven_delete command:
- Warns about incoming backlinks before deletion
- Moves files to .trash/ for recovery (not permanent deletion)
- Updates the index properly

By default, files are moved to a trash directory (.trash/).
Warns about backlinks (objects that reference the deleted item).

Bulk operations:
Use --stdin to read object IDs from stdin (one per line).
IMPORTANT: Bulk operations return preview by default. Changes are NOT applied unless confirm=true.`,
		Args: []ArgMeta{
			{Name: "object_id", Description: "Object ID to delete (e.g., people/freya)", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "force", Description: "Skip confirmation prompt", Type: FlagTypeBool},
			{Name: "stdin", Description: "Read object IDs from stdin for bulk operations", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply bulk changes (without this flag, shows preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn delete people/freya --json",
			"rvn delete projects/old --force --json",
		},
		UseCases: []string{
			"Delete a file safely (NEVER use 'rm' shell command)",
			"Remove objects while checking for broken links",
			"Move files to trash with backlink warnings",
		},
	},
	"move": {
		Name:        "move",
		Description: "Move or rename an object within the vault",
		LongDesc: `Move or rename a file/object within the vault.

⚠️ IMPORTANT FOR AGENTS: ALWAYS use this command instead of shell commands like 'mv'.
Using 'mv' directly will NOT update references to the file, causing broken links
throughout the vault. The raven_move command automatically updates all [[references]]
that point to the moved file.

SECURITY: Both source and destination must be within the vault.
Files cannot be moved outside the vault, and external files cannot be moved in.

This command:
- Validates paths are within the vault
- Updates all references to the moved file (--update-refs, default: true)
- Warns if moving to a type's default directory with mismatched type
- Creates destination directories if needed

If moving a file to a type's default directory (e.g., people/) but the file
has a different type, returns a warning with needs_confirm=true. The agent
should ask the user how to proceed.

Bulk operations:
Use --stdin to read object IDs from stdin (one per line).
Destination must be a directory (ending with /).
IMPORTANT: Bulk operations return preview by default. Changes are NOT applied unless confirm=true.`,
		Args: []ArgMeta{
			{Name: "source", Description: "Source file path (e.g., inbox/note.md or people/loki)", Required: false},
			{Name: "destination", Description: "Destination path (e.g., people/loki-archived or archive/projects/)", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "force", Description: "Skip confirmation prompts", Type: FlagTypeBool},
			{Name: "update-refs", Description: "Update references to moved file (default: true)", Type: FlagTypeBool, Default: "true"},
			{Name: "skip-type-check", Description: "Skip type-directory mismatch warning", Type: FlagTypeBool},
			{Name: "stdin", Description: "Read object IDs from stdin for bulk operations", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply bulk changes (without this flag, shows preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn move people/loki people/loki-archived --json",
			"rvn move inbox/task.md projects/website/task.md --json",
			"rvn move drafts/person.md people/freya.md --update-refs --json",
		},
		UseCases: []string{
			"Rename a file in place (NEVER use 'mv' shell command)",
			"Move file to different directory with reference updates",
			"Reorganize vault structure while keeping links intact",
			"Archive old content without breaking references",
		},
	},
	"query": {
		Name:        "query",
		Description: "Run a query using the Raven query language",
		LongDesc: `Query objects or traits using the Raven query language.

Query syntax:
- Object queries: object:<type> [predicates...]
  Examples: object:project .status:active, object:meeting refs:[[people/freya]]
- Trait queries: trait:<name> [predicates...]
  Examples: trait:due value:past, trait:highlight on:book

Common predicates:
- .field:value — Filter by field (.status:active, .priority:high)
- has:trait — Has trait directly (has:due, has:priority)
- refs:[[target]] — References target (refs:[[people/freya]])
- within:type — Trait is inside object type (within:meeting)
- value:X — Trait value equals X (value:past, value:high)

Special date values for trait:due:
- value:past, value:today, value:tomorrow, value:this-week, value:next-week

Use --ids to output just IDs (one per line) for piping to other commands.
Use --apply to run a bulk operation directly on query results.

For bulk operations:
- Returns preview by default. Changes are NOT applied unless confirm=true.
- Supported commands: set, delete, add, move`,
		Args: []ArgMeta{
			{Name: "query_string", Description: "Query string (e.g., 'object:project .status:active' or saved query name)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "list", Description: "List available saved queries", Type: FlagTypeBool},
			{Name: "refresh", Description: "Refresh stale files before query (auto-reindex changed files)", Type: FlagTypeBool},
			{Name: "ids", Description: "Output only object/trait IDs, one per line (for piping)", Type: FlagTypeBool},
			{Name: "apply", Description: "Apply bulk operation to results (e.g., 'set status=done', 'delete', 'add @reviewed')", Type: FlagTypeString},
			{Name: "confirm", Description: "Apply bulk changes (without this flag, shows preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn query 'object:project .status:active' --json",
			"rvn query 'object:meeting has:due' --json",
			"rvn query 'trait:due value:past' --json",
			"rvn query 'trait:due value:past' --ids",
			"rvn query 'trait:due value:past' --apply 'set status=overdue' --json",
			"rvn query 'trait:due value:past' --apply 'set status=overdue' --confirm --json",
			"rvn query tasks --json",
			"rvn query --list --json",
		},
		UseCases: []string{
			"Find objects matching specific criteria",
			"Find traits with specific values",
			"Bulk update query results with --apply",
			"Pipe query results to other commands with --ids",
		},
	},
	"query_add": {
		Name:        "query_add",
		Description: "Add a saved query to raven.yaml",
		Args: []ArgMeta{
			{Name: "name", Description: "Name for the new query", Required: true},
			{Name: "query_string", Description: "Query string (e.g., 'object:project .status:active' or 'trait:due value:past')", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "description", Description: "Human-readable description", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn query add tasks 'trait:due' --json",
			"rvn query add overdue 'trait:due value:past' --json",
			"rvn query add active-projects 'object:project .status:active' --json",
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
		LongDesc: `Read raw file content.

For agents: Use this command instead of shell commands like 'cat', 'head', or 'tail'
to read vault files. This ensures consistent behavior and proper path resolution
within the vault.`,
		Args: []ArgMeta{
			{Name: "path", Description: "File path relative to vault", Required: true},
		},
		Examples: []string{
			"rvn read daily/2025-02-01.md --json",
			"rvn read people/freya.md --json",
		},
		UseCases: []string{
			"Read vault file content (use instead of 'cat', 'head', 'tail')",
			"Inspect file before editing",
			"Get full content after finding object via query",
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

By default, performs an incremental reindex that only processes files that have
changed since the last index. Deleted files are automatically detected and
removed from the index.

Use --full to force a complete rebuild of the entire index.`,
		Examples: []string{
			"rvn reindex",
			"rvn reindex --dry-run",
			"rvn reindex --full",
		},
		Flags: []FlagMeta{
			{Name: "full", Description: "Force full reindex of all files (default is incremental)", Type: FlagTypeBool},
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

Scoping:
- Pass a file path, directory, or reference to check a subset of the vault
- Use --type to check all objects of a specific type
- Use --trait to check all usages of a specific trait
- Use --issues to check only specific issue types
- Use --exclude to skip specific issue types

For agents: Use this tool to discover issues, then use the fix_command suggestions to resolve them.
Ask the user for clarification when needed (e.g., which type to use for missing references).`,
		Args: []ArgMeta{
			{Name: "path", Description: "File, directory, or reference to check (optional, defaults to entire vault)", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "type", Short: "t", Description: "Check only objects of this type", Type: FlagTypeString},
			{Name: "trait", Description: "Check only usages of this trait", Type: FlagTypeString},
			{Name: "issues", Description: "Only check these issue types (comma-separated)", Type: FlagTypeString},
			{Name: "exclude", Description: "Exclude these issue types (comma-separated)", Type: FlagTypeString},
			{Name: "errors-only", Description: "Only report errors, skip warnings", Type: FlagTypeBool},
			{Name: "create-missing", Description: "Interactively create missing pages (CLI only, not for agents)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn check --json",
			"rvn check people/freya.md --json",
			"rvn check projects/ --json",
			"rvn check freya --json",
			"rvn check --type project --json",
			"rvn check --trait due --json",
			"rvn check --issues missing_reference,unknown_type --json",
			"rvn check --exclude unused_type,unused_trait --json",
		},
		UseCases: []string{
			"Validate entire vault for issues",
			"Check a specific file after editing",
			"Verify all objects of a type are valid",
			"Check all trait usages for correct values",
			"Focus on specific issue types",
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
		LongDesc: `Add a new type definition to schema.yaml.

When creating a type, you can specify a name_field which designates which field
serves as the display name for objects of this type. The title argument to
'rvn new' will auto-populate this field.

If the name_field doesn't exist, it will be auto-created as a required string field.

For agents: When helping users create types, ask what field should be used as the
display name. Common choices are 'name' (for people, companies) or 'title' 
(for documents, projects).`,
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the new type", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "default-path", Description: "Default directory for files of this type", Type: FlagTypeString, Examples: []string{"people/", "projects/"}},
			{Name: "name-field", Description: "Field to use as display name (auto-created if doesn't exist)", Type: FlagTypeString, Examples: []string{"name", "title"}},
		},
		Examples: []string{
			"rvn schema add type person --name-field name --default-path people/ --json",
			"rvn schema add type project --name-field title --default-path projects/ --json",
		},
		UseCases: []string{
			"Create a new type for organizing objects",
			"Define a type with a display name field for easier object creation",
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
		LongDesc: `Add a field to an existing type definition.

Field Types:
  string      Plain text value
  string[]    Array of text values (e.g., tags)
  number      Numeric value
  date        Date in YYYY-MM-DD format
  datetime    Date and time
  bool        Boolean (true/false)
  enum        Single value from a list (requires --values)
  enum[]      Multiple values from a list (requires --values)
  ref         Reference to another object (requires --target)
  ref[]       Array of references (requires --target)

Common patterns:
  Single reference:     --type ref --target person
  Array of references:  --type ref[] --target person
  Tags/keywords:        --type string[]
  Status field:         --type enum --values active,paused,done

The command validates inputs and provides helpful suggestions if the syntax is incorrect.`,
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type to add field to", Required: true},
			{Name: "field_name", Description: "Name of the new field", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "type", Description: "Field type: string, number, date, datetime, bool, enum, ref (add [] for arrays)", Type: FlagTypeString, Default: "string"},
			{Name: "required", Description: "Mark field as required", Type: FlagTypeBool},
			{Name: "target", Description: "Target type for ref/ref[] fields (required for references)", Type: FlagTypeString},
			{Name: "values", Description: "Allowed values for enum/enum[] fields (comma-separated)", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema add field person email --type string --required --json",
			"rvn schema add field person tags --type string[] --json",
			"rvn schema add field project owner --type ref --target person --json",
			"rvn schema add field team members --type ref[] --target person --json",
			"rvn schema add field project status --type enum --values active,paused,done --json",
		},
		UseCases: []string{
			"Add a text field to a type",
			"Add an array field for tags or keywords",
			"Add a reference to link objects together",
			"Add an array of references (e.g., team members, attendees)",
			"Add an enum field with predefined choices",
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
		LongDesc: `Update an existing type definition in schema.yaml.

Use --name-field to set or change which field serves as the display name.
If the field doesn't exist, it will be auto-created as a required string field.
Use --name-field="-" to remove the name_field setting.`,
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the type to update", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "default-path", Description: "Update default directory for files", Type: FlagTypeString},
			{Name: "name-field", Description: "Set/update display name field (use '-' to remove)", Type: FlagTypeString, Examples: []string{"name", "title", "-"}},
			{Name: "add-trait", Description: "Add a trait to this type", Type: FlagTypeString},
			{Name: "remove-trait", Description: "Remove a trait from this type", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema update type person --name-field name --json",
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
	"schema_rename_type": {
		Name:        "schema rename type",
		Description: "Rename a type and update all references",
		LongDesc: `Rename a type in schema.yaml and update all files that use it.

This command:
1. Renames the type in schema.yaml
2. Updates all 'type:' frontmatter fields in files
3. Updates all ::type() embedded declarations
4. Updates all ref field targets pointing to the old type

IMPORTANT: Returns preview by default. Changes are NOT applied unless confirm=true.

For agents: After renaming, run raven_reindex(full=true) to update the index.`,
		Args: []ArgMeta{
			{Name: "old_name", Description: "Current type name", Required: true},
			{Name: "new_name", Description: "New type name", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "confirm", Description: "Apply the rename (default: preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn schema rename type event meeting --json",
			"rvn schema rename type event meeting --confirm --json",
		},
		UseCases: []string{
			"Rename a type while keeping all files valid",
			"Refactor schema type names",
			"Migrate from old naming conventions",
		},
	},
	"set": {
		Name:        "set",
		Description: "Set frontmatter fields on an object",
		LongDesc: `Set one or more frontmatter fields on an existing object.

The object ID can be a full path (e.g., "people/freya") or a short reference
that uniquely identifies an object. Field values are validated against the
schema if the object has a known type.

Use this to update existing objects' metadata without manually editing files.

Bulk operations:
Use --stdin to read object IDs from stdin (one per line).
IMPORTANT: Bulk operations return preview by default. Changes are NOT applied unless confirm=true.`,
		Args: []ArgMeta{
			{Name: "object_id", Description: "Object to update (e.g., people/freya)", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "fields", Description: "Fields to update (object with key-value pairs)", Type: FlagTypePosKeyValue, Examples: []string{`{"email": "freya@asgard.realm"}`, `{"status": "active", "priority": "high"}`}},
			{Name: "stdin", Description: "Read object IDs from stdin for bulk operations", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply bulk changes (without this flag, shows preview only)", Type: FlagTypeBool},
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
			"Bulk update multiple objects via --stdin",
		},
	},
	"edit": {
		Name:        "edit",
		Description: "Surgical text replacement in vault files",
		LongDesc: `Replace a unique string in a vault file with another string.

⚠️ IMPORTANT FOR AGENTS: Use this command instead of shell tools like 'sed' or 'awk'
to edit vault files. This ensures proper preview/confirm workflow and maintains
file integrity within the vault.

The string to replace must appear exactly once in the file to prevent 
ambiguous edits.

IMPORTANT: Returns preview by default. Changes are NOT applied unless confirm=true.

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
			"Edit vault files (use instead of 'sed', 'awk', or direct file writes)",
			"Add wiki links to existing text",
			"Fix typos in notes",
			"Add traits to existing lines",
			"Delete specific content with preview",
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
	"daily": {
		Name:        "daily",
		Description: "Open or create a daily note",
		LongDesc: `Open or create a daily note for a given date.

If no date is provided, opens today's note. Creates the file if it doesn't exist.`,
		Args: []ArgMeta{
			{Name: "date", Description: "Date (today, yesterday, tomorrow, YYYY-MM-DD)", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "edit", Short: "e", Description: "Open the note in the configured editor", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn daily --json",
			"rvn daily yesterday --json",
			"rvn daily 2025-02-01 --json",
		},
		UseCases: []string{
			"Access or create today's daily note",
			"Navigate to past daily notes",
		},
	},
	"untyped": {
		Name:        "untyped",
		Description: "List pages without an explicit type",
		LongDesc:    `Lists all markdown files that don't have an explicit type in their frontmatter (fallback to 'page' type).`,
		Examples: []string{
			"rvn untyped --json",
		},
		UseCases: []string{
			"Find notes that need to be typed",
			"Identify pages that could benefit from schema",
		},
	},
	"open": {
		Name:        "open",
		Description: "Open a file in your editor",
		LongDesc: `Opens a file in your configured editor.

The reference can be a short reference (cursor), a partial path (companies/cursor),
or a full path (objects/companies/cursor.md).

The editor is determined by the 'editor' setting in config.toml or $EDITOR.`,
		Args: []ArgMeta{
			{Name: "reference", Description: "Reference to the file (short name, partial path, or full path)", Required: true},
		},
		Examples: []string{
			"rvn open cursor --json",
			"rvn open companies/cursor --json",
			"rvn open people/freya --json",
		},
		UseCases: []string{
			"Quickly open a file by its short name",
			"Open files using references without knowing full paths",
		},
	},
	"workflow_list": {
		Name:        "workflow list",
		Description: "List available workflows",
		LongDesc:    `Lists all workflows defined in raven.yaml with their descriptions and required inputs.`,
		Examples: []string{
			"rvn workflow list --json",
		},
		UseCases: []string{
			"Discover available workflows",
			"See what inputs each workflow requires",
		},
	},
	"workflow_show": {
		Name:        "workflow show",
		Description: "Show workflow details",
		LongDesc:    `Shows the full definition of a workflow including inputs, context queries, and prompt.`,
		Args: []ArgMeta{
			{Name: "name", Description: "Workflow name", Required: true},
		},
		Examples: []string{
			"rvn workflow show meeting-prep --json",
		},
		UseCases: []string{
			"Inspect workflow configuration",
			"Understand what a workflow does before running it",
		},
	},
	"workflow_render": {
		Name:        "workflow render",
		Description: "Render a workflow with context",
		LongDesc: `Renders a workflow and returns the prompt with pre-gathered context.

This command:
1. Loads the workflow definition
2. Validates inputs
3. Runs all context queries (read, query, backlinks, search)
4. Renders the template with input and context substitution
5. Returns the complete prompt and gathered context

The returned prompt and context are ready for an agent to execute.`,
		Args: []ArgMeta{
			{Name: "name", Description: "Workflow name", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "input", Description: "Set input value (repeatable)", Type: FlagTypeKeyValue, Examples: []string{"meeting_id=meetings/alice-1on1", "question=How does auth work?"}},
		},
		Examples: []string{
			"rvn workflow render meeting-prep --input meeting_id=meetings/alice-1on1 --json",
			"rvn workflow render research --input question=\"How does the auth system work?\" --json",
		},
		UseCases: []string{
			"Execute a workflow with specific inputs",
			"Get a pre-formatted prompt for agent execution",
			"Gather context before running a complex task",
		},
	},
}
