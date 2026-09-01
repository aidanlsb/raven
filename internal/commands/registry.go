// Package commands provides a central registry of Raven CLI commands.
// This registry is the single source of truth for command metadata,
// used by both the CLI and MCP server.
package commands

// Meta defines metadata for a CLI command that can be used to
// generate both Cobra commands and MCP tool schemas.
type Meta struct {
	Name                string          // Command name (e.g., "trait", "add", "new")
	CLIPath             []string        // Optional explicit CLI invocation path segments (e.g., ["vault", "config", "show"]); falls back to Name when empty
	Use                 string          // Cobra-style usage string for this command (local, not full invocation)
	Description         string          // Short description
	LongDesc            string          // Long description (for --help)
	Category            Category        // Command grouping for discovery surfaces
	Access              AccessMode      // Read/write classification for discovery surfaces
	Risk                RiskLevel       // Safe/mutating/destructive classification
	VaultScope          VaultScope      // Whether the command requires a resolved vault path
	Args                []ArgMeta       // Positional arguments
	Flags               []FlagMeta      // Command flags
	BulkStdinArgName    string          // Canonical args key used for bulk IDs when --stdin is available
	BulkStdinArgAliases []string        // Backward-compatible aliases accepted for the bulk ID arg
	VariadicJoin        bool            // When true, join variadic args with spaces (e.g., docs search)
	MutexGroups         [][]string      // Mutually exclusive flag groups (e.g., [["type", "core"], ["content", "content-file"]])
	Examples            []string        // Usage examples
	UseCases            []string        // Agent use cases (for MCP hints)
}

// ArgMeta defines a positional argument.
type ArgMeta struct {
	Name            string   // Argument name
	Description     string   // Description
	Required        bool     // Is this argument required for canonical/MCP invocation?
	CLIOptional     bool     `json:"-"` // Can interactive CLI omit this and prompt/pick instead?
	Variadic        bool     `json:"-"` // Is this a repeated positional CLI argument represented as a string array canonically?
	StdinIndependent bool   `json:"-"` // When true, this arg is still consumed from positionals even with --stdin
	Completions     []string // Static completions (if any)
	DynamicComp     string   // Dynamic completion type: "types", "traits", "files"
	Examples        []string `json:"-"` // Example values
}

// FlagMeta defines a command flag.
type FlagMeta struct {
	Name        string   // Flag name (e.g., "value", "to")
	Short       string   // Short flag (e.g., "v" for -v)
	Description string   // Description
	Type        FlagType // Type of flag
	Default     string   // Default value
	Required    bool     // Whether callers must provide the flag
	Examples    []string // Example values
	ArgsKey     string   // Override the args map key (defaults to Name if empty)
}

// FlagType represents the type of a flag.
type FlagType string

type Category string

const (
	CategoryQuery       Category = "query"
	CategoryContent     Category = "content"
	CategorySchema      Category = "schema"
	CategoryNavigation  Category = "navigation"
	CategoryMaintenance Category = "maintenance"
	CategoryVault       Category = "vault"
)

type AccessMode string

const (
	AccessRead  AccessMode = "read"
	AccessWrite AccessMode = "write"
)

type RiskLevel string

type VaultScope string

const (
	RiskSafe        RiskLevel = "safe"
	RiskMutating    RiskLevel = "mutating"
	RiskDestructive RiskLevel = "destructive"
)

const (
	VaultScopeRequired VaultScope = "required"
	VaultScopeNone     VaultScope = "none"
)

const (
	FlagTypeString      FlagType = "string"
	FlagTypeBool        FlagType = "bool"
	FlagTypeInt         FlagType = "int"
	FlagTypeKeyValue    FlagType = "key=value"     // For repeatable flags: --field name=value, --input name=value
	FlagTypePosKeyValue FlagType = "pos-key=value" // For positional key=value args (e.g., `set <reference> field=value...`)
	FlagTypeStringSlice FlagType = "stringSlice"   // For repeatable string flags
	FlagTypeJSON        FlagType = "json"          // JSON object payloads
)

const barePickerInsertModeHelp = "\n\nThe bare-command picker starts in insert mode: type immediately to filter, or press Esc for normal-mode shortcuts."

// Registry holds all registered commands.
var Registry = mergeRegistries(
	contentRegistry,
	maintenanceRegistry,
	navigationRegistry,
	queryRegistry,
	schemaRegistry,
	skillsRegistry,
	systemRegistry,
	vaultRegistry,
)

func mergeRegistries(groups ...map[string]Meta) map[string]Meta {
	size := 0
	for _, group := range groups {
		size += len(group)
	}

	registry := make(map[string]Meta, size)
	for _, group := range groups {
		for commandID, meta := range group {
			if _, exists := registry[commandID]; exists {
				panic("commands: duplicate registry command ID " + commandID)
			}
			registry[commandID] = meta
		}
	}
	return registry
}
