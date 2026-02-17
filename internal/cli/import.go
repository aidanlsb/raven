package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	importFile         string
	importMapping      string
	importMapFlags     []string
	importKey          string
	importContentField string
	importDryRun       bool
	importCreateOnly   bool
	importUpdateOnly   bool
	importConfirm      bool
)

var importCmd = &cobra.Command{
	Use:   "import [type]",
	Short: "Import objects from JSON data",
	Long: `Import objects from external JSON data into the vault.

Reads a JSON array (or single object) and creates or updates vault objects
by mapping input fields to a schema type's fields.

Input can come from stdin or a file (--file). Field mappings can be specified
inline (--map) or via a mapping file (--mapping).

For homogeneous imports (single type), specify the type as a positional argument
or in the mapping file. For heterogeneous imports (mixed types), use a mapping
file with type_field and per-type mappings.

By default, import performs an upsert: it creates new objects and updates
existing ones. Use --create-only or --update-only to restrict behavior.

Examples:
  # Simple import from stdin
  echo '[{"name": "Freya", "email": "freya@asgard.realm"}]' | rvn import person

  # With field mapping
  echo '[{"full_name": "Thor"}]' | rvn import person --map full_name=name

  # From a file with a mapping file
  rvn import --mapping contacts.yaml --file contacts.json

  # Dry run to preview changes
  echo '[{"name": "Loki"}]' | rvn import person --dry-run

  # Import with content (page body) from a JSON field
  echo '[{"name": "Freya", "bio": "Goddess of love"}]' | rvn import person --content-field bio

  # Heterogeneous import with mapping file
  rvn import --mapping migration.yaml --file dump.json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runImport,
}

// importMappingConfig represents a parsed mapping configuration.
type importMappingConfig struct {
	// Homogeneous: single type
	Type         string            `yaml:"type"`
	Key          string            `yaml:"key"`
	Map          map[string]string `yaml:"map"`
	ContentField string            `yaml:"content_field"`

	// Heterogeneous: multiple types
	TypeField string                       `yaml:"type_field"`
	Types     map[string]importTypeMapping `yaml:"types"`
}

// importTypeMapping defines the mapping for one source type.
type importTypeMapping struct {
	Type         string            `yaml:"type"` // Raven type name
	Key          string            `yaml:"key"`
	Map          map[string]string `yaml:"map"`
	ContentField string            `yaml:"content_field"`
}

// importResult tracks the outcome for one item.
type importResult struct {
	ID     string `json:"id"`
	Action string `json:"action"` // "created", "updated", "skipped", "error"
	File   string `json:"file,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func runImport(cmd *cobra.Command, args []string) error {
	vaultPath := getVaultPath()

	// Load schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	// Load vault config
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}
	creator := newObjectCreationContext(vaultPath, sch, vaultCfg.GetObjectsRoot(), vaultCfg.GetPagesRoot(), vaultCfg.GetTemplateDirectory())

	// Build the mapping config from flags and/or mapping file
	mappingCfg, err := buildMappingConfig(args)
	if err != nil {
		return handleError(ErrInvalidInput, err, "")
	}

	// Validate that referenced types exist in the schema
	if err := validateMappingTypes(mappingCfg, sch); err != nil {
		return handleError(ErrTypeNotFound, err, "Check schema.yaml for available types")
	}

	// Read JSON input
	items, err := readJSONInput()
	if err != nil {
		return handleError(ErrInvalidInput, err, "Expected a JSON array of objects or a single JSON object")
	}

	if len(items) == 0 {
		return handleErrorMsg(ErrInvalidInput, "no items to import", "Provide a non-empty JSON array")
	}

	// Process each item
	var results []importResult
	var warnings []Warning

	for i, item := range items {
		// Resolve which type and field mapping apply to this item
		itemCfg, err := resolveItemMapping(item, mappingCfg, sch)
		if err != nil {
			results = append(results, importResult{
				ID:     fmt.Sprintf("item[%d]", i),
				Action: "skipped",
				Reason: err.Error(),
			})
			continue
		}

		// Apply field mappings
		mapped := applyFieldMappings(item, itemCfg.FieldMap)

		// Extract content field (remove from fields so it doesn't become frontmatter)
		var contentValue string
		if itemCfg.ContentField != "" {
			contentValue = extractContentField(mapped, itemCfg.ContentField)
		}

		// Determine match key value for upsert
		matchValue, ok := matchKeyValue(mapped, itemCfg.MatchKey)
		if !ok {
			results = append(results, importResult{
				ID:     fmt.Sprintf("item[%d]", i),
				Action: "skipped",
				Reason: fmt.Sprintf("missing match key '%s'", itemCfg.MatchKey),
			})
			continue
		}

		// Resolve target path
		targetPath := creator.resolveTargetPath(matchValue, itemCfg.TypeName)
		exists := creator.exists(matchValue, itemCfg.TypeName)

		if exists && importCreateOnly {
			results = append(results, importResult{
				ID:     targetPath,
				Action: "skipped",
				Reason: "already exists (--create-only)",
			})
			continue
		}

		if !exists && importUpdateOnly {
			results = append(results, importResult{
				ID:     targetPath,
				Action: "skipped",
				Reason: "does not exist (--update-only)",
			})
			continue
		}

		if importDryRun {
			action := "create"
			if exists {
				action = "update"
			}
			results = append(results, importResult{
				ID:     targetPath,
				Action: action,
				File:   pages.SlugifyPath(targetPath) + ".md",
			})
			continue
		}

		if exists {
			// Update existing object
			result, w := importUpdateObject(vaultPath, targetPath, itemCfg.TypeName, mapped, contentValue, sch, vaultCfg)
			results = append(results, result)
			warnings = append(warnings, w...)
		} else {
			// Create new object
			result, w := importCreateObject(creator, matchValue, targetPath, itemCfg.TypeName, matchValue, mapped, contentValue, vaultCfg)
			results = append(results, result)
			warnings = append(warnings, w...)
		}
	}

	return outputImportResults(results, warnings)
}

