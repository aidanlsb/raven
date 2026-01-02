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
	FlagTypeString   FlagType = "string"
	FlagTypeBool     FlagType = "bool"
	FlagTypeInt      FlagType = "int"
	FlagTypeKeyValue FlagType = "key=value" // For --field name=value
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
			{Name: "field", Description: "Set field value (repeatable)", Type: FlagTypeKeyValue, Examples: []string{"name=Alice", "email=a@b.com"}},
		},
		Examples: []string{
			"rvn new person \"Alice Chen\" --json",
			"rvn new person \"Alice\" --field name=\"Alice Chen\" --json",
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
For creating NEW typed objects, use 'rvn new' instead.`,
		Args: []ArgMeta{
			{Name: "text", Description: "Text to add (can include @traits and [[refs]])", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "to", Description: "Target EXISTING file path (must exist)", Type: FlagTypeString, Examples: []string{"projects/website.md", "inbox.md"}},
		},
		Examples: []string{
			"rvn add \"Quick thought\" --json",
			"rvn add \"@priority(high) Urgent task\" --json",
			"rvn add \"Note\" --to projects/website.md --json",
		},
		UseCases: []string{
			"Quick capture to daily note",
			"Add tasks to existing project files",
			"Append notes to existing documents",
		},
	},
	"delete": {
		Name:        "delete",
		Description: "Delete an object from the vault",
		LongDesc: `Delete a file/object from the vault.

By default, files are moved to a trash directory (.trash/).
Warns about backlinks (objects that reference the deleted item).`,
		Args: []ArgMeta{
			{Name: "object_id", Description: "Object ID to delete (e.g., people/alice)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "force", Description: "Skip confirmation prompt", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn delete people/alice --json",
			"rvn delete projects/old --force --json",
		},
	},
	"trait": {
		Name:        "trait",
		Description: "Query traits by type",
		Args: []ArgMeta{
			{Name: "trait_type", Description: "Trait type to query (e.g., due, status)", Required: true, DynamicComp: "traits"},
		},
		Flags: []FlagMeta{
			{Name: "value", Description: "Filter by value (today, past, this-week, or literal)", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn trait due --json",
			"rvn trait due --value past --json",
			"rvn trait status --value todo --json",
		},
	},
	"query": {
		Name:        "query",
		Description: "Run a saved query",
		Args: []ArgMeta{
			{Name: "query_name", Description: "Name of the saved query", Required: true, DynamicComp: "queries"},
		},
		Flags: []FlagMeta{
			{Name: "list", Description: "List available saved queries", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn query tasks --json",
			"rvn query overdue --json",
			"rvn query --list --json",
		},
	},
	"type": {
		Name:        "type",
		Description: "List objects by type",
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type name (e.g., person, project)", Required: false, DynamicComp: "types"},
		},
		Flags: []FlagMeta{
			{Name: "list", Description: "List available types with counts", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn type person --json",
			"rvn type --list --json",
		},
	},
	"tag": {
		Name:        "tag",
		Description: "Query objects by tag",
		Args: []ArgMeta{
			{Name: "tag", Description: "Tag name (without #)", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "list", Description: "List all tags with counts", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn tag important --json",
			"rvn tag --list --json",
		},
	},
	"backlinks": {
		Name:        "backlinks",
		Description: "Find objects that reference a target",
		Args: []ArgMeta{
			{Name: "target", Description: "Target object ID (e.g., people/alice)", Required: true},
		},
		Examples: []string{
			"rvn backlinks people/alice --json",
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
			"rvn read people/alice.md --json",
		},
	},
	"stats": {
		Name:        "stats",
		Description: "Show vault statistics",
		Examples: []string{
			"rvn stats --json",
		},
	},
	"check": {
		Name:        "check",
		Description: "Validate vault against schema",
		Flags: []FlagMeta{
			{Name: "create-missing", Description: "Interactively create missing pages", Type: FlagTypeBool},
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
}
