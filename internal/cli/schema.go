package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/schemasvc"
)

var schemaCmd = &cobra.Command{
	Use:   "schema [types|traits|type <name>|trait <name>|core [name]|template ...]",
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
		case "core":
			if len(args) == 1 {
				return listSchemaCore(vaultPath, start)
			}
			if len(args) > 2 {
				return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema core subcommand: %s", args[2]), "Use: schema core [name]")
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
		default:
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema subcommand: %s", args[0]), "Use: types, traits, type <name>, trait <name>, core [name], or template ...")
		}
	},
}

func dumpFullSchema(vaultPath string, start time.Time) error {
	result := executeCanonicalCommand("schema", vaultPath, nil)
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}

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

	// Human-readable output
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

func listSchemaTypes(vaultPath string, start time.Time) error {
	result := executeCanonicalCommand("schema", vaultPath, map[string]interface{}{"subcommand": "types"})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)
	types, err := decodeSchemaValue[map[string]schemasvc.TypeSchema](data["types"])
	if err != nil {
		return err
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
	result := executeCanonicalCommand("schema", vaultPath, map[string]interface{}{"subcommand": "traits"})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)
	traits, err := decodeSchemaValue[map[string]schemasvc.TraitSchema](data["traits"])
	if err != nil {
		return err
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
	result := executeCanonicalCommand("schema", vaultPath, map[string]interface{}{"subcommand": "core"})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
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

func getSchemaCore(vaultPath, coreTypeName string, start time.Time) error {
	result := executeCanonicalCommand("schema", vaultPath, map[string]interface{}{"subcommand": "core", "name": coreTypeName})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
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

func getSchemaType(vaultPath, typeName string, start time.Time) error {
	result := executeCanonicalCommand("schema", vaultPath, map[string]interface{}{"subcommand": "type", "name": typeName})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)
	typeJSON, err := decodeSchemaValue[schemasvc.TypeSchema](data["type"])
	if err != nil {
		return err
	}

	// Human-readable output
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
			fmt.Printf("    %s: %s%s%s%s\n", name, fieldType, required, isNameField, fieldDescription)
		}
	}

	return nil
}

func getSchemaTrait(vaultPath, traitName string, start time.Time) error {
	result := executeCanonicalCommand("schema", vaultPath, map[string]interface{}{"subcommand": "trait", "name": traitName})
	if isJSONOutput() {
		outputCanonicalResultJSON(result)
		return nil
	}
	if err := handleCanonicalFailure(result); err != nil {
		return err
	}
	data := canonicalDataMap(result)
	traitJSON, err := decodeSchemaValue[schemasvc.TraitSchema](data["trait"])
	if err != nil {
		return err
	}

	// Human-readable output
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
