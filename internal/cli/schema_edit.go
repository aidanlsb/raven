package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/audit"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Flags for schema add commands
var (
	schemaAddDefaultPath string
	schemaAddFieldType   string
	schemaAddRequired    bool
	schemaAddDefault     string
	schemaAddValues      string
	schemaAddTarget      string
)

// Flags for schema update commands
var (
	schemaUpdateDefaultPath string
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

// logSchemaChange logs a schema modification to the audit log.
func logSchemaChange(vaultPath, operation, kind, name string, changes []string) {
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return // Vault config not loaded, skip audit
	}
	logger := audit.New(vaultPath, vaultCfg.IsAuditLogEnabled())
	if logger == nil || !logger.Enabled() {
		return
	}
	extra := map[string]interface{}{
		"kind": kind,
	}
	if changes != nil {
		extra["changes"] = changes
	}
	logger.Log(audit.Entry{
		Operation: "schema_" + operation,
		Entity:    "schema",
		ID:        name,
		Extra:     extra,
	})
}

var schemaAddCmd = &cobra.Command{
	Use:   "add <type|trait|field> <name> [parent-type]",
	Short: "Add a type, trait, or field to the schema",
	Long: `Add new definitions to schema.yaml.

Subcommands:
  type <name>              Add a new type
  trait <name>             Add a new trait
  field <type> <field>     Add a field to an existing type

Examples:
  rvn schema add type event --default-path events/
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
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if type already exists
	if _, exists := sch.Types[typeName]; exists {
		return handleErrorMsg(ErrObjectExists, fmt.Sprintf("type '%s' already exists", typeName), "")
	}

	// Check built-in types
	if typeName == "page" || typeName == "section" || typeName == "date" {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type", typeName), "Choose a different name")
	}

	// Read current schema file to preserve formatting
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	// Parse as YAML to modify
	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	// Ensure types map exists
	types, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		types = make(map[string]interface{})
		schemaDoc["types"] = types
	}

	// Build new type definition
	newType := make(map[string]interface{})
	if schemaAddDefaultPath != "" {
		newType["default_path"] = schemaAddDefaultPath
	}

	types[typeName] = newType

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"added":        "type",
			"name":         typeName,
			"default_path": schemaAddDefaultPath,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Added type '%s' to schema.yaml\n", typeName)
	if schemaAddDefaultPath != "" {
		fmt.Printf("  default_path: %s\n", schemaAddDefaultPath)
	}
	return nil
}

func addTrait(vaultPath, traitName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if trait already exists
	if _, exists := sch.Traits[traitName]; exists {
		return handleErrorMsg(ErrObjectExists, fmt.Sprintf("trait '%s' already exists", traitName), "")
	}

	// Read current schema file
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	// Ensure traits map exists
	traits, ok := schemaDoc["traits"].(map[string]interface{})
	if !ok {
		traits = make(map[string]interface{})
		schemaDoc["traits"] = traits
	}

	// Build new trait definition
	newTrait := make(map[string]interface{})

	// Default type is string
	traitType := schemaAddFieldType
	if traitType == "" {
		traitType = "string"
	}
	newTrait["type"] = traitType

	if schemaAddValues != "" {
		newTrait["values"] = strings.Split(schemaAddValues, ",")
	}
	if schemaAddDefault != "" {
		newTrait["default"] = schemaAddDefault
	}

	traits[traitName] = newTrait

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		result := map[string]interface{}{
			"added": "trait",
			"name":  traitName,
			"type":  traitType,
		}
		if schemaAddValues != "" {
			result["values"] = strings.Split(schemaAddValues, ",")
		}
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Added trait '%s' to schema.yaml\n", traitName)
	fmt.Printf("  type: %s\n", traitType)
	if schemaAddValues != "" {
		fmt.Printf("  values: %s\n", schemaAddValues)
	}
	return nil
}

func addField(vaultPath, typeName, fieldName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if type exists
	typeDef, exists := sch.Types[typeName]
	if !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "Add the type first with 'rvn schema add type'")
	}

	// Check if field already exists
	if typeDef.Fields != nil {
		if _, exists := typeDef.Fields[fieldName]; exists {
			return handleErrorMsg(ErrObjectExists, fmt.Sprintf("field '%s' already exists on type '%s'", fieldName, typeName), "")
		}
	}

	// Read current schema file
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	// Navigate to type definition
	types, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrSchemaInvalid, "types section not found", "")
	}

	typeNode, ok := types[typeName].(map[string]interface{})
	if !ok {
		typeNode = make(map[string]interface{})
		types[typeName] = typeNode
	}

	// Ensure fields map exists
	fields, ok := typeNode["fields"].(map[string]interface{})
	if !ok {
		fields = make(map[string]interface{})
		typeNode["fields"] = fields
	}

	// Build new field definition
	newField := make(map[string]interface{})

	fieldType := schemaAddFieldType
	if fieldType == "" {
		fieldType = "string"
	}
	newField["type"] = fieldType

	if schemaAddRequired {
		newField["required"] = true
	}
	if schemaAddDefault != "" {
		newField["default"] = schemaAddDefault
	}
	if schemaAddValues != "" {
		newField["values"] = strings.Split(schemaAddValues, ",")
	}
	if schemaAddTarget != "" {
		newField["target"] = schemaAddTarget
	}

	fields[fieldName] = newField

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		result := map[string]interface{}{
			"added":    "field",
			"type":     typeName,
			"field":    fieldName,
			"field_type": fieldType,
			"required": schemaAddRequired,
		}
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Added field '%s' to type '%s'\n", fieldName, typeName)
	fmt.Printf("  type: %s\n", fieldType)
	if schemaAddRequired {
		fmt.Println("  required: true")
	}
	return nil
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

		// Additional validation
		var issues []string

		// Check that ref targets exist
		for typeName, typeDef := range sch.Types {
			if typeDef.Fields != nil {
				for fieldName, fieldDef := range typeDef.Fields {
					if fieldDef != nil && fieldDef.Type == schema.FieldTypeRef && fieldDef.Target != "" {
						if _, exists := sch.Types[fieldDef.Target]; !exists {
							issues = append(issues, fmt.Sprintf("Type '%s' field '%s' references unknown type '%s'", typeName, fieldName, fieldDef.Target))
						}
					}
				}
			}

			// Check that declared traits exist
			for _, traitName := range typeDef.Traits.List() {
				if _, exists := sch.Traits[traitName]; !exists {
					issues = append(issues, fmt.Sprintf("Type '%s' declares unknown trait '%s'", typeName, traitName))
				}
			}
		}

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
  rvn schema update type person --default-path people/
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
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Check built-in types
	if typeName == "page" || typeName == "section" || typeName == "date" {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type and cannot be modified", typeName), "")
	}

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if type exists
	if _, exists := sch.Types[typeName]; !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "Use 'rvn schema add type' to create it")
	}

	// Read current schema file
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	types := schemaDoc["types"].(map[string]interface{})
	typeNode, ok := types[typeName].(map[string]interface{})
	if !ok {
		typeNode = make(map[string]interface{})
		types[typeName] = typeNode
	}

	changes := []string{}

	// Apply updates
	if schemaUpdateDefaultPath != "" {
		typeNode["default_path"] = schemaUpdateDefaultPath
		changes = append(changes, fmt.Sprintf("default_path=%s", schemaUpdateDefaultPath))
	}

	// Handle trait additions/removals
	if schemaUpdateAddTrait != "" {
		// Check trait exists
		if _, exists := sch.Traits[schemaUpdateAddTrait]; !exists {
			return handleErrorMsg(ErrTraitNotFound, fmt.Sprintf("trait '%s' not found", schemaUpdateAddTrait), "Add it first with 'rvn schema add trait'")
		}

		traits, ok := typeNode["traits"].([]interface{})
		if !ok {
			traits = []interface{}{}
		}
		// Check if already present
		found := false
		for _, t := range traits {
			if t.(string) == schemaUpdateAddTrait {
				found = true
				break
			}
		}
		if !found {
			traits = append(traits, schemaUpdateAddTrait)
			typeNode["traits"] = traits
			changes = append(changes, fmt.Sprintf("added trait %s", schemaUpdateAddTrait))
		}
	}

	if schemaUpdateRemoveTrait != "" {
		traits, ok := typeNode["traits"].([]interface{})
		if ok {
			newTraits := []interface{}{}
			for _, t := range traits {
				if t.(string) != schemaUpdateRemoveTrait {
					newTraits = append(newTraits, t)
				}
			}
			typeNode["traits"] = newTraits
			changes = append(changes, fmt.Sprintf("removed trait %s", schemaUpdateRemoveTrait))
		}
	}

	if len(changes) == 0 {
		return handleErrorMsg(ErrInvalidInput, "no changes specified", "Use flags like --default-path, --add-trait, --remove-trait")
	}

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	// Audit log
	logSchemaChange(vaultPath, "update", "type", typeName, changes)

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"updated": "type",
			"name":    typeName,
			"changes": changes,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Updated type '%s'\n", typeName)
	for _, c := range changes {
		fmt.Printf("  %s\n", c)
	}
	return nil
}

func updateTrait(vaultPath, traitName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if trait exists
	if _, exists := sch.Traits[traitName]; !exists {
		return handleErrorMsg(ErrTraitNotFound, fmt.Sprintf("trait '%s' not found", traitName), "Use 'rvn schema add trait' to create it")
	}

	// Read current schema file
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	traits := schemaDoc["traits"].(map[string]interface{})
	traitNode, ok := traits[traitName].(map[string]interface{})
	if !ok {
		traitNode = make(map[string]interface{})
		traits[traitName] = traitNode
	}

	changes := []string{}

	// Apply updates
	if schemaUpdateFieldType != "" {
		traitNode["type"] = schemaUpdateFieldType
		changes = append(changes, fmt.Sprintf("type=%s", schemaUpdateFieldType))
	}

	if schemaUpdateValues != "" {
		traitNode["values"] = strings.Split(schemaUpdateValues, ",")
		changes = append(changes, fmt.Sprintf("values=%s", schemaUpdateValues))
	}

	if schemaUpdateDefault != "" {
		traitNode["default"] = schemaUpdateDefault
		changes = append(changes, fmt.Sprintf("default=%s", schemaUpdateDefault))
	}

	if len(changes) == 0 {
		return handleErrorMsg(ErrInvalidInput, "no changes specified", "Use flags like --type, --values, --default")
	}

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	// Audit log
	logSchemaChange(vaultPath, "update", "trait", traitName, changes)

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"updated": "trait",
			"name":    traitName,
			"changes": changes,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Updated trait '%s'\n", traitName)
	for _, c := range changes {
		fmt.Printf("  %s\n", c)
	}
	return nil
}

func updateField(vaultPath, typeName, fieldName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if type exists
	typeDef, exists := sch.Types[typeName]
	if !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "")
	}

	// Check if field exists
	if typeDef.Fields == nil {
		return handleErrorMsg(ErrFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName), "Use 'rvn schema add field' to create it")
	}
	if _, exists := typeDef.Fields[fieldName]; !exists {
		return handleErrorMsg(ErrFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName), "Use 'rvn schema add field' to create it")
	}

	// Check data integrity for required changes
	if schemaUpdateRequired == "true" {
		// This would make the field required - check all objects have it
		db, err := index.Open(vaultPath)
		if err == nil {
			defer db.Close()
			objects, err := db.QueryObjects(typeName)
			if err == nil && len(objects) > 0 {
				var missing []string
				for _, obj := range objects {
					var fields map[string]interface{}
					if obj.Fields != "" && obj.Fields != "{}" {
						json.Unmarshal([]byte(obj.Fields), &fields)
					}
					if _, hasField := fields[fieldName]; !hasField {
						missing = append(missing, obj.ID)
					}
				}
				if len(missing) > 0 {
					details := map[string]interface{}{
						"missing_field":   fieldName,
						"affected_count":  len(missing),
						"affected_objects": missing,
					}
					if len(missing) > 5 {
						details["affected_objects"] = append(missing[:5], "... and more")
					}
					return handleErrorWithDetails(ErrDataIntegrityBlock, 
						fmt.Sprintf("%d objects of type '%s' lack field '%s'", len(missing), typeName, fieldName),
						"Add the field to these files, then retry",
						details)
				}
			}
		}
	}

	// Read current schema file
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	types := schemaDoc["types"].(map[string]interface{})
	typeNode := types[typeName].(map[string]interface{})
	fields := typeNode["fields"].(map[string]interface{})
	fieldNode, ok := fields[fieldName].(map[string]interface{})
	if !ok {
		fieldNode = make(map[string]interface{})
		fields[fieldName] = fieldNode
	}

	changes := []string{}

	// Apply updates
	if schemaUpdateFieldType != "" {
		fieldNode["type"] = schemaUpdateFieldType
		changes = append(changes, fmt.Sprintf("type=%s", schemaUpdateFieldType))
	}

	if schemaUpdateRequired != "" {
		required := schemaUpdateRequired == "true"
		fieldNode["required"] = required
		changes = append(changes, fmt.Sprintf("required=%v", required))
	}

	if schemaUpdateDefault != "" {
		fieldNode["default"] = schemaUpdateDefault
		changes = append(changes, fmt.Sprintf("default=%s", schemaUpdateDefault))
	}

	if schemaUpdateValues != "" {
		fieldNode["values"] = strings.Split(schemaUpdateValues, ",")
		changes = append(changes, fmt.Sprintf("values=%s", schemaUpdateValues))
	}

	if schemaUpdateTarget != "" {
		fieldNode["target"] = schemaUpdateTarget
		changes = append(changes, fmt.Sprintf("target=%s", schemaUpdateTarget))
	}

	if len(changes) == 0 {
		return handleErrorMsg(ErrInvalidInput, "no changes specified", "Use flags like --type, --required, --default")
	}

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	// Audit log
	logSchemaChange(vaultPath, "update", "field", fmt.Sprintf("%s.%s", typeName, fieldName), changes)

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"updated": "field",
			"type":    typeName,
			"field":   fieldName,
			"changes": changes,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Updated field '%s' on type '%s'\n", fieldName, typeName)
	for _, c := range changes {
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
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Check built-in types
	if typeName == "page" || typeName == "section" || typeName == "date" {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type and cannot be removed", typeName), "")
	}

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if type exists
	if _, exists := sch.Types[typeName]; !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "")
	}

	// Check for affected objects
	var warnings []Warning
	db, err := index.Open(vaultPath)
	if err == nil {
		defer db.Close()
		objects, err := db.QueryObjects(typeName)
		if err == nil && len(objects) > 0 {
				warning := Warning{
				Code:    "ORPHANED_FILES",
				Message: fmt.Sprintf("%d files of type '%s' will become 'page' type", len(objects), typeName),
			}
			warnings = append(warnings, warning)

			if !schemaRemoveForce && !isJSONOutput() {
				fmt.Printf("Warning: %d files of type '%s' will become 'page' type:\n", len(objects), typeName)
				for i, obj := range objects {
					if i >= 5 {
						fmt.Printf("  ... and %d more\n", len(objects)-5)
						break
					}
					fmt.Printf("  - %s\n", obj.FilePath)
				}
				fmt.Print("Continue? [y/N] ")
				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) != "y" {
					return handleErrorMsg(ErrConfirmationRequired, "operation cancelled", "Use --force to skip confirmation")
				}
			}
		}
	}

	// Read current schema file
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	types := schemaDoc["types"].(map[string]interface{})
	delete(types, typeName)

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	// Audit log
	logSchemaChange(vaultPath, "remove", "type", typeName, nil)

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		result := map[string]interface{}{
			"removed": "type",
			"name":    typeName,
		}
		outputSuccessWithWarnings(result, warnings, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Removed type '%s' from schema.yaml\n", typeName)
	return nil
}

func removeTrait(vaultPath, traitName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if trait exists
	if _, exists := sch.Traits[traitName]; !exists {
		return handleErrorMsg(ErrTraitNotFound, fmt.Sprintf("trait '%s' not found", traitName), "")
	}

	// Check for affected trait instances
	var warnings []Warning
	db, err := index.Open(vaultPath)
	if err == nil {
		defer db.Close()
		instances, err := db.QueryTraits(traitName, nil)
		if err == nil && len(instances) > 0 {
			warning := Warning{
				Code:    "ORPHANED_TRAITS",
				Message: fmt.Sprintf("%d instances of @%s will remain in files (no longer indexed)", len(instances), traitName),
			}
			warnings = append(warnings, warning)

			if !schemaRemoveForce && !isJSONOutput() {
				fmt.Printf("Warning: %d instances of @%s will remain in files (no longer indexed)\n", len(instances), traitName)
				fmt.Print("Continue? [y/N] ")
				var response string
				fmt.Scanln(&response)
				if strings.ToLower(response) != "y" {
					return handleErrorMsg(ErrConfirmationRequired, "operation cancelled", "Use --force to skip confirmation")
				}
			}
		}
	}

	// Check if any types declare this trait
	for typeNameCheck, typeDef := range sch.Types {
		for _, t := range typeDef.Traits.List() {
			if t == traitName {
				warnings = append(warnings, Warning{
					Code:    "TYPE_USES_TRAIT",
					Message: fmt.Sprintf("type '%s' declares trait '%s'", typeNameCheck, traitName),
				})
			}
		}
	}

	// Read current schema file
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	traits := schemaDoc["traits"].(map[string]interface{})
	delete(traits, traitName)

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	// Audit log
	logSchemaChange(vaultPath, "remove", "trait", traitName, nil)

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		result := map[string]interface{}{
			"removed": "trait",
			"name":    traitName,
		}
		outputSuccessWithWarnings(result, warnings, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Removed trait '%s' from schema.yaml\n", traitName)
	return nil
}

func removeField(vaultPath, typeName, fieldName string, start time.Time) error {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Load existing schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check if type exists
	typeDef, exists := sch.Types[typeName]
	if !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "")
	}

	// Check if field exists
	if typeDef.Fields == nil {
		return handleErrorMsg(ErrFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName), "")
	}
	fieldDef, exists := typeDef.Fields[fieldName]
	if !exists {
		return handleErrorMsg(ErrFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", fieldName, typeName), "")
	}

	// If field is required, block removal unless user fixes files first
	if fieldDef.Required {
		db, err := index.Open(vaultPath)
		if err == nil {
			defer db.Close()
			objects, err := db.QueryObjects(typeName)
			if err == nil && len(objects) > 0 {
				return handleErrorWithDetails(ErrDataIntegrityBlock,
					fmt.Sprintf("cannot remove required field '%s': %d objects have this field", fieldName, len(objects)),
					"First make the field optional with 'rvn schema update field', then remove it",
					map[string]interface{}{
						"field":          fieldName,
						"type":           typeName,
						"affected_count": len(objects),
					})
			}
		}
	}

	// Read current schema file
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	types := schemaDoc["types"].(map[string]interface{})
	typeNode := types[typeName].(map[string]interface{})
	fields := typeNode["fields"].(map[string]interface{})
	delete(fields, fieldName)

	// If fields is empty, remove the fields key entirely
	if len(fields) == 0 {
		delete(typeNode, "fields")
	}

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := os.WriteFile(schemaPath, output, 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	// Audit log
	logSchemaChange(vaultPath, "remove", "field", fmt.Sprintf("%s.%s", typeName, fieldName), nil)

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

func init() {
	// Add flags to schema add command
	schemaAddCmd.Flags().StringVar(&schemaAddDefaultPath, "default-path", "", "Default path for new type files")
	schemaAddCmd.Flags().StringVar(&schemaAddFieldType, "type", "", "Field/trait type (string, date, enum, ref, bool)")
	schemaAddCmd.Flags().BoolVar(&schemaAddRequired, "required", false, "Mark field as required")
	schemaAddCmd.Flags().StringVar(&schemaAddDefault, "default", "", "Default value")
	schemaAddCmd.Flags().StringVar(&schemaAddValues, "values", "", "Enum values (comma-separated)")
	schemaAddCmd.Flags().StringVar(&schemaAddTarget, "target", "", "Target type for ref fields")

	// Add flags to schema update command
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateDefaultPath, "default-path", "", "Update default path for type")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateFieldType, "type", "", "Update field/trait type")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateRequired, "required", "", "Update required status (true/false)")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateDefault, "default", "", "Update default value")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateValues, "values", "", "Update enum values (comma-separated)")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateTarget, "target", "", "Update target type for ref fields")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateAddTrait, "add-trait", "", "Add a trait to the type")
	schemaUpdateCmd.Flags().StringVar(&schemaUpdateRemoveTrait, "remove-trait", "", "Remove a trait from the type")

	// Add flags to schema remove command
	schemaRemoveCmd.Flags().BoolVar(&schemaRemoveForce, "force", false, "Skip confirmation prompts")

	// Add subcommands to schema command
	schemaCmd.AddCommand(schemaAddCmd)
	schemaCmd.AddCommand(schemaUpdateCmd)
	schemaCmd.AddCommand(schemaRemoveCmd)
	schemaCmd.AddCommand(schemaValidateCmd)
}
