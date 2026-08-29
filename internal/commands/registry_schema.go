package commands

var schemaRegistry = map[string]Meta{
	"template": {
		Name:        "template",
		Description: "Manage template files under directories.template",
		LongDesc: `Manage template files under directories.template.

Template files contain Markdown body content only. Raven writes object
frontmatter separately when applying templates, so template files must not
include YAML frontmatter blocks.

Use this command group for template file lifecycle operations:
- create/update template files
- interactively author template files in your editor
- list template files
- delete template files safely`,
		Examples: []string{
			"rvn template list --json",
			"rvn template write meeting.md --content \"# {{title}}\" --json",
			"rvn template write meeting.md --edit",
			"rvn template delete meeting.md --json",
		},
		UseCases: []string{
			"Manage template-file lifecycle separately from schema template bindings",
			"Create/update template files before binding them in schema",
			"Delete template files safely with in-use checks",
		},
	},
	"template_list": {
		Name:        "template list",
		Description: "List template files",
		Examples: []string{
			"rvn template list --json",
		},
	},
	"template_write": {
		Name:        "template write",
		Description: "Create or update a template file",
		LongDesc: `Create or update a template file under directories.template.

This command replaces the full file body with --content.
For human authoring, use --edit to open the current template body in the
configured editor and save the edited result back through the same template
path validation and write flow.

Template files contain Markdown body content only. Do not include YAML
frontmatter; Raven writes object frontmatter separately when applying the
template.

Use --content for scripts and agents. Use --edit for interactive terminal
editing. The two flags are mutually exclusive.`,
		Args: []ArgMeta{
			{Name: "path", Description: "Template file path under directories.template", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "content", Description: "Template file content (full file body)", Type: FlagTypeString},
			{Name: "edit", Description: "Open the template body in the configured editor before writing", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn template write meeting.md --content \"# {{title}}\" --json",
			"rvn template write meeting.md --edit",
			"rvn template write templates/interview/technical.md --content \"## Technical interview\" --json",
		},
	},
	"template_delete": {
		Name:        "template delete",
		Description: "Delete a template file (moves to .trash)",
		LongDesc: `Delete a template file under directories.template.

By default, deletion is blocked when schema templates still reference
the file path. Use --force to bypass that check.

The file is moved to .trash/ for recovery.`,
		Args: []ArgMeta{
			{Name: "path", Description: "Template file path under directories.template", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "force", Description: "Delete even if schema templates still reference this file", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn template delete meeting.md --json",
			"rvn template delete meeting.md --force --json",
		},
	},
	"schema": {
		Name:        "schema",
		Use:         "schema [types|traits|type <name>|trait <name>|core [name]|template ...]",
		Description: "Introspect the schema",
		Args: []ArgMeta{
			{Name: "subcommand", Description: "types, traits, type, trait, core", Required: false},
			{Name: "name", Description: "Type/trait/core name (required for subcommand=type|trait|core)", Required: false},
		},
		Examples: []string{
			"rvn schema --json",
			"rvn schema types --json",
			"rvn schema type person --json",
			"rvn schema core --json",
			"rvn schema core date --json",
		},
	},
	"schema_add_type": {
		Name:        "schema add type",
		CLIPath:     []string{"schema", "add", "type"},
		Description: "Add a new type to the schema",
		LongDesc: `Add a new type definition to schema.yaml.

When creating a type, you can specify a name_field which designates which field
serves as the display name for objects of this type. The title argument to
'rvn new' will auto-populate this field.

If --default-path is omitted, Raven defaults it to "<type>/".

If the name_field doesn't exist, it will be auto-created as a required string field.
Use --description to add optional context for humans and agents.

For agents: When helping users create types, ask what field should be used as the
display name. Common choices are 'name' (for people, companies) or 'title' 
(for documents, projects).`,
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the new type", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "default-path", Description: "Default directory for files of this type (defaults to <type>/)", Type: FlagTypeString, Examples: []string{"person/", "project/"}},
			{Name: "name-field", Description: "Field to use as display name (auto-created if doesn't exist)", Type: FlagTypeString, Examples: []string{"name", "title"}},
			{Name: "description", Description: "Optional description for this type", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema add type person --name-field name --default-path person/ --json",
			"rvn schema add type person --description \"People and contacts\" --json",
			"rvn schema add type project --name-field title --default-path project/ --json",
		},
		UseCases: []string{
			"Create a new type for organizing objects",
			"Define a type with a display name field for easier object creation",
		},
	},
	"schema_add_trait": {
		Name:        "schema add trait",
		CLIPath:     []string{"schema", "add", "trait"},
		Description: "Add a new trait to the schema",
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the new trait", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "type", Description: "Trait value type: string, number, url, date, datetime, enum, bool, ref (add [] for arrays)", Type: FlagTypeString, Default: "string"},
			{Name: "values", Description: "Enum values (comma-separated)", Type: FlagTypeString},
			{Name: "default", Description: "Default trait value", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema add trait priority --type enum --values high,medium,low --json",
		},
	},
	"schema_add_field": {
		Name:        "schema add field",
		CLIPath:     []string{"schema", "add", "field"},
		Description: "Add a field to an existing type",
		LongDesc: `Add a field to an existing type definition.

Field Types:
  string      Plain text value
  string[]    Array of text values (e.g., tags)
  number      Numeric value
  number[]    Array of numeric values
  url         URL/link value (must include scheme, e.g., https://...)
  url[]       Array of URL/link values
  date        Date in YYYY-MM-DD format
  date[]      Array of dates
  datetime    Date and time
  datetime[]  Array of date/time values
  bool        Boolean (true/false)
  bool[]      Array of booleans
  enum        Single value from a list (requires --values)
  enum[]      Multiple values from a list (requires --values)
  ref         Reference to another object (requires --target)
  ref[]       Array of references (requires --target)

Common patterns:
  Single reference:     --type ref --target person
  Array of references:  --type ref[] --target person
  Tags/keywords:        --type string[]
  Status field:         --type enum --values active,paused,done

IMPORTANT - Common mistakes to avoid:
  ✗ --type array              (WRONG: 'array' is not a type)
  ✓ --type string[]           (RIGHT: use [] suffix for arrays)
  
  ✗ --type ref[]              (WRONG: missing --target)
  ✓ --type ref[] --target person  (RIGHT: ref types need --target)
  
  ✗ --type list               (WRONG: 'list' is not a type)
  ✓ --type string[]           (RIGHT: use string[] for text lists)

The command validates inputs and provides helpful suggestions if the syntax is incorrect.`,
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type to add field to", Required: true},
			{Name: "field_name", Description: "Name of the new field", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "type", Description: "Field type: string, number, url, date, datetime, bool, enum, ref (add [] for arrays)", Type: FlagTypeString, Default: "string"},
			{Name: "required", Description: "Mark field as required", Type: FlagTypeBool},
			{Name: "default", Description: "Default value for the field", Type: FlagTypeString},
			{Name: "target", Description: "Target type for ref/ref[] fields (required for references)", Type: FlagTypeString},
			{Name: "values", Description: "Allowed values for enum/enum[] fields (comma-separated)", Type: FlagTypeString},
			{Name: "description", Description: "Optional description for this field", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema add field person email --type string --required --json",
			"rvn schema add field person email --description \"Primary contact email\" --json",
			"rvn schema add field person tags --type string[] --json",
			"rvn schema add field project owner --type ref --target person --json",
			"rvn schema add field team members --type ref[] --target person --json",
			"rvn schema add field project status --type enum --values active,paused,done --json",
		},
		UseCases: []string{
			"Add a text field to a type",
			"Add an array field for tags or keywords",
			"Add a reference to link objects together",
			"Add an array of references (e.g., team members, attendees)",
			"Add an enum field with predefined choices",
		},
	},
	"schema_validate": {
		Name:        "schema validate",
		Description: "Validate the schema for correctness",
		Examples: []string{
			"rvn schema validate --json",
		},
	},
	"schema_update_type": {
		Name:        "schema update type",
		CLIPath:     []string{"schema", "update", "type"},
		Description: "Update an existing type in the schema",
		LongDesc: `Update an existing type definition in schema.yaml.

Use --name-field to set or change which field serves as the display name.
If the field doesn't exist, it will be auto-created as a required string field.
Use --description to set optional context for this type.
Use --description="-" to remove an existing description.
Use --name-field="-" to remove the name_field setting.`,
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the type to update", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "default-path", Description: "Update default directory for files", Type: FlagTypeString},
			{Name: "name-field", Description: "Set/update display name field (use '-' to remove)", Type: FlagTypeString, Examples: []string{"name", "title", "-"}},
			{Name: "description", Description: "Set/update description (use '-' to remove)", Type: FlagTypeString},
			{Name: "add-trait", Description: "Add a trait to this type", Type: FlagTypeString},
			{Name: "remove-trait", Description: "Remove a trait from this type", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema update type person --name-field name --json",
			"rvn schema update type person --default-path person/ --json",
			"rvn schema update type person --description \"People and contacts\" --json",
			"rvn schema update type meeting --add-trait due --json",
		},
	},
	"schema_update_trait": {
		Name:        "schema update trait",
		CLIPath:     []string{"schema", "update", "trait"},
		Description: "Update non-conversion metadata on an existing trait",
		LongDesc: `Update non-conversion metadata on an existing trait.

Use --default to change the default without rewriting live annotations.

--type and --values are rejected because changing either without migrating live
values can invalidate vault data. Use 'rvn schema convert trait' with the
required --map-json mapping instead; it previews by default and applies only
with --confirm.`,
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the trait to update", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "type", Description: "Rejected by update; use schema convert trait with --map-json", Type: FlagTypeString},
			{Name: "values", Description: "Rejected by update; use schema convert trait with --map-json", Type: FlagTypeString},
			{Name: "default", Description: "Update default value", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema update trait priority --default high --json",
			`rvn schema convert trait priority --map-json '{"urgent":"critical","high":"high","medium":"medium","low":"low"}' --json`,
		},
	},
	"schema_update_field": {
		Name:        "schema update field",
		CLIPath:     []string{"schema", "update", "field"},
		Description: "Update non-conversion metadata on an existing field",
		LongDesc: `Update an existing field's non-conversion properties.

Note: Making a field required will be blocked if any objects lack that field.
Add the field to all objects first, then make it required.
Use --description to set optional context for this field.
Use --description="-" to remove an existing description.

--type and --values are rejected because changing either without migrating live
values can invalidate vault data. Use 'rvn schema convert field' with the
required --map-json mapping instead; it previews by default and applies only
with --confirm.`,
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type containing the field", Required: true},
			{Name: "field_name", Description: "Field to update", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "type", Description: "Rejected by update; use schema convert field with --map-json", Type: FlagTypeString},
			{Name: "required", Description: "Update required status (true/false)", Type: FlagTypeString},
			{Name: "default", Description: "Update default value", Type: FlagTypeString},
			{Name: "values", Description: "Rejected by update; use schema convert field with --map-json", Type: FlagTypeString},
			{Name: "target", Description: "Update target type for ref fields", Type: FlagTypeString},
			{Name: "description", Description: "Set/update description (use '-' to remove)", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema update field person email --required=true --json",
			"rvn schema update field project status --default=active --json",
			"rvn schema update field person email --description \"Primary contact email\" --json",
			`rvn schema convert field project status --map-json '{"backlog":"todo","active":"active","done":"done"}' --json`,
		},
	},
	"schema_remove_type": {
		Name:        "schema remove type",
		CLIPath:     []string{"schema", "remove", "type"},
		Description: "Remove a type from the schema",
		LongDesc: `Remove a type definition from schema.yaml.

Existing files of this type will become 'page' type (fallback).
Use --force to skip confirmation prompt.`,
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the type to remove", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "force", Description: "Skip confirmation prompt", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn schema remove type event --json",
			"rvn schema remove type legacy --force --json",
		},
	},
	"schema_remove_trait": {
		Name:        "schema remove trait",
		CLIPath:     []string{"schema", "remove", "trait"},
		Description: "Remove a trait from the schema",
		LongDesc: `Remove a trait definition from schema.yaml.

Existing @trait instances will remain in files but no longer be indexed.
Use --force to skip confirmation prompt.`,
		Args: []ArgMeta{
			{Name: "name", Description: "Name of the trait to remove", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "force", Description: "Skip confirmation prompt", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn schema remove trait priority --json",
		},
	},
	"schema_remove_field": {
		Name:        "schema remove field",
		CLIPath:     []string{"schema", "remove", "field"},
		Description: "Remove a field from a type",
		LongDesc: `Remove a field from a type definition.

If the field is required, removal will be blocked until you make it optional first.
Existing field values will remain in files but no longer be validated.`,
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type containing the field", Required: true},
			{Name: "field_name", Description: "Field to remove", Required: true},
		},
		Examples: []string{
			"rvn schema remove field person nickname --json",
		},
	},
	"schema_convert_trait": {
		Name:        "schema convert trait",
		CLIPath:     []string{"schema", "convert", "trait"},
		Description: "Convert a trait's type or values and migrate every annotation",
		LongDesc: `Convert a trait's values, optionally changing its type, and migrate schema.yaml plus all matching annotations.

This is the only supported command for changing an existing trait's type or
allowed values; schema update trait rejects --type and --values.

--map-json must be a JSON object whose keys are old values and whose values use
the target type's JSON representation. The map must cover every finite
schema-allowed value (all enum members or true/false for bool), the current
default, and every value observed in live @trait annotations. The command fails
without writing anything when any value is missing.

For collection types, array-to-array conversions map each member independently.
Scalar-to-array conversions map each scalar to an explicit JSON array.
Collection-to-scalar conversion is rejected because it has no unambiguous
reduction rule.

Omit --type for a same-type remap. Supply --type to change the type.
Returns a preview by default; changes are not applied unless confirm=true.

After applying, run 'rvn reindex --full --json' and 'rvn check --json'.`,
		Args: []ArgMeta{
			{Name: "name", Description: "Trait to convert", Required: true, DynamicComp: "traits"},
		},
		Flags: []FlagMeta{
			{Name: "type", Description: "Target trait type; omit for a same-type value remap", Type: FlagTypeString},
			{Name: "map-json", Description: "Exhaustive JSON object mapping old values to target-type values", Type: FlagTypeJSON, Required: true},
			{Name: "confirm", Description: "Apply the conversion (default: preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			`rvn schema convert trait priority --type bool --map-json '{"high":true,"medium":true,"low":false}' --json`,
			`rvn schema convert trait priority --map-json '{"urgent":"critical","high":"high","medium":"medium","low":"low"}' --json`,
			`rvn schema convert trait tags --map-json '{"old":"new","keep":"keep"}' --confirm --json`,
		},
		UseCases: []string{
			"Convert an enum trait to a boolean while keeping annotations valid",
			"Rename enum members across schema defaults and live annotations",
			"Remap every member of an array-valued trait",
		},
	},
	"schema_convert_field": {
		Name:        "schema convert field",
		CLIPath:     []string{"schema", "convert", "field"},
		Description: "Convert a field's type or values and migrate every object",
		LongDesc: `Convert a field's values, optionally changing its type, and migrate schema.yaml plus matching object frontmatter.

This is the only supported command for changing an existing field's type or
allowed values; schema update field rejects --type and --values.

--map-json must be a JSON object whose keys are old values and whose values use
the target type's JSON representation. The map must cover every finite
schema-allowed value (all enum members or true/false for bool), the current
default, and every value observed in live frontmatter for the selected type.
The command fails without writing anything when any value is missing.

For collection types, array-to-array conversions map each member independently.
Scalar-to-array conversions map each scalar to an explicit JSON array.
Collection-to-scalar conversion is rejected because it has no unambiguous
reduction rule.

Existing ref/ref[] fields can convert between scalar and collection forms while
preserving their target. Conversion from a non-reference field to ref/ref[] is
rejected because this command does not infer a reference target.

Omit --type for a same-type remap. Supply --type to change the type.
Returns a preview by default; changes are not applied unless confirm=true.

After applying, run 'rvn reindex --full --json' and 'rvn check --json'.`,
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type containing the field", Required: true, DynamicComp: "types"},
			{Name: "field_name", Description: "Field to convert", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "type", Description: "Target field type; omit for a same-type value remap", Type: FlagTypeString},
			{Name: "map-json", Description: "Exhaustive JSON object mapping old values to target-type values", Type: FlagTypeJSON, Required: true},
			{Name: "confirm", Description: "Apply the conversion (default: preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			`rvn schema convert field project status --type enum --map-json '{"true":"done","false":"todo"}' --json`,
			`rvn schema convert field project status --map-json '{"backlog":"todo","active":"active","done":"done"}' --json`,
			`rvn schema convert field project labels --map-json '{"old":"new","keep":"keep"}' --confirm --json`,
		},
		UseCases: []string{
			"Convert a boolean field to an enum while keeping frontmatter valid",
			"Rename enum members across schema defaults and live objects",
			"Remap every member of an array-valued field",
		},
	},
	"schema_rename_type": {
		Name:        "schema rename type",
		CLIPath:     []string{"schema", "rename", "type"},
		Description: "Rename a type and update all references",
		LongDesc: `Rename a type in schema.yaml and update all files that use it.

This command:
1. Renames the type in schema.yaml
2. Updates all 'type:' frontmatter fields in files
3. Updates all ref field targets pointing to the old type
4. Optionally updates the type description (--description)
5. Optionally renames default_path directory and moves matching files with reference updates (--rename-default-path)

For agents: After renaming, run 'rvn reindex --full --json' to update the index.`,
		Args: []ArgMeta{
			{Name: "old_name", Description: "Current type name", Required: true},
			{Name: "new_name", Description: "New type name", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "confirm", Description: "Apply the rename (default: preview only)", Type: FlagTypeBool},
			{Name: "description", Description: "Set the renamed type description (use '-', 'none', or '\"\"' to clear)", Type: FlagTypeString},
			{Name: "rename-default-path", Description: "Also rename type default_path directory and move matching files (with reference updates)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn schema rename type event meeting --json",
			"rvn schema rename type event meeting --description 'Meetings and calls' --json",
			"rvn schema rename type event meeting --confirm --json",
			"rvn schema rename type event meeting --confirm --rename-default-path --json",
		},
		UseCases: []string{
			"Rename a type while keeping all files valid",
			"Refactor schema type names",
			"Migrate from old naming conventions",
		},
	},
	"schema_rename_field": {
		Name:        "schema rename field",
		CLIPath:     []string{"schema", "rename", "field"},
		Description: "Rename a field on a type and update all downstream uses",
		LongDesc: `Rename a field on a specific type and update all downstream places that use that field.

This command:
1. Renames types.<type>.fields.<old_field> -> <new_field> in schema.yaml
2. If name_field == <old_field>, updates it to <new_field>
3. Updates type templates that reference {{field.<old_field>}} (template files)
4. Renames frontmatter keys in files whose type matches the target type
5. Updates saved queries in raven.yaml that parse as type:<type> (best-effort)

For agents: After renaming, run 'rvn reindex --full --json' to update the index.`,
		Args: []ArgMeta{
			{Name: "type_name", Description: "Type containing the field", Required: true, DynamicComp: "types"},
			{Name: "old_field", Description: "Current field name", Required: true},
			{Name: "new_field", Description: "New field name", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "confirm", Description: "Apply the rename (default: preview only)", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn schema rename field person email email_address --json",
			"rvn schema rename field person email email_address --confirm --json",
		},
		UseCases: []string{
			"Rename a field on a type safely with preview/confirm",
			"Refactor schema field names while keeping files consistent",
			"Update frontmatter keys and saved queries after field rename",
		},
	},
	"schema_template_list": {
		Name:        "schema template list",
		CLIPath:     []string{"schema", "template", "list"},
		Description: "List schema templates or target bindings",
		LongDesc: `List schema template definitions, or list template bindings for one target.

Without --type/--core, lists all schema templates defined in the top-level templates block.
With --type or --core, lists the bound template IDs and default template for that target.`,
		Flags: []FlagMeta{
			{Name: "type", Description: "List bindings for this schema type", Type: FlagTypeString},
			{Name: "core", Description: "List bindings for this core type (date or page)", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema template list --json",
			"rvn schema template list --type interview --json",
			"rvn schema template list --core date --json",
		},
	},
	"schema_template_get": {
		Name:        "schema template get",
		CLIPath:     []string{"schema", "template", "get"},
		Description: "Show a schema template definition",
		Args: []ArgMeta{
			{Name: "template_id", Description: "Schema template ID", Required: true},
		},
		Examples: []string{
			"rvn schema template get interview_technical --json",
		},
	},
	"schema_template_set": {
		Name:        "schema template set",
		CLIPath:     []string{"schema", "template", "set"},
		Description: "Create or update a schema template definition",
		LongDesc: `Create or update a schema template definition.

The referenced template file must live under directories.template and contain
Markdown body content only. Template files cannot include YAML frontmatter;
Raven writes object frontmatter separately when applying templates.`,
		Args: []ArgMeta{
			{Name: "template_id", Description: "Schema template ID", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "file", Description: "Template file path under directories.template", Type: FlagTypeString, Examples: []string{"templates/interview/technical.md"}},
			{Name: "description", Description: "Template description (use '-' to clear)", Type: FlagTypeString},
		},
		Examples: []string{
			"rvn schema template set interview_technical --file templates/interview/technical.md --json",
			"rvn schema template set interview_technical --file templates/interview/technical.md --description \"Technical interview prompt\" --json",
		},
	},
	"schema_template_remove": {
		Name:        "schema template remove",
		CLIPath:     []string{"schema", "template", "remove"},
		Description: "Remove a schema template definition",
		Args: []ArgMeta{
			{Name: "template_id", Description: "Schema template ID", Required: true},
		},
		Examples: []string{
			"rvn schema template remove interview_technical --json",
		},
	},
	"schema_template_bind": {
		Name:        "schema template bind",
		CLIPath:     []string{"schema", "template", "bind"},
		Description: "Bind a schema template ID to a type or core type, optionally as its default",
		LongDesc: `Bind a schema template ID to a type or core type.

Use --default to also make the template the target's default. This also changes
the default when the template is already bound.`,
		Args: []ArgMeta{
			{Name: "template_id", Description: "Schema template ID", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "type", Description: "Target schema type", Type: FlagTypeString},
			{Name: "core", Description: "Target core type (date or page)", Type: FlagTypeString},
			{Name: "default", Description: "Also set this template as the target default", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn schema template bind interview_technical --type interview --json",
			"rvn schema template bind interview_technical --type interview --default --json",
			"rvn schema template bind daily_default --core date --default --json",
		},
	},
	"schema_template_unbind": {
		Name:        "schema template unbind",
		CLIPath:     []string{"schema", "template", "unbind"},
		Description: "Unbind a schema template ID from a type or core type, clearing its default if requested",
		LongDesc: `Unbind a schema template ID from a type or core type.

Unbinding the target's current default is blocked unless --clear-default is
provided, which clears the default before removing the binding.`,
		Args: []ArgMeta{
			{Name: "template_id", Description: "Schema template ID", Required: true},
		},
		Flags: []FlagMeta{
			{Name: "type", Description: "Target schema type", Type: FlagTypeString},
			{Name: "core", Description: "Target core type (date or page)", Type: FlagTypeString},
			{Name: "clear-default", Description: "Allow unbinding the current default by clearing the default first", Type: FlagTypeBool},
		},
		Examples: []string{
			"rvn schema template unbind interview_technical --type interview --json",
			"rvn schema template unbind interview_technical --type interview --clear-default --json",
			"rvn schema template unbind daily_default --core date --clear-default --json",
		},
	},
}
