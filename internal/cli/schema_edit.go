package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemasvc"
)

// Flags for schema add commands
var (
	schemaAddDefaultPath string
	schemaAddNameField   string
	schemaAddDescription string
	schemaAddFieldType   string
	schemaAddRequired    bool
	schemaAddDefault     string
	schemaAddValues      string
	schemaAddTarget      string
)

// Flags for schema update commands
var (
	schemaUpdateDefaultPath string
	schemaUpdateNameField   string
	schemaUpdateDescription string
	schemaUpdateFieldType   string
	schemaUpdateRequired    string // "true", "false", or "" (no change)
	schemaUpdateDefault     string
	schemaUpdateValues      string
	schemaUpdateTarget      string
	schemaUpdateAddTrait    string
	schemaUpdateRemoveTrait string
)

// Flags for schema remove commands
var (
	schemaRemoveForce bool
)

// Flags for schema rename commands
var (
	schemaRenameConfirm           bool
	schemaRenameDefaultPathRename bool
)

var schemaAddCmd = &cobra.Command{
	Use:   "add <type|trait|field> <name> [parent-type]",
	Short: "Add a type, trait, or field to the schema",
	Long: `Add new definitions to schema.yaml.

Subcommands:
  type <name>              Add a new type
  trait <name>             Add a new trait
  field <type> <field>     Add a field to an existing type

Examples:
  rvn schema add type event --default-path event/
  rvn schema add trait priority --type enum --values high,medium,low
  rvn schema add field person email --type string --required`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		kind := args[0]

		switch kind {
		case "type":
			return addType(vaultPath, args[1], start)
		case "trait":
			return addTrait(vaultPath, args[1], start)
		case "field":
			if len(args) < 3 {
				return handleErrorMsg(ErrMissingArgument, "field requires type and field name", "Usage: rvn schema add field <type> <field>")
			}
			return addField(vaultPath, args[1], args[2], start)
		default:
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema kind: %s", kind), "Use: type, trait, or field")
		}
	},
}

func addType(vaultPath, typeName string, start time.Time) error {
	// Interactive prompt for name_field if not provided and not in JSON mode
	nameField := schemaAddNameField
	if nameField == "" && !isJSONOutput() {
		fmt.Print("Which field should be the display name? (common: name, title; leave blank for none): ")
		var input string
		fmt.Scanln(&input)
		nameField = strings.TrimSpace(input)
	}

	// Default to a path that matches the type name exactly unless explicitly overridden.
	defaultPath := schemaAddDefaultPath
	if strings.TrimSpace(defaultPath) == "" {
		defaultPath = paths.NormalizeDirRoot(typeName)
	}

	result, err := schemasvc.AddType(schemasvc.AddTypeRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		DefaultPath: defaultPath,
		NameField:   nameField,
		Description: schemaAddDescription,
	})
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		data := map[string]interface{}{
			"added":        "type",
			"name":         result.Name,
			"default_path": result.DefaultPath,
		}
		if result.Description != "" {
			data["description"] = result.Description
		}
		if result.NameField != "" {
			data["name_field"] = result.NameField
			data["auto_created_field"] = result.AutoCreatedField
		}
		outputSuccess(data, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Added type '%s' to schema.yaml\n", result.Name)
	fmt.Printf("  default_path: %s\n", result.DefaultPath)
	if result.Description != "" {
		fmt.Printf("  description: %s\n", result.Description)
	}
	if result.NameField != "" {
		fmt.Printf("  name_field: %s (auto-created as required string)\n", result.NameField)
	}
	return nil
}

