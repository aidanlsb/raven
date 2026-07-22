package commands

var skillsRegistry = map[string]Meta{
	"skill_list": {
		Name:        "skill list",
		Description: "List Raven-provided skills",
		VaultScope:  VaultScopeNone,
		LongDesc: `List bundled Raven skills and their installation status.

Skills use the Agent Skills standard and default to ~/.agents/skills for user
scope or .agents/skills in the current project. Use --dest to inspect a
different install root.`,
		Flags: []FlagMeta{
			{Name: "scope", Description: "Install scope: user or project", Type: FlagTypeString, Default: "user", Examples: []string{"user", "project"}},
			{Name: "dest", Description: "Override install root path", Type: FlagTypeString},
			{Name: "installed", Description: "Show installed skills only", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn skill list --json",
			"rvn skill list --scope project --installed --json",
			"rvn skill list --dest /path/to/skills --json",
		},
		UseCases: []string{
			"Discover which Raven skills are available",
			"Check which Raven skills are installed",
		},
	},
	"skill_sync": {
		Name:        "skill sync",
		Description: "Sync Raven-provided Agent Skills",
		VaultScope:  VaultScopeNone,
		LongDesc: `Sync bundled Raven skills using the Agent Skills standard.

With a skill name, installs that shipped skill if missing or aligns it with the
current shipped version if already Raven-managed.

Without a skill name, updates/removes existing Raven-managed skills identified
by their receipts. Missing shipped skills are reported but not installed.

The default install root is ~/.agents/skills for user scope or .agents/skills
in the current project. Use --dest to install elsewhere.

Preview is returned by default. Use --confirm to apply writes.`,
		Args: []ArgMeta{
			{Name: "name", Description: "Optional shipped skill name to sync", Required: false, Completions: []string{"raven-core", "raven-maintenance", "raven-onboarding", "raven-query", "raven-schema", "raven-templates", "raven-vault-admin"}},
		},
		Flags: []FlagMeta{
			{Name: "scope", Description: "Install scope: user or project", Type: FlagTypeString, Default: "user", Examples: []string{"user", "project"}},
			{Name: "dest", Description: "Override install root path", Type: FlagTypeString},
			{Name: "confirm", Description: "Apply changes (without this flag, shows preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn skill sync raven-core --confirm --json",
			"rvn skill sync --scope project --json",
		},
		UseCases: []string{
			"Install or refresh one Raven skill",
			"Update installed Raven-managed skills to shipped versions",
			"Preview packaged skill sync before writing files",
		},
	},
	"skill_install": {
		Name:        "skill install",
		Use:         "install [skill...]",
		Description: "Install shipped Raven skills",
		VaultScope:  VaultScopeNone,
		LongDesc: `Install shipped Raven skills in one command.

This is the primary first-run path for agents and users. With no arguments it
installs the full set of shipped skills; pass one or more skill names to narrow
the install to specific skills. Missing skills are installed and any already
Raven-managed skills are aligned with the current shipped version. Existing
non-Raven skills at the same path are left untouched.

The default install root is ~/.agents/skills for user scope or .agents/skills
in the current project. Use --dest to install elsewhere.

In an interactive terminal Raven prints the plan and prompts before writing.
For non-interactive or --json runs, pass --yes to apply; without it the command
returns a preview and reports that confirmation is required.

To only update or realign skills that are already installed, use
'rvn skill sync' instead.`,
		Args: []ArgMeta{
			{Name: "names", Description: "Shipped skill names to install (default: all shipped skills)", Variadic: true, Examples: []string{"raven-core", "raven-query"}},
		},
		Flags: []FlagMeta{
			{Name: "scope", Description: "Install scope: user or project", Type: FlagTypeString, Default: "user", Examples: []string{"user", "project"}},
			{Name: "dest", Description: "Override install root path", Type: FlagTypeString},
			{Name: "yes", Description: "Apply changes without prompting (required for --json/non-interactive runs)", Type: FlagTypeBool},
			{Name: "confirm", Description: "Alias for --yes", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn skill install",
			"rvn skill install --yes --json",
			"rvn skill install raven-core raven-query --yes --json",
			"rvn skill install --scope project --yes --json",
		},
		UseCases: []string{
			"First-run install of all shipped Raven skills",
			"Install a specific set of Raven skills",
			"Preview the skill install before writing files",
		},
	},
	"skill_remove": {
		Name:        "skill remove",
		Description: "Remove an installed Raven skill",
		VaultScope:  VaultScopeNone,
		LongDesc: `Remove one installed Raven skill from the resolved install root.

Preview is returned by default. Use --confirm to apply removal.`,
		Args: []ArgMeta{
			{Name: "name", Description: "Skill name to remove", Required: true, Completions: []string{"raven-core", "raven-maintenance", "raven-onboarding", "raven-query", "raven-schema", "raven-templates", "raven-vault-admin"}},
		},
		Flags: []FlagMeta{
			{Name: "scope", Description: "Install scope: user or project", Type: FlagTypeString, Default: "user", Examples: []string{"user", "project"}},
			{Name: "dest", Description: "Override install root path", Type: FlagTypeString},
			{Name: "confirm", Description: "Apply changes (without this flag, shows preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn skill remove raven-core --confirm --json",
			"rvn skill remove raven-schema --scope project --json",
		},
		UseCases: []string{
			"Preview skill removal before deleting files",
			"Uninstall a Raven skill",
		},
	},
	"skill_doctor": {
		Name:        "skill doctor",
		Description: "Inspect Agent Skills installation health",
		VaultScope:  VaultScopeNone,
		LongDesc:    `Inspect the resolved install root and installed Raven skills.`,
		Flags: []FlagMeta{
			{Name: "scope", Description: "Install scope: user or project", Type: FlagTypeString, Default: "user", Examples: []string{"user", "project"}},
			{Name: "dest", Description: "Override install root path", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn skill doctor --json",
			"rvn skill doctor --scope project --json",
		},
		UseCases: []string{
			"Verify skill install roots and installed skill presence",
			"Debug skill installation path resolution",
		},
	},
}
