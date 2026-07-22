package commands

var navigationRegistry = map[string]Meta{
	"date": {
		Name:        "date",
		Description: "Date hub - all activity for a date",
		Args: []ArgMeta{
			{Name: "date", Description: "Date (today, yesterday, YYYY-MM-DD)", Required: false},
		},
		Examples: []string{
			"rvn date today --json",
			"rvn date 2025-02-01 --json",
		},
	},
	"read": {
		Name:        "read",
		Use:         "read [reference]",
		Description: "Read a file (raw or enriched)",
		LongDesc: `Read and output a file from the vault.

Prefer the canonical object ID (for example, people/freya). File paths, aliases,
and unambiguous short forms are also accepted as resolution inputs, but short
forms are not the preferred input for automation.

By default, this command returns enriched output (rendered wikilinks + backlinks).
Use --raw to output only the raw file content (recommended for agents preparing precise edits).

Section references such as project/website#tasks are supported: without an
explicit line range, output is limited to that section's subtree (the section
plus its child sections).

Use --sections to return the file's section outline (id, title, level, line
ranges, parent) instead of content. With a section reference, the outline is
scoped to that section's subtree.

In an interactive terminal, bare 'rvn read' launches Raven's picker.
When an interactive read reference is ambiguous, Raven prompts you to choose the target.

For long files, you can request a specific range with --start-line/--end-line, and/or
ask for structured line output with --lines for copy-paste-safe anchors.` + barePickerInsertModeHelp,
		Args: []ArgMeta{
			{Name: "path", Description: "Canonical object ID or other accepted reference input", Required: true, CLIOptional: true},
		},
		Flags: []FlagMeta{
			{Name: "raw", Description: "Output only raw file content (no backlinks, no rendered links)", Type: FlagTypeBool},
			{Name: "no-links", Description: "Disable clickable hyperlinks in terminal output", Type: FlagTypeBool},
			{Name: "lines", Description: "Include structured lines with line numbers (recommended for agents)", Type: FlagTypeBool},
			{Name: "start-line", Description: "Start line (1-indexed, inclusive) for raw output", Type: FlagTypeInt},
			{Name: "end-line", Description: "End line (1-indexed, inclusive) for raw output", Type: FlagTypeInt},
			{Name: "sections", Description: "Return the section outline (headings with ids, levels, line ranges) instead of content", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn read 2025-02-01 --json",
			"rvn read people/freya.md --json",
			"rvn read people/freya --raw --json",
			"rvn read people/freya --raw --start-line 10 --end-line 40 --json",
			"rvn read people/freya --raw --lines --json",
			"rvn read projects/website#tasks --json",
			"rvn read projects/website --sections --json",
		},
		UseCases: []string{
			"Read vault file content (use instead of 'cat', 'head', 'tail')",
			"Interactively pick a file to read in Raven's picker",
			"Interactively disambiguate read references in Raven's picker",
			"Inspect file before editing (prefer --raw for exact string matching)",
			"Extract copy-paste-safe anchors with --lines or line ranges for long files",
			"Discover a file's sections and their IDs with --sections before targeted writes",
			"Get full content after finding object via query",
		},
	},
	"daily": {
		Name:        "daily",
		Description: "Resolve or create a daily note",
		LongDesc: `Resolve or create a daily note for a given date.

If no date is provided, resolves today's note. Creates the file if it doesn't exist.

The response surfaces the identity pair: data.file is the vault-relative path
(e.g. daily/2026-03-15.md) and data.id is the canonical object ID, which is the
bare ISO date (2026-03-15). Use data.id for [[refs]], not the file path.`,
		Args: []ArgMeta{
			{Name: "date", Description: "Date (today, yesterday, tomorrow, YYYY-MM-DD)", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "template", Description: "Core date template ID to use when creating a new daily note", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn daily --json",
			"rvn daily yesterday --json",
			"rvn daily 2025-02-01 --json",
			"rvn daily 2025-02-01 --template daily_default --json",
		},
		UseCases: []string{
			"Access or create today's daily note",
			"Navigate to past daily notes",
		},
	},
	"open": {
		Name:        "open",
		Use:         "open [reference]",
		Description: "Open a file in your editor",
		LongDesc: `Opens a file in your configured editor.

Prefer the canonical object ID (for example, companies/cursor). File paths,
aliases, and unambiguous short forms are also accepted as resolution inputs,
but short forms are not the preferred input for automation.

The editor is determined by the 'editor' setting in config.toml or $EDITOR.

In an interactive terminal, bare 'rvn open' launches Raven's picker
over indexed object, section, and asset references.

When an interactive open reference is ambiguous, Raven prompts you to choose the
target. Non-interactive and JSON output still return REF_AMBIGUOUS with the
candidate matches.

Use --stdin to read object IDs from stdin (one per line) and open them all.
This is useful for piping query results to open multiple files at once.` + barePickerInsertModeHelp,
		Args: []ArgMeta{
			{Name: "reference", Description: "Canonical object ID or other accepted reference input", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "stdin", Description: "Read object IDs from stdin for bulk open", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn open companies/cursor --json",
			"rvn query 'type:project .status==active' --ids | rvn open --stdin --json",
		},
		UseCases: []string{
			"Open a file by its canonical object ID",
			"Interactively pick a file to open in Raven's picker",
			"Interactively disambiguate open references in Raven's picker",
			"Open multiple files from query results",
		},
	},
}
