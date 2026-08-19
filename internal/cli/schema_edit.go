package cli

import (
	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
)

const schemaAddLong = `Add new definitions to schema.yaml.

Subcommands:
  type <name>              Add a new type
  trait <name>             Add a new trait
  field <type> <field>     Add a field to an existing type

Examples:
  rvn schema add type event --default-path event/
  rvn schema add trait priority --type enum --values high,medium,low
  rvn schema add field person email --type string --required`

// schemaAddCmd is the "schema add" subtree, generated from registry metadata
// (CLIPath) via buildRegistrySubtree. Leaf render/build hooks are wired through
// the spec below; adding a new schema_add_* entry needs no new Cobra vars.
var schemaAddCmd = buildRegistrySubtree(registrySubtreeSpec{
	Prefix:    []string{"schema", "add"},
	VaultPath: getVaultPath,
	Root: registryGroup{
		Use:        "add",
		Short:      "Add a type, trait, or field to the schema",
		Long:       schemaAddLong,
		ParentOnly: true,
	},
	Renders: map[string]func(*cobra.Command, commandexec.Result) error{
		"schema_add_type":  renderSchemaAddType,
		"schema_add_trait": renderSchemaAddTrait,
		"schema_add_field": renderSchemaAddField,
	},
	Leaves: map[string]canonicalLeafOptions{
		"schema_add_type": {BuildArgs: buildSchemaAddTypeArgs},
	},
})

var schemaValidateCmd = newCanonicalLeafCommand("schema_validate", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderSchemaValidate,
})

// =============================================================================
// UPDATE COMMANDS
// =============================================================================

const schemaUpdateLong = `Update existing definitions in schema.yaml.

Subcommands:
  type <name>              Update an existing type
  trait <name>             Update non-conversion trait metadata
  field <type> <field>     Update non-conversion field metadata

Update does not change an existing field or trait's type or allowed values.
The --type and --values flags are rejected for those subcommands. Use schema
convert field|trait with the required --map-json mapping for type or value
remaps; conversion previews by default and requires --confirm to apply.

Examples:
  rvn schema update type person --default-path person/
  rvn schema update trait priority --default high
  rvn schema update field person email --required=true
  rvn schema update type meeting --add-trait due`

// schemaUpdateCmd is the "schema update" subtree, generated from registry
// metadata via buildRegistrySubtree.
var schemaUpdateCmd = buildRegistrySubtree(registrySubtreeSpec{
	Prefix:    []string{"schema", "update"},
	VaultPath: getVaultPath,
	Root: registryGroup{
		Use:        "update",
		Short:      "Update a type, trait, or field in the schema",
		Long:       schemaUpdateLong,
		ParentOnly: true,
	},
	Renders: map[string]func(*cobra.Command, commandexec.Result) error{
		"schema_update_type":  renderSchemaUpdateType,
		"schema_update_trait": renderSchemaUpdateTrait,
		"schema_update_field": renderSchemaUpdateField,
	},
})

// =============================================================================
// REMOVE COMMANDS
// =============================================================================

const schemaRemoveLong = `Remove definitions from schema.yaml.

Subcommands:
  type <name>              Remove a type (objects become 'page' type)
  trait <name>             Remove a trait (existing instances remain in files)
  field <type> <field>     Remove a field from a type

By default, warns about affected files. Use --force to skip warnings.

Examples:
  rvn schema remove type event
  rvn schema remove trait priority --force
  rvn schema remove field person nickname`

// schemaRemoveCmd is the "schema remove" subtree, generated from registry
// metadata via buildRegistrySubtree. The type/trait leaves keep their
// interactive confirm flows via per-leaf Invoke hooks.
var schemaRemoveCmd = buildRegistrySubtree(registrySubtreeSpec{
	Prefix:    []string{"schema", "remove"},
	VaultPath: getVaultPath,
	Root: registryGroup{
		Use:        "remove",
		Short:      "Remove a type, trait, or field from the schema",
		Long:       schemaRemoveLong,
		ParentOnly: true,
	},
	Renders: map[string]func(*cobra.Command, commandexec.Result) error{
		"schema_remove_type":  renderSchemaRemoveType,
		"schema_remove_trait": renderSchemaRemoveTrait,
		"schema_remove_field": renderSchemaRemoveField,
	},
	Leaves: map[string]canonicalLeafOptions{
		"schema_remove_type":  {Invoke: invokeSchemaRemoveType},
		"schema_remove_trait": {Invoke: invokeSchemaRemoveTrait},
	},
})

