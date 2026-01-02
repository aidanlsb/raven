package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ravenscroftj/raven/internal/audit"
	"github.com/ravenscroftj/raven/internal/config"
	"github.com/ravenscroftj/raven/internal/parser"
	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var setCmd = &cobra.Command{
	Use:   "set <object-id> <field=value>...",
	Short: "Set frontmatter fields on an object",
	Long: `Set one or more frontmatter fields on an existing object.

The object ID can be a full path (e.g., "people/alice") or a short reference
that uniquely identifies an object. Field values are validated against the
schema if the object has a known type.

Examples:
  rvn set people/alice email=alice@example.com
  rvn set people/alice name="Alice Chen" email=alice@new.com
  rvn set projects/website status=active priority=high
  rvn set projects/website --json    # Machine-readable output`,
	Args: cobra.MinimumNArgs(2),
	RunE: runSet,
}

func runSet(cmd *cobra.Command, args []string) error {
	vaultPath := getVaultPath()
	objectID := args[0]
	fieldArgs := args[1:]

	// Parse field=value arguments
	updates := make(map[string]string)
	for _, arg := range fieldArgs {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return handleErrorMsg(ErrInvalidInput,
				fmt.Sprintf("invalid field format: %s", arg),
				"Use format: field=value")
		}
		updates[parts[0]] = parts[1]
	}

	if len(updates) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no fields to set", "Usage: rvn set <object-id> field=value...")
	}

	// Resolve object ID to file path
	filePath, err := resolveObjectToFile(vaultPath, objectID)
	if err != nil {
		return handleError(ErrFileDoesNotExist, err, "")
	}

	// Load schema for validation
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	// Load vault config for audit logging
	vaultCfg, _ := config.LoadVaultConfig(vaultPath)

	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	// Parse frontmatter
	fm, err := parser.ParseFrontmatter(string(content))
	if err != nil {
		return handleError(ErrInvalidInput, err, "Failed to parse frontmatter")
	}
	if fm == nil {
		return handleErrorMsg(ErrInvalidInput, "file has no frontmatter", "The file must have YAML frontmatter (---) to set fields")
	}

	// Get the object type
	objectType := fm.ObjectType
	if objectType == "" {
		objectType = "page"
	}

	// Validate fields against schema
	typeDef, hasType := sch.Types[objectType]
	var validationWarnings []string

	for fieldName, value := range updates {
		if hasType && typeDef != nil {
			// Check if this is a valid field
			fieldDef, isField := typeDef.Fields[fieldName]
			_, isTrait := typeDef.Traits.Configs[fieldName]
			traitDef := sch.Traits[fieldName]

			if !isField && !isTrait && fieldName != "tags" {
				// Unknown field - warn but allow (for flexibility)
				validationWarnings = append(validationWarnings,
					fmt.Sprintf("'%s' is not a declared field or trait for type '%s'", fieldName, objectType))
			}

			// Validate enum values
			if isField && fieldDef != nil && fieldDef.Type == schema.FieldTypeEnum && len(fieldDef.Values) > 0 {
				if !contains(fieldDef.Values, value) {
					return handleErrorMsg(ErrValidationFailed,
						fmt.Sprintf("invalid value '%s' for field '%s'", value, fieldName),
						fmt.Sprintf("Allowed values: %s", strings.Join(fieldDef.Values, ", ")))
				}
			}

			// Validate trait enum values
			if isTrait && traitDef != nil && traitDef.Type == schema.FieldTypeEnum && len(traitDef.Values) > 0 {
				if !contains(traitDef.Values, value) {
					return handleErrorMsg(ErrValidationFailed,
						fmt.Sprintf("invalid value '%s' for trait '%s'", value, fieldName),
						fmt.Sprintf("Allowed values: %s", strings.Join(traitDef.Values, ", ")))
				}
			}
		}
	}

	// Build updated frontmatter
	newContent, err := updateFrontmatter(string(content), fm, updates)
	if err != nil {
		return handleError(ErrFileWriteError, err, "Failed to update frontmatter")
	}

	// Write the file back
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	// Log to audit
	logger := audit.New(vaultPath, vaultCfg.IsAuditLogEnabled())
	relPath, _ := filepath.Rel(vaultPath, filePath)
	// Convert updates to map[string]interface{} for logging
	changesMap := make(map[string]interface{})
	for k, v := range updates {
		changesMap[k] = v
	}
	logger.LogUpdate(relPath, objectID, changesMap)

	// Output
	if isJSONOutput() {
		result := map[string]interface{}{
			"file":           relPath,
			"object_id":      objectID,
			"type":           objectType,
			"updated_fields": updates,
		}
		if len(validationWarnings) > 0 {
			var warnings []Warning
			for _, w := range validationWarnings {
				warnings = append(warnings, Warning{
					Code:    WarnUnknownField,
					Message: w,
				})
			}
			outputSuccessWithWarnings(result, warnings, nil)
		} else {
			outputSuccess(result, nil)
		}
		return nil
	}

	// Human-readable output
	fmt.Printf("Updated %s:\n", relPath)
	var fieldNames []string
	for name := range updates {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)
	for _, name := range fieldNames {
		fmt.Printf("  %s = %s\n", name, updates[name])
	}
	for _, warning := range validationWarnings {
		fmt.Printf("  Warning: %s\n", warning)
	}

	return nil
}

