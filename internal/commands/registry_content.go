package commands

var contentRegistry = map[string]Meta{
	"new": {
		Name:        "new",
		Description: "Create a new typed object",
		LongDesc: `Creates a new note with the specified type.

⚠️ IMPORTANT FOR AGENTS: ALWAYS use this command to create new vault objects instead
of writing files directly with 'echo', 'touch', or file writing tools. The raven_new
command applies templates, validates against the schema, and ensures proper indexing.

The type is required. If title is not provided in interactive CLI mode, you will
be prompted for it. Interactive CLI mode prompts for schema fields; optional
fields can be skipped with a blank response. Field values can also be provided
via --field flags or --fields-json.

Use --field for shell-friendly literal values. Use --fields-json when exact type
control matters, such as preserving the string "true" instead of a boolean or
providing arrays/nulls explicitly.

Titles are treated as display names and are stored verbatim in frontmatter
(populating name_field when configured). When no --path is given, the title is
slugified into the filename/path, so titles may contain "/" or other characters
that are unsafe in paths (e.g. "config.VaultConfig duplicates internal/paths"
becomes config-vaultconfig-duplicates-internal-paths.md). The object ID stays
derived from the resulting file path. Use --path to explicitly control the object
path (its "/" segments map to directories).

If the type has a name_field configured (e.g., name_field: name), the title
argument automatically populates that field. This means for a person type with
name_field: name, you can just call: rvn new person "Freya" --json
and the name field will be set to "Freya" automatically.

For agents/MCP: Raven runs non-interactively with --json, so title must be provided.
For agents: If required fields are missing, returns error with details including
a retry_with template. Check if the type has name_field set (via raven_schema type <name>)
to understand which fields are auto-populated.

Identity in the response: on success the file path (data.file, where the file
lives, vault-relative) and the canonical object ID (data.id, the value to use in
references) are surfaced as a pair. These often differ (e.g. file type/person/freya.md
links as person/freya), so use data.id for [[refs]] and follow-up commands rather
than deriving an ID from the path — that avoids non_canonical_ref / non_canonical_path.

Permissive writes: if a ref field points at a target that does not exist yet, the
object is still created. The successful response adds data.missing_refs,
data.missing_ref_items, and a REF_TARGET_MISSING warning per missing target. Interactive
CLI offers to create the missing pages; agents can run 'rvn check create-missing'.`,
		Args: []ArgMeta{
			{Name: "type", Description: "Object type (e.g., person, project)", Required: true, DynamicComp: "types"},
			// NOTE: Title is optional in interactive CLI mode, but required in --json (MCP) mode.
			{Name: "title", Description: "Title/name for the object (auto-populates name_field if configured)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "field", Description: "Set field value (can be repeated): --field name=value", Type: FlagTypeKeyValue, Examples: []string{`{"name": "Freya", "email": "a@b.com"}`}},
			{Name: "fields-json", Description: "Set frontmatter fields via JSON object (typed values)", Type: FlagTypeJSON},
			{Name: "path", Description: "Explicit target path (overrides title-derived path)", Type: FlagTypeString, Examples: []string{"people/freya-2026", "note/raven-friction"}},
			{Name: "template", Description: "Type template ID to use for object creation", Type: FlagTypeString, Examples: []string{"interview_technical", "interview_screen"}},
		},
		Examples: []string{
			"rvn new person \"Freya\" --json",
			"rvn new note \"Raven Friction\" --path note/raven-friction --json",
			"rvn new project \"Website Redesign\" --json",
			"rvn new interview \"Jane Doe @ Acme\" --template technical --json",
			"rvn new book \"The Prose Edda\" --field author=people/snorri --json",
			"rvn new person \"Freya\" --fields-json '{\"email\":\"freya@asgard.realm\"}' --json",
		},
		UseCases: []string{
			"Create a new typed object (NEVER write vault files directly)",
			"Create a new person entry with schema validation",
			"Create a new project file with template applied",
		},
	},
	"add": {
		Name:        "add",
		Use:         "add <text>",
		Description: "Quick capture - append text to daily note or inbox",
		LongDesc: `Quickly capture a thought, task, or note.

By default, appends to today's daily note. Configure destination in raven.yaml.
Only works on files that already exist (daily notes are auto-created).
Auto-reindex is ON by default; configure via auto_reindex in raven.yaml.
For creating NEW typed objects, use 'rvn new' instead.
Single-target adds apply immediately; --confirm only controls bulk --stdin adds.

Bulk operations:
Use --stdin to read object IDs from stdin and append text to each.
Section IDs (e.g., project/website#tasks) are supported: text is appended
inside the targeted section instead of at the end of the file.
Bulk operations preview changes by default; use --confirm to apply.

Configuration (raven.yaml):
  capture:
    destination: daily      # "daily" or a file path
    heading: "## Captured"  # Optional existing heading to append under

Use --to file#section to append inside a section. Section-targeted adds, including
section IDs supplied through --stdin, insert at the end of the section's DIRECT
content, before any child headings. Structural section placement instead uses
the full subtree boundary; create or move headings with 'rvn section create' and
'rvn section move'.

The removed --heading and --create-heading flags are hard errors. To create a
heading, run 'rvn section create <file> "<title>" --level N', then append body
content with 'rvn add <text> --to <file#section>'. Add also rejects text that
contains Markdown headings; headings are section lifecycle, not body content.

A configured capture.heading may target an existing literal Markdown heading,
but add never creates a missing heading.

Permissive writes: if appended text contains a [[ref]] whose target does not exist
yet, the write still succeeds. The response adds data.missing_refs,
data.missing_ref_items, and a REF_TARGET_MISSING warning per missing target.

If text starts with a dash, put it after -- so it is not parsed as a flag:
  rvn add --to today -- "- Review the rollout"`,
		Args: []ArgMeta{
			{Name: "text", Description: "Text to add (can include @traits and [[refs]])", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "to", Description: "Target file (path or reference like 'cursor')", Type: FlagTypeString, Examples: []string{"projects/website.md", "inbox.md", "tomorrow"}},
			{Name: "stdin", Description: "Read object IDs from stdin (one per line)", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply bulk changes (without this flag, bulk shows preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn add \"Quick thought\" --json",
			"rvn add \"@priority(high) Urgent task\" --json",
			"rvn add \"Note\" --to projects/website.md --json",
			"rvn add \"Plan\" --to tomorrow --json",
			"rvn add --to today -- \"- Review the rollout\"",
			"rvn add \"Bug report\" --to project/raven#bugs-fixes --json",
			"rvn query \"section .title==Tasks\" --ids | rvn add \"Review backlog\" --stdin --confirm --json",
		},
		UseCases: []string{
			"Quick capture to daily note",
			"Add tasks to existing project files",
			"Append notes to existing documents",
		},
	},
	"upsert": {
		Name:        "upsert",
		Description: "Create or update a typed object idempotently",
		LongDesc: `Create or update a typed object deterministically.

This command is the canonical idempotent write primitive for generated artifacts.
It creates a new object when missing, or updates the existing one in place.

Semantics:
- By default, identity is derived from <type> + <title> (same routing/slug logic as 'new')
- Use --path to override the storage path explicitly while keeping title as display metadata
- Frontmatter fields provided via --field are merged/updated
- If --content is provided, the body is fully replaced (idempotent reruns)
- Use --content-file to read the replacement body from a file, or '-' for stdin
- Returns status: created, updated, or unchanged
- Surfaces the identity pair on success: data.file (vault-relative path) and
  data.id (canonical object ID). Use data.id for [[refs]] and follow-up commands;
  it often differs from the path, so do not derive an ID from data.file.

Boundary with add:
- add: append-only capture/logging, intentionally non-idempotent
- upsert: canonical state write, idempotent convergence target

Use this for generated outputs like briefs/reports/summaries where reruns should
converge to one current state rather than append history.

Use --field for shell-friendly literal values. Use --fields-json when exact type
control matters, such as preserving the string "true" instead of a boolean or
providing arrays/nulls explicitly.

Permissive writes: if a ref field or body [[ref]] points at a target that does not
exist yet, the write still succeeds. The response adds data.missing_refs,
data.missing_ref_items, and a REF_TARGET_MISSING warning per missing target.`,
		Args: []ArgMeta{
			{Name: "type", Description: "Object type (e.g., brief, report)", Required: true, DynamicComp: "types"},
			{Name: "title", Description: "Title/name for the object (stable identity key)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "field", Description: "Set field value (can be repeated): --field name=value", Type: FlagTypeKeyValue, Examples: []string{`{"source": "daily-brief", "status": "ready"}`}},
			{Name: "fields-json", Description: "Set/update frontmatter fields as a JSON object", Type: FlagTypeJSON},
			{Name: "content", Description: "Replace body content (idempotent full-body mode)", Type: FlagTypeString},
			{Name: "content-file", Description: "Read replacement body content from a file, or '-' for stdin", Type: FlagTypeString, Examples: []string{"/tmp/brief.md", "-"}},
			{Name: "path", Description: "Explicit target path (overrides title-derived path)", Type: FlagTypeString, Examples: []string{"brief/daily-2026-02-14", "note/raven-friction"}},
		},
		Examples: []string{
			"rvn upsert brief \"Daily Brief 2026-02-14\" --content \"# Daily Brief\" --json",
			"rvn upsert brief \"Daily Brief 2026-02-14\" --content-file /tmp/brief.md --json",
			"rvn upsert brief \"Daily Brief 2026-02-14\" --content-file - --json < /tmp/brief.md",
			"rvn upsert note \"Raven Friction\" --path note/raven-friction --content \"# Notes\" --json",
			"rvn upsert report \"Q1 Status\" --field owner=people/freya --field status=draft --json",
		},
		UseCases: []string{
			"Idempotently persist generated outputs",
			"Create-or-update canonical report/brief objects",
			"Replace an object's body deterministically on reruns",
		},
	},
	"delete": {
		Name:        "delete",
		Description: "Delete an object or file from the vault",
		LongDesc: `Delete a file-backed object or an explicit non-Markdown file path from the vault.

⚠️ IMPORTANT FOR AGENTS: ALWAYS use this command instead of shell commands like 'rm'.
Using 'rm' directly will NOT warn about backlinks (other files that reference this one),
potentially creating broken links throughout the vault. The raven_delete command:
- Reports incoming backlink warnings
- Moves files to deletion.trash_dir for recovery (not permanent deletion)
- Updates the index properly

By default, files are moved to a trash directory (.trash/). Recover with trash
list followed by preview-first restore; do not move entries manually.
Warns about backlinks to Raven objects. File-link integrity is reported by
broken_file_link in 'rvn check'.

Single-file delete:
Applies immediately when invoked (CLI JSON and MCP). Pass --dry-run to preview the
deletion (and its backlink impact) without applying. Interactive CLI terminals still
show a confirmation prompt unless --force is set. Only call delete after user intent
is clear; when unsure, inspect the object and run backlinks first.

Bulk operations:
Use --stdin to read object references or explicit file paths from stdin (one per line).
IMPORTANT:
- Bulk operations return preview by default. Changes are NOT applied unless confirm=true.`,
		Args: []ArgMeta{
			{Name: "reference", Description: "Object reference or explicit non-Markdown file path to delete", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "force", Description: "Skip confirmation prompt", Type: FlagTypeBool},
			{Name: "stdin", Description: "Read object references or file paths from stdin (one per line)", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply bulk delete (without this flag, bulk shows preview only)", Type: FlagTypeBool},
			{Name: "dry-run", Description: "Preview a single-file delete without applying it", Type: FlagTypeBool},
		},
		BulkStdinArgName: "references",
		Examples: []string{
			"rvn delete people/freya --json",
			"rvn delete people/freya --dry-run --json",
			"rvn delete files/paper.pdf --dry-run --json",
			"rvn delete projects/old --force",
		},
		UseCases: []string{
			"Delete a file safely (NEVER use 'rm' shell command)",
			"Remove objects with backlink warnings",
			"Move files to trash with backlink warnings",
		},
	},
	"restore": {
		Name:        "restore",
		Use:         "restore <trash-reference-or-path>",
		Description: "Restore an object or file from the configured vault trash",
		LongDesc: `Restores one file from deletion.trash_dir to the mirrored vault path.

Pass a canonical reference returned by trash list, an exact trash_path, the path
relative to the trash directory, or the restore_path. Resolution is exact and
fails when a reference matches multiple trash entries; retry with trash_path to
select one explicitly.

Restore never overwrites an existing destination. Move or delete the occupying
file before retrying.

Preview is default. Pass --confirm to move the file back into the vault. A
successful restore updates the derived index and re-runs reference resolution,
so incoming references to a restored Markdown ID resolve again when possible.`,
		Category:   CategoryContent,
		Access:     AccessWrite,
		Risk:       RiskMutating,
		VaultScope: VaultScopeRequired,
		Args: []ArgMeta{
			{Name: "reference", Description: "Trash reference or exact trash/restore path", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "confirm", Description: "Apply the restore (without this flag, shows preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn restore people/freya --json",
			"rvn restore people/freya --confirm --json",
			"rvn restore .trash/files/paper.pdf --confirm --json",
		},
		UseCases: []string{
			"Preview recovery of a deleted object or file",
			"Restore a deleted object and heal incoming references",
			"Recover a specific entry by its exact trash path",
		},
	},
	"move": {
		Name:        "move",
		Description: "Move or rename an object or file within the vault",
		LongDesc: `Move or rename a file or object within the vault.

⚠️ IMPORTANT FOR AGENTS: ALWAYS use this command instead of shell commands like 'mv'.
Using 'mv' directly will NOT update references to the file, causing broken links
throughout the vault. The raven_move command automatically updates [[references]],
schema-typed frontmatter ref/ref[] fields, and indexed Markdown file links/images
that point to the moved file.

SECURITY: Both source and destination must be within the vault.
Files cannot be moved outside the vault, and external files cannot be moved in.

This command:
- Validates paths are within the vault
- Updates all body references, schema-typed frontmatter ref/ref[] fields, and
  normalized-key-matched file links to the moved file (--update-refs, default: true)
- Preserves non-Markdown filenames and extensions
- Warns if moving to a type's default directory with mismatched type
- Creates destination directories if needed

If moving a file to a type's default directory (e.g., people/) but the file
has a different type, returns a warning with needs_confirm=true (without moving).
The agent should ask the user how to proceed and re-run with --skip-type-check.

Single-object move:
Applies immediately when invoked (CLI JSON and MCP). Pass --dry-run to preview the
move and the references it would update without applying. Preview/apply responses
list affected source objects in updated_refs and identify frontmatter fields in
updated_ref_fields with source_id, file, and field.

Section IDs are not valid move sources. Use 'rvn section move <file#section>' to
reorder or reparent a heading without changing its identity. Use 'rvn section
rename <file#section> "<new heading text>"' only to rename a heading and rewrite
inbound fragment references. This is a hard error for both single and bulk move
inputs.

Bulk operations:
Use --stdin to read object IDs from stdin (one per line).
Destination must be a directory (ending with /).
IMPORTANT: Bulk operations return preview by default. Changes are NOT applied unless confirm=true.`,
		Args: []ArgMeta{
			{Name: "source", Description: "Source object reference or explicit non-Markdown file path; section IDs are rejected", Required: false},
			{Name: "destination", Description: "Destination path (e.g., people/loki-archived or archive/projects/)", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "force", Description: "Skip confirmation prompts", Type: FlagTypeBool},
			{Name: "update-refs", Description: "Update body, frontmatter ref-field, and file-link references to moved file", Type: FlagTypeBool, Default: "true"},
			{Name: "skip-type-check", Description: "Skip type-directory mismatch warning", Type: FlagTypeBool},
			{Name: "stdin", Description: "Read object IDs from stdin (one per line)", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply bulk move (without this flag, bulk shows preview only)", Type: FlagTypeBool},
			{Name: "dry-run", Description: "Preview a single-object move without applying it", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn move people/loki people/loki-archived --json",
			"rvn move people/loki people/loki-archived --dry-run --json",
			"rvn move inbox/task.md projects/website/task.md --json",
			"rvn move drafts/person.md people/freya.md --update-refs --json",
			"rvn move files/paper.pdf files/archive/paper.pdf --json",
		},
		UseCases: []string{
			"Rename a file in place (NEVER use 'mv' shell command)",
			"Move file to different directory with reference updates",
			"Reorganize vault structure while keeping links intact",
			"Archive old content without breaking references",
		},
	},
	"section_rename": {
		Name:        "section rename",
		CLIPath:     []string{"section", "rename"},
		Description: "Rename a section heading and rewrite inbound fragment references",
		Category:    CategoryContent,
		Access:      AccessWrite,
		Risk:        RiskMutating,
		LongDesc: `Rename a Markdown section heading in place.

The source must be a section ID such as project/website#tasks. The destination
is plain heading text, not a Markdown heading and not another section ID. Raven
preserves the heading level, derives the new slug from the text, and rewrites
inbound [[...#old-slug]] references to the new slug.

The rename fails without writing if the new slug duplicates another section or
would shift another section's slug. It applies immediately by default; pass
--dry-run to preview the new section ID and reference rewrites.`,
		Args: []ArgMeta{
			{Name: "section_id", Description: "Section ID to rename (e.g., project/website#tasks)", Required: true},
			{Name: "new_heading_text", Description: "New heading text as plain text, without a leading #", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "dry-run", Description: "Preview the section rename without applying it", Type: FlagTypeBool},
		},
		Examples: []string{
			`rvn section rename project/website#tasks "Completed Tasks" --json`,
			`rvn section rename project/website#tasks "Completed Tasks" --dry-run --json`,
		},
		UseCases: []string{
			"Rename a section heading without breaking inbound fragment references",
			"Preview the slug and reference impact of a heading rename",
		},
	},
	"section_create": {
		Name:        "section create",
		CLIPath:     []string{"section", "create"},
		Use:         `create <file> "<title>" --level N`,
		Description: "Create a Markdown section at an explicit structural boundary",
		Category:    CategoryContent,
		Access:      AccessWrite,
		Risk:        RiskMutating,
		LongDesc: `Create an empty Markdown section heading in an existing file.

The title is plain text and --level is required; do not include Markdown '#'
prefixes in the title. With no anchor, the heading is appended at end of file.

Structural anchors are mutually exclusive:
- --after inserts after the anchor's complete subtree, including all descendants.
- --before inserts immediately before the anchor heading.
- --under inserts as the anchor's last direct child.

For --after and --before, --level must equal the anchor level. For --under,
--level must equal the anchor level plus one. Raven never rewrites the requested
level. Creation fails without writing if the new slug collides or if any existing
section slug would shift.

This command applies immediately by default. Pass --dry-run to preview and
receive the canonical new section ID without writing.`,
		Args: []ArgMeta{
			{Name: "file", Description: "Existing file reference that will contain the section", Required: true},
			{Name: "title", Description: "Plain section title without a leading #", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "level", Description: "Markdown heading level from 1 through 6", Type: FlagTypeInt, Required: true},
			{Name: "after", Description: "Insert after this section's complete subtree", Type: FlagTypeString},
			{Name: "before", Description: "Insert immediately before this section heading", Type: FlagTypeString},
			{Name: "under", Description: "Insert as the last direct child of this section", Type: FlagTypeString},
			{Name: "dry-run", Description: "Preview section creation without applying it", Type: FlagTypeBool},
		},
		Examples: []string{
			`rvn section create project/website "Tasks" --level 2 --json`,
			`rvn section create project/website "Follow-up" --level 3 --under project/website#tasks --json`,
			`rvn section create project/website "Notes" --level 2 --before project/website#archive --dry-run --json`,
		},
		UseCases: []string{
			"Create a section without writing Markdown headings directly",
			"Insert a sibling after a complete section subtree",
			"Create a direct child at an explicit heading level",
		},
	},
	"section_delete": {
		Name:        "section delete",
		CLIPath:     []string{"section", "delete"},
		Use:         "delete <reference>",
		Description: "Delete a section heading and its complete subtree",
		Category:    CategoryContent,
		Access:      AccessWrite,
		Risk:        RiskDestructive,
		LongDesc: `Delete a Markdown section heading and its complete subtree.

The reference must be a section ID such as project/website#tasks. File/object
references and non-Markdown file paths are rejected. Raven removes the heading,
its direct body, and every descendant heading and body up to the same complete
subtree boundary used by 'section move'. Parent and sibling sections are left
in place.

This destructive command previews by default. The preview reports the exact
inclusive line range, removed Markdown content, every section ID in the
subtree, and inbound references that would become stale. Raven leaves those
inbound references unchanged because it cannot infer a safe replacement target;
repair or remove them explicitly. Pass --confirm to apply the deletion.`,
		Args: []ArgMeta{
			{Name: "reference", Description: "Section reference to delete (e.g., project/website#tasks)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "confirm", Description: "Delete the section subtree (without this flag, shows preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			`rvn section delete project/website#tasks --json`,
			`rvn section delete project/website#tasks --confirm --json`,
		},
		UseCases: []string{
			"Preview the exact heading subtree and inbound references before deletion",
			"Delete a section subtree without disturbing its parent or siblings",
			"Remove a heading through Raven instead of hand-editing Markdown",
		},
	},
	"section_move": {
		Name:        "section move",
		CLIPath:     []string{"section", "move"},
		Description: "Reorder or reparent a section and its complete subtree",
		Category:    CategoryContent,
		Access:      AccessWrite,
		Risk:        RiskMutating,
		LongDesc: `Move a Markdown section without renaming it.

The source heading and every descendant in its complete subtree move together.
The heading text, level, slug, and canonical section ID stay unchanged. Use
'section rename' for identity changes.

Structural anchors are mutually exclusive:
- --after inserts after the anchor's complete subtree.
- --before inserts immediately before the anchor heading.
- --under reparents the source as the anchor's last direct child.

For --after and --before, the source and anchor must have equal heading levels.
For --under, the source level must equal the anchor level plus one. Raven never
promotes or demotes headings. Anchors in another file, anchors inside the source
subtree, missing anchors, and identity-changing placements are hard errors.
With no anchor, the complete subtree is moved to end of file.

This command applies immediately by default. Pass --dry-run to preview without
writing.`,
		Args: []ArgMeta{
			{Name: "section_id", Description: "Section ID to move (e.g., project/website#tasks)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "after", Description: "Move after this section's complete subtree", Type: FlagTypeString},
			{Name: "before", Description: "Move immediately before this section heading", Type: FlagTypeString},
			{Name: "under", Description: "Move as the last direct child of this section", Type: FlagTypeString},
			{Name: "dry-run", Description: "Preview the section move without applying it", Type: FlagTypeBool},
		},
		Examples: []string{
			`rvn section move project/website#notes --after project/website#tasks --json`,
			`rvn section move project/website#follow-up --under project/website#tasks --json`,
			`rvn section move project/website#archive --dry-run --json`,
		},
		UseCases: []string{
			"Reorder sibling sections while preserving nested children",
			"Reparent a section without changing its heading level or identity",
			"Move a complete section subtree to end of file",
		},
	},
	"reclassify": {
		Name:        "reclassify",
		Description: "Change an object's type",
		LongDesc: `Change an object's type, updating frontmatter fields, applying defaults
for the new type, and optionally moving the file to the new type's default directory.

Required fields on the new type:
- If a required field has a Default value, it is applied automatically
- Missing required fields can be supplied via --field flags or --fields-json
- In JSON mode: returns REQUIRED_FIELD_MISSING error with retry_with template

Use --field for shell-friendly Raven field literals. Use --fields-json when exact
type control matters, for example preserving the string "false" instead of a
boolean false.

Fields present on the old type but not defined on the new type are
identified as "dropped fields" and require confirmation before removal.
Use --force to skip this confirmation.

The file is automatically moved to the new type's default_path unless
--no-move is specified or no default_path is defined for the new type.
References are updated when the file moves (controlled by --update-refs).

Bulk operations:
Use --stdin to read references from stdin (one per line), with the target type
as the only positional argument. Bulk reclassification previews moves, field
changes, required-field failures, and reference updates by default. Use
--confirm to apply. Items that would drop fields still require --force.`,
		Args: []ArgMeta{
			{Name: "reference", Description: "Object reference (prefer canonical ID; other resolvable forms are accepted)", Required: false},
			{Name: "new-type", Description: "Target type name", Required: true, DynamicComp: "types"},
		},
		Flags: []FlagMeta{
			{Name: "field", Description: "Set field value (can be repeated): --field name=value", Type: FlagTypeKeyValue, Examples: []string{`{"author": "[[people/snorri]]", "genre": "mythology"}`}},
			{Name: "fields-json", Description: "Set/update frontmatter fields as a JSON object", Type: FlagTypeJSON},
			{Name: "no-move", Description: "Skip moving file to new type's default_path", Type: FlagTypeBool},
			{Name: "update-refs", Description: "Update references when file moves", Type: FlagTypeBool, Default: "true"},
			{Name: "force", Description: "Skip confirmation prompts", Type: FlagTypeBool},
			{Name: "stdin", Description: "Read references from stdin for bulk operations", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply bulk reclassification (without this flag, bulk shows preview only)", Type: FlagTypeBool},
		},
		BulkStdinArgName: "references",
		Examples: []string{
			"rvn reclassify inbox/note book --json",
			"rvn reclassify people/freya company --field industry=tech --json",
			`rvn reclassify people/freya company --fields-json '{"legal_name":"false"}' --json`,
			"rvn reclassify pages/draft project --no-move --json",
			"rvn reclassify inbox/note book --force --json",
			"rvn query 'type:note' --ids | rvn reclassify doc --stdin --json",
			"rvn query 'type:note' --ids | rvn reclassify doc --stdin --confirm --force --json",
		},
		UseCases: []string{
			"Change an object's type after creation",
			"Reclassify a page to a specific schema type",
			"Move an object to the correct type directory automatically",
			"Convert between custom types with field mapping",
			"Bulk reclassify objects from stdin with preview and confirmation",
		},
	},
	"set": {
		Name:        "set",
		Use:         "set <reference> <field=value>...",
		Description: "Set frontmatter fields on an object",
		LongDesc: `Set one or more frontmatter fields on an existing object.

Use the canonical object ID (e.g., "people/freya"). Other accepted reference
forms can also resolve an object, but short forms are not preferred for
automation. Field values are validated against the schema if the object has a
known type. Unknown fields are rejected.

Use this to update existing objects' metadata without manually editing files.

Use positional field=value arguments for shell-friendly literal updates.
Use --fields-json when you need exact type control (for example, preserving the
string "true" instead of coercing it to a boolean).

Single-object set applies immediately. Pass --dry-run to preview the resulting
field changes (including previous values) without writing.

Bulk operations:
Use --stdin to read references from stdin (one per line). Bulk updates accept
both field=value literals and --fields-json typed values.
IMPORTANT: Bulk operations return preview by default. Changes are NOT applied unless confirm=true.

Permissive writes: if a ref field is set to a target that does not exist yet, the
update still succeeds. The response adds data.missing_refs, data.missing_ref_items,
and a REF_TARGET_MISSING warning per missing target.`,
		Args: []ArgMeta{
			{Name: "reference", Description: "Object reference to update (e.g., people/freya)", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "fields", Description: "Fields to update using Raven field literals (key=value semantics)", Type: FlagTypePosKeyValue, Examples: []string{`{"email": "freya@asgard.realm"}`, `{"status": "active", "priority": "high"}`}},
			{Name: "fields-json", Description: "Fields to update as a JSON object with exact typed values", Type: FlagTypeJSON},
			{Name: "stdin", Description: "Read references from stdin for bulk operations", Type: FlagTypeBool},
			{Name: "confirm", Description: "Apply bulk changes (without this flag, bulk shows preview only)", Type: FlagTypeBool},
			{Name: "dry-run", Description: "Preview a single-object set without applying it", Type: FlagTypeBool},
		},
		BulkStdinArgName: "references",
		Examples: []string{
			"rvn set people/freya email=freya@asgard.realm --json",
			`rvn set people/freya --fields-json '{"email":"true"}' --json`,
			"rvn set people/freya name=\"Freya\" status=active --json",
			"rvn set projects/website priority=high --dry-run --json",
		},
		UseCases: []string{
			"Update a person's email or status",
			"Change project priority or status",
			"Set task due dates or assignments",
			"Modify any frontmatter field on an object",
			"Bulk update multiple objects via --stdin",
		},
	},
	"unset": {
		Name:        "unset",
		Use:         "unset <reference> <field>...",
		Description: "Remove frontmatter fields from an object",
		LongDesc: `Remove one or more frontmatter fields from an existing file-level object.

Use the canonical object ID (e.g., "people/freya"). Other accepted reference
forms can also resolve an object, but short forms are not preferred for
automation. Fields are removed from the YAML frontmatter entirely; they are not
set to null.

Unset can remove optional schema fields and unknown frontmatter keys. This makes
it useful after schema migrations where removed fields still exist on older
objects and appear as unknown_frontmatter_key issues in rvn check.

The reserved type field cannot be unset; use reclassify to change object type.`,
		Args: []ArgMeta{
			{Name: "reference", Description: "Object reference to update (e.g., people/freya)", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "fields", Description: "Frontmatter field names to remove (repeatable; MCP should pass an array)", Type: FlagTypeStringSlice, Examples: []string{`["date", "link"]`}},
		},
		Examples: []string{
			"rvn unset docs/cleanup date link --json",
			`rvn unset docs/cleanup --fields date --fields link --json`,
		},
		UseCases: []string{
			"Remove stale frontmatter keys after a schema migration",
			"Clean up unknown_frontmatter_key issues reported by rvn check",
			"Delete optional metadata from an object without editing the file manually",
		},
	},
	"update": {
		Name:        "update",
		Use:         "update <trait_id> <new_value>",
		Description: "Update a trait's value",
		LongDesc: `Update the value of a trait annotation.

Trait IDs look like "path/file.md:trait:N" and can be obtained via:
  - rvn query "trait:todo" --ids

Single-object update applies immediately. Pass --dry-run to preview the value
change without writing.

Bulk operations:
Use --stdin to read trait IDs from stdin (one per line).
Use repeated --trait-id flags to provide an explicit trait ID list without stdin.
IMPORTANT: Bulk operations return preview by default. Changes are NOT applied unless confirm=true.`,
		Args: []ArgMeta{
			{Name: "trait_id", Description: "Trait ID to update (e.g., daily/2026-01-25.md:trait:0)", Required: false},
			{Name: "value", Description: "New trait value", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "stdin", Description: "Read trait IDs from stdin for bulk operations", Type: FlagTypeBool},
			{Name: "trait-id", Description: "Trait ID for explicit-list bulk update (repeatable)", Type: FlagTypeStringSlice, Examples: []string{"daily/2026-01-25.md:trait:0"}},
			{Name: "confirm", Description: "Apply bulk changes (without this flag, bulk shows preview only)", Type: FlagTypeBool},
			{Name: "dry-run", Description: "Preview a single-object update without applying it", Type: FlagTypeBool},
		},
		BulkStdinArgName: "trait_ids",
		Examples: []string{
			"rvn update daily/2026-01-25.md:trait:0 done --json",
			"rvn update daily/2026-01-25.md:trait:0 done --dry-run --json",
			"rvn query 'trait:todo' --ids | rvn update --stdin done --confirm --json",
		},
		UseCases: []string{
			"Update a specific trait by ID",
			"Bulk update trait values via --stdin",
			"Bulk update an explicit list of trait IDs without stdin piping",
		},
	},
	"edit": {
		Name:        "edit",
		Description: "Surgical text replacement in vault content files",
		LongDesc: `Replace a unique string in a vault content file with another string.

⚠️ IMPORTANT FOR AGENTS: Use this command instead of shell tools like 'sed' or 'awk'
to edit vault content files. This maintains file integrity within the vault.

Scope:
- Supported: markdown content files such as objects, pages, and daily notes
- Not supported: raven.yaml, schema.yaml, template files, or protected/system-managed paths

Use dedicated Raven command surfaces for vault configuration, schema updates, and template management.

The string to replace must appear exactly once in the target scope to prevent
ambiguous edits. File targets use the whole file. Section targets such as
object#section use the section subtree, so repeated text outside that section
does not make the edit ambiguous.

IMPORTANT: Applies immediately. Pass --dry-run to preview the before/after diff
without writing.

Whitespace matters—old_str must match exactly including indentation.
For multi-line replacements, include newlines in both old_str and new_str.

Supports two input modes:
  - Single edit: <reference> <old_str> <new_str>
  - Batch edits via JSON: <reference> --edits-json '{"edits":[{"old_str":"from","new_str":"to"}]}'

Permissive writes: if an applied edit introduces a [[ref]] whose target does not
exist yet, the edit still succeeds. The response adds data.missing_refs,
data.missing_ref_items, and a REF_TARGET_MISSING warning per missing target.`,
		Args: []ArgMeta{
			{Name: "reference", Description: "File path, object reference, or section reference relative to vault root", Required: true},
			{Name: "old_str", Description: "String to replace (must be unique in target scope, single-edit mode)", Required: false},
			{Name: "new_str", Description: "Replacement string (can be empty to delete, single-edit mode)", Required: false},
		},
		Flags: []FlagMeta{
			{Name: "dry-run", Description: "Preview the edit without applying it", Type: FlagTypeBool},
			{Name: "edits-json", Description: "JSON object with ordered edits, e.g. '{\"edits\":[{\"old_str\":\"from\",\"new_str\":\"to\"}]}'", Type: FlagTypeJSON},
		},
		Examples: []string{
			`rvn edit "daily/2025-12-27.md" "- Churn analysis" "- [[project/churn-analysis|Churn analysis]]" --json`,
			`rvn edit "pages/notes.md" "reccommendation" "recommendation" --dry-run --json`,
			`rvn edit "project/raven#working-docs" "old link" "new link" --json`,
			`rvn edit "daily/2026-01-02.md" "- old task" "" --json`,
			`rvn edit "pages/notes.md" --edits-json '{"edits":[{"old_str":"reccommendation","new_str":"recommendation"},{"old_str":"Status: draft","new_str":"Status: active"}]}' --json`,
		},
		UseCases: []string{
			"Edit markdown vault content files (use instead of 'sed', 'awk', or direct file writes)",
			"Add wiki links to existing text",
			"Fix typos in notes",
			"Apply multiple ordered replacements in one command",
			"Add traits to existing lines",
			"Delete specific content (use --dry-run to preview first)",
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

Without --dry-run, import applies changes immediately.

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