// buildMappingConfig constructs a mapping config from CLI flags and/or mapping file.
func buildMappingConfig(args []string) (*importMappingConfig, error) {
	cfg := &importMappingConfig{
		Map: make(map[string]string),
	}

	// Load mapping file if specified
	if importMapping != "" {
		data, err := os.ReadFile(importMapping)
		if err != nil {
			return nil, fmt.Errorf("failed to read mapping file: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse mapping file: %w", err)
		}
	}

	// CLI type argument overrides mapping file type
	if len(args) > 0 {
		cfg.Type = args[0]
	}

	// CLI --map flags are merged into the mapping (override file mappings)
	for _, m := range importMapFlags {
		parts := strings.SplitN(m, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --map format: %q (expected key=value)", m)
		}
		cfg.Map[parts[0]] = parts[1]
	}

	// CLI --key flag overrides mapping file key
	if importKey != "" {
		cfg.Key = importKey
	}

	// CLI --content-field flag overrides mapping file content_field
	if importContentField != "" {
		cfg.ContentField = importContentField
	}

	// Validate: must have either a type or type_field
	if cfg.Type == "" && cfg.TypeField == "" {
		return nil, fmt.Errorf("no type specified: provide a type argument or use a mapping file with 'type' or 'type_field'")
	}

	return cfg, nil
}

// validateMappingTypes checks that all referenced types exist in the schema.
func validateMappingTypes(cfg *importMappingConfig, sch *schema.Schema) error {
	if cfg.Type != "" {
		if _, ok := sch.Types[cfg.Type]; !ok && !schema.IsBuiltinType(cfg.Type) {
			return fmt.Errorf("type '%s' not found in schema", cfg.Type)
		}
	}

	for sourceName, typeMapping := range cfg.Types {
		if _, ok := sch.Types[typeMapping.Type]; !ok && !schema.IsBuiltinType(typeMapping.Type) {
			return fmt.Errorf("type '%s' (mapped from '%s') not found in schema", typeMapping.Type, sourceName)
		}
	}

	return nil
}

// readJSONInput reads JSON input from stdin or --file flag.
func readJSONInput() ([]map[string]interface{}, error) {
	var data []byte
	var err error

	if importFile != "" {
		data, err = os.ReadFile(importFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", importFile, err)
		}
	} else {
		data, err = os.ReadFile("/dev/stdin")
		if err != nil {
			return nil, fmt.Errorf("failed to read stdin: %w", err)
		}
	}

	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return nil, fmt.Errorf("empty input")
	}

	// Try as array first
	var items []map[string]interface{}
	if err := json.Unmarshal(data, &items); err == nil {
		return items, nil
	}

	// Try as single object
	var single map[string]interface{}
	if err := json.Unmarshal(data, &single); err == nil {
		return []map[string]interface{}{single}, nil
	}

	return nil, fmt.Errorf("input is not valid JSON (expected array or object)")
}

