package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/schemasvc"
)

var schemaCmd = &cobra.Command{
	Use:   "schema [types|traits|type <name>|trait <name>|core [name]|add|update|remove|rename|template ...]",
	Short: "Introspect the schema",
	Long: `Query the schema for types and traits.

This command is useful for agents to discover what's available.

Examples:
  rvn schema --json           # Full schema dump
  rvn schema types --json     # List all types
  rvn schema traits --json    # List all traits
  rvn schema type person --json   # Get type details
  rvn schema core --json      # List core type config
  rvn schema core date --json # Get core date config
  rvn schema trait due --json     # Get trait details
  rvn schema template list --json
  rvn schema template list --type interview --json
  rvn schema template list --core date --json`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runSchemaIntrospection(nil, renderFullSchema)
	},
}

var schemaTypesCmd = &cobra.Command{
	Use:   "types",
	Short: "List all types",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runSchemaIntrospection(map[string]interface{}{"subcommand": "types"}, renderSchemaTypes)
	},
}

var schemaTraitsCmd = &cobra.Command{
	Use:   "traits",
	Short: "List all traits",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runSchemaIntrospection(map[string]interface{}{"subcommand": "traits"}, renderSchemaTraits)
	},
}

var schemaTypeCmd = &cobra.Command{
	Use:   "type <name>",
	Short: "Show details for a type",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		name := args[0]
		return runSchemaIntrospection(
			map[string]interface{}{"subcommand": "type", "name": name},
			func(result commandexec.Result) error { return renderSchemaType(name, result) },
		)
	},
}

var schemaTraitCmd = &cobra.Command{
	Use:   "trait <name>",
	Short: "Show details for a trait",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		name := args[0]
		return runSchemaIntrospection(
			map[string]interface{}{"subcommand": "trait", "name": name},
			func(result commandexec.Result) error { return renderSchemaTrait(name, result) },
		)
	},
}

var schemaCoreCmd = &cobra.Command{
	Use:   "core [name]",
	Short: "Show core type configuration",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return runSchemaIntrospection(map[string]interface{}{"subcommand": "core"}, renderSchemaCoreList)
		}
		name := args[0]
		return runSchemaIntrospection(
			map[string]interface{}{"subcommand": "core", "name": name},
			func(result commandexec.Result) error { return renderSchemaCore(name, result) },
		)
	},
}

