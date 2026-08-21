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
	"skill_install": {
		Name:        "skill install",
		Use:         "install [skill...]",
		Description: "Install or align shipped Raven skills",
		VaultScope:  VaultScopeNone,
		LongDesc: `Reconcile shipped Raven skills in one command.

With no arguments, Raven reconciles the complete Raven-managed skill set to the
catalog shipped with this rvn version: missing shipped skills are installed,
existing Raven-managed skills are aligned with the shipped version, and
receipt-managed skills that are no longer shipped are removed. Files not listed
in a Raven receipt and non-Raven skills at the same path are left untouched.

Pass one or more skill names to install or align only those shipped skills.
Named installs do not remove unrelated Raven-managed skills.

The default install root is ~/.agents/skills for user scope or .agents/skills
in the current project. Use --dest to install elsewhere.

In an interactive terminal Raven prints the plan and prompts before writing.
For non-interactive or --json runs, pass --confirm to apply; without it the
command returns a preview and reports that confirmation is required.`,
		Args: []ArgMeta{
			{Name: "names", Description: "Shipped skill names to install or align (default: reconcile full catalog)", Variadic: true, Examples: []string{"raven-core", "raven-query"}},
		},
		Flags: []FlagMeta{
			{Name: "scope", Description: "Install scope: user or project", Type: FlagTypeString, Default: "user", Examples: []string{"user", "project"}},
			{Name: "dest", Description: "Override install root path", Type: FlagTypeString},
			{Name: "confirm", Description: "Apply changes (without this flag, shows preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn skill install",
			"rvn skill install --confirm --json",
			"rvn skill install raven-core raven-query --confirm --json",
			"rvn skill install --scope project --confirm --json",
		},
		UseCases: []string{
			"Install or update the full shipped Raven skill catalog",
			"Remove receipt-managed skills that are no longer shipped",
			"Install or align a specific set of shipped Raven skills",
			"Preview skill reconciliation before writing files",
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