// =============================================================================
// CONVERT COMMANDS
// =============================================================================

const schemaConvertLong = `Convert trait or field values and migrate schema.yaml plus matching vault data.

Subcommands:
  trait <name>
  field <type> <field>

This is the only supported path for changing an existing field or trait's type
or allowed values; schema update field|trait rejects --type and --values.

--map-json is required and must exhaustively cover enum/bool allowed values,
the current default, and every observed live value. Conversion is preview-only
unless --confirm is supplied. Array-to-array conversion maps each member;
scalar-to-array mappings use explicit JSON arrays.

Examples:
  rvn schema convert trait priority --type bool --map-json '{"high":true,"medium":true,"low":false}'
  rvn schema convert field project status --type enum --map-json '{"true":"done","false":"todo"}'
  rvn schema convert trait priority --map-json '{"urgent":"critical","high":"high","medium":"medium","low":"low"}'`

var schemaConvertCmd = buildRegistrySubtree(registrySubtreeSpec{
	Prefix:    []string{"schema", "convert"},
	VaultPath: getVaultPath,
	Root: registryGroup{
		Use:        "convert",
		Short:      "Convert schema values and migrate vault data",
		Long:       schemaConvertLong,
		ParentOnly: true,
	},
	Renders: map[string]func(*cobra.Command, commandexec.Result) error{
		"schema_convert_trait": renderSchemaConvert,
		"schema_convert_field": renderSchemaConvert,
	},
})

// =============================================================================
// RENAME COMMANDS
// =============================================================================

const schemaRenameLong = `Rename a type or a field in the schema and update downstream usages.

Subcommands:
  type  <old_name> <new_name>
  field <type> <old_field> <new_field>

Rename type updates:
1. Type definition key in schema.yaml
2. All 'type:' frontmatter fields
3. All ref field targets pointing to the old type

Rename field updates:
1. Field key in schema.yaml for the target type
2. If name_field == old_field, updates it to new_field
3. Type templates referencing {{field.old_field}} (template files)
4. Object frontmatter keys for files with type:<type>
5. Saved queries in raven.yaml (best-effort for type:<type> queries)

By default, previews changes. Use --confirm to apply.
When type default_path clearly matches the type name, you can also rename
that directory with --rename-default-path.

Examples:
  rvn schema rename type event meeting
  rvn schema rename type event meeting --confirm

  rvn schema rename field person email email_address
  rvn schema rename field person email email_address --confirm`

// schemaRenameCmd is the "schema rename" subtree, generated from registry
// metadata via buildRegistrySubtree. The type leaf keeps its interactive
// default-path confirm flow via a per-leaf Invoke hook.
var schemaRenameCmd = buildRegistrySubtree(registrySubtreeSpec{
	Prefix:    []string{"schema", "rename"},
	VaultPath: getVaultPath,
	Root: registryGroup{
		Use:        "rename",
		Short:      "Rename a type or field and update references",
		Long:       schemaRenameLong,
		ParentOnly: true,
	},
	Renders: map[string]func(*cobra.Command, commandexec.Result) error{
		"schema_rename_type":  renderSchemaRenameType,
		"schema_rename_field": renderSchemaRenameField,
	},
	Leaves: map[string]canonicalLeafOptions{
		"schema_rename_type": {Invoke: invokeSchemaRenameType},
	},
})

func init() {
	// schemaAddCmd/schemaUpdateCmd/schemaRemoveCmd/schemaConvertCmd/schemaRenameCmd are
	// registry-generated subtrees (see buildRegistrySubtree specs above).
	// schema_validate remains a hand-wired direct leaf of schemaCmd.
	schemaCmd.AddCommand(schemaAddCmd)
	schemaCmd.AddCommand(schemaUpdateCmd)
	schemaCmd.AddCommand(schemaRemoveCmd)
	schemaCmd.AddCommand(schemaConvertCmd)
	schemaCmd.AddCommand(schemaRenameCmd)
	schemaCmd.AddCommand(schemaValidateCmd)
}
