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
	FlagTypeKeyValue    FlagType = "key=value"     // For repeatable flags: --field name=value, --input name=value
	FlagTypePosKeyValue FlagType = "pos-key=value" // For positional key=value args (e.g., `set <id> field=value...`)
	FlagTypeStringSlice FlagType = "stringSlice"   // For repeatable string flags
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
		Description: "Quick capture - append text to daily note or inbox",
		LongDesc: `Quickly capture a thought, task, or note.

By default, appends to today's daily note. Configure destination in raven.yaml.
Only works on files that already exist (daily notes are auto-created).
Timestamps are OFF by default; use --timestamp to include the current time.
Auto-reindex is ON by default; configure via auto_reindex in raven.yaml.
For creating NEW typed objects, use 'rvn new' instead.

Bulk operations:
Use --stdin to read object IDs from stdin and append text to each.
Bulk operations preview changes by default; use --confirm to apply.

Configuration (raven.yaml):
  capture:
    destination: daily      # "daily" or a file path
    heading: "## Captured"  # Optional heading to append under
    timestamp: false        # Prefix with time (default: false)`,
		Args: []ArgMeta{
			{Name: "text", Description: "Text to add (can include @traits and [[refs]])", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "to", Description: "Target file path or daily note date (today/tomorrow/yesterday/YYYY-MM-DD)", Type: FlagTypeString, Examples: []string{"projects/website.md", "inbox.md", "tomorrow"}},
			{Name: "timestamp", Description: "Prefix with current time (HH:MM)", Type: FlagTypeBool},
			{Name: "stdin", Description: "Read object IDs from stdin for bulk operations", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply bulk changes (without this flag, shows preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn add \"Quick thought\" --json",
			"rvn add \"@priority(high) Urgent task\" --json",
			"rvn add \"Note\" --to projects/website.md --json",
			"rvn add \"Plan\" --to tomorrow --json",
			"rvn add \"Call Odin\" --timestamp --json",
		},
		UseCases: []string{
			"Quick capture to daily note",
			"Add tasks to existing project files",
			"Append notes to existing documents",
			"Log timestamped events with --timestamp",
		},
	},
	"upsert": {
		Name:        "upsert",
		Description: "Create or update a typed object idempotently",
		LongDesc: `Create or update a typed object deterministically.

This command is the canonical idempotent write primitive for generated artifacts.
It creates a new object when missing, or updates the existing one in place.

Semantics:
- Identity is derived from <type> + <title> (same routing/slug logic as 'new')
- Frontmatter fields provided via --field are merged/updated
- If --content is provided, the body is fully replaced (idempotent reruns)
- Returns status: created, updated, or unchanged

Boundary with add:
- add: append-only capture/logging, intentionally non-idempotent
- upsert: canonical state write, idempotent convergence target

Use this for workflow outputs like briefs/reports/summaries where reruns should
converge to one current state rather than append history.`,
		Args: []ArgMeta{
			{Name: "type", Description: "Object type (e.g., brief, report)", Required: true, DynamicComp: "types"},
			{Name: "title", Description: "Title/name for the object (stable identity key)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "field", Description: "Set/update frontmatter fields (repeatable)", Type: FlagTypeKeyValue, Examples: []string{`{"source": "daily-brief", "status": "ready"}`}},
			{Name: "content", Description: "Replace body content (full-body idempotent mode)", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn upsert brief \"Daily Brief 2026-02-14\" --content \"# Daily Brief\" --json",
			"rvn upsert report \"Q1 Status\" --field owner=people/freya --field status=draft --json",
		},
		UseCases: []string{
			"Idempotently persist generated workflow outputs",
			"Create-or-update canonical report/brief objects",
			"Replace an object's body deterministically on reruns",
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
  Examples: object:project .status==active, object:meeting refs([[people/freya]])
- Trait queries: trait:<name> [predicates...]
  Examples: trait:due .value==past, trait:highlight on(object:book)

Common predicates:
- .field==value — Filter by field (.status==active, .priority==high)
- has(trait:...) — Has trait matching subquery
- refs([[target]]) — References target (refs([[people/freya]]))
- refs(object:type) — References objects matching subquery (refs(object:project .status==active))
- within(object:type) — Trait is inside object type (within(object:meeting))
- .value==X — Trait value equals X (.value==past, .value==high)
- content("text") — Full-text search within content (content("meeting notes"))

Special date values for trait:due:
- .value==past, .value==today, .value==tomorrow, .value==this-week, .value==next-week

Saved query inputs must be declared with args: in raven.yaml when using {{args.<name>}}.
You can then pass inputs by position (in args order) or as key=value pairs.

Use --ids to output just IDs (one per line) for piping to other commands.
Use --apply to run a bulk operation directly on query results.

For object queries (object:...):
- Returns preview by default. Changes are NOT applied unless confirm=true.
- Supported commands: set, delete, add, move

For trait queries (trait:...):
- Returns preview by default. Changes are NOT applied unless confirm=true.
- Supported command: update value=<new_value> (updates trait values in-place)
- Example: trait:todo .value==todo --apply "update value=done" marks todos as done`,
		Args: []ArgMeta{
			{Name: "query_string", Description: "Query string (e.g., 'object:project .status==active' or saved query name) optionally followed by saved-query inputs", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "list", Description: "List available saved queries", Type: FlagTypeBool},
			{Name: "refresh", Description: "Refresh stale files before query (auto-reindex changed files)", Type: FlagTypeBool},
			{Name: "ids", Description: "Output only object/trait IDs, one per line (for piping)", Type: FlagTypeBool},
			{Name: "apply", Description: "Apply bulk operation to results (e.g., 'set status=done', 'delete', 'add @reviewed', 'update value=done')", Type: FlagTypeStringSlice},
			{Name: "confirm", Description: "Apply bulk changes (without this flag, shows preview only)", Type: FlagTypeBool},
			{Name: "pipe", Description: "Force pipe-friendly output format", Type: FlagTypeBool},
			{Name: "no-pipe", Description: "Force human-readable output format", Type: FlagTypeBool},
			{Name: "inputs", Description: "Saved query inputs as key=value pairs", Type: FlagTypePosKeyValue, Examples: []string{`{"project": "projects/raven"}`}},
		},
		Examples: []string{
			"rvn query 'object:project .status==active' --json",
			"rvn query 'object:meeting has(trait:due)' --json",
			"rvn query 'trait:due .value==past' --json",
			"rvn query 'trait:due .value==past' --ids",
			"rvn query 'object:project .status==active' --apply 'set status=done' --confirm --json",
			"rvn query 'trait:todo .value==todo' --apply 'update value=done' --confirm --json",
			"rvn query tasks --json",
			"rvn query project-todos raven --json",
			"rvn query project-todos project=projects/raven --json",
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
		Name:        "query add",
		Description: "Add a saved query to raven.yaml",
		Args: []ArgMeta{
			{Name: "name", Description: "Name for the new query", Required: true},
			{Name: "query_string", Description: "Query string (e.g., 'object:project .status==active' or 'trait:due .value==past')", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "description", Description: "Human-readable description", Type: FlagTypeString},
			{Name: "arg", Description: "Declare saved query input name (repeatable, sets positional order)", Type: FlagTypeStringSlice},
		},
		Examples: []string{
			"rvn query add tasks 'trait:due' --json",
			"rvn query add overdue 'trait:due .value==past' --json",
			"rvn query add active-projects 'object:project .status==active' --json",
			"rvn query add project-todos 'trait:todo refs([[{{args.project}}]])' --arg project --json",
		},
	},
	"query_remove": {
		Name:        "query remove",
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
	"outlinks": {
		Name:        "outlinks",
		Description: "Find links referenced by an object",
		Args: []ArgMeta{
			{Name: "source", Description: "Source object ID (e.g., projects/bifrost)", Required: true},
		},
		Examples: []string{
			"rvn outlinks projects/bifrost --json",
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
		Description: "Read a file (raw or enriched)",
		LongDesc: `Read and output a file from the vault.

The reference can be a short reference (freya), partial path (people/freya),
or full path (people/freya.md).

By default, this command returns enriched output (rendered wikilinks + backlinks).
Use --raw to output only the raw file content (recommended for agents preparing precise edits).

For long files, you can request a specific range with --start-line/--end-line, and/or
ask for structured line output with --lines for copy-paste-safe anchors.`,
		Args: []ArgMeta{
			{Name: "path", Description: "Reference to read (short ref, partial path, or full path)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "raw", Description: "Output only raw file content (no backlinks, no rendered links)", Type: FlagTypeBool},
			{Name: "no-links", Description: "Disable clickable hyperlinks in terminal output", Type: FlagTypeBool},
			{Name: "lines", Description: "Include structured lines with line numbers (recommended for agents)", Type: FlagTypeBool},
			{Name: "start-line", Description: "Start line (1-indexed, inclusive) for raw output", Type: FlagTypeInt},
			{Name: "end-line", Description: "End line (1-indexed, inclusive) for raw output", Type: FlagTypeInt},
		},
		Examples: []string{
			"rvn read daily/2025-02-01.md --json",
			"rvn read people/freya.md --json",
			"rvn read people/freya --raw --json",
			"rvn read people/freya --raw --start-line 10 --end-line 40 --json",
			"rvn read people/freya --raw --lines --json",
		},
		UseCases: []string{
			"Read vault file content (use instead of 'cat', 'head', 'tail')",
			"Inspect file before editing (prefer --raw for exact string matching)",
			"Extract copy-paste-safe anchors with --lines or line ranges for long files",
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
	"version": {
		Name:        "version",
		Description: "Show Raven version and build information",
		LongDesc: `Shows version and build metadata for the currently running rvn binary.

Useful for confirming which binary is on PATH after upgrades, especially when
multiple installs exist on the system.`,
		Examples: []string{
			"rvn version",
			"rvn version --json",
		},
		UseCases: []string{
			"Confirm the installed rvn binary version after go install",
			"Diagnose PATH conflicts when multiple rvn binaries exist",
			"Collect build metadata for bug reports",
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
			{Name: "strict", Description: "Treat warnings as errors", Type: FlagTypeBool},
			{Name: "type", Short: "t", Description: "Check only objects of this type", Type: FlagTypeString},
			{Name: "trait", Description: "Check only usages of this trait", Type: FlagTypeString},
			{Name: "issues", Description: "Only check these issue types (comma-separated)", Type: FlagTypeString},
			{Name: "exclude", Description: "Exclude these issue types (comma-separated)", Type: FlagTypeString},
			{Name: "errors-only", Description: "Only report errors, skip warnings", Type: FlagTypeBool},
			{Name: "by-file", Description: "Group issues by file path", Type: FlagTypeBool},
			{Name: "verbose", Short: "V", Description: "Show all issues with full details", Type: FlagTypeBool},
			{Name: "fix", Description: "Auto-fix simple issues (short refs -> full paths)", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply fixes (without this flag, shows preview only)", Type: FlagTypeBool},
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

IMPORTANT - Common mistakes to avoid:
  ✗ --type array              (WRONG: 'array' is not a type)
  ✓ --type string[]           (RIGHT: use [] suffix for arrays)
  
  ✗ --type ref[]              (WRONG: missing --target)
  ✓ --type ref[] --target person  (RIGHT: ref types need --target)
  
  ✗ --type list               (WRONG: 'list' is not a type)
  ✓ --type string[]           (RIGHT: use string[] for text lists)

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
	"schema_rename_field": {
		Name:        "schema rename field",
		Description: "Rename a field on a type and update all downstream uses",
		LongDesc: `Rename a field on a specific type and update all downstream places that use that field.

This command:
1. Renames types.<type>.fields.<old_field> -> <new_field> in schema.yaml
2. If name_field == <old_field>, updates it to <new_field>
3. Updates type templates that reference {{field.<old_field>}} (inline schema template or template file)
4. Renames frontmatter keys in files whose type matches the target type
5. Renames keys inside ::type(...) declarations (only for the target type)
6. Updates saved queries in raven.yaml that parse as object:<type> (best-effort)

IMPORTANT: Returns preview by default. Changes are NOT applied unless confirm=true.

For agents: After renaming, run raven_reindex(full=true) to update the index.`,
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type containing the field", Required: true, DynamicComp: "types"},
			{Name: "old_field", Description: "Current field name", Required: true},
			{Name: "new_field", Description: "New field name", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "confirm", Description: "Apply the rename (default: preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn schema rename field person email email_address --json",
			"rvn schema rename field person email email_address --confirm --json",
		},
		UseCases: []string{
			"Rename a field on a type safely with preview/confirm",
			"Refactor schema field names while keeping files consistent",
			"Update embedded ::type(...) declarations and saved queries after field rename",
		},
	},
	"schema_template_get": {
		Name:        "schema template get",
		Description: "Get the template for a type",
		LongDesc: `Show the template configured for a type.

Returns the template specification (inline content or file path), the source type
("inline", "file", or "none"), and the resolved template content.

For file-based templates, the content is loaded from the referenced file.
If the file doesn't exist, content will be empty.`,
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type to get template for (e.g., meeting, project)", Required: true, DynamicComp: "types"},
		},
		Examples: []string{
			"rvn schema template get meeting --json",
			"rvn schema template get project --json",
		},
		UseCases: []string{
			"View the current template for a type",
			"Check if a type has a template configured",
			"Inspect template content before editing",
		},
	},
	"schema_template_set": {
		Name:        "schema template set",
		Description: "Set or update the template for a type",
		LongDesc: `Set the template for a type in schema.yaml.

Use --content to set an inline template, or --file to reference a template file
(path relative to vault root). These flags are mutually exclusive.

Inline templates are stored directly in schema.yaml. File-based templates store
just the path reference, and the file content is loaded at render time.

Template variables: {{title}}, {{slug}}, {{type}}, {{date}}, {{datetime}},
{{year}}, {{month}}, {{day}}, {{weekday}}, {{field.<name>}}`,
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type to set template for (e.g., meeting, project)", Required: true, DynamicComp: "types"},
		},
		Flags: []FlagMeta{
			{Name: "content", Description: "Inline template content (mutually exclusive with --file)", Type: FlagTypeString},
			{Name: "file", Description: "Path to template file relative to vault root (mutually exclusive with --content)", Type: FlagTypeString, Examples: []string{"templates/meeting.md", "templates/project.md"}},
		},
		Examples: []string{
			`rvn schema template set meeting --content "# {{title}}\n\n**Date:** {{date}}\n\n## Attendees\n\n## Notes\n\n## Action Items" --json`,
			"rvn schema template set meeting --file templates/meeting.md --json",
			`rvn schema template set project --content "# {{title}}\n\n## Overview\n\n## Goals\n\n## Timeline" --json`,
		},
		UseCases: []string{
			"Set an inline template for a type",
			"Point a type to a template file",
			"Update an existing template",
		},
	},
	"schema_template_remove": {
		Name:        "schema template remove",
		Description: "Remove the template from a type",
		LongDesc: `Remove the template configuration from a type in schema.yaml.

This clears the template field from the type definition. The type will no longer
use a template when creating new objects with 'rvn new'.

Note: This does not delete template files from disk. If the type uses a file-based
template, the file will remain in the vault.`,
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type to remove template from (e.g., meeting, project)", Required: true, DynamicComp: "types"},
		},
		Examples: []string{
			"rvn schema template remove meeting --json",
			"rvn schema template remove project --json",
		},
		UseCases: []string{
			"Remove a template from a type",
			"Clear template configuration",
		},
	},
	"schema_template_render": {
		Name:        "schema template render",
		Description: "Preview a type's template with variables applied",
		LongDesc: `Render a type's template with variable substitution applied.

This is useful for previewing what a template will look like before creating objects.
If --title is not provided, a sample title is used. Field variables can be supplied
with --field flags.

Template variables: {{title}}, {{slug}}, {{type}}, {{date}}, {{datetime}},
{{year}}, {{month}}, {{day}}, {{weekday}}, {{field.<name>}}`,
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type whose template to render (e.g., meeting, project)", Required: true, DynamicComp: "types"},
		},
		Flags: []FlagMeta{
			{Name: "title", Description: "Title to use for rendering (default: sample title)", Type: FlagTypeString},
			{Name: "field", Description: "Set field value for rendering (repeatable)", Type: FlagTypeKeyValue, Examples: []string{`{"attendees": "Alice, Bob"}`, `{"status": "active"}`}},
		},
		Examples: []string{
			"rvn schema template render meeting --json",
			"rvn schema template render meeting --title \"Weekly Standup\" --json",
			`rvn schema template render meeting --title "1:1 with Alice" --field attendees="Alice, Bob" --json`,
		},
		UseCases: []string{
			"Preview what a template looks like with variables filled in",
			"Verify template syntax and variable substitution",
			"Test template changes before committing them",
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
	"update": {
		Name:        "update",
		Description: "Update a trait's value",
		LongDesc: `Update the value of a trait annotation.

Trait IDs look like "path/file.md:trait:N" and can be obtained via:
  - rvn query "trait:todo" --ids
  - rvn last <nums>

Bulk operations:
Use --stdin to read trait IDs from stdin (one per line).
IMPORTANT: Bulk operations return preview by default. Changes are NOT applied unless confirm=true.`,
		Args: []ArgMeta{
			{Name: "trait_id", Description: "Trait ID to update (e.g., daily/2026-01-25.md:trait:0)", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "value", Description: "New trait value (positional key=value)", Type: FlagTypePosKeyValue, Examples: []string{`{"value": "done"}`}},
			{Name: "stdin", Description: "Read trait IDs from stdin for bulk operations", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply bulk changes (without this flag, shows preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn update daily/2026-01-25.md:trait:0 value=done --json",
			"rvn query 'trait:todo' --ids | rvn update --stdin value=done --confirm --json",
		},
		UseCases: []string{
			"Update a specific trait by ID",
			"Bulk update trait values via --stdin",
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

The editor is determined by the 'editor' setting in config.toml or $EDITOR.

Use --stdin to read object IDs from stdin (one per line) and open them all.
This is useful for piping query results to open multiple files at once.`,
		Args: []ArgMeta{
			{Name: "reference", Description: "Reference to the file (short name, partial path, or full path)", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "stdin", Description: "Read object IDs from stdin for bulk open", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn open cursor --json",
			"rvn open companies/cursor --json",
			"rvn query 'object:project .status==active' --ids | rvn open --stdin --json",
		},
		UseCases: []string{
			"Quickly open a file by its short name",
			"Open files using references without knowing full paths",
			"Open multiple files from query results",
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
		LongDesc:    `Shows the full definition of a workflow including inputs and steps.`,
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
	"workflow_run": {
		Name:        "workflow run",
		Description: "Run a workflow until an agent step",
		LongDesc: `Runs a workflow's deterministic tool steps in order until it reaches an agent step.

When an agent step is reached, this command returns:
- the rendered agent prompt string
- the declared agent outputs (e.g., markdown)
- the accumulated step outputs so far
`,
		Args: []ArgMeta{
			{Name: "name", Description: "Workflow name", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "input", Description: "Set input value (repeatable)", Type: FlagTypeKeyValue, Examples: []string{"meeting_id=meetings/alice-1on1", "question=How does auth work?"}},
		},
		Examples: []string{
			"rvn workflow run meeting-prep --input meeting_id=meetings/alice-1on1 --json",
		},
		UseCases: []string{
			"Get a pre-formatted prompt and grounded context for agent execution",
			"Run deterministic pre-processing before an agent step",
		},
	},
	"last": {
		Name:        "last",
		Description: "Show or select results from the last query",
		LongDesc: `Show or select results from the most recent query.

Without arguments, displays all results from the last query with their numbers.
With number arguments, outputs the selected IDs for piping to other commands.

Number formats:
  1         Single result
  1,3,5     Multiple results (comma-separated)  
  1-5       Range of results
  1,3-5,7   Mixed format

The last query is saved to .raven/last-query.json whenever you run a query.
Results include a 'num' field that can be used to reference specific items.

With --apply, applies an operation directly to selected results without piping.`,
		Args: []ArgMeta{
			{Name: "nums", Description: "Result numbers to select (e.g., '1,3,5' or '1-5')", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "apply", Description: "Apply an operation to selected results (e.g., 'update value=done')", Type: FlagTypeString},
			{Name: "confirm", Description: "Apply changes (without this flag, shows preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn last --json",
			"rvn last 1,3 --json",
			"rvn last 1-5 --apply \"update value=done\" --confirm --json",
		},
		UseCases: []string{
			"View results from the most recent query",
			"Select specific items from query results for bulk operations",
			"Mark specific todos done without re-running the query",
		},
	},
	"resolve": {
		Name:        "resolve",
		Description: "Resolve a reference to its target object",
		LongDesc: `Resolve a reference (short name, alias, path, date, etc.) and return
information about the target object.

This is a pure query — it does not modify anything. The result always returns
"resolved": true/false to indicate whether the reference was successfully resolved.

Supports all reference formats:
- Short names: "freya" → people/freya
- Full paths: "people/freya" or "people/freya.md"
- Aliases: "The Queen" → people/freya
- Name field values: "The Prose Edda" → books/the-prose-edda
- Date references: "2025-02-01" → daily/2025-02-01
- Dynamic dates: "today", "yesterday", "tomorrow"
- Section references: "projects/website#tasks"

If the reference is ambiguous (matches multiple objects), returns all matches
with their match sources.`,
		Args: []ArgMeta{
			{Name: "reference", Description: "Reference to resolve (short name, path, alias, date, etc.)", Required: true},
		},
		Examples: []string{
			"rvn resolve freya --json",
			"rvn resolve people/freya --json",
			"rvn resolve today --json",
			"rvn resolve \"The Prose Edda\" --json",
		},
		UseCases: []string{
			"Check if a reference resolves before using it",
			"Discover the full object ID and type for a short name",
			"Disambiguate references that might match multiple objects",
			"Validate references without side effects",
		},
	},
	"import": {
		Name:        "import",
		Description: "Import objects from JSON data",
		LongDesc: `Import objects from external JSON data into the vault.

Reads a JSON array (or single object) and creates or updates vault objects
by mapping input fields to a schema type's fields.

Input can come from stdin or a file (--file). Field mappings can be specified
inline (--map) or via a YAML mapping file (--mapping).

For homogeneous imports (single type), specify the type as a positional argument
or in the mapping file. For heterogeneous imports (mixed types), use a mapping
file with type_field and per-type mappings.

By default, import performs an upsert: creates new objects and updates existing
ones. Use --create-only or --update-only to restrict behavior.

Mapping file format (homogeneous):
  type: person
  key: name
  map:
    full_name: name
    mail: email

Mapping file format (heterogeneous):
  type_field: kind
  types:
    contact:
      type: person
      key: name
      map:
        full_name: name
    task:
      type: project
      map:
        title: name`,
		Args: []ArgMeta{
			{Name: "type", Description: "Target Raven type (for homogeneous imports)", Required: false, DynamicComp: "types"},
		},
		Flags: []FlagMeta{
			{Name: "file", Description: "Read JSON from file instead of stdin", Type: FlagTypeString},
			{Name: "mapping", Description: "Path to YAML mapping file", Type: FlagTypeString},
			{Name: "map", Description: "Field mapping: external_key=schema_field (repeatable)", Type: FlagTypeStringSlice},
			{Name: "key", Description: "Field used for matching existing objects (default: type's name_field)", Type: FlagTypeString},
			{Name: "content-field", Description: "JSON field to use as page body content", Type: FlagTypeString},
			{Name: "dry-run", Description: "Preview changes without writing", Type: FlagTypeBool},
			{Name: "create-only", Description: "Only create new objects, skip updates", Type: FlagTypeBool},
			{Name: "update-only", Description: "Only update existing objects, skip creation", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply changes", Type: FlagTypeBool},
		},
		Examples: []string{
			`echo '[{"name": "Freya"}]' | rvn import person --json`,
			`echo '[{"full_name": "Thor"}]' | rvn import person --map full_name=name --json`,
			"rvn import --mapping contacts.yaml --file contacts.json --json",
			"rvn import --mapping migration.yaml --file dump.json --dry-run --json",
		},
		UseCases: []string{
			"Import contacts, events, or tasks from external tools",
			"Migrate data from another note-taking app",
			"Bulk-create objects from structured data",
			"Sync external data sources into the vault",
		},
	},
}
