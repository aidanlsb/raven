package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
)

var schemaCmd = &cobra.Command{
	Use:   "schema [types|traits|type <name>|trait <name>|core [name]|template ...|commands]",
	Short: "Introspect the schema",
	Long: `Query the schema for types, traits, and commands.

This command is useful for agents to discover what's available.

Examples:
  rvn schema --json           # Full schema dump
  rvn schema types --json     # List all types
  rvn schema traits --json    # List all traits
  rvn schema type person --json   # Get type details
  rvn schema core --json      # List core type config
  rvn schema core date --json # Get core date config
  rvn schema trait due --json     # Get trait details
  rvn schema commands --json      # List available commands
  rvn schema template list --json
  rvn schema type interview template list --json
  rvn schema core date template list --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()

		// If no subcommand, return full schema
		if len(args) == 0 {
			return dumpFullSchema(vaultPath, start)
		}

		// Template-related subcommands:
		// - schema template ...
		// - schema type <type_name> template ...
		// - schema core <core_type> template ...
		if args[0] == "template" {
			return runSchemaTemplateCommand(vaultPath, args[1:], start)
		}
		// Also support MCP-style positional order:
		// - schema type template <action> <type_name> [template_id]
		// - schema core template <action> <core_type> [template_id]
		if len(args) >= 4 && args[0] == "type" && args[1] == "template" {
			templateArgs := append([]string{args[2]}, args[4:]...)
			return runSchemaTypeTemplateCommand(vaultPath, args[3], templateArgs, start)
		}
		if len(args) >= 4 && args[0] == "core" && args[1] == "template" {
			templateArgs := append([]string{args[2]}, args[4:]...)
			return runSchemaCoreTemplateCommand(vaultPath, args[3], templateArgs, start)
		}
		if len(args) >= 3 && args[0] == "type" && args[2] == "template" {
			return runSchemaTypeTemplateCommand(vaultPath, args[1], args[3:], start)
		}
		if len(args) >= 3 && args[0] == "core" && args[2] == "template" {
			return runSchemaCoreTemplateCommand(vaultPath, args[1], args[3:], start)
		}

		switch args[0] {
		case "types":
			return listSchemaTypes(vaultPath, start)
		case "traits":
			return listSchemaTraits(vaultPath, start)
		case "core":
			if len(args) == 1 {
				return listSchemaCore(vaultPath, start)
			}
			if len(args) > 2 {
				return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema core subcommand: %s", args[2]), "Use: schema core [name] or schema core <name> template ...")
			}
			return getSchemaCore(vaultPath, args[1], start)
		case "type":
			if len(args) < 2 {
				return handleErrorMsg(ErrMissingArgument, "specify a type name", "Usage: rvn schema type <name>")
			}
			return getSchemaType(vaultPath, args[1], start)
		case "trait":
			if len(args) < 2 {
				return handleErrorMsg(ErrMissingArgument, "specify a trait name", "Usage: rvn schema trait <name>")
			}
			return getSchemaTrait(vaultPath, args[1], start)
		case "commands":
			return listSchemaCommands(start)
		default:
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema subcommand: %s", args[0]), "Use: types, traits, type <name>, trait <name>, core [name], template ..., or commands")
		}
	},
}

func dumpFullSchema(vaultPath string, start time.Time) error {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, fmt.Errorf("failed to load raven.yaml: %w", err), "Fix raven.yaml and try again")
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		schemaJSON := buildSchemaResult(sch, vaultCfg)
		outputSuccess(schemaJSON, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Printf("Schema (version %d)\n\n", sch.Version)

	fmt.Println("Types:")
	for name := range sch.Types {
		if schema.IsBuiltinType(name) {
			continue
		}
		fmt.Printf("  %s\n", name)
	}
	fmt.Println("  page (built-in)")
	fmt.Println("  section (built-in)")
	fmt.Println("  date (built-in)")

	if len(sch.Core) > 0 {
		fmt.Println("\nCore:")
		coreNames := []string{"date", "page", "section"}
		for _, name := range coreNames {
			coreDef := sch.Core[name]
			if coreDef == nil {
				continue
			}
			if coreDef.DefaultTemplate != "" {
				fmt.Printf("  %s: default_template=%s\n", name, coreDef.DefaultTemplate)
			} else if len(coreDef.Templates) > 0 {
				fmt.Printf("  %s: templates=%v\n", name, coreDef.Templates)
			} else {
				fmt.Printf("  %s: {}\n", name)
			}
		}
	}

	fmt.Println("\nTraits:")
	for name := range sch.Traits {
		fmt.Printf("  %s\n", name)
	}

	if vaultCfg != nil && len(vaultCfg.Queries) > 0 {
		fmt.Println("\nSaved Queries:")
		for name := range vaultCfg.Queries {
			fmt.Printf("  %s\n", name)
		}
	}

	return nil
}

func listSchemaTypes(vaultPath string, start time.Time) error {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	elapsed := time.Since(start).Milliseconds()

	// Collect types
	types := make(map[string]TypeSchema)

	// User-defined types
	for name, typeDef := range sch.Types {
		types[name] = buildTypeSchema(name, typeDef, false)
	}

	// Built-in types
	types["page"] = TypeSchema{Name: "page", Builtin: true}
	types["section"] = TypeSchema{Name: "section", Builtin: true}
	types["date"] = TypeSchema{Name: "date", Builtin: true}

	if isJSONOutput() {
		// Count types without name_field that have required string fields
		var typesWithoutNameField []string
		for name, typeDef := range sch.Types {
			if typeDef != nil && typeDef.NameField == "" && !isBuiltinType(name) {
				// Check if type has a required string field that could be a name_field
				for _, fieldDef := range typeDef.Fields {
					if fieldDef != nil && fieldDef.Required && fieldDef.Type == schema.FieldTypeString {
						typesWithoutNameField = append(typesWithoutNameField, name)
						break
					}
				}
			}
		}

		result := map[string]interface{}{
			"types": types,
		}

		// Add hint for agents about name_field
		if len(typesWithoutNameField) > 0 {
			sort.Strings(typesWithoutNameField)
			result["hint"] = map[string]interface{}{
				"message":                  "Some types have required string fields but no name_field configured. Setting name_field enables auto-population from the title argument in raven_new.",
				"types_without_name_field": typesWithoutNameField,
				"fix_command":              "raven_schema_update_type with name-field parameter",
			}
		}

		outputSuccess(result, &Meta{Count: len(types), QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Println("Types:")
	var names []string
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t := types[name]
		if t.Builtin {
			fmt.Printf("  %s (built-in)\n", name)
		} else {
			fmt.Printf("  %s\n", name)
		}
	}

	return nil
}

func listSchemaTraits(vaultPath string, start time.Time) error {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	elapsed := time.Since(start).Milliseconds()

	// Collect traits
	traits := make(map[string]TraitSchema)
	for name, traitDef := range sch.Traits {
		traits[name] = buildTraitSchema(name, traitDef)
	}

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"traits": traits,
		}, &Meta{Count: len(traits), QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Println("Traits:")
	var names []string
	for name := range traits {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t := traits[name]
		if t.Type != "" {
			fmt.Printf("  %s (%s)\n", name, t.Type)
		} else {
			fmt.Printf("  %s\n", name)
		}
	}

	return nil
}

func listSchemaCore(vaultPath string, start time.Time) error {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}
	elapsed := time.Since(start).Milliseconds()

	result := map[string]CoreTypeSchema{
		"date":    buildCoreTypeSchema("date", sch.Core["date"]),
		"page":    buildCoreTypeSchema("page", sch.Core["page"]),
		"section": buildCoreTypeSchema("section", sch.Core["section"]),
	}

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{"core": result}, &Meta{Count: len(result), QueryTimeMs: elapsed})
		return nil
	}

	fmt.Println("Core types:")
	names := []string{"date", "page", "section"}
	for _, name := range names {
		core := result[name]
		if len(core.Templates) > 0 {
			fmt.Printf("  %s templates=%v", name, core.Templates)
			if core.DefaultTemplate != "" {
				fmt.Printf(" default=%s", core.DefaultTemplate)
			}
			fmt.Println()
			continue
		}
		if core.DefaultTemplate != "" {
			fmt.Printf("  %s default=%s\n", name, core.DefaultTemplate)
			continue
		}
		fmt.Printf("  %s\n", name)
	}
	return nil
}

func getSchemaCore(vaultPath, coreTypeName string, start time.Time) error {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	if !schema.IsBuiltinType(coreTypeName) {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("core type '%s' not found", coreTypeName), "Available core types: date, page, section")
	}
	elapsed := time.Since(start).Milliseconds()
	coreJSON := buildCoreTypeSchema(coreTypeName, sch.Core[coreTypeName])

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{"core": coreJSON}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("Core type: %s\n", coreTypeName)
	if len(coreJSON.Templates) > 0 {
		fmt.Println("  Templates:")
		templates := append([]string(nil), coreJSON.Templates...)
		sort.Strings(templates)
		for _, templateID := range templates {
			fmt.Printf("    - %s\n", templateID)
		}
	}
	if coreJSON.DefaultTemplate != "" {
		fmt.Printf("  Default template: %s\n", coreJSON.DefaultTemplate)
	}
	return nil
}

func getSchemaType(vaultPath, typeName string, start time.Time) error {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	elapsed := time.Since(start).Milliseconds()

	// Check for built-in types
	if schema.IsBuiltinType(typeName) {
		typeJSON := TypeSchema{Name: typeName, Builtin: true}
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{"type": typeJSON}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		fmt.Printf("Type: %s (built-in)\n", typeName)
		return nil
	}

	typeDef, ok := sch.Types[typeName]
	if !ok {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "Run 'rvn schema types' to see available types")
	}

	typeJSON := buildTypeSchema(typeName, typeDef, false)

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{"type": typeJSON}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Printf("Type: %s\n", typeName)
	if typeDef.Description != "" {
		fmt.Printf("  Description: %s\n", typeDef.Description)
	}
	if typeDef.DefaultPath != "" {
		fmt.Printf("  Default path: %s\n", typeDef.DefaultPath)
	}
	if typeDef.NameField != "" {
		fmt.Printf("  Name field: %s\n", typeDef.NameField)
	}
	if typeDef.Template != "" {
		fmt.Printf("  Template: %s\n", typeDef.Template)
	}
	if len(typeDef.Templates) > 0 {
		fmt.Println("  Templates:")
		templateIDs := append([]string(nil), typeDef.Templates...)
		sort.Strings(templateIDs)
		for _, templateID := range templateIDs {
			fmt.Printf("    - %s\n", templateID)
		}
	}
	if typeDef.DefaultTemplate != "" {
		fmt.Printf("  Default template: %s\n", typeDef.DefaultTemplate)
	}
	if len(typeDef.Fields) > 0 {
		fmt.Println("  Fields:")
		for name, field := range typeDef.Fields {
			required := ""
			if field != nil && field.Required {
				required = " (required)"
			}
			fieldType := "string"
			if field != nil && field.Type != "" {
				fieldType = string(field.Type)
			}
			isNameField := ""
			if name == typeDef.NameField {
				isNameField = " [name_field]"
			}
			fieldDescription := ""
			if field != nil && field.Description != "" {
				fieldDescription = " - " + field.Description
			}
			fmt.Printf("    %s: %s%s%s%s\n", name, fieldType, required, isNameField, fieldDescription)
		}
	}

	return nil
}

func getSchemaTrait(vaultPath, traitName string, start time.Time) error {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	elapsed := time.Since(start).Milliseconds()

	traitDef, ok := sch.Traits[traitName]
	if !ok {
		return handleErrorMsg(ErrTraitNotFound, fmt.Sprintf("trait '%s' not found", traitName), "Run 'rvn schema traits' to see available traits")
	}

	traitJSON := buildTraitSchema(traitName, traitDef)

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{"trait": traitJSON}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Printf("Trait: %s\n", traitName)
	if traitDef.Type != "" {
		fmt.Printf("  Type: %s\n", traitDef.Type)
	}
	if len(traitDef.Values) > 0 {
		fmt.Printf("  Values: %v\n", traitDef.Values)
	}
	if traitDef.Default != "" {
		fmt.Printf("  Default: %s\n", traitDef.Default)
	}

	return nil
}

func listSchemaCommands(start time.Time) error {
	elapsed := time.Since(start).Milliseconds()

	// Generate commands from the registry - single source of truth!
	cmds := make(map[string]CommandSchema)
	for name, meta := range commands.Registry {
		cmd := CommandSchema{
			Description: meta.Description,
			Examples:    meta.Examples,
			UseCases:    meta.UseCases,
		}

		// Add args
		for _, arg := range meta.Args {
			cmd.Args = append(cmd.Args, arg.Name)
		}

		// Add flags
		if len(meta.Flags) > 0 {
			cmd.Flags = make(map[string]FlagSchema)
			for _, flag := range meta.Flags {
				cmd.Flags["--"+flag.Name] = FlagSchema{
					Type:        string(flag.Type),
					Description: flag.Description,
					Examples:    flag.Examples,
				}
			}
		}

		cmds[name] = cmd
	}

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"commands": cmds,
		}, &Meta{Count: len(cmds), QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Println("Available commands:")
	var names []string
	for name := range cmds {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		c := cmds[name]
		fmt.Printf("  %-18s %s\n", name, c.Description)
	}
	fmt.Println("\nUse 'rvn schema commands --json' for full details.")

	return nil
}

func buildSchemaResult(sch *schema.Schema, vaultCfg *config.VaultConfig) SchemaResult {
	result := SchemaResult{
		Version: sch.Version,
		Types:   make(map[string]TypeSchema),
		Core:    make(map[string]CoreTypeSchema),
		Traits:  make(map[string]TraitSchema),
	}

	// User-defined types
	for name, typeDef := range sch.Types {
		result.Types[name] = buildTypeSchema(name, typeDef, false)
	}

	// Built-in types
	result.Types["page"] = TypeSchema{Name: "page", Builtin: true}
	result.Types["section"] = TypeSchema{Name: "section", Builtin: true}
	result.Types["date"] = TypeSchema{Name: "date", Builtin: true}

	// Core type config
	result.Core["date"] = buildCoreTypeSchema("date", sch.Core["date"])
	result.Core["page"] = buildCoreTypeSchema("page", sch.Core["page"])
	result.Core["section"] = buildCoreTypeSchema("section", sch.Core["section"])

	// Traits
	for name, traitDef := range sch.Traits {
		result.Traits[name] = buildTraitSchema(name, traitDef)
	}

	if len(sch.Templates) > 0 {
		result.Templates = make(map[string]TemplateSchema, len(sch.Templates))
		for id, templateDef := range sch.Templates {
			if templateDef == nil {
				continue
			}
			result.Templates[id] = TemplateSchema{
				ID:          id,
				File:        templateDef.File,
				Description: templateDef.Description,
			}
		}
	}

	// Queries from vault config
	if vaultCfg != nil && len(vaultCfg.Queries) > 0 {
		result.Queries = make(map[string]SavedQueryInfo)
		for name, q := range vaultCfg.Queries {
			result.Queries[name] = SavedQueryInfo{
				Name:        name,
				Query:       q.Query,
				Args:        q.Args,
				Description: q.Description,
			}
		}
	}

	return result
}

func buildTypeSchema(name string, typeDef *schema.TypeDefinition, builtin bool) TypeSchema {
	result := TypeSchema{
		Name:    name,
		Builtin: builtin,
	}

	if typeDef != nil {
		result.DefaultPath = typeDef.DefaultPath
		result.Description = typeDef.Description
		result.NameField = typeDef.NameField
		result.Template = typeDef.Template
		result.Templates = append([]string(nil), typeDef.Templates...)
		result.DefaultTemplate = typeDef.DefaultTemplate

		if len(typeDef.Fields) > 0 {
			result.Fields = make(map[string]FieldSchema)
			for fieldName, fieldDef := range typeDef.Fields {
				if fieldDef != nil {
					defaultStr := ""
					if fieldDef.Default != nil {
						defaultStr = fmt.Sprintf("%v", fieldDef.Default)
					}
					result.Fields[fieldName] = FieldSchema{
						Type:        string(fieldDef.Type),
						Required:    fieldDef.Required,
						Default:     defaultStr,
						Values:      fieldDef.Values,
						Target:      fieldDef.Target,
						Description: fieldDef.Description,
					}
				}
			}
		}
	}

	return result
}

func buildCoreTypeSchema(name string, coreDef *schema.CoreTypeDefinition) CoreTypeSchema {
	result := CoreTypeSchema{Name: name}
	if coreDef == nil {
		return result
	}
	result.Templates = append([]string(nil), coreDef.Templates...)
	result.DefaultTemplate = coreDef.DefaultTemplate
	return result
}

func buildTraitSchema(name string, traitDef *schema.TraitDefinition) TraitSchema {
	result := TraitSchema{Name: name}
	if traitDef != nil {
		result.Type = string(traitDef.Type)
		result.Values = traitDef.Values
		if traitDef.Default != nil {
			result.Default = fmt.Sprintf("%v", traitDef.Default)
		}
	}
	return result
}

// isBuiltinType is deprecated - use schema.IsBuiltinType instead.
// Keeping for now to avoid breaking any internal callers.
func isBuiltinType(name string) bool {
	return schema.IsBuiltinType(name)
}

func init() {
	schemaCmd.Flags().StringVar(&schemaTemplateFileFlag, "file", "", "Template file path under directories.template (for `schema template set`)")
	schemaCmd.Flags().StringVar(&schemaTemplateDescriptionFlag, "description", "", "Template description (for `schema template set`; use '-' to clear)")
	schemaCmd.Flags().BoolVar(&schemaTypeTemplateClearFlag, "clear", false, "Clear type/core default template (for `schema type <type_name> template default --clear` and `schema core <core_type> template default --clear`)")
	rootCmd.AddCommand(schemaCmd)
}
