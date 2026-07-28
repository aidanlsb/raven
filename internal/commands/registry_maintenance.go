package commands

var maintenanceRegistry = map[string]Meta{
	"reindex": {
		Name:        "reindex",
		Description: "Rebuild the SQLite index from managed vault files",
		LongDesc: `Parses managed markdown files in the vault and rebuilds the SQLite index.

Use this after:
- Bulk file operations outside of Raven
- Schema changes that affect indexing
- Recovering from index corruption

By default, performs an incremental reindex that only processes files that have
changed since the last index. Deleted files are automatically detected and
removed from the index.

Non-Markdown files are not indexed as entities. Markdown file and URL targets
are represented by the link edge index.

Paths matched by raven.yaml exclude patterns are skipped and removed from the
index during incremental reindexing.

Raven records interrupted or intentionally deferred post-mutation index work in
.raven/index-dirty.json. Incremental reindex forces those paths even when file
mtimes are unchanged and clears each entry only after successful projection. An
operation interrupted before its paths were known triggers one full vault scan.

Use --full to force a complete rebuild of the entire index.

Incremental reindexing uses SQLite WAL and can run while readers such as rvn lsp
hold the index open. Full reindexing and incompatible-schema replacement require
exclusive access; stop the LSP or wait for the other process if the index is
locked.

When the on-disk index schema is incompatible with this Raven version, reindex
replaces the derived database and automatically performs a full rebuild. The
index remains unavailable until that rebuild completes; if it is interrupted,
run reindex again. A dry run reports the required rebuild without replacing the
on-disk index.`,
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
		Description: "Validate managed vault files against schema",
		LongDesc: `Validates managed files in the vault against the schema.

Returns structured issues with:
- issue_type: unknown_type, missing_reference, broken_file_link, markdown_link_to_vault_note, directory_type_mismatch, undefined_trait, unknown_frontmatter_key, etc.
- fix_command: Suggested CLI command to fix the issue
- fix_hint: Human-readable explanation of how to fix

The summary groups issues by type with counts and top values, making it easy to prioritize fixes.

Scoping:
- Pass a reference to a file, directory, or object to check a subset of the vault
- Use --type to check all objects of a specific type
- Use --trait to check all usages of a specific trait
- Use --issues to check only specific issue types
- Use --exclude to skip specific issue types

Paths matched by raven.yaml exclude patterns are outside Raven management and
are not checked.

File-link existence is evaluated against the current filesystem when check
runs. URL targets are never fetched and are not reported as broken.
Inline Markdown links/images targeting in-vault .md notes are reported because
they must use Raven wikilinks to participate in backlinks and reference
rewrites.

For agents: Use this tool to discover issues, then use the fix_command suggestions to resolve them.
For missing_reference summaries, preview generated pages with 'rvn check create-missing --json'
before applying with 'rvn check create-missing --confirm --json'.
Ask the user for clarification when needed (e.g., which type to use for missing references).`,
		Args: []ArgMeta{
			{Name: "reference", Description: "File, directory, or object reference to check (optional, defaults to entire vault)", Required: false},
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
		},
		Examples: []string{
			"rvn check --json",
			"rvn check people/freya.md --json",
			"rvn check projects/ --json",
			"rvn check people/freya --json",
			"rvn check --type project --json",
			"rvn check --trait due --json",
			"rvn check --issues missing_reference,unknown_type --json",
			"rvn check --exclude unused_type,unused_trait --json",
			"rvn check create-missing --json",
			"rvn check create-missing --confirm --json",
		},
		UseCases: []string{
			"Validate entire vault for issues",
			"Check a specific file after editing",
			"Verify all objects of a type are valid",
			"Check all trait usages for correct values",
			"Focus on specific issue types",
		},
	},
	"check_fix": {
		Name:        "check fix",
		Description: "Preview or apply safe auto-fixes for check findings",
		LongDesc: `Runs check, then previews or applies only unambiguous safe fixes.

Preview is default; use --confirm to apply.

Auto-fixable issue types include:
- short_ref_could_be_full_path: rewrite short refs to canonical object IDs
- invalid_enum_value: remove unnecessary quotes around enum trait values
- non_canonical_ref: strip configured root prefix from wikilink targets
- non_canonical_path: move file under the configured directory root for its type
  and rewrite all references that point at it`,
		Args: []ArgMeta{
			{Name: "reference", Description: "File, directory, or object reference to check before fixing (optional, defaults to entire vault)", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "strict", Description: "Treat warnings as errors", Type: FlagTypeBool},
			{Name: "type", Short: "t", Description: "Check only objects of this type", Type: FlagTypeString},
			{Name: "trait", Description: "Check only usages of this trait", Type: FlagTypeString},
			{Name: "issues", Description: "Only check these issue types (comma-separated)", Type: FlagTypeString},
			{Name: "exclude", Description: "Exclude these issue types (comma-separated)", Type: FlagTypeString},
			{Name: "errors-only", Description: "Only report errors, skip warnings", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply fixes (without this flag, shows preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn check fix --json",
			"rvn check fix people/freya.md --json",
			"rvn check fix --type project --confirm --json",
		},
		UseCases: []string{
			"Preview deterministic auto-fixes before applying",
			"Apply canonical-reference, quoted-enum, and canonical-layout fixes safely",
		},
	},
	"check create-missing": {
		Name:        "check create-missing",
		Description: "Create missing references discovered by check",
		LongDesc: `Runs check, then creates missing referenced pages.

Non-JSON mode prompts interactively.
JSON mode previews by default. Add --confirm to create only deterministic typed targets.`,
		Flags: []FlagMeta{
			{Name: "strict", Description: "Treat warnings as errors", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply create-missing changes in non-interactive mode (without this flag, shows preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn check create-missing --json",
			"rvn check create-missing --confirm --json",
		},
		UseCases: []string{
			"Create missing pages from check findings",
			"Run interactive missing-reference creation in terminal mode",
		},
	},
}
