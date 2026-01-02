package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/ravenscroftj/raven/internal/config"
	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/spf13/cobra"
)

// SchemaJSON is the JSON representation of the full schema.
type SchemaJSON struct {
	Version int                      `json:"version"`
	Types   map[string]TypeSchemaJSON `json:"types"`
	Traits  map[string]TraitSchemaJSON `json:"traits"`
	Queries map[string]QuerySchemaJSON `json:"queries,omitempty"`
}

// TypeSchemaJSON is the JSON representation of a type definition.
type TypeSchemaJSON struct {
	Name          string                     `json:"name"`
	Builtin       bool                       `json:"builtin"`
	DefaultPath   string                     `json:"default_path,omitempty"`
	Fields        map[string]FieldSchemaJSON `json:"fields,omitempty"`
	Traits        []string                   `json:"traits,omitempty"`
	RequiredTraits []string                  `json:"required_traits,omitempty"`
}

// FieldSchemaJSON is the JSON representation of a field definition.
type FieldSchemaJSON struct {
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Default  string   `json:"default,omitempty"`
	Values   []string `json:"values,omitempty"` // For enum types
	Target   string   `json:"target,omitempty"` // For ref types
}

// TraitSchemaJSON is the JSON representation of a trait definition.
type TraitSchemaJSON struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Values  []string `json:"values,omitempty"`
	Default string   `json:"default,omitempty"`
}

// QuerySchemaJSON is the JSON representation of a saved query.
type QuerySchemaJSON struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Types       []string          `json:"types,omitempty"`
	Traits      []string          `json:"traits,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Filters     map[string]string `json:"filters,omitempty"`
}

// CommandSchemaJSON describes a command for agent discovery.
type CommandSchemaJSON struct {
	Description   string              `json:"description"`
	DefaultTarget string              `json:"default_target,omitempty"`
	Args          []string            `json:"args,omitempty"`
	Flags         map[string]FlagJSON `json:"flags,omitempty"`
	Examples      []string            `json:"examples,omitempty"`
	UseCases      []string            `json:"use_cases,omitempty"`
}

// FlagJSON describes a command flag.
type FlagJSON struct {
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description"`
	Examples    []string `json:"examples,omitempty"`
}

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
		schemaJSON := buildSchemaJSON(sch, vaultCfg)
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
	types := make(map[string]TypeSchemaJSON)

	// User-defined types
	for name, typeDef := range sch.Types {
		types[name] = buildTypeSchemaJSON(name, typeDef, false)
	}

	// Built-in types
	types["page"] = TypeSchemaJSON{Name: "page", Builtin: true}
	types["section"] = TypeSchemaJSON{Name: "section", Builtin: true}
	types["date"] = TypeSchemaJSON{Name: "date", Builtin: true}

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
	traits := make(map[string]TraitSchemaJSON)
	for name, traitDef := range sch.Traits {
		traits[name] = buildTraitSchemaJSON(name, traitDef)
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
		typeJSON := TypeSchemaJSON{Name: typeName, Builtin: true}
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

	typeJSON := buildTypeSchemaJSON(typeName, typeDef, false)

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

	traitJSON := buildTraitSchemaJSON(traitName, traitDef)

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

	commands := map[string]CommandSchemaJSON{
		"new": {
			Description: "Create a new typed object (person, project, etc.). If required fields are missing, returns an error listing them - ask user for values and retry with fields parameter.",
			Args:        []string{"type", "title"},
			Flags: map[string]FlagJSON{
				"--field": {
					Type:        "key=value",
					Description: "Set field value (can be repeated for multiple fields)",
					Examples:    []string{"--field name=\"Alice Smith\"", "--field email=alice@example.com"},
				},
			},
			UseCases: []string{
				"Create a new person entry",
				"Create a new project file",
				"Create any typed object defined in schema",
			},
			Examples: []string{
				"rvn new person \"Alice Smith\" --field name=\"Alice Smith\" --json",
				"rvn new project \"Mobile App\" --json",
				"rvn new meeting \"Team Sync\" --json",
			},
		},
		"read": {
			Description: "Read raw file content",
			Args:        []string{"path"},
			Examples:    []string{"rvn read daily/2025-02-01.md --json"},
		},
		"add": {
			Description:   "Append content to EXISTING files or today's daily note. Only works on files that already exist (daily notes auto-created). For new objects, use 'new' command.",
			DefaultTarget: "Today's daily note",
			Args:          []string{"text"},
			Flags: map[string]FlagJSON{
				"--to": {
					Type:        "path",
					Description: "Target EXISTING file path. File must already exist. Omit to use daily note.",
					Examples:    []string{"projects/website.md", "inbox.md", "daily/2025-02-01.md"},
				},
			},
			UseCases: []string{
				"Quick capture to daily note",
				"Add tasks to existing project files",
				"Append notes to existing documents",
			},
			Examples: []string{
				"rvn add \"Quick thought\" --json",
				"rvn add \"New task\" --to projects/website.md --json",
				"rvn add \"@priority(high) Urgent task\" --json",
			},
		},
		"trait": {
			Description: "Query traits by type",
			Args:        []string{"trait_name"},
			Flags: map[string]FlagJSON{
				"--value": {Description: "Filter by value (supports: today, past, this-week, or literal values)"},
			},
			Examples: []string{"rvn trait due --json", "rvn trait due --value past --json"},
		},
		"query": {
			Description: "Run saved queries",
			Args:        []string{"query_name"},
			Flags: map[string]FlagJSON{
				"--list": {Description: "List available saved queries"},
			},
			Examples: []string{"rvn query tasks --json", "rvn query overdue --json"},
		},
		"type": {
			Description: "List objects by type",
			Args:        []string{"type_name"},
			Flags: map[string]FlagJSON{
				"--list": {Description: "List available types with counts"},
			},
			Examples: []string{"rvn type person --json", "rvn type meeting --json"},
		},
		"tag": {
			Description: "Query objects by tags",
			Args:        []string{"tag_name"},
			Flags: map[string]FlagJSON{
				"--list": {Description: "List all tags with counts"},
			},
			Examples: []string{"rvn tag important --json", "rvn tag --list --json"},
		},
		"backlinks": {
			Description: "Find objects that reference a target",
			Args:        []string{"target"},
			Examples:    []string{"rvn backlinks people/alice --json"},
		},
		"date": {
			Description: "Date hub - all activity for a date",
			Args:        []string{"date"},
			Flags: map[string]FlagJSON{
				"--edit": {Description: "Open the daily note in editor"},
			},
			Examples: []string{"rvn date today --json", "rvn date 2025-02-01 --json"},
		},
		"stats": {
			Description: "Show vault statistics",
			Examples:    []string{"rvn stats --json"},
		},
		"check": {
			Description: "Validate vault against schema",
			Flags: map[string]FlagJSON{
				"--create-missing": {Description: "Interactively create missing referenced pages"},
			},
			Examples: []string{"rvn check --json"},
		},
	}

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"commands": commands,
		}, &Meta{Count: len(commands), QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Println("Available commands:")
	var names []string
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		c := commands[name]
		fmt.Printf("  %-12s %s\n", name, c.Description)
	}
	fmt.Println("\nUse 'rvn schema commands --json' for full details.")

	return nil
}