// resolveObjectToFile resolves an object ID to an absolute file path.
func resolveObjectToFile(vaultPath, objectID string) (string, error) {
	// Normalize the object ID
	objectID = strings.TrimSuffix(objectID, ".md")

	// Try direct path first
	filePath := filepath.Join(vaultPath, objectID+".md")
	if _, err := os.Stat(filePath); err == nil {
		return filePath, nil
	}

	// Try with different casing/slugification
	// Walk the vault to find matching file
	var foundPath string
	err := filepath.WalkDir(vaultPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if d.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		// Get relative path and compare
		relPath, _ := filepath.Rel(vaultPath, path)
		relID := strings.TrimSuffix(relPath, ".md")

		// Exact match
		if relID == objectID {
			foundPath = path
			return filepath.SkipAll
		}

		// Slugified match
		if slugifyPath(relID) == slugifyPath(objectID) {
			foundPath = path
			return filepath.SkipAll
		}

		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return "", err
	}

	if foundPath != "" {
		return foundPath, nil
	}

	return "", fmt.Errorf("object not found: %s", objectID)
}

// slugifyPath slugifies each component of a path for matching.
func slugifyPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = strings.ToLower(strings.ReplaceAll(part, " ", "-"))
	}
	return strings.Join(parts, "/")
}

// updateFrontmatter updates the frontmatter in the content with new field values.
func updateFrontmatter(content string, fm *parser.Frontmatter, updates map[string]string) (string, error) {
	lines := strings.Split(content, "\n")

	// Find frontmatter boundaries
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", fmt.Errorf("no frontmatter found")
	}

	endLine := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endLine = i
			break
		}
	}

	if endLine == -1 {
		return "", fmt.Errorf("unclosed frontmatter")
	}

	// Parse existing frontmatter as a map to preserve order and unknown fields
	frontmatterContent := strings.Join(lines[1:endLine], "\n")
	var yamlData map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterContent), &yamlData); err != nil {
		return "", fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	if yamlData == nil {
		yamlData = make(map[string]interface{})
	}

	// Apply updates
	for key, value := range updates {
		// Handle special cases for value parsing
		parsedValue := parseFieldValue(value)
		yamlData[key] = parsedValue
	}

	// Marshal back to YAML
	newFrontmatter, err := yaml.Marshal(yamlData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	// Reconstruct the file
	var result strings.Builder
	result.WriteString("---\n")
	result.Write(newFrontmatter)
	result.WriteString("---")

	// Add the rest of the content
	if endLine+1 < len(lines) {
		result.WriteString("\n")
		result.WriteString(strings.Join(lines[endLine+1:], "\n"))
	}

	return result.String(), nil
}

// parseFieldValue parses a field value string into an appropriate type.
func parseFieldValue(value string) interface{} {
	// Handle arrays: [a, b, c]
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		inner := value[1 : len(value)-1]
		parts := strings.Split(inner, ",")
		var items []interface{}
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				items = append(items, parseFieldValue(part))
			}
		}
		return items
	}

	// Handle references: [[path]]
	if strings.HasPrefix(value, "[[") && strings.HasSuffix(value, "]]") {
		return value // Keep as-is for references
	}

	// Handle booleans
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}

	// Handle quoted strings - remove quotes
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
		(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
		return value[1 : len(value)-1]
	}

	// Default to string
	return value
}

// contains checks if a slice contains a value.
func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

func init() {
	rootCmd.AddCommand(setCmd)
}