// importItemConfig holds the resolved mapping for a single item.
type importItemConfig struct {
	TypeName     string
	FieldMap     map[string]string
	MatchKey     string
	ContentField string
}

// resolveItemMapping determines the Raven type, field map, match key, and content field for an item.
func resolveItemMapping(item map[string]interface{}, cfg *importMappingConfig, sch *schema.Schema) (*importItemConfig, error) {
	result := &importItemConfig{}

	if cfg.TypeField != "" {
		// Heterogeneous: look up type from item field
		sourceType, ok := item[cfg.TypeField]
		if !ok {
			return nil, fmt.Errorf("missing type field '%s'", cfg.TypeField)
		}
		sourceTypeStr, ok := sourceType.(string)
		if !ok {
			return nil, fmt.Errorf("type field '%s' is not a string", cfg.TypeField)
		}

		typeMapping, ok := cfg.Types[sourceTypeStr]
		if !ok {
			return nil, fmt.Errorf("no mapping for source type '%s'", sourceTypeStr)
		}

		result.TypeName = typeMapping.Type
		result.FieldMap = typeMapping.Map
		result.MatchKey = typeMapping.Key
		result.ContentField = typeMapping.ContentField
	} else {
		// Homogeneous: single type
		result.TypeName = cfg.Type
		result.FieldMap = cfg.Map
		result.MatchKey = cfg.Key
		result.ContentField = cfg.ContentField
	}

	// Default match key to the type's name_field
	if result.MatchKey == "" {
		if typeDef, ok := sch.Types[result.TypeName]; ok && typeDef != nil && typeDef.NameField != "" {
			result.MatchKey = typeDef.NameField
		}
	}

	// If still no match key, error
	if result.MatchKey == "" {
		return nil, fmt.Errorf("no match key: set --key or configure name_field for type '%s'", result.TypeName)
	}

	if result.FieldMap == nil {
		result.FieldMap = make(map[string]string)
	}

	return result, nil
}

// applyFieldMappings renames input keys according to the field map.
// Keys not in the map pass through unchanged.
func applyFieldMappings(item map[string]interface{}, fieldMap map[string]string) map[string]interface{} {
	result := make(map[string]interface{}, len(item))

	// Build reverse map for quick lookup
	for inputKey, value := range item {
		if schemaField, ok := fieldMap[inputKey]; ok {
			result[schemaField] = value
		} else {
			result[inputKey] = value
		}
	}

	return result
}

// matchKeyValue extracts the match key value from the mapped item.
// The match key field name refers to the schema field name (after mapping).
func matchKeyValue(mapped map[string]interface{}, matchKey string) (string, bool) {
	val, ok := mapped[matchKey]
	if !ok {
		return "", false
	}

	switch v := val.(type) {
	case string:
		if v == "" {
			return "", false
		}
		return v, true
	case float64:
		return fmt.Sprintf("%v", v), true
	default:
		return fmt.Sprintf("%v", v), true
	}
}

// importCreateObject creates a new vault object from imported data.
func importCreateObject(creator objectCreationContext, targetPath, resolvedTargetPath, typeName, title string, fields map[string]interface{}, content string, vaultCfg *config.VaultConfig) (importResult, []Warning) {
	var warnings []Warning

	// Convert fields to string map for pages.Create
	stringFields := fieldsToStringMap(fields, typeName)

	// Remove the type field from frontmatter data (it's set by the type system)
	delete(stringFields, "type")

	createResult, err := creator.create(objectCreateParams{
		typeName:   typeName,
		title:      title,
		targetPath: targetPath,
		fields:     stringFields,
	})
	if err != nil {
		return importResult{
			ID:     resolvedTargetPath,
			Action: "error",
			Reason: err.Error(),
		}, warnings
	}

	// Append content to the file if provided
	if content != "" {
		if err := appendContentToFile(createResult.FilePath, content); err != nil {
			return importResult{
				ID:     resolvedTargetPath,
				Action: "error",
				Reason: fmt.Sprintf("failed to write content: %v", err),
			}, warnings
		}
	}

	// Auto-reindex
	maybeReindex(creator.vaultPath, createResult.FilePath, vaultCfg)

	return importResult{
		ID:     vaultCfg.FilePathToObjectID(createResult.RelativePath),
		Action: "created",
		File:   createResult.RelativePath,
	}, warnings
}