// runSchemaIntrospection delegates a read-only schema query to the canonical
// `schema` command and renders human output via render when not in JSON mode.
func runSchemaIntrospection(args map[string]interface{}, render func(commandexec.Result) error) error {
	result := executeCanonicalCommand("schema", getVaultPath(), args)
	if isJSONOutput() {
		return outputCanonicalResultJSON(result)
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	return render(result)
}

func renderFullSchema(result commandexec.Result) error {
	data := canonicalDataMap(result)
	version, _ := data["version"].(int)
	types, err := decodeSchemaValue[map[string]schemasvc.TypeSchema](data["types"])
	if err != nil {
		return err
	}
	core, _ := decodeSchemaValue[map[string]schemasvc.CoreTypeSchema](data["core"])
	traits, err := decodeSchemaValue[map[string]schemasvc.TraitSchema](data["traits"])
	if err != nil {
		return err
	}
	queries, _ := decodeSchemaValue[map[string]schemasvc.SavedQueryInfo](data["queries"])

	fmt.Printf("Schema (version %d)\n\n", version)

	fmt.Println("Types:")
	var typeNames []string
	for name, typeSchema := range types {
		if !typeSchema.Builtin {
			typeNames = append(typeNames, name)
		}
	}
	sort.Strings(typeNames)
	for _, name := range typeNames {
		fmt.Printf("  %s\n", name)
	}
	fmt.Println("  page (built-in)")
	fmt.Println("  section (built-in)")
	fmt.Println("  date (built-in)")

	fmt.Println("\nCore:")
	coreNames := []string{"date", "page", "section"}
	for _, name := range coreNames {
		coreDef, ok := core[name]
		if !ok {
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

	fmt.Println("\nTraits:")
	var traitNames []string
	for name := range traits {
		traitNames = append(traitNames, name)
	}
	sort.Strings(traitNames)
	for _, name := range traitNames {
		fmt.Printf("  %s\n", name)
	}

	if len(queries) > 0 {
		fmt.Println("\nSaved Queries:")
		var queryNames []string
		for name := range queries {
			queryNames = append(queryNames, name)
		}
		sort.Strings(queryNames)
		for _, name := range queryNames {
			fmt.Printf("  %s\n", name)
		}
	}

	return nil
}

func renderSchemaTypes(result commandexec.Result) error {
	data := canonicalDataMap(result)
	types, err := decodeSchemaValue[map[string]schemasvc.TypeSchema](data["types"])
	if err != nil {
		return err
	}

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

func renderSchemaTraits(result commandexec.Result) error {
	data := canonicalDataMap(result)
	traits, err := decodeSchemaValue[map[string]schemasvc.TraitSchema](data["traits"])
	if err != nil {
		return err
	}

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

func renderSchemaCoreList(result commandexec.Result) error {
	data := canonicalDataMap(result)
	core, err := decodeSchemaValue[map[string]schemasvc.CoreTypeSchema](data["core"])
	if err != nil {
		return err
	}

	fmt.Println("Core types:")
	names := []string{"date", "page", "section"}
	for _, name := range names {
		coreType, ok := core[name]
		if !ok {
			continue
		}
		if len(coreType.Templates) > 0 {
			fmt.Printf("  %s templates=%v", name, coreType.Templates)
			if coreType.DefaultTemplate != "" {
				fmt.Printf(" default=%s", coreType.DefaultTemplate)
			}
			fmt.Println()
			continue
		}
		if coreType.DefaultTemplate != "" {
			fmt.Printf("  %s default=%s\n", name, coreType.DefaultTemplate)
			continue
		}
		fmt.Printf("  %s\n", name)
	}
	return nil
}

func renderSchemaCore(coreTypeName string, result commandexec.Result) error {
	data := canonicalDataMap(result)
	coreJSON, err := decodeSchemaValue[schemasvc.CoreTypeSchema](data["core"])
	if err != nil {
		return err
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

func renderSchemaType(typeName string, result commandexec.Result) error {
	data := canonicalDataMap(result)
	typeJSON, err := decodeSchemaValue[schemasvc.TypeSchema](data["type"])
	if err != nil {
		return err
	}

	fmt.Printf("Type: %s\n", typeName)
	if typeJSON.Builtin {
		fmt.Printf("  Built-in: true\n")
		return nil
	}
	if typeJSON.Description != "" {
		fmt.Printf("  Description: %s\n", typeJSON.Description)
	}
	if typeJSON.DefaultPath != "" {
		fmt.Printf("  Default path: %s\n", typeJSON.DefaultPath)
	}
	if typeJSON.NameField != "" {
		fmt.Printf("  Name field: %s\n", typeJSON.NameField)
	}
	if typeJSON.Template != "" {
		fmt.Printf("  Template: %s\n", typeJSON.Template)
	}
	if len(typeJSON.Templates) > 0 {
		fmt.Println("  Templates:")
		templateIDs := append([]string(nil), typeJSON.Templates...)
		sort.Strings(templateIDs)
		for _, templateID := range templateIDs {
			fmt.Printf("    - %s\n", templateID)
		}
	}
	if typeJSON.DefaultTemplate != "" {
		fmt.Printf("  Default template: %s\n", typeJSON.DefaultTemplate)
	}
	if len(typeJSON.Fields) > 0 {
		fmt.Println("  Fields:")
		fieldNames := make([]string, 0, len(typeJSON.Fields))
		for name := range typeJSON.Fields {
			fieldNames = append(fieldNames, name)
		}
		sort.Strings(fieldNames)
		for _, name := range fieldNames {
			field := typeJSON.Fields[name]
			required := ""
			if field.Required {
				required = " (required)"
			}
			fieldType := field.Type
			if fieldType == "" {
				fieldType = "string"
			}
			isNameField := ""
			if name == typeJSON.NameField {
				isNameField = " [name_field]"
			}
			fieldDescription := ""
			if field.Description != "" {
				fieldDescription = " - " + field.Description
			}
			enumValues := ""
			if len(field.Values) > 0 {
				enumValues = " values=[" + strings.Join(field.Values, ", ") + "]"
			}
			fmt.Printf("    %s: %s%s%s%s%s\n", name, fieldType, required, isNameField, enumValues, fieldDescription)
		}
	}

	return nil
}

func renderSchemaTrait(traitName string, result commandexec.Result) error {
	data := canonicalDataMap(result)
	traitJSON, err := decodeSchemaValue[schemasvc.TraitSchema](data["trait"])
	if err != nil {
		return err
	}

	fmt.Printf("Trait: %s\n", traitName)
	if traitJSON.Type != "" {
		fmt.Printf("  Type: %s\n", traitJSON.Type)
	}
	if len(traitJSON.Values) > 0 {
		fmt.Printf("  Values: %v\n", traitJSON.Values)
	}
	if traitJSON.Default != "" {
		fmt.Printf("  Default: %s\n", traitJSON.Default)
	}

	return nil
}

func init() {
	// Introspection subcommands are thin, human-rendering presentations that all
	// delegate to the single canonical `schema` command, so they have no
	// per-subcommand registry metadata and are marked as local leaves.
	for _, cmd := range []*cobra.Command{
		schemaTypesCmd,
		schemaTraitsCmd,
		schemaTypeCmd,
		schemaTraitCmd,
		schemaCoreCmd,
	} {
		markLocalLeaf(cmd)
		schemaCmd.AddCommand(cmd)
	}
	rootCmd.AddCommand(schemaCmd)
}

func decodeSchemaValue[T any](raw interface{}) (T, error) {
	var out T
	if raw == nil {
		return out, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, err
	}
	return out, nil
}