func addTrait(vaultPath, traitName string, start time.Time) error {
	result, err := schemasvc.AddTrait(schemasvc.AddTraitRequest{
		VaultPath: vaultPath,
		TraitName: traitName,
		TraitType: schemaAddFieldType,
		Values:    schemaAddValues,
		Default:   schemaAddDefault,
	})
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		data := map[string]interface{}{
			"added": "trait",
			"name":  result.Name,
			"type":  result.Type,
		}
		if len(result.Values) > 0 {
			data["values"] = result.Values
		}
		outputSuccess(data, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Added trait '%s' to schema.yaml\n", result.Name)
	fmt.Printf("  type: %s\n", result.Type)
	if len(result.Values) > 0 {
		fmt.Printf("  values: %s\n", schemaAddValues)
	}
	return nil
}

// validFieldTypes are the base field types supported by Raven
var validFieldTypes = map[string]bool{
	"string":   true,
	"number":   true,
	"url":      true,
	"date":     true,
	"datetime": true,
	"bool":     true,
	"enum":     true,
	"ref":      true,
}

func normalizeFieldTypeAlias(baseType string) string {
	switch strings.ToLower(baseType) {
	case "boolean":
		return "bool"
	default:
		return strings.ToLower(baseType)
	}
}

// FieldTypeValidation holds the result of validating a field type specification
type FieldTypeValidation struct {
	Valid      bool
	BaseType   string
	IsArray    bool
	Error      string
	Suggestion string
	Examples   []string
	ValidTypes []string
	TargetHint string
}

// validateFieldTypeSpec validates the --type and related flags for adding a field
func validateFieldTypeSpec(fieldType, target, values string, sch *schema.Schema) FieldTypeValidation {
	result := FieldTypeValidation{
		ValidTypes: []string{"string", "number", "url", "date", "datetime", "bool", "enum", "ref"},
	}

	// Handle empty type (defaults to string)
	if fieldType == "" {
		fieldType = "string"
	}

	// Check for array suffix
	isArray := strings.HasSuffix(fieldType, "[]")
	baseType := normalizeFieldTypeAlias(strings.TrimSuffix(fieldType, "[]"))

	result.BaseType = baseType
	result.IsArray = isArray

	// Check if this looks like a schema type name (common mistake).
	// Do not reject names that are also valid field types (e.g., built-in "date").
	if sch != nil {
		if _, isSchemaType := sch.Types[baseType]; isSchemaType && !validFieldTypes[baseType] {
			result.Error = fmt.Sprintf("'%s' is a type name, not a field type", baseType)
			result.Suggestion = fmt.Sprintf("To reference objects of type '%s', use --type ref --target %s", baseType, baseType)
			if isArray {
				result.Examples = []string{
					fmt.Sprintf("--type ref[] --target %s  (array of %s references)", baseType, baseType),
				}
			} else {
				result.Examples = []string{
					fmt.Sprintf("--type ref --target %s  (single %s reference)", baseType, baseType),
					fmt.Sprintf("--type ref[] --target %s  (array of %s references)", baseType, baseType),
				}
			}
			return result
		}

		// Also check if adding [] to a type name
		if _, isSchemaType := sch.Types[strings.TrimSuffix(baseType, "[]")]; isSchemaType && !validFieldTypes[strings.TrimSuffix(baseType, "[]")] {
			cleanType := strings.TrimSuffix(baseType, "[]")
			result.Error = fmt.Sprintf("'%s' is a type name, not a field type", cleanType)
			result.Suggestion = fmt.Sprintf("To reference an array of '%s' objects, use --type ref[] --target %s", cleanType, cleanType)
			result.Examples = []string{
				fmt.Sprintf("--type ref[] --target %s", cleanType),
			}
			return result
		}
	}

	// Check if base type is valid
	if !validFieldTypes[baseType] {
		result.Error = fmt.Sprintf("'%s' is not a valid field type", fieldType)
		result.Suggestion = "Valid types: string, number, url, date, datetime, bool, enum, ref (add [] suffix for arrays)"
		result.Examples = []string{
			"--type string        (text)",
			"--type string[]      (array of text, e.g., tags)",
			"--type url           (web link)",
			"--type ref --target person   (reference to a person)",
			"--type ref[] --target person (array of person references)",
			"--type enum --values a,b,c   (single choice from list)",
		}
		return result
	}

	// Validate ref type requires target
	if (baseType == "ref") && target == "" {
		result.Error = "ref fields require --target to specify which type they reference"
		result.Suggestion = "Add --target <type_name> to specify the referenced type"
		if sch != nil && len(sch.Types) > 0 {
			var typeNames []string
			for name := range sch.Types {
				typeNames = append(typeNames, name)
			}
			// Sort for consistent output
			sort.Strings(typeNames)
			if len(typeNames) > 3 {
				typeNames = typeNames[:3]
			}
			result.Examples = []string{}
			for _, t := range typeNames {
				if isArray {
					result.Examples = append(result.Examples, fmt.Sprintf("--type ref[] --target %s", t))
				} else {
					result.Examples = append(result.Examples, fmt.Sprintf("--type ref --target %s", t))
				}
			}
		}
		result.TargetHint = "Available types can be listed with 'rvn schema types'"
		return result
	}

	// Validate enum type requires values
	if (baseType == "enum") && values == "" {
		result.Error = "enum fields require --values to specify allowed values"
		result.Suggestion = "Add --values with comma-separated allowed values"
		result.Examples = []string{
			"--type enum --values active,paused,done",
			"--type enum[] --values red,green,blue  (allows multiple selections)",
		}
		return result
	}

	// Warn if target is provided but type is not ref
	if target != "" && baseType != "ref" {
		result.Error = fmt.Sprintf("--target is only valid for ref fields, but type is '%s'", fieldType)
		result.Suggestion = "Either change --type to ref (or ref[]) or remove --target"
		result.Examples = []string{
			fmt.Sprintf("--type ref --target %s  (single reference)", target),
			fmt.Sprintf("--type ref[] --target %s  (array of references)", target),
		}
		return result
	}

	// Validate target type exists if specified
	if target != "" && sch != nil {
		if _, exists := sch.Types[target]; !exists {
			// Check built-in types
			if !schema.IsBuiltinType(target) {
				result.Error = fmt.Sprintf("target type '%s' does not exist in schema", target)
				result.Suggestion = fmt.Sprintf("Either create the type first with 'rvn schema add type %s' or use an existing type", target)
				if len(sch.Types) > 0 {
					var typeNames []string
					for name := range sch.Types {
						typeNames = append(typeNames, name)
					}
					sort.Strings(typeNames)
					result.TargetHint = fmt.Sprintf("Existing types: %s", strings.Join(typeNames, ", "))
				}
				return result
			}
		}
	}

	result.Valid = true
	return result
}

func addField(vaultPath, typeName, fieldName string, start time.Time) error {
	result, err := schemasvc.AddField(schemasvc.AddFieldRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		FieldName:   fieldName,
		FieldType:   schemaAddFieldType,
		Required:    schemaAddRequired,
		Default:     schemaAddDefault,
		Values:      schemaAddValues,
		Target:      schemaAddTarget,
		Description: schemaAddDescription,
	})
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		data := map[string]interface{}{
			"added":      "field",
			"type":       result.TypeName,
			"field":      result.FieldName,
			"field_type": result.FieldType,
			"required":   result.Required,
		}
		if result.Description != "" {
			data["description"] = result.Description
		}
		outputSuccess(data, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Added field '%s' to type '%s'\n", result.FieldName, result.TypeName)
	fmt.Printf("  type: %s\n", result.FieldType)
	if result.Required {
		fmt.Println("  required: true")
	}
	if result.Description != "" {
		fmt.Printf("  description: %s\n", result.Description)
	}
	return nil
}

func mapSchemaServiceError(err error) error {
	var svcErr *schemasvc.Error
	if errors.As(err, &svcErr) {
		suggestion := svcErr.Suggestion
		if !isJSONOutput() && len(svcErr.Details) > 0 {
			if examples := detailExamples(svcErr.Details["examples"]); len(examples) > 0 {
				suggestion += "\n\nExamples:\n"
				for _, ex := range examples {
					suggestion += "  " + ex + "\n"
				}
			}
		}
		if len(svcErr.Details) > 0 {
			return handleErrorWithDetails(string(svcErr.Code), svcErr.Message, suggestion, svcErr.Details)
		}
		return handleErrorMsg(string(svcErr.Code), svcErr.Message, suggestion)
	}
	return handleError(ErrInternal, err, "")
}

func detailExamples(raw interface{}) []string {
	if raw == nil {
		return nil
	}
	switch vals := raw.(type) {
	case []string:
		return vals
	case []interface{}:
		out := make([]string, 0, len(vals))
		for _, v := range vals {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

var schemaValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the schema",
	Long: `Validate schema.yaml for correctness.

Checks:
  - Valid YAML syntax
  - Valid field types
  - Valid trait types
  - Referenced types exist
  - name_field references valid string fields
  - No circular references

Examples:
  rvn schema validate
  rvn schema validate --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()

		// Try to load the schema - this validates most things
		sch, err := schema.Load(vaultPath)
		if err != nil {
			return handleError(ErrSchemaInvalid, err, "Fix the errors and try again")
		}

		// Use the comprehensive validation function
		issues := schema.ValidateSchema(sch)

		elapsed := time.Since(start).Milliseconds()

		if isJSONOutput() {
			result := map[string]interface{}{
				"valid":  len(issues) == 0,
				"issues": issues,
				"types":  len(sch.Types),
				"traits": len(sch.Traits),
			}
			outputSuccess(result, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		if len(issues) > 0 {
			fmt.Printf("Schema validation found %d issues:\n", len(issues))
			for _, issue := range issues {
				fmt.Printf("  ⚠ %s\n", issue)
			}
			return nil
		}

		fmt.Printf("✓ Schema is valid (%d types, %d traits)\n", len(sch.Types), len(sch.Traits))
		return nil
	},
}

// =============================================================================
// UPDATE COMMANDS
// =============================================================================

var schemaUpdateCmd = &cobra.Command{
	Use:   "update <type|trait|field> <name> [parent-type]",
	Short: "Update a type, trait, or field in the schema",
	Long: `Update existing definitions in schema.yaml.

Subcommands:
  type <name>              Update an existing type
  trait <name>             Update an existing trait  
  field <type> <field>     Update a field on an existing type

Examples:
  rvn schema update type person --default-path person/
  rvn schema update trait priority --values critical,high,medium,low
  rvn schema update field person email --required=true
  rvn schema update type meeting --add-trait due`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		kind := args[0]

		switch kind {
		case "type":
			return updateType(vaultPath, args[1], start)
		case "trait":
			return updateTrait(vaultPath, args[1], start)
		case "field":
			if len(args) < 3 {
				return handleErrorMsg(ErrMissingArgument, "field requires type and field name", "Usage: rvn schema update field <type> <field>")
			}
			return updateField(vaultPath, args[1], args[2], start)
		default:
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema kind: %s", kind), "Use: type, trait, or field")
		}
	},
}

func updateType(vaultPath, typeName string, start time.Time) error {
	result, err := schemasvc.UpdateType(schemasvc.UpdateTypeRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		DefaultPath: schemaUpdateDefaultPath,
		NameField:   schemaUpdateNameField,
		Description: schemaUpdateDescription,
		AddTrait:    schemaUpdateAddTrait,
		RemoveTrait: schemaUpdateRemoveTrait,
	})
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"updated": "type",
			"name":    typeName,
			"changes": result.Changes,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Updated type '%s'\n", typeName)
	for _, c := range result.Changes {
		fmt.Printf("  %s\n", c)
	}
	return nil
}

func updateTrait(vaultPath, traitName string, start time.Time) error {
	result, err := schemasvc.UpdateTrait(schemasvc.UpdateTraitRequest{
		VaultPath: vaultPath,
		TraitName: traitName,
		TraitType: schemaUpdateFieldType,
		Values:    schemaUpdateValues,
		Default:   schemaUpdateDefault,
	})
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"updated": "trait",
			"name":    traitName,
			"changes": result.Changes,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Updated trait '%s'\n", traitName)
	for _, c := range result.Changes {
		fmt.Printf("  %s\n", c)
	}
	return nil
}

func updateField(vaultPath, typeName, fieldName string, start time.Time) error {
	result, err := schemasvc.UpdateField(schemasvc.UpdateFieldRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		FieldName:   fieldName,
		FieldType:   schemaUpdateFieldType,
		Required:    schemaUpdateRequired,
		Default:     schemaUpdateDefault,
		Values:      schemaUpdateValues,
		Target:      schemaUpdateTarget,
		Description: schemaUpdateDescription,
	})
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"updated": "field",
			"type":    typeName,
			"field":   fieldName,
			"changes": result.Changes,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Updated field '%s' on type '%s'\n", fieldName, typeName)
	for _, c := range result.Changes {
		fmt.Printf("  %s\n", c)
	}
	return nil
}

// =============================================================================
// REMOVE COMMANDS
// =============================================================================

var schemaRemoveCmd = &cobra.Command{
	Use:   "remove <type|trait|field> <name> [parent-type]",
	Short: "Remove a type, trait, or field from the schema",
	Long: `Remove definitions from schema.yaml.

Subcommands:
  type <name>              Remove a type (objects become 'page' type)
  trait <name>             Remove a trait (existing instances remain in files)
  field <type> <field>     Remove a field from a type

By default, warns about affected files. Use --force to skip warnings.

Examples:
  rvn schema remove type event
  rvn schema remove trait priority --force
  rvn schema remove field person nickname`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		kind := args[0]

		switch kind {
		case "type":
			return removeType(vaultPath, args[1], start)
		case "trait":
			return removeTrait(vaultPath, args[1], start)
		case "field":
			if len(args) < 3 {
				return handleErrorMsg(ErrMissingArgument, "field requires type and field name", "Usage: rvn schema remove field <type> <field>")
			}
			return removeField(vaultPath, args[1], args[2], start)
		default:
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema kind: %s", kind), "Use: type, trait, or field")
		}
	},
}

func removeType(vaultPath, typeName string, start time.Time) error {
	interactive := !isJSONOutput()
	result, err := schemasvc.RemoveType(schemasvc.RemoveTypeRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		Force:       schemaRemoveForce,
		Interactive: interactive,
	})
	if err != nil {
		var svcErr *schemasvc.Error
		if errors.As(err, &svcErr) && svcErr.Code == schemasvc.ErrorConfirmation && !schemaRemoveForce && !isJSONOutput() {
			count := detailInt(svcErr.Details, "affected_count")
			if count > 0 {
				fmt.Printf("Warning: %d files of type '%s' will become 'page' type:\n", count, typeName)
				for _, filePath := range detailStringSlice(svcErr.Details, "affected_files") {
					fmt.Printf("  - %s\n", filePath)
				}
				if remaining := detailInt(svcErr.Details, "remaining_count"); remaining > 0 {
					fmt.Printf("  ... and %d more\n", remaining)
				}
			}
			if !promptForConfirm("Continue?") {
				return handleErrorMsg(ErrConfirmationRequired, "operation cancelled", "Use --force to skip confirmation")
			}
			result, err = schemasvc.RemoveType(schemasvc.RemoveTypeRequest{
				VaultPath:   vaultPath,
				TypeName:    typeName,
				Force:       true,
				Interactive: false,
			})
			if err != nil {
				return mapSchemaServiceError(err)
			}
		} else {
			return mapSchemaServiceError(err)
		}
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		data := map[string]interface{}{
			"removed": "type",
			"name":    typeName,
		}
		outputSuccessWithWarnings(data, schemaWarnings(result.Warnings), &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Removed type '%s' from schema.yaml\n", typeName)
	return nil
}

func removeTrait(vaultPath, traitName string, start time.Time) error {
	interactive := !isJSONOutput()
	result, err := schemasvc.RemoveTrait(schemasvc.RemoveTraitRequest{
		VaultPath:   vaultPath,
		TraitName:   traitName,
		Force:       schemaRemoveForce,
		Interactive: interactive,
	})
	if err != nil {
		var svcErr *schemasvc.Error
		if errors.As(err, &svcErr) && svcErr.Code == schemasvc.ErrorConfirmation && !schemaRemoveForce && !isJSONOutput() {
			count := detailInt(svcErr.Details, "affected_count")
			if count > 0 {
				fmt.Printf("Warning: %d instances of @%s will remain in files (no longer indexed)\n", count, traitName)
			}
			if !promptForConfirm("Continue?") {
				return handleErrorMsg(ErrConfirmationRequired, "operation cancelled", "Use --force to skip confirmation")
			}
			result, err = schemasvc.RemoveTrait(schemasvc.RemoveTraitRequest{
				VaultPath:   vaultPath,
				TraitName:   traitName,
				Force:       true,
				Interactive: false,
			})
			if err != nil {
				return mapSchemaServiceError(err)
			}
		} else {
			return mapSchemaServiceError(err)
		}
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		data := map[string]interface{}{
			"removed": "trait",
			"name":    traitName,
		}
		outputSuccessWithWarnings(data, schemaWarnings(result.Warnings), &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Removed trait '%s' from schema.yaml\n", traitName)
	return nil
}

func removeField(vaultPath, typeName, fieldName string, start time.Time) error {
	_, err := schemasvc.RemoveField(schemasvc.RemoveFieldRequest{
		VaultPath: vaultPath,
		TypeName:  typeName,
		FieldName: fieldName,
	})
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"removed": "field",
			"type":    typeName,
			"field":   fieldName,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Removed field '%s' from type '%s'\n", fieldName, typeName)
	return nil
}

func schemaWarnings(serviceWarnings []schemasvc.Warning) []Warning {
	if len(serviceWarnings) == 0 {
		return nil
	}
	warnings := make([]Warning, 0, len(serviceWarnings))
	for _, warning := range serviceWarnings {
		warnings = append(warnings, Warning{
			Code:    warning.Code,
			Message: warning.Message,
		})
	}
	return warnings
}

func detailInt(details map[string]interface{}, key string) int {
	if details == nil {
		return 0
	}
	value, ok := details[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func detailStringSlice(details map[string]interface{}, key string) []string {
	if details == nil {
		return nil
	}
	raw, ok := details[key]
	if !ok {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return typed
	case []interface{}:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				values = append(values, str)
			}
		}
		return values
	default:
		return nil
	}
}

// =============================================================================
// RENAME COMMANDS
// =============================================================================

var schemaRenameCmd = &cobra.Command{
	Use:   "rename <type|field> ...",
	Short: "Rename a type or field and update references",
	Long: `Rename a type or a field in the schema and update downstream usages.

Subcommands:
  type  <old_name> <new_name>
  field <type> <old_field> <new_field>

Rename type updates:
1. Type definition key in schema.yaml
2. All 'type:' frontmatter fields
3. All ::type() embedded declarations
4. All ref field targets pointing to the old type

Rename field updates:
1. Field key in schema.yaml for the target type
2. If name_field == old_field, updates it to new_field
3. Type templates referencing {{field.old_field}} (template files)
4. Object frontmatter keys for files with type:<type>
5. Field keys inside ::type(...) declarations (only for that type)
6. Saved queries in raven.yaml (best-effort for object:<type> queries)

By default, previews changes. Use --confirm to apply.
When type default_path clearly matches the type name, you can also rename
that directory with --rename-default-path.

Examples:
  rvn schema rename type event meeting
  rvn schema rename type event meeting --confirm

  rvn schema rename field person email email_address
  rvn schema rename field person email email_address --confirm`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		kind := args[0]

		switch kind {
		case "type":
			if len(args) != 3 {
				return handleErrorMsg(ErrMissingArgument, "type rename requires old and new names", "Usage: rvn schema rename type <old_name> <new_name>")
			}
			return renameType(vaultPath, args[1], args[2], start)
		case "field":
			if len(args) != 4 {
				return handleErrorMsg(ErrMissingArgument, "field rename requires type, old field, and new field", "Usage: rvn schema rename field <type> <old_field> <new_field>")
			}
			return renameField(vaultPath, args[1], args[2], args[3], start)
		default:
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema kind: %s", kind), "Currently supported: type, field")
		}
	},
}

func renameField(vaultPath, typeName, oldField, newField string, start time.Time) error {
	result, err := schemasvc.RenameField(schemasvc.RenameFieldRequest{
		VaultPath: vaultPath,
		TypeName:  typeName,
		OldField:  oldField,
		NewField:  newField,
		Confirm:   schemaRenameConfirm,
	})
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()
	if isJSONOutput() {
		if result.Preview {
			outputSuccess(map[string]interface{}{
				"preview":       true,
				"type":          result.TypeName,
				"old_field":     result.OldField,
				"new_field":     result.NewField,
				"total_changes": result.TotalChanges,
				"changes":       result.Changes,
				"hint":          result.Hint,
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		outputSuccess(map[string]interface{}{
			"renamed":         true,
			"type":            result.TypeName,
			"old_field":       result.OldField,
			"new_field":       result.NewField,
			"changes_applied": result.ChangesApplied,
			"hint":            result.Hint,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	if result.Preview {
		fmt.Printf("Preview: Rename field '%s.%s' to '%s.%s'\n\n", result.TypeName, result.OldField, result.TypeName, result.NewField)
		fmt.Printf("Changes to be made (%d total):\n", result.TotalChanges)
		printFieldRenameChanges(result.Changes)
		fmt.Printf("\nRun with --confirm to apply these changes.\n")
		return nil
	}

	fmt.Printf("✓ Renamed field '%s.%s' to '%s.%s'\n", result.TypeName, result.OldField, result.TypeName, result.NewField)
	fmt.Printf("  Applied %d changes\n", result.ChangesApplied)
	fmt.Printf("\n%s.\n", result.Hint)
	return nil
}

func renameType(vaultPath, oldName, newName string, start time.Time) error {
	preview, err := schemasvc.RenameType(schemasvc.RenameTypeRequest{
		VaultPath: vaultPath,
		OldName:   oldName,
		NewName:   newName,
		Confirm:   false,
	})
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed := time.Since(start).Milliseconds()
	if !schemaRenameConfirm {
		if isJSONOutput() {
			data := map[string]interface{}{
				"preview":       true,
				"old_name":      preview.OldName,
				"new_name":      preview.NewName,
				"total_changes": preview.TotalChanges,
				"changes":       preview.Changes,
				"hint":          preview.Hint,
			}
			if preview.DefaultPathRenameAvailable {
				data["default_path_rename_available"] = true
				data["default_path_old"] = preview.DefaultPathOld
				data["default_path_new"] = preview.DefaultPathNew
				data["optional_total_changes"] = preview.OptionalTotalChanges
				data["optional_changes"] = preview.OptionalChanges
				data["files_to_move"] = preview.FilesToMove
			}
			outputSuccess(data, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		fmt.Printf("Preview: Rename type '%s' to '%s'\n\n", preview.OldName, preview.NewName)
		fmt.Printf("Changes to be made (%d total):\n", preview.TotalChanges)
		printTypeRenameChanges(preview.Changes)
		if preview.DefaultPathRenameAvailable {
			fmt.Printf("\nOptional default directory rename (%d changes):\n", preview.OptionalTotalChanges)
			fmt.Printf("  default_path: %s → %s\n", preview.DefaultPathOld, preview.DefaultPathNew)
			if preview.FilesToMove > 0 {
				fmt.Printf("  files to move: %d\n", preview.FilesToMove)
			}
			fmt.Printf("  (add --rename-default-path to apply these optional changes)\n")
		}
		fmt.Printf("\nRun with --confirm to apply these changes.\n")
		return nil
	}

	applyDefaultPathRename := schemaRenameDefaultPathRename
	if preview.DefaultPathRenameAvailable && !applyDefaultPathRename && shouldPromptForConfirm() {
		prompt := fmt.Sprintf("Also rename default_path '%s' -> '%s'?", preview.DefaultPathOld, preview.DefaultPathNew)
		if preview.FilesToMove > 0 {
			prompt = fmt.Sprintf(
				"Also rename default_path '%s' -> '%s' and move %d files with reference updates?",
				preview.DefaultPathOld,
				preview.DefaultPathNew,
				preview.FilesToMove,
			)
		}
		applyDefaultPathRename = promptForConfirm(prompt)
	}

	result, err := schemasvc.RenameType(schemasvc.RenameTypeRequest{
		VaultPath:         vaultPath,
		OldName:           oldName,
		NewName:           newName,
		Confirm:           true,
		RenameDefaultPath: applyDefaultPathRename,
	})
	if err != nil {
		return mapSchemaServiceError(err)
	}

	elapsed = time.Since(start).Milliseconds()
	if isJSONOutput() {
		data := map[string]interface{}{
			"renamed":         true,
			"old_name":        result.OldName,
			"new_name":        result.NewName,
			"changes_applied": result.ChangesApplied,
			"hint":            result.Hint,
		}
		if result.DefaultPathRenameAvailable {
			data["default_path_rename_available"] = true
			data["default_path_renamed"] = result.DefaultPathRenamed
			data["default_path_old"] = result.DefaultPathOld
			data["default_path_new"] = result.DefaultPathNew
			data["files_moved"] = result.FilesMoved
			data["reference_files_updated"] = result.ReferenceFilesUpdated
		}
		outputSuccess(data, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Renamed type '%s' to '%s'\n", result.OldName, result.NewName)
	fmt.Printf("  Applied %d changes\n", result.ChangesApplied)
	if result.DefaultPathRenameAvailable {
		if result.DefaultPathRenamed {
			fmt.Printf("  Renamed default_path %s → %s\n", result.DefaultPathOld, result.DefaultPathNew)
			fmt.Printf("  Moved %d files and updated references in %d files\n", result.FilesMoved, result.ReferenceFilesUpdated)
		} else {
			fmt.Printf("  Default path remains %s (use --rename-default-path to rename to %s)\n", result.DefaultPathOld, result.DefaultPathNew)
		}
	}
	fmt.Printf("\n%s.\n", result.Hint)
	return nil
}

func printFieldRenameChanges(changes []schemasvc.FieldRenameChange) {
	byFile := make(map[string][]schemasvc.FieldRenameChange)
	for _, change := range changes {
		byFile[change.FilePath] = append(byFile[change.FilePath], change)
	}

	files := make([]string, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}
	sort.Strings(files)

	for _, file := range files {
		fileChanges := byFile[file]
		fmt.Printf("\n  %s:\n", file)
		for _, change := range fileChanges {
			if change.Line > 0 {
				fmt.Printf("    Line %d: %s\n", change.Line, change.Description)
			} else {
				fmt.Printf("    %s\n", change.Description)
			}
		}
	}
}

func printTypeRenameChanges(changes []schemasvc.TypeRenameChange) {
	byFile := make(map[string][]schemasvc.TypeRenameChange)
	for _, change := range changes {
		byFile[change.FilePath] = append(byFile[change.FilePath], change)
	}

	files := make([]string, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}
	sort.Strings(files)

	for _, file := range files {
		fileChanges := byFile[file]
		fmt.Printf("\n  %s:\n", file)
		for _, change := range fileChanges {
			if change.Line > 0 {
				fmt.Printf("    Line %d: %s\n", change.Line, change.Description)
			} else {
				fmt.Printf("    %s\n", change.Description)
			}
		}
	}
}

func init() {
	// Add flags to schema add command
	schemaAddCmd.Flags().StringVar(&schemaAddDefaultPath, "default-path", "", "Default path for new type files (default: <type>/)")
	schemaAddCmd.Flags().StringVar(&schemaAddNameField, "name-field", "", "Field to use as display name (auto-created if doesn't exist)")
	schemaAddCmd.Flags().StringVar(&schemaAddDescription, "description", "", "Optional description for the type or field")
	schemaAddCmd.Flags().StringVar(&schemaAddFieldType, "type", "", "Field/trait type (string, number, url, date, datetime, enum, ref, bool)")
	schemaAddCmd.Flags().BoolVar(&schemaAddRequired, "required", false, "Mark field as required")
	schemaAddCmd.Flags().StringVar(&schemaAddDefault, "default", "", "Default value")
	schemaAddCmd.Flags().StringVar(&schemaAddValues, "values", "", "Enum values (comma-separated)")
	schemaAddCmd.Flags().StringVar(&schemaAddTarget, "target", "", "Target type for ref fields")

	// Add flags to schema update command
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateDefaultPath, "default-path", "", "Update default path for type")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateNameField, "name-field", "", "Set/update display name field (use '-' to remove)")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateDescription, "description", "", "Set/update description (use '-' to remove)")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateFieldType, "type", "", "Update field/trait type")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateRequired, "required", "", "Update required status (true/false)")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateDefault, "default", "", "Update default value")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateValues, "values", "", "Update enum values (comma-separated)")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateTarget, "target", "", "Update target type for ref fields")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateAddTrait, "add-trait", "", "Add a trait to the type")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateRemoveTrait, "remove-trait", "", "Remove a trait from the type")

	// Add flags to schema remove command
	schemaRemoveCmd.Flags().BoolVar(&schemaRemoveForce, "force", false, "Skip confirmation prompts")

	// Add flags to schema rename command
	schemaRenameCmd.Flags().BoolVar(&schemaRenameConfirm, "confirm", false, "Apply the rename (default: preview only)")
	schemaRenameCmd.Flags().BoolVar(&schemaRenameDefaultPathRename, "rename-default-path", false, "Also rename type default_path directory and move matching files (with reference updates)")

	// Add subcommands to schema command
	schemaCmd.AddCommand(schemaAddCmd)
	schemaCmd.AddCommand(schemaUpdateCmd)
	schemaCmd.AddCommand(schemaRemoveCmd)
	schemaCmd.AddCommand(schemaRenameCmd)
	schemaCmd.AddCommand(schemaValidateCmd)
}