// importUpdateObject updates an existing vault object with imported data.
func importUpdateObject(vaultPath, targetPath, typeName string, fields map[string]interface{}, newContent string, sch *schema.Schema, vaultCfg *config.VaultConfig) (importResult, []Warning) {
	var warnings []Warning

	slugPath := pages.SlugifyPath(targetPath)
	filePath := filepath.Join(vaultPath, slugPath)
	if !strings.HasSuffix(filePath, ".md") {
		filePath += ".md"
	}

	// Read existing file
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return importResult{
			ID:     targetPath,
			Action: "error",
			Reason: fmt.Sprintf("read error: %v", err),
		}, warnings
	}

	// Parse frontmatter
	fm, err := parser.ParseFrontmatter(string(fileData))
	if err != nil || fm == nil {
		return importResult{
			ID:     targetPath,
			Action: "error",
			Reason: "failed to parse frontmatter",
		}, warnings
	}

	// Convert fields to string map for update
	updates := fieldsToStringMap(fields, typeName)
	delete(updates, "type")

	// Resolve date keywords
	fieldDefs := fieldDefsForObjectType(sch, typeName)
	resolvedUpdates := resolveDateKeywordsForUpdates(updates, fieldDefs)

	// Update frontmatter
	updatedFile, err := updateFrontmatter(string(fileData), fm, resolvedUpdates)
	if err != nil {
		return importResult{
			ID:     targetPath,
			Action: "error",
			Reason: fmt.Sprintf("update error: %v", err),
		}, warnings
	}

	// Replace body content if content field was provided
	if newContent != "" {
		updatedFile = replaceBodyContent(updatedFile, newContent)
	}

	// Write back
	if err := atomicfile.WriteFile(filePath, []byte(updatedFile), 0o644); err != nil {
		return importResult{
			ID:     targetPath,
			Action: "error",
			Reason: fmt.Sprintf("write error: %v", err),
		}, warnings
	}

	// Auto-reindex
	maybeReindex(vaultPath, filePath, vaultCfg)

	relPath, _ := filepath.Rel(vaultPath, filePath)

	return importResult{
		ID:     vaultCfg.FilePathToObjectID(relPath),
		Action: "updated",
		File:   relPath,
	}, warnings
}

// fieldsToStringMap converts a map[string]interface{} to map[string]string for frontmatter use.
func fieldsToStringMap(fields map[string]interface{}, typeName string) map[string]string {
	result := make(map[string]string, len(fields))
	for k, v := range fields {
		// Skip internal/meta fields
		if k == "type" {
			continue
		}
		switch val := v.(type) {
		case string:
			result[k] = val
		case float64:
			// Preserve integer representation for whole numbers
			if val == float64(int64(val)) {
				result[k] = fmt.Sprintf("%d", int64(val))
			} else {
				result[k] = fmt.Sprintf("%v", val)
			}
		case bool:
			result[k] = fmt.Sprintf("%v", val)
		case []interface{}:
			// Convert arrays to bracket notation: [a, b, c]
			var parts []string
			for _, item := range val {
				parts = append(parts, fmt.Sprintf("%v", item))
			}
			result[k] = "[" + strings.Join(parts, ", ") + "]"
		case nil:
			// Skip null values
		default:
			result[k] = fmt.Sprintf("%v", val)
		}
	}
	return result
}

