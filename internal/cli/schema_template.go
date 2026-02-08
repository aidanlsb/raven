package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/slugs"
	"github.com/aidanlsb/raven/internal/template"
)

// Flags for schema template commands
var (
	schemaTemplateContent string
	schemaTemplateFile    string
	schemaTemplateTitle   string
	schemaTemplateFields  []string
)

var schemaTemplateCmd = &cobra.Command{
	Use:   "template <get|set|remove|render> <type>",
	Short: "Manage type templates",
	Long: `Manage templates for types in schema.yaml.

Subcommands:
  get <type>        Show the current template for a type
  set <type>        Set or update a type's template
  remove <type>     Remove the template from a type
  render <type>     Preview template with variables applied

Examples:
  rvn schema template get meeting --json
  rvn schema template set meeting --content "# {{title}}" --json
  rvn schema template set meeting --file templates/meeting.md --json
  rvn schema template remove meeting --json
  rvn schema template render meeting --title "Standup" --json`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		action := args[0]

		switch action {
		case "get":
			return templateGet(vaultPath, args[1], start)
		case "set":
			return templateSet(vaultPath, args[1], start)
		case "remove":
			return templateRemove(vaultPath, args[1], start)
		case "render":
			return templateRender(vaultPath, args[1], start)
		default:
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown template action: %s", action), "Use: get, set, remove, or render")
		}
	},
}

func templateGet(vaultPath, typeName string, start time.Time) error {
	// Check built-in types
	if schema.IsBuiltinType(typeName) {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type", typeName), "Built-in types do not support custom templates")
	}

	// Load schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check type exists
	typeDef, exists := sch.Types[typeName]
	if !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "Run 'rvn schema types' to see available types")
	}

	elapsed := time.Since(start).Milliseconds()

	spec := ""
	if typeDef != nil {
		spec = typeDef.Template
	}

	// Determine source and resolve content
	source := "none"
	content := ""
	if spec != "" {
		if looksLikeTemplatePath(spec) {
			source = "file"
		} else {
			source = "inline"
		}
		// Load the resolved content
		loaded, err := template.Load(vaultPath, spec)
		if err == nil {
			content = loaded
		}
	}

	if isJSONOutput() {
		result := map[string]interface{}{
			"type":   typeName,
			"source": source,
		}
		if spec != "" {
			result["spec"] = spec
		}
		if content != "" {
			result["content"] = content
		}
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	if source == "none" {
		fmt.Printf("Type '%s' has no template configured.\n", typeName)
		return nil
	}

	fmt.Printf("Type: %s\n", typeName)
	fmt.Printf("Source: %s\n", source)
	if source == "file" {
		fmt.Printf("File: %s\n", spec)
	}
	fmt.Printf("\nTemplate content:\n%s\n", content)
	return nil
}

func templateSet(vaultPath, typeName string, start time.Time) error {
	// Check built-in types
	if schema.IsBuiltinType(typeName) {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type and cannot be modified", typeName), "")
	}

	// Validate mutually exclusive flags
	if schemaTemplateContent == "" && schemaTemplateFile == "" {
		return handleErrorMsg(ErrMissingArgument, "either --content or --file is required", "Use --content for inline templates or --file for file-based templates")
	}
	if schemaTemplateContent != "" && schemaTemplateFile != "" {
		return handleErrorMsg(ErrInvalidInput, "--content and --file are mutually exclusive", "Use one or the other")
	}

	// Load schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check type exists
	if _, exists := sch.Types[typeName]; !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "Use 'rvn schema add type' to create it")
	}

	// Determine the template spec to store
	var templateSpec string
	var sourceType string

	if schemaTemplateFile != "" {
		// File-based template: validate the file exists within the vault
		fullPath := filepath.Join(vaultPath, schemaTemplateFile)
		if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
			return handleErrorMsg(ErrFileOutsideVault, fmt.Sprintf("template file must be within the vault: %s", schemaTemplateFile), "Provide a path relative to the vault root")
		}
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			return handleErrorMsg(ErrFileNotFound, fmt.Sprintf("template file not found: %s", schemaTemplateFile), "Create the file first or use --content for inline templates")
		}
		templateSpec = schemaTemplateFile
		sourceType = "file"
	} else {
		// Inline template
		templateSpec = schemaTemplateContent
		sourceType = "inline"
	}

	// Read and modify schema.yaml
	schemaPath := paths.SchemaPath(vaultPath)
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	types, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrSchemaInvalid, "types section not found", "")
	}

	typeNode, ok := types[typeName].(map[string]interface{})
	if !ok {
		typeNode = make(map[string]interface{})
		types[typeName] = typeNode
	}

	typeNode["template"] = templateSpec

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		result := map[string]interface{}{
			"type":   typeName,
			"source": sourceType,
			"spec":   templateSpec,
		}
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Set template for type '%s' (%s)\n", typeName, sourceType)
	return nil
}

