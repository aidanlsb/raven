package commands

var vaultRegistry = map[string]Meta{
	"config": {
		Name:        "config",
		Description: "Manage global config.toml settings",
		VaultScope:  VaultScopeNone,
		LongDesc: `Manage global Raven config.toml settings.

Use this command group to initialize, inspect, and edit machine-level configuration
such as editor settings, state file location, and default vault selection.`,
		Examples: []string{
			"rvn config --json",
			"rvn config show --json",
			"rvn config init --json",
			"rvn config set --editor code --editor-mode auto --json",
			"rvn config unset --ui-accent --ui-code-theme --ui-markdown-style --json",
		},
		UseCases: []string{
			"Inspect resolved global config and state paths",
			"Create config.toml from defaults on first run",
			"Update global editor and UI settings via CLI/MCP",
		},
	},
	"config_show": {
		Name:        "config show",
		Description: "Show current global config.toml values",
		VaultScope:  VaultScopeNone,
		Examples: []string{
			"rvn config show --json",
		},
	},
	"config_init": {
		Name:        "config init",
		Description: "Create default global config.toml if missing",
		VaultScope:  VaultScopeNone,
		LongDesc: `Create a default global config.toml file at the resolved config path.

If the file already exists, no changes are made.`,
		Examples: []string{
			"rvn config init --json",
			"rvn config init --config /tmp/raven/config.toml --json",
		},
	},
	"config_set": {
		Name:        "config set",
		Description: "Set one or more global config.toml fields",
		VaultScope:  VaultScopeNone,
		LongDesc: `Set one or more explicit global config fields.

Only known fields can be changed with this command.
Use 'config unset' to clear fields.`,
		Flags: []FlagMeta{
			{Name: "editor", Description: "Set editor command", Type: FlagTypeString},
			{Name: "editor-mode", Description: "Set editor mode (auto|terminal|gui)", Type: FlagTypeString, Examples: []string{"auto", "terminal", "gui"}},
			{Name: "state-file", Description: "Set state.toml path (absolute or relative to config directory)", Type: FlagTypeString},
			{Name: "default-vault", Description: "Set default_vault to a configured vault name", Type: FlagTypeString},
			{Name: "ui-accent", Description: "Set UI accent color (ANSI 0-255 or #RRGGBB)", Type: FlagTypeString},
			{Name: "ui-code-theme", Description: "Set markdown code theme name", Type: FlagTypeString},
			{Name: "ui-markdown-style", Description: "Set full Glamour markdown style (auto, raven, built-in style name, or JSON path)", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn config set --editor code --json",
			"rvn config set --editor-mode terminal --json",
			"rvn config set --state-file state.toml --json",
			"rvn config set --default-vault work --json",
			"rvn config set --ui-accent 39 --ui-code-theme monokai --ui-markdown-style auto --json",
		},
		UseCases: []string{
			"Configure global editor behavior",
			"Set default_vault without editing TOML directly",
			"Adjust CLI UI accent and markdown rendering style",
		},
	},
	"config_unset": {
		Name:        "config unset",
		Description: "Clear one or more global config.toml fields",
		VaultScope:  VaultScopeNone,
		LongDesc:    "Clear one or more explicit global config fields.",
		Flags: []FlagMeta{
			{Name: "editor", Description: "Clear editor", Type: FlagTypeBool},
			{Name: "editor-mode", Description: "Clear editor_mode", Type: FlagTypeBool},
			{Name: "state-file", Description: "Clear state_file", Type: FlagTypeBool},
			{Name: "default-vault", Description: "Clear default_vault", Type: FlagTypeBool},
			{Name: "ui-accent", Description: "Clear ui.accent", Type: FlagTypeBool},
			{Name: "ui-code-theme", Description: "Clear ui.code_theme", Type: FlagTypeBool},
			{Name: "ui-markdown-style", Description: "Clear ui.markdown_style", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn config unset --editor --json",
			"rvn config unset --default-vault --json",
			"rvn config unset --ui-accent --ui-code-theme --ui-markdown-style --json",
		},
	},
	"vault": {
		Name:        "vault",
		Use:         "vault [subcommand]",
		Description: "Manage configured vaults and active selection",
		VaultScope:  VaultScopeNone,
		LongDesc: `Manage configured vaults and active selection.

The active vault is stored in state.toml.
The default vault is stored in config.toml and used as fallback.`,
		Examples: []string{
			"rvn vault --json",
			"rvn vault list --json",
			"rvn vault current --json",
			"rvn vault path --json",
			"rvn vault stats --json",
			"rvn vault config show --json",
			"rvn vault add work /Users/you/work-notes --json",
			"rvn vault use work --json",
			"rvn vault pin personal --json",
			"rvn vault remove personal --clear-default --clear-active --json",
			"rvn vault clear --json",
		},
		UseCases: []string{
			"List configured vaults and inspect active/default markers",
			"Inspect or mutate structured raven.yaml settings via 'vault config'",
			"Register named vault paths in config.toml",
			"Switch active vault for subsequent CLI and MCP commands",
			"Pin a default fallback vault in config.toml",
		},
	},
	"vault_list": {
		Name:        "vault list",
		Description: "List configured vaults",
		VaultScope:  VaultScopeNone,
		Examples: []string{
			"rvn vault list --json",
		},
	},
	"vault_current": {
		Name:        "vault current",
		Description: "Show the current resolved vault",
		VaultScope:  VaultScopeNone,
		Examples: []string{
			"rvn vault current --json",
		},
	},
	"vault_path": {
		Name:        "vault path",
		Description: "Print the resolved vault directory path",
		Examples: []string{
			"rvn vault path --json",
		},
	},
	"vault_stats": {
		Name:        "vault stats",
		Description: "Show vault statistics",
		Examples: []string{
			"rvn vault stats --json",
		},
	},
	"vault_use": {
		Name:        "vault use",
		Description: "Set the active vault in state.toml",
		VaultScope:  VaultScopeNone,
		LongDesc:    "Set the active vault in state.toml.",
		Args: []ArgMeta{
			{Name: "name", Description: "Configured vault name", Required: true},
		},
		Examples: []string{
			"rvn vault use work --json",
		},
	},
	"vault_add": {
		Name:        "vault add",
		Description: "Add a vault to config.toml",
		VaultScope:  VaultScopeNone,
		LongDesc: `Add or update a named vault entry in the global config file.

By default, adding an existing name fails. Use --replace to update the existing path.
Use --pin to also set default_vault to the added vault.`,
		Args: []ArgMeta{
			{Name: "name", Description: "Vault name", Required: true},
			{Name: "path", Description: "Vault directory path", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "replace", Description: "Replace existing vault path if name already exists", Type: FlagTypeBool},
			{Name: "pin", Description: "Also set this vault as default_vault", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn vault add work /Users/you/work-notes --json",
			"rvn vault add personal /Users/you/personal-notes --pin --json",
			"rvn vault add work /Users/you/new-work-notes --replace --json",
		},
		UseCases: []string{
			"Register a new named vault in global config",
			"Update a vault path with --replace",
			"Set the added vault as default with --pin",
		},
	},
	"vault_remove": {
		Name:        "vault remove",
		Description: "Remove a vault from config.toml",
		VaultScope:  VaultScopeNone,
		LongDesc: `Remove a named vault entry from the global config file.

Safety checks:
- Removing the current default vault requires --clear-default
- Removing the current active vault requires --clear-active`,
		Args: []ArgMeta{
			{Name: "name", Description: "Configured vault name", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "clear-default", Description: "Clear default_vault when removing the default", Type: FlagTypeBool},
			{Name: "clear-active", Description: "Clear active_vault when removing the active vault", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn vault remove personal --json",
			"rvn vault remove personal --clear-default --json",
			"rvn vault remove personal --clear-default --clear-active --json",
		},
		UseCases: []string{
			"Delete stale vault entries from global config",
			"Safely remove default/active vaults with explicit clearing flags",
		},
	},
	"vault_pin": {
		Name:        "vault pin",
		Description: "Set default_vault in config.toml",
		VaultScope:  VaultScopeNone,
		Args: []ArgMeta{
			{Name: "name", Description: "Configured vault name", Required: true},
		},
		Examples: []string{
			"rvn vault pin personal --json",
		},
	},
	"vault_clear": {
		Name:        "vault clear",
		Description: "Clear active vault from state.toml",
		VaultScope:  VaultScopeNone,
		Examples: []string{
			"rvn vault clear --json",
		},
	},
	"vault_config_show": {
		Name:        "vault config show",
		CLIPath:     []string{"vault", "config", "show"},
		Description: "Show current raven.yaml values",
		LongDesc: `Show current effective raven.yaml values.

The JSON output includes resolved defaults for directories, assets, capture,
deletion, auto_reindex, protected_prefixes, and exclude patterns.`,
		Examples: []string{
			"rvn vault config show --json",
		},
		UseCases: []string{
			"Inspect structured vault-level configuration without reading raven.yaml directly",
		},
	},
	"vault_config_auto_reindex_set": {
		Name:        "vault config auto-reindex set",
		CLIPath:     []string{"vault", "config", "auto-reindex", "set"},
		Description: "Set an explicit auto_reindex value in raven.yaml",
		LongDesc: `Set the vault's auto_reindex behavior explicitly.

Use --value=false to disable automatic incremental reindexing after write commands.
Without --value, this command sets auto_reindex=true explicitly.`,
		Flags: []FlagMeta{
			{Name: "value", Description: "Explicit auto_reindex value", Type: FlagTypeBool, Default: "true"},
		},
		Examples: []string{
			"rvn vault config auto-reindex set --json",
			"rvn vault config auto-reindex set --value=false --json",
		},
	},
	"vault_config_auto_reindex_unset": {
		Name:        "vault config auto-reindex unset",
		CLIPath:     []string{"vault", "config", "auto-reindex", "unset"},
		Description: "Clear the explicit auto_reindex field from raven.yaml",
		LongDesc: `Remove the explicit auto_reindex field and fall back to Raven's default behavior.

The effective default is auto_reindex=true.`,
		Examples: []string{
			"rvn vault config auto-reindex unset --json",
		},
	},
	"vault_config_protected_prefixes_list": {
		Name:        "vault config protected-prefixes list",
		CLIPath:     []string{"vault", "config", "protected-prefixes", "list"},
		Description: "List configured protected_prefixes from raven.yaml",
		Examples: []string{
			"rvn vault config protected-prefixes list --json",
		},
	},
	"vault_config_protected_prefixes_add": {
		Name:        "vault config protected-prefixes add",
		CLIPath:     []string{"vault", "config", "protected-prefixes", "add"},
		Description: "Add one protected prefix to raven.yaml",
		LongDesc: `Add a vault-relative directory prefix to protected_prefixes.

Prefixes are normalized with a trailing slash.`,
		Args: []ArgMeta{
			{Name: "prefix", Description: "Vault-relative directory prefix to protect", Required: true},
		},
		Examples: []string{
			"rvn vault config protected-prefixes add private --json",
			"rvn vault config protected-prefixes add templates/generated --json",
		},
	},
	"vault_config_protected_prefixes_remove": {
		Name:        "vault config protected-prefixes remove",
		CLIPath:     []string{"vault", "config", "protected-prefixes", "remove"},
		Description: "Remove one protected prefix from raven.yaml",
		Args: []ArgMeta{
			{Name: "prefix", Description: "Configured protected prefix to remove", Required: true},
		},
		Examples: []string{
			"rvn vault config protected-prefixes remove private/ --json",
		},
	},
	"vault_config_exclude_list": {
		Name:        "vault config exclude list",
		CLIPath:     []string{"vault", "config", "exclude", "list"},
		Description: "List configured exclude patterns from raven.yaml",
		Examples: []string{
			"rvn vault config exclude list --json",
		},
	},
	"vault_config_exclude_add": {
		Name:        "vault config exclude add",
		CLIPath:     []string{"vault", "config", "exclude", "add"},
		Description: "Add one exclude pattern to raven.yaml",
		LongDesc: `Add a gitignore-style pattern to the top-level exclude list.

Excluded paths are not managed by Raven: check, reindex, query, and content
mutation commands ignore or reject matching paths.`,
		Args: []ArgMeta{
			{Name: "pattern", Description: "Gitignore-style exclude pattern", Required: true},
		},
		Examples: []string{
			"rvn vault config exclude add AGENTS.md --json",
			"rvn vault config exclude add .cursor/ --json",
			"rvn vault config exclude add '*.plan.md' --json",
		},
	},
	"vault_config_exclude_remove": {
		Name:        "vault config exclude remove",
		CLIPath:     []string{"vault", "config", "exclude", "remove"},
		Description: "Remove one exclude pattern from raven.yaml",
		Args: []ArgMeta{
			{Name: "pattern", Description: "Configured exclude pattern to remove", Required: true},
		},
		Examples: []string{
			"rvn vault config exclude remove .cursor/ --json",
		},
	},
	"vault_config_directories_get": {
		Name:        "vault config directories get",
		CLIPath:     []string{"vault", "config", "directories", "get"},
		Description: "Show current directories config from raven.yaml",
		Examples: []string{
			"rvn vault config directories get --json",
		},
	},
	"vault_config_directories_set": {
		Name:        "vault config directories set",
		CLIPath:     []string{"vault", "config", "directories", "set"},
		Description: "Set one or more directories fields in raven.yaml",
		Flags: []FlagMeta{
			{Name: "daily", Description: "Set directories.daily", Type: FlagTypeString},
			{Name: "type", Description: "Set directories.type", Type: FlagTypeString},
			{Name: "page", Description: "Set directories.page", Type: FlagTypeString},
			{Name: "template", Description: "Set directories.template", Type: FlagTypeString},
			{Name: "assets", Description: "Set directories.assets", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn vault config directories set --daily journal --json",
			"rvn vault config directories set --type type --page page --template templates --json",
			"rvn vault config directories set --assets assets --json",
		},
	},
	"vault_config_directories_unset": {
		Name:        "vault config directories unset",
		CLIPath:     []string{"vault", "config", "directories", "unset"},
		Description: "Clear one or more directories fields from raven.yaml",
		Flags: []FlagMeta{
			{Name: "daily", Description: "Clear directories.daily", Type: FlagTypeBool},
			{Name: "type", Description: "Clear directories.type", Type: FlagTypeBool},
			{Name: "page", Description: "Clear directories.page", Type: FlagTypeBool},
			{Name: "template", Description: "Clear directories.template", Type: FlagTypeBool},
			{Name: "assets", Description: "Clear directories.assets", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn vault config directories unset --page --json",
			"rvn vault config directories unset --daily --type --page --template --json",
		},
	},
	"vault_config_capture_get": {
		Name:        "vault config capture get",
		CLIPath:     []string{"vault", "config", "capture", "get"},
		Description: "Show current capture config from raven.yaml",
		Examples: []string{
			"rvn vault config capture get --json",
		},
	},
	"vault_config_capture_set": {
		Name:        "vault config capture set",
		CLIPath:     []string{"vault", "config", "capture", "set"},
		Description: "Set one or more capture fields in raven.yaml",
		Flags: []FlagMeta{
			{Name: "destination", Description: "Set capture.destination", Type: FlagTypeString},
			{Name: "heading", Description: "Set capture.heading", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn vault config capture set --destination inbox.md --json",
			"rvn vault config capture set --heading '## Captured' --json",
		},
	},
	"vault_config_capture_unset": {
		Name:        "vault config capture unset",
		CLIPath:     []string{"vault", "config", "capture", "unset"},
		Description: "Clear one or more capture fields from raven.yaml",
		Flags: []FlagMeta{
			{Name: "destination", Description: "Clear capture.destination", Type: FlagTypeBool},
			{Name: "heading", Description: "Clear capture.heading", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn vault config capture unset --heading --json",
			"rvn vault config capture unset --destination --heading --json",
		},
	},
	"vault_config_deletion_get": {
		Name:        "vault config deletion get",
		CLIPath:     []string{"vault", "config", "deletion", "get"},
		Description: "Show current deletion config from raven.yaml",
		Examples: []string{
			"rvn vault config deletion get --json",
		},
	},
	"vault_config_deletion_set": {
		Name:        "vault config deletion set",
		CLIPath:     []string{"vault", "config", "deletion", "set"},
		Description: "Set one or more deletion fields in raven.yaml",
		Flags: []FlagMeta{
			{Name: "behavior", Description: "Set deletion.behavior (trash|permanent)", Type: FlagTypeString, Examples: []string{"trash", "permanent"}},
			{Name: "trash-dir", Description: "Set deletion.trash_dir", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn vault config deletion set --behavior permanent --json",
			"rvn vault config deletion set --trash-dir archive/trash --json",
		},
	},
	"vault_config_deletion_unset": {
		Name:        "vault config deletion unset",
		CLIPath:     []string{"vault", "config", "deletion", "unset"},
		Description: "Clear one or more deletion fields from raven.yaml",
		Flags: []FlagMeta{
			{Name: "behavior", Description: "Clear deletion.behavior", Type: FlagTypeBool},
			{Name: "trash-dir", Description: "Clear deletion.trash_dir", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn vault config deletion unset --trash-dir --json",
			"rvn vault config deletion unset --behavior --trash-dir --json",
		},
	},
}
