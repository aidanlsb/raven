package commands

var systemRegistry = map[string]Meta{
	"init": {
		Name:        "init",
		Description: "Initialize a new vault at a path",
		VaultScope:  VaultScopeNone,
		LongDesc: `Creates a new vault at the specified path with default configuration files.

Creates:
  - raven.yaml   (vault configuration)
  - schema.yaml  (types and traits)
  - .raven/      (index directory)
  - .gitignore   (ignores derived files)

Also attempts to fetch docs into Raven's global docs directory. If docs fetch fails, initialization
still succeeds and returns a warning with a retry command.

First-run vault policy (applied in both --json and interactive mode):
  - The new vault is always auto-registered in global config under a suggested name.
  - If it is the first vault on the machine (no default, no active, no other registered
    vault), it is also set as the default and active vault, so first-run just works.
  - If another vault already exists, the new vault is registered and made active immediately;
    the existing default is left unchanged. Human and JSON output identify the new active
    vault, the previous active/resolved vault, and the exact command to switch back.
  - If global config/state cannot be loaded, or registration/activation cannot be persisted,
    init returns a failure with details.initialized=true and the created vault path.

The post_init object reports what happened (is_first_vault, has_existing_default, registered,
is_default, is_active, activated), structured active_vault / previous_active_vault details,
switch_back, invocable actions, and guidance. Changing the default remains an explicit choice.`,
		Args: []ArgMeta{
			{Name: "path", Description: "Directory path to initialize as a vault", Required: true},
		},
		Examples: []string{
			"rvn init /path/to/new/vault --json",
			"rvn init ./notes --json",
		},
		UseCases: []string{
			"Bootstrap a new vault from an agent or script",
			"Create required Raven config and schema files in one step",
			"Initialize first-run setup before any other MCP tool calls",
		},
	},
	"serve": {
		Name:        "serve",
		Description: "Run Raven as an MCP server",
		VaultScope:  VaultScopeNone,
		LongDesc: `Run Raven as an MCP server over stdio.

Vault-scoped calls must provide vault or vault_path unless the server is pinned
with --vault or --vault-path.`,
		Examples: []string{
			"rvn serve",
			"rvn serve --vault personal",
			"rvn serve --vault-path /path/to/vault",
		},
		UseCases: []string{
			"Launch Raven MCP server for local clients",
		},
	},
	"lsp": {
		Name:        "lsp",
		Description: "Run Raven as an LSP server",
		VaultScope:  VaultScopeNone,
		LongDesc:    "Run Raven as a Language Server Protocol server over stdio for editor integration (diagnostics, quick-fix code actions, completion, go-to-definition, find-references, hover).",
		Examples: []string{
			"rvn lsp",
			"rvn lsp --vault personal",
		},
		UseCases: []string{
			"Launch the Raven language server from an editor LSP client",
		},
	},
	"mcp_install": {
		Name:        "mcp install",
		Description: "Add raven to an MCP client config",
		VaultScope:  VaultScopeNone,
		LongDesc:    "Add raven to a supported MCP client config file.",
		Flags: []FlagMeta{
			{Name: "client", Description: "MCP client (codex, claude-code, claude-desktop, cursor)", Type: FlagTypeString, Default: "claude-code"},
			{Name: "config", Description: "Path to config file", Type: FlagTypeString},
			{Name: "vault", Description: "Pin a named vault", Type: FlagTypeString},
			{Name: "vault-path", Description: "Pin an explicit vault path", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn mcp install --client codex",
			"rvn mcp install --client claude-code",
			"rvn mcp install --client claude-desktop --vault work",
			"rvn mcp install --client cursor --config ~/.config/raven/work.toml",
		},
		UseCases: []string{
			"Install Raven MCP entry into a client config",
		},
	},
	"mcp_remove": {
		Name:        "mcp remove",
		Description: "Remove raven from an MCP client config",
		VaultScope:  VaultScopeNone,
		LongDesc:    "Remove raven from a supported MCP client config file.",
		Flags: []FlagMeta{
			{Name: "client", Description: "MCP client (codex, claude-code, claude-desktop, cursor)", Type: FlagTypeString, Default: "claude-code"},
		},
		Examples: []string{
			"rvn mcp remove --client codex",
			"rvn mcp remove --client claude-code",
		},
		UseCases: []string{
			"Remove Raven MCP entry from a client config",
		},
	},
	"mcp_status": {
		Name:        "mcp status",
		Description: "Show raven MCP status across all clients",
		VaultScope:  VaultScopeNone,
		LongDesc:    "Show whether raven is configured in supported MCP clients.",
		Examples: []string{
			"rvn mcp status",
			"rvn mcp status --json",
		},
		UseCases: []string{
			"Check MCP installation status across clients",
		},
	},
	"mcp_show": {
		Name:        "mcp show",
		Description: "Print MCP config snippet for manual setup",
		VaultScope:  VaultScopeNone,
		LongDesc:    "Print the client-specific config snippet for manually configuring Raven as an MCP server.",
		Flags: []FlagMeta{
			{Name: "client", Description: "MCP client (codex, claude-code, claude-desktop, cursor)", Type: FlagTypeString},
			{Name: "config", Description: "Path to config file", Type: FlagTypeString},
			{Name: "vault", Description: "Pin a named vault", Type: FlagTypeString},
			{Name: "vault-path", Description: "Pin an explicit vault path", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn mcp show --client codex",
			"rvn mcp show --client claude-code",
			"rvn mcp show --client cursor --vault work",
			"rvn mcp show --client claude-code --config ~/.config/raven/work.toml",
		},
		UseCases: []string{
			"Generate manual MCP configuration snippet",
		},
	},
	"docs": {
		Name:        "docs",
		Description: "Browse long-form Markdown documentation",
		VaultScope:  VaultScopeNone,
		LongDesc: `Browse long-form documentation stored in Raven's global docs directory.

Use this command for guides and references.
When an existing cache was fetched by an older Raven release, docs read commands lazily
refresh it from the installed CLI version tag. Refresh failures return a warning and keep
serving the existing cache. A missing cache still requires 'rvn docs fetch'.
When run in an interactive terminal, 'rvn docs' opens Raven's picker.
In non-interactive or JSON mode, bare 'rvn docs' lists the available sections.
In the picker, use l to move forward into a section/topic and h to go back.
For command-level usage, use 'rvn help <command>'.`,
		Args: []ArgMeta{
			{Name: "section", Description: "Docs section (e.g., getting-started, types-and-traits, querying)", Required: false},
			{Name: "topic", Description: "Topic slug within the section (e.g., query-language)", Required: false},
		},
		Examples: []string{
			"rvn docs --json",
			"rvn docs fetch --json",
			"rvn docs getting-started --json",
			"rvn docs querying query-language --json",
			"rvn docs search \"saved query\" --json",
		},
		UseCases: []string{
			"Fetch, force-refresh, or pin docs with `docs fetch`",
			"List docs sections and topic counts",
			"Interactively select docs in Raven's picker",
			"Browse docs topics by section",
			"Open and read a specific docs page",
			"Find long-form guidance outside command help",
		},
	},
	"docs_fetch": {
		Name:        "docs fetch",
		Description: "Fetch docs into Raven's global docs directory",
		VaultScope:  VaultScopeNone,
		LongDesc: `Download docs from Raven's source repository into Raven's global docs directory.

This replaces the global docs cache.
By default, docs are fetched from the "main" ref.`,
		Flags: []FlagMeta{
			{Name: "ref", Description: "Git ref to fetch (branch, tag, or commit)", Type: FlagTypeString, Default: "main"},
			{Name: "source", Description: "Override docs archive base URL", Type: FlagTypeString, Default: "https://codeload.github.com/aidanlsb/raven/tar.gz"},
		},
		Examples: []string{
			"rvn docs fetch --json",
			"rvn docs fetch --ref v0.5.0 --json",
		},
		UseCases: []string{
			"Sync docs after init",
			"Force-refresh docs without reinstalling rvn",
			"Pin docs to a specific ref for reproducibility",
		},
	},
	"docs_search": {
		Name:        "docs search",
		Description: "Search long-form Markdown documentation",
		VaultScope:  VaultScopeNone,
		Args: []ArgMeta{
			{Name: "query", Description: "Search query text", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "limit", Short: "n", Description: "Maximum number of matches (default: 20)", Type: FlagTypeInt, Default: "20"},
			{Name: "offset", Description: "Number of matches to skip before returning results", Type: FlagTypeInt},
			{Name: "section", Short: "s", Description: "Filter search to one docs section", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn docs search query --json",
			"rvn docs search \"saved query\" --limit 10 --json",
			"rvn docs search \"saved query\" --limit 10 --offset 10 --json",
			"rvn docs search \"saved query\" --section reference --json",
		},
		UseCases: []string{
			"Search guides and references by keyword",
			"Limit docs search to a specific section",
			"Page through docs search matches when has_more is true",
			"Locate docs pages when topic slug is unknown",
		},
	},
	"version": {
		Name:        "version",
		Description: "Show Raven version and build information",
		VaultScope:  VaultScopeNone,
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
}