func templateRemove(vaultPath, typeName string, start time.Time) error {
	// Check built-in types
	if schema.IsBuiltinType(typeName) {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type and cannot be modified", typeName), "")
	}

	// Load schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check type exists
	typeDef, exists := sch.Types[typeName]
	if !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "")
	}

	// Check if there's a template to remove
	if typeDef == nil || typeDef.Template == "" {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("type '%s' has no template configured", typeName), "Nothing to remove")
	}

	// Read and modify schema.yaml
	schemaPath := paths.SchemaPath(vaultPath)
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return handleError(ErrSchemaInvalid, err, "")
	}

	types, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrSchemaInvalid, "types section not found", "")
	}

	typeNode, ok := types[typeName].(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrSchemaInvalid, fmt.Sprintf("type '%s' definition is invalid", typeName), "")
	}

	delete(typeNode, "template")

	// Write back
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"type":    typeName,
			"removed": true,
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Printf("✓ Removed template from type '%s'\n", typeName)
	return nil
}

func templateRender(vaultPath, typeName string, start time.Time) error {
	// Check built-in types
	if schema.IsBuiltinType(typeName) {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type", typeName), "Built-in types do not support custom templates")
	}

	// Load schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}

	// Check type exists
	typeDef, exists := sch.Types[typeName]
	if !exists {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "Run 'rvn schema types' to see available types")
	}

	// Check if there's a template
	if typeDef == nil || typeDef.Template == "" {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("type '%s' has no template configured", typeName), "Set one with 'rvn schema template set'")
	}

	// Load template content
	templateContent, err := template.Load(vaultPath, typeDef.Template)
	if err != nil {
		return handleError(ErrFileReadError, err, "Failed to load template")
	}
	if templateContent == "" {
		return handleErrorMsg(ErrFileNotFound, "template content is empty", "Check the template configuration")
	}

	// Build title
	title := schemaTemplateTitle
	if title == "" {
		title = "Sample " + typeName
	}

	// Parse field flags
	fieldValues := make(map[string]string)
	for _, f := range schemaTemplateFields {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) == 2 {
			fieldValues[parts[0]] = parts[1]
		}
	}

	// Apply template
	slug := slugs.ComponentSlug(title)
	vars := template.NewVariables(title, typeName, slug, fieldValues)
	rendered := template.Apply(templateContent, vars)

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		result := map[string]interface{}{
			"type":     typeName,
			"title":    title,
			"rendered": rendered,
		}
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Printf("Rendered template for type '%s' (title: %s):\n\n", typeName, title)
	fmt.Println(rendered)
	return nil
}

func init() {
	// Add flags to schema template command
	schemaTemplateCmd.Flags().StringVar(&schemaTemplateContent, "content", "", "Inline template content")
	schemaTemplateCmd.Flags().StringVar(&schemaTemplateFile, "file", "", "Path to template file (relative to vault root)")
	schemaTemplateCmd.Flags().StringVar(&schemaTemplateTitle, "title", "", "Title for template rendering")
	schemaTemplateCmd.Flags().StringArrayVar(&schemaTemplateFields, "field", nil, "Set field value for rendering (can be repeated): --field key=value")
}