// outputImportResults outputs the import results in human-readable or JSON format.
func outputImportResults(results []importResult, warnings []Warning) error {
	// Count outcomes
	var created, updated, skipped, errored int
	for _, r := range results {
		switch r.Action {
		case "created", "create":
			created++
		case "updated", "update":
			updated++
		case "skipped":
			skipped++
		case "error":
			errored++
		}
	}

	if isJSONOutput() {
		data := map[string]interface{}{
			"total":   len(results),
			"created": created,
			"updated": updated,
			"skipped": skipped,
			"errors":  errored,
			"results": results,
		}
		if len(warnings) > 0 {
			outputSuccessWithWarnings(data, warnings, nil)
		} else {
			outputSuccess(data, nil)
		}
		return nil
	}

	// Human-readable output
	if importDryRun {
		fmt.Println(ui.Bold.Render("Dry run — no changes made:"))
	}

	for _, r := range results {
		switch r.Action {
		case "created":
			fmt.Println(ui.Checkf("Created %s", ui.FilePath(r.File)))
		case "create":
			fmt.Printf("  %s %s\n", ui.Bold.Render("create"), ui.FilePath(r.File))
		case "updated":
			fmt.Println(ui.Checkf("Updated %s", ui.FilePath(r.File)))
		case "update":
			fmt.Printf("  %s %s\n", ui.Bold.Render("update"), ui.FilePath(r.File))
		case "skipped":
			fmt.Printf("  %s %s: %s\n", ui.Warning("skip"), r.ID, r.Reason)
		case "error":
			fmt.Printf("  %s %s: %s\n", ui.Warning("error"), r.ID, r.Reason)
		}
	}

	// Summary line
	var parts []string
	if created > 0 {
		parts = append(parts, fmt.Sprintf("%d created", created))
	}
	if updated > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", updated))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if errored > 0 {
		parts = append(parts, fmt.Sprintf("%d errors", errored))
	}
	if len(parts) > 0 {
		fmt.Printf("\n%s\n", strings.Join(parts, ", "))
	}

	for _, w := range warnings {
		fmt.Printf("  %s\n", ui.Warning(w.Message))
	}

	return nil
}

// extractContentField removes the content field from the mapped item and returns its value as a string.
func extractContentField(mapped map[string]interface{}, contentField string) string {
	val, ok := mapped[contentField]
	if !ok {
		return ""
	}
	delete(mapped, contentField)

	switch v := val.(type) {
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

// appendContentToFile appends markdown content to an existing file.
func appendContentToFile(filePath, content string) error {
	existing, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var result strings.Builder
	result.Write(existing)

	// Ensure there's a blank line before content
	existingStr := string(existing)
	if !strings.HasSuffix(existingStr, "\n\n") {
		if strings.HasSuffix(existingStr, "\n") {
			result.WriteString("\n")
		} else {
			result.WriteString("\n\n")
		}
	}

	result.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		result.WriteString("\n")
	}

	return atomicfile.WriteFile(filePath, []byte(result.String()), 0o644)
}

// replaceBodyContent replaces everything after the frontmatter with new content.
func replaceBodyContent(fileContent, newBody string) string {
	lines := strings.Split(fileContent, "\n")

	_, endLine, ok := parser.FrontmatterBounds(lines)
	if !ok || endLine == -1 {
		// No frontmatter found — just return the new body
		return newBody
	}

	// Keep frontmatter + closing ---
	var result strings.Builder
	for i := 0; i <= endLine; i++ {
		result.WriteString(lines[i])
		result.WriteString("\n")
	}

	// Add blank line + new content
	result.WriteString("\n")
	result.WriteString(newBody)
	if !strings.HasSuffix(newBody, "\n") {
		result.WriteString("\n")
	}

	return result.String()
}

func init() {
	importCmd.Flags().StringVar(&importFile, "file", "", "Read JSON from file instead of stdin")
	importCmd.Flags().StringVar(&importMapping, "mapping", "", "Path to YAML mapping file")
	importCmd.Flags().StringArrayVar(&importMapFlags, "map", nil, "Field mapping: external_key=schema_field (repeatable)")
	importCmd.Flags().StringVar(&importKey, "key", "", "Field used for matching existing objects (default: type's name_field)")
	importCmd.Flags().StringVar(&importContentField, "content-field", "", "JSON field to use as page body content")
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Preview changes without writing")
	importCmd.Flags().BoolVar(&importCreateOnly, "create-only", false, "Only create new objects, skip updates")
	importCmd.Flags().BoolVar(&importUpdateOnly, "update-only", false, "Only update existing objects, skip creation")
	importCmd.Flags().BoolVar(&importConfirm, "confirm", false, "Apply changes (for future bulk safety)")
	rootCmd.AddCommand(importCmd)
}
