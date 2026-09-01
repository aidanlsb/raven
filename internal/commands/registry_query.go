package commands

var queryRegistry = map[string]Meta{
	"query": {
		Name:        "query",
		Use:         "query <query_string|saved-query> [inputs...]",
		Description: "Run a query using the Raven query language",
		LongDesc: `Query items, sections, traits, or outgoing link edges using the Raven query language.

Prefer query when the structure is known and you need real Raven items or
trait instances, or when you need indexed file/URL link metadata. Use search for broad
text discovery when you do not yet know the right type, trait, or structural
context. Search returns file/snippet matches; query returns schema-aware item
rows, section rows, real trait rows, or outgoing link-edge rows.

Choose the right retrieval tool:
- query — structured filtering by type/section/trait/link, field values, scope,
  and references. Returns real items you can filter and bulk-apply to.
- search — free-text discovery when you do not know the structure yet. Returns
  file/snippet matches only. search "@todo" finds the text "@todo", NOT real
  todo traits; use query 'trait:todo' for real traits.
- backlinks <reference> — all incoming references to one specific object or section.
  (Equivalent query: refd(...); read also appends backlinks.)
- outlinks <reference> — all outgoing references from one specific object.
- read <reference> — full file content after you already identified the object.
- resolve <reference> — turn an accepted reference input into its canonical target ID
  without reading content; use that ID when authoring references.
- date <date> / daily <date> — everything for one date. In queries, use
  type:date .date==<date> for daily-note objects, or trait:due .value==<date>
  for due items on that date.

Query syntax:
- Type queries: type:<type> [predicates...]
  Examples: type:project .status==active, type:meeting refs([[people/freya]])
- Section queries: section [predicates...]
  Examples: section .title==Tasks, section within(type:project),
  section refs([[people/freya]])
  The section root is always bare; type:section is invalid.
- Trait queries: trait:<name> [predicates...]
  Examples: trait:due .value<today, trait:highlight in(type:book)
- Link queries: link [predicates...]
  Examples: link .ext==pdf within(type:project), link .is_image==true

Common predicates:
- .field==value — Filter by field (.status==active, .priority==high)
- in(type:...|section...) — Direct containing scope matches subquery
- within(type:...|section...) — Any containing scope matches subquery
- has(section...|trait:...) — Directly contains section or trait matching subquery
- contains(section...|trait:...) — Recursively contains section or trait matching subquery
- refs([[target]]) — References target (refs([[people/freya]]))
- refs(type:...) — References items matching subquery (refs(type:project .status==active))
- links(<link-predicate>) — Contains an outgoing non-Raven file or URL link
  matching the shared link fields (links(.ext==pdf)). Unlike refs()/refd(),
  links() is outgoing-only: external files and URLs are leaves, so there is no
  linkd() inverse.
- refs()/links() scope follows the source root: type queries inspect the whole
  file, section queries inspect the complete section subtree, and trait queries
  inspect only the trait's source line.
- Prefer canonical object IDs in direct reference targets; bare short forms are
  resolution sugar and can become ambiguous.
- .value==X — Trait value equals X (.value==today, .value==high)
- oneof(.field, [a,b]) — Field/value is one of a set (this is set membership, NOT
  the scope predicate in(...); see below)
- content("text") — Full-text search within content (content("meeting notes"))

Link rows are outgoing Markdown links/images to non-Raven targets. They expose
.source_id, .source_type, .file_path, .line, .position_start, .position_end,
.raw_target, .display, .is_image, .scheme, .ext, and .normalized_key.
The link root and links(...) share this complete field vocabulary, including
numeric comparisons for line and positions. Equality on .normalized_key and
.raw_target is case-sensitive; equality on other string link fields is
case-insensitive. Every link field is present, so exists()/!exists() are
invalid; use an empty-value comparison such as .ext=="" when appropriate.
Use within(type:...) or within(section ...) to filter where a link occurs. Link
queries do not support links(), refs(), refd(), content(), in(), or arrays. To
find trait lines with matching links, use trait:<name> links(...), not
link within(trait:<name>).

Scope predicates are root-dependent, and traits attach to the nearest section:
- Prefer the forgiving forms contains(...) and within(...) unless you specifically
  want a direct-only match. A @todo under a "## Tasks" heading is NOT directly on
  the project object, so type:project has(trait:todo) usually returns nothing;
  use type:project contains(trait:todo .value==todo) instead. From the trait side,
  use trait:todo within(type:project ...), not in(type:project ...).
- Downward (container -> contents), on type:/section: has() = direct child,
  contains() = anywhere in the section tree.
- Upward (contents -> container), on trait:/section: in() = direct parent scope,
  within() = any ancestor scope.
- Naming collision: in(...) is a SCOPE predicate (containment). Set membership is
  oneof(.field, [...]). They are unrelated despite the "in" wording.

Common agent patterns:
- Real open todos: trait:todo .value==todo
- Open todos in briefs: trait:todo .value==todo within(type:brief)
- Distinguish real traits from plain-text mentions: use trait:todo ... instead of search "@todo"
- Open todos under a section/topic heading: trait:todo .value==todo within(section includes(.title, "pricing"))
- Open todos in a daily-note range: trait:todo .value==todo within(type:date .date>=2026-05-01 .date<=2026-05-31)
- Projects that contain open todos anywhere: type:project contains(trait:todo .value==todo)
- Path + structure together: type:page matches(.path, "^pages/work/") contains(trait:todo .value==todo)

Special date values for trait, type:date .date, and date-target ref field comparisons:
- today, tomorrow, yesterday
Datetime literals compare as datetimes, for example .value>="2026-03-01T09:30".

Saved query inputs must be declared in the saved query definition when using {{args.<name>}}.
You can then pass inputs by position (in args order) or as key=value pairs.
Saved query definitions contain only RQL, declared args, and a description.
Pass runtime flags such as --refresh, --limit, --apply, and --confirm on each
query invocation; they are never persisted with the saved query.

Use --ids to output just IDs (one per line) for piping to other commands.
For link queries, --ids projects each matching edge's source_id.
Use --limit/--offset for paginated result windows.
Use --count-only to return only the total match count without items.

Results are unlimited by default (--limit 0). To batch a large result set
without pulling everything at once:
1. Probe the size with --count-only to get the total.
2. Page with --limit and --offset, looping while the JSON has_more field is
   true and starting each next request at the returned next_offset.
The JSON envelope includes total, returned, offset, limit, and has_more (plus
next_offset when has_more is true) so you can loop without guessing.
Use --browse to open an interactive Raven picker with filtering and editor
handoff for the selected result.
Use --no-links to disable clickable terminal hyperlinks in human-readable
results.
Use --apply to run a bulk operation directly on query results.
Do not combine --apply with --limit, --offset, or --count-only.
Section queries return stable IDs, but file-level bulk move rejects section
sources; use rvn section move for each intended section instead of --apply.
Link queries return edge rows and do not support --apply.
For sections, pipe IDs to add instead: query "section ..." --ids | rvn add <text> --stdin.

For type queries (type:...):
- Returns preview by default. Changes are NOT applied unless confirm=true.
- Supported commands: set, delete, add, move

For trait queries (trait:...):
- Returns preview by default. Changes are NOT applied unless confirm=true.
- Supported command: update <new_value> (updates trait values in-place)
- Example: trait:todo .value==todo --apply "update done" marks todos as done`,
		Args: []ArgMeta{
			{Name: "query_string", Description: "Query string (e.g., 'type:project .status==active', 'link .ext==pdf', or saved query name) optionally followed by saved-query inputs.", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "refresh", Description: "Refresh stale and journaled pending files before query", Type: FlagTypeBool},
			{Name: "ids", Description: "Output only result IDs; link queries output source IDs, one per edge", Type: FlagTypeBool},
			{Name: "limit", Description: "Maximum number of query results to return (0 means no limit)", Type: FlagTypeInt},
			{Name: "offset", Description: "Zero-based offset for query results", Type: FlagTypeInt},
			{Name: "count-only", Description: "Return only the total count of matches (no items or IDs)", Type: FlagTypeBool},
			{Name: "apply", Description: "Apply bulk operation to results (e.g., 'set status=done', 'delete', 'add @reviewed', 'update done')", Type: FlagTypeStringSlice},
			{Name: "confirm", Description: "Apply bulk changes (without this flag, shows preview only)", Type: FlagTypeBool},
			{Name: "pipe", Description: "Force pipe-friendly output for shell pipelines (jq, head, sort)", Type: FlagTypeBool},
			{Name: "no-pipe", Description: "Force human-readable output format", Type: FlagTypeBool},
			{Name: "browse", Description: "Interactively browse results in Raven's picker and open the selected result in the configured editor", Type: FlagTypeBool},
			{Name: "no-links", Description: "Disable clickable hyperlinks in terminal output", Type: FlagTypeBool},
			{Name: "inputs", Description: "Saved query inputs as key=value pairs", Type: FlagTypePosKeyValue, Examples: []string{`{"project": "projects/raven"}`}},
		},
		Examples: []string{
			"rvn query 'type:project .status==active' --json",
			"rvn query 'type:project contains(trait:todo .value==todo)' --json",
			"rvn query 'type:meeting has(trait:due)' --json",
			"rvn query 'type:brief .date==today' --json",
			"rvn query 'trait:due .value<today' --json",
			"rvn query 'link .ext==pdf within(type:project)' --json",
			"rvn query 'link .is_image==true' --json",
			"rvn query 'trait:due .value<today' --ids",
			"rvn query 'trait:todo .value==todo' --limit 50 --offset 100 --json",
			"rvn query 'trait:todo .value==todo' --count-only --json",
			"rvn query 'type:issue .status==open' --browse",
			"rvn query 'type:project .status==active' --apply 'set status=done' --confirm --json",
			"rvn query 'trait:todo .value==todo' --apply 'update done' --confirm --json",
			"rvn query tasks --json",
			"rvn query tasks --limit 100 --json",
			"rvn query project-todos raven --json",
			"rvn query project-todos project=projects/raven --json",
			"rvn query open-projects --apply 'set status=done' --confirm --json",
		},
		UseCases: []string{
			"Find items matching specific criteria",
			"Find traits with specific values",
			"Find outgoing file and URL links by target metadata and source scope",
			"Probe result size with --count-only before reading",
			"Page through large result sets with --limit/--offset while has_more is true",
			"Bulk update query results with --apply",
			"Pipe query results to other commands with --ids",
		},
	},
	"query_saved_list": {
		Name:        "query saved list",
		Description: "List saved queries",
		Examples: []string{
			"rvn query saved list --json",
		},
	},
	"query_saved_get": {
		Name:        "query saved get",
		Description: "Show one saved query definition",
		Args: []ArgMeta{
			{Name: "name", Description: "Saved query name", Required: true, DynamicComp: "queries"},
		},
		Examples: []string{
			"rvn query saved get overdue --json",
		},
	},
	"query_saved_set": {
		Name:        "query saved set",
		Description: "Create or replace a saved query in raven.yaml",
		LongDesc: `Create or replace a named RQL definition in raven.yaml.

Saved queries persist only the query string, declared args, and description.
Runtime execution flags belong on "rvn query <saved-query>" and cannot be saved
as part of the definition.`,
		Args: []ArgMeta{
			{Name: "name", Description: "Name for the saved query", Required: true},
			{Name: "query_string", Description: "Query string (e.g., 'type:project .status==active' or 'trait:due .value<today')", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "description", Description: "Human-readable description", Type: FlagTypeString},
			{Name: "arg", Description: "Declare saved query input name (repeatable, sets positional order)", Type: FlagTypeStringSlice},
		},
		Examples: []string{
			"rvn query saved set tasks 'trait:due' --json",
			"rvn query saved set overdue 'trait:due .value<today' --json",
			"rvn query saved set active-projects 'type:project .status==active' --json",
			"rvn query saved set project-todos 'trait:todo refs([[{{args.project}}]])' --arg project --json",
		},
	},
	"query_saved_remove": {
		Name:        "query saved remove",
		Description: "Remove a saved query from raven.yaml",
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the query to remove", Required: true, DynamicComp: "queries"},
		},
		Examples: []string{
			"rvn query saved remove overdue --json",
		},
	},
	"backlinks": {
		Name:        "backlinks",
		Use:         "backlinks [reference]",
		Description: "Find objects that reference a target object or section",
		LongDesc: `Find objects that reference a target object or section.

In an interactive terminal, bare 'rvn backlinks' launches Raven's picker
over indexed object and section references.
When an interactive backlinks reference is ambiguous, Raven prompts you to choose the reference.
Use --browse to browse incoming references interactively and open the selected reference location.
Use --stdin to read references from stdin and return grouped results for each reference.
Non-interactive use requires either a reference or --stdin input.` + barePickerInsertModeHelp,
		Args: []ArgMeta{
			{Name: "reference", Description: "Object or section reference (e.g., people/freya, projects/raven#status)", Required: false, CLIOptional: true},
		},
		Flags: []FlagMeta{
			{Name: "browse", Description: "Interactively browse backlinks in Raven's picker and open the selected reference", Type: FlagTypeBool},
			{Name: "stdin", Description: "Read references from stdin and return grouped backlinks", Type: FlagTypeBool},
		},
		BulkStdinArgName: "references",
		Examples: []string{
			"rvn backlinks people/freya --json",
			"rvn backlinks people/freya --browse",
			"rvn query 'type:project .status==active' --ids | rvn backlinks --stdin --json",
		},
		UseCases: []string{
			"Find all files that reference an object or section",
			"Interactively pick a reference in Raven's picker",
			"Interactively disambiguate backlinks references in Raven's picker",
			"Browse incoming references and open one at the reference line",
			"Traverse backlinks for multiple references with grouped output",
			"Audit incoming links before moving or deleting content",
		},
	},
	"outlinks": {
		Name:        "outlinks",
		Use:         "outlinks [reference]",
		Description: "Find Raven references made by an object",
		LongDesc: `Find object and section references made by an object.

In an interactive terminal, bare 'rvn outlinks' launches Raven's picker
over indexed object and section references.
When an interactive outlinks reference is ambiguous, Raven prompts you to choose the reference.
Use --browse to browse outgoing references interactively and open the selected reference location.
Use --stdin to read references from stdin and return grouped results for each reference.
Non-interactive use requires either a reference or --stdin input.` + barePickerInsertModeHelp,
		Args: []ArgMeta{
			{Name: "reference", Description: "Object reference (e.g., projects/bifrost)", Required: false, CLIOptional: true},
		},
		Flags: []FlagMeta{
			{Name: "browse", Description: "Interactively browse outlinks in Raven's picker and open the selected reference", Type: FlagTypeBool},
			{Name: "stdin", Description: "Read references from stdin and return grouped outlinks", Type: FlagTypeBool},
		},
		BulkStdinArgName: "references",
		Examples: []string{
			"rvn outlinks projects/bifrost --json",
			"rvn outlinks projects/bifrost --browse",
			"rvn query 'type:project .status==active' --ids | rvn outlinks --stdin --json",
		},
		UseCases: []string{
			"Inspect the outgoing links from an object",
			"Interactively pick a reference in Raven's picker",
			"Interactively disambiguate outlinks references in Raven's picker",
			"Browse outgoing references and open one at the reference line",
			"Traverse outlinks for multiple references with grouped output",
			"Follow references from a file to related objects and sections",
		},
	},
	"search": {
		Name:         "search",
		Use:          "search [query]",
		Description:  "Full-text search across all vault content",
		VariadicJoin: true,
		LongDesc: `Search for content across all files in the vault.

Use search for open-ended text discovery when you do NOT yet know the type,
trait, or structure. For structured retrieval of real items, traits, or links,
prefer 'rvn query' (see 'rvn query --help' for the full retrieval decision tree).

Important: search matches raw text, not Raven structure. search "@todo" finds the
literal text "@todo" and cannot distinguish a real @todo trait from prose that
merely mentions it. To find real todo traits, use: rvn query 'trait:todo'. The
same applies to any @trait token — reach for query 'trait:<name>' once you know
the structure.

When to use which:
- search — you only have a text fragment ("find files mentioning pricing").
- query ... content("term") — text match scoped to a type/section/trait root.
- backlinks <reference> / query ... refd(...) — what references a specific object or section.
- resolve <reference> — map an accepted reference input to its canonical object ID.

Uses full-text search with relevance ranking. Supports:
  - Simple words: "meeting notes" (finds pages containing both words)
  - Phrases: '"team meeting"' (exact phrase match)
  - Prefix matching: "meet*" (matches meeting, meetings, etc.)
  - Boolean: "meeting AND notes", "meeting OR notes", "meeting NOT private"

Results are ranked by relevance with snippets showing matched content.
Use --type to filter results to specific object types.

In an interactive terminal, bare 'rvn search' launches Raven's picker over
indexed files. Non-interactive use still requires a query string.` + barePickerInsertModeHelp,
		Args: []ArgMeta{
			{Name: "query", Description: "Search query (words, phrases, or boolean expressions)", Required: true, CLIOptional: true, Variadic: true},
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
			"Interactively pick an indexed file in Raven's picker",
			"Search for content across the entire vault",
			"Locate pages by partial matches",
			"Find all mentions of a person or concept",
		},
	},
	"resolve": {
		Name:        "resolve",
		Use:         "resolve [reference]",
		Description: "Resolve a reference to its target object",
		LongDesc: `Resolve any accepted reference input and return information about
the canonical target object.

This is a pure query — it does not modify anything. The result always returns
"resolved": true/false to indicate whether the reference was successfully resolved.

In an interactive terminal, bare 'rvn resolve' launches Raven's picker
over indexed object and section references.
Non-interactive use still requires a reference.

Supports all reference formats:
- Canonical object IDs: "people/freya"
- File paths: "people/freya.md"
- Aliases: "The Queen" → people/freya
- Name field values: "The Prose Edda" → books/the-prose-edda
- Date references: "2025-02-01" → 2025-02-01
- Dynamic dates: "today", "yesterday", "tomorrow"
- Section references: "projects/website#tasks"
- Unambiguous short names: "freya" → people/freya

If the reference is ambiguous (matches multiple objects), returns all matches
with their match sources. Use the returned canonical ID when authoring
references. Short forms are supported as resolution sugar, not as the preferred
authoring form.` + barePickerInsertModeHelp,
		Args: []ArgMeta{
			{Name: "reference", Description: "Canonical object ID or other accepted reference input to resolve", Required: true, CLIOptional: true},
		},
		Examples: []string{
			"rvn resolve people/freya --json",
			"rvn resolve today --json",
			"rvn resolve \"The Prose Edda\" --json",
			"rvn resolve freya --json",
		},
		UseCases: []string{
			"Check if a reference resolves before using it",
			"Interactively pick a reference in Raven's picker",
			"Normalize an accepted reference input to its canonical object ID",
			"Inspect how an alias or legacy short form resolves",
			"Disambiguate references that might match multiple objects",
			"Validate references without side effects",
		},
	},
}