func buildSchemaJSON(sch *schema.Schema, vaultCfg *config.VaultConfig) SchemaJSON {
	result := SchemaJSON{
		Version: sch.Version,
		Types:   make(map[string]TypeSchemaJSON),
		Traits:  make(map[string]TraitSchemaJSON),
	}

	// User-defined types
	for name, typeDef := range sch.Types {
		result.Types[name] = buildTypeSchemaJSON(name, typeDef, false)
	}

	// Built-in types
	result.Types["page"] = TypeSchemaJSON{Name: "page", Builtin: true}
	result.Types["section"] = TypeSchemaJSON{Name: "section", Builtin: true}
	result.Types["date"] = TypeSchemaJSON{Name: "date", Builtin: true}

	// Traits
	for name, traitDef := range sch.Traits {
		result.Traits[name] = buildTraitSchemaJSON(name, traitDef)
	}

	// Queries from vault config
	if vaultCfg != nil && len(vaultCfg.Queries) > 0 {
		result.Queries = make(map[string]QuerySchemaJSON)
		for name, q := range vaultCfg.Queries {
			result.Queries[name] = QuerySchemaJSON{
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

func buildTypeSchemaJSON(name string, typeDef *schema.TypeDefinition, builtin bool) TypeSchemaJSON {
	result := TypeSchemaJSON{
		Name:    name,
		Builtin: builtin,
	}

	if typeDef != nil {
		result.DefaultPath = typeDef.DefaultPath

		if len(typeDef.Fields) > 0 {
			result.Fields = make(map[string]FieldSchemaJSON)
			for fieldName, fieldDef := range typeDef.Fields {
				if fieldDef != nil {
					defaultStr := ""
					if fieldDef.Default != nil {
						defaultStr = fmt.Sprintf("%v", fieldDef.Default)
					}
					result.Fields[fieldName] = FieldSchemaJSON{
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

func buildTraitSchemaJSON(name string, traitDef *schema.TraitDefinition) TraitSchemaJSON {
	result := TraitSchemaJSON{Name: name}
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
