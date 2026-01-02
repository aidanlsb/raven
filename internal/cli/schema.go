package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/spf13/cobra"
)

var schemaCmd = &cobra.Command{
	Use:   "schema [types|traits|type <name>|trait <name>|commands]",
	Short: "Introspect the schema",
	Long: `Query the schema for types, traits, and commands.

This command is useful for agents to discover what's available.

Examples:
  rvn schema --json           # Full schema dump
  rvn schema types --json     # List all types
  rvn schema traits --json    # List all traits
  rvn schema type person --json   # Get type details
  rvn schema trait due --json     # Get trait details
  rvn schema commands --json      # List available commands`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()

		// If no subcommand, return full schema
		if len(args) == 0 {
			return dumpFullSchema(vaultPath, start)
		}

		switch args[0] {
		case "types":
			return listSchemaTypes(vaultPath, start)
		case "traits":
			return listSchemaTraits(vaultPath, start)
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
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema subcommand: %s", args[0]), "Use: types, traits, type <name>, trait <name>, or commands")
		}
	},
}

func dumpFullSchema(vaultPath string, start time.Time) error {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	vaultCfg, _ := config.LoadVaultConfig(vaultPath)

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
		fmt.Printf("  %s\n", name)
	}
	fmt.Println("  page (built-in)")
	fmt.Println("  section (built-in)")
	fmt.Println("  date (built-in)")

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
		outputSuccess(map[string]interface{}{
			"types": types,
		}, &Meta{Count: len(types), QueryTimeMs: elapsed})
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

func getSchemaType(vaultPath, typeName string, start time.Time) error {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	elapsed := time.Since(start).Milliseconds()

	// Check for built-in types
	switch typeName {
	case "page", "section", "date":
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
	if typeDef.DefaultPath != "" {
		fmt.Printf("  Default path: %s\n", typeDef.DefaultPath)
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
			fmt.Printf("    %s: %s%s\n", name, fieldType, required)
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

	// Traits
	for name, traitDef := range sch.Traits {
		result.Traits[name] = buildTraitSchema(name, traitDef)
	}

	// Queries from vault config
	if vaultCfg != nil && len(vaultCfg.Queries) > 0 {
		result.Queries = make(map[string]SavedQueryInfo)
		for name, q := range vaultCfg.Queries {
			result.Queries[name] = SavedQueryInfo{
				Name:        name,
				Description: q.Description,
				Types:       q.Types,
				Traits:      q.Traits,
				Tags:        q.Tags,
				Filters:     q.Filters,
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

		if len(typeDef.Fields) > 0 {
			result.Fields = make(map[string]FieldSchema)
			for fieldName, fieldDef := range typeDef.Fields {
				if fieldDef != nil {
					defaultStr := ""
					if fieldDef.Default != nil {
						defaultStr = fmt.Sprintf("%v", fieldDef.Default)
					}
					result.Fields[fieldName] = FieldSchema{
						Type:     string(fieldDef.Type),
						Required: fieldDef.Required,
						Default:  defaultStr,
						Values:   fieldDef.Values,
						Target:   fieldDef.Target,
					}
				}
			}
		}

		if len(typeDef.Traits.List()) > 0 {
			result.Traits = typeDef.Traits.List()
			for _, traitName := range result.Traits {
				if typeDef.Traits.IsRequired(traitName) {
					result.RequiredTraits = append(result.RequiredTraits, traitName)
				}
			}
		}
	}

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

func init() {
	rootCmd.AddCommand(schemaCmd)
}
