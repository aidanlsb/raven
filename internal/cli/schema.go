package cli

import (
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
	result, err := schemasvc.FullSchema(vaultPath)
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Printf("Schema (version %d)\n\n", result.Version)

	fmt.Println("Types:")
	var typeNames []string
	for name, typeSchema := range result.Types {
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
		coreDef, ok := result.Core[name]
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
	for name := range result.Traits {
		traitNames = append(traitNames, name)
	}
	sort.Strings(traitNames)
	for _, name := range traitNames {
		fmt.Printf("  %s\n", name)
	}

	if len(result.Queries) > 0 {
		fmt.Println("\nSaved Queries:")
		var queryNames []string
		for name := range result.Queries {
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
	result, err := schemasvc.Types(vaultPath)
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		data := map[string]interface{}{
			"types": result.Types,
		}
		if result.Hint != nil {
			data["hint"] = result.Hint
		}
		outputSuccess(data, &Meta{Count: len(result.Types), QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Println("Types:")
	var names []string
	for name := range result.Types {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t := result.Types[name]
		if t.Builtin {
			fmt.Printf("  %s (built-in)\n", name)
		} else {
			fmt.Printf("  %s\n", name)
		}
	}

	return nil
}

func listSchemaTraits(vaultPath string, start time.Time) error {
	result, err := schemasvc.Traits(vaultPath)
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"traits": result.Traits,
		}, &Meta{Count: len(result.Traits), QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Println("Traits:")
	var names []string
	for name := range result.Traits {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t := result.Traits[name]
		if t.Type != "" {
			fmt.Printf("  %s (%s)\n", name, t.Type)
		} else {
			fmt.Printf("  %s\n", name)
		}
	}

	return nil
}

func listSchemaCore(vaultPath string, start time.Time) error {
	result, err := schemasvc.CoreList(vaultPath)
	if err != nil {
		return mapSchemaServiceError(err)
	}
	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{"core": result.Core}, &Meta{Count: len(result.Core), QueryTimeMs: elapsed})
		return nil
	}

	fmt.Println("Core types:")
	names := []string{"date", "page", "section"}
	for _, name := range names {
		core, ok := result.Core[name]
		if !ok {
			continue
		}
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
	result, err := schemasvc.CoreByName(vaultPath, coreTypeName)
	if err != nil {
		return mapSchemaServiceError(err)
	}
	elapsed := time.Since(start).Milliseconds()
	coreJSON := result.Core

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
	result, err := schemasvc.TypeByName(vaultPath, typeName)
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()
	typeJSON := result.Type

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{"type": typeJSON}, &Meta{QueryTimeMs: elapsed})
		return nil
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
	result, err := schemasvc.TraitByName(vaultPath, traitName)
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()
	traitJSON := result.Trait

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{"trait": traitJSON}, &Meta{QueryTimeMs: elapsed})
		return nil
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
