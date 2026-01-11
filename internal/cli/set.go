package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

var (
	setStdin   bool
	setConfirm bool
)

var setCmd = &cobra.Command{
	Use:   "set <object-id> <field=value>...",
	Short: "Set frontmatter fields on an object",
	Long: `Set one or more frontmatter fields on an existing object.

The object ID can be a full path (e.g., "people/freya") or a short reference
that uniquely identifies an object. Field values are validated against the
schema if the object has a known type.

Bulk operations:
  Use --stdin to read object IDs from stdin (one per line).
  Bulk operations preview changes by default; use --confirm to apply.

Examples:
  rvn set people/freya email=freya@asgard.realm
  rvn set people/freya name="Freya" email=freya@vanaheim.realm
  rvn set projects/website status=active priority=high
  rvn set projects/website --json

Bulk examples:
  rvn query "object:project .status:active" --ids | rvn set --stdin status=archived
  rvn query "trait:due value:past" --ids | rvn set --stdin status=overdue --confirm`,
	Args: cobra.ArbitraryArgs,
	RunE: runSet,
}

func runSet(cmd *cobra.Command, args []string) error {
	vaultPath := getVaultPath()

	// Handle --stdin mode for bulk operations
	if setStdin {
		return runSetBulk(cmd, args, vaultPath)
	}

	// Single object mode - requires object-id and at least one field=value
	if len(args) < 2 {
		return handleErrorMsg(ErrMissingArgument, "requires object-id and field=value arguments", "Usage: rvn set <object-id> field=value...")
	}

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

	return setSingleObject(vaultPath, objectID, updates)
}

// runSetBulk handles bulk set operations from stdin.
func runSetBulk(cmd *cobra.Command, args []string, vaultPath string) error {
	// Parse field=value arguments (all args are field=value in stdin mode)
	updates := make(map[string]string)
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return handleErrorMsg(ErrInvalidInput,
				fmt.Sprintf("invalid field format: %s", arg),
				"Use format: field=value")
		}
		updates[parts[0]] = parts[1]
	}

	if len(updates) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no fields to set", "Usage: rvn set --stdin field=value...")
	}

	// Read IDs from stdin (both file-level and embedded)
	fileIDs, embeddedIDs, err := ReadIDsFromStdin()
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	// Combine all IDs - we now support embedded objects
	ids := append(fileIDs, embeddedIDs...)

	if len(ids) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
	}

	var warnings []Warning

	// Load schema for validation
	sch, _ := schema.Load(vaultPath)

	// Load vault config (optional, used for roots + auto-reindex)
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil || vaultCfg == nil {
		vaultCfg = &config.VaultConfig{}
	}

	// If not confirming, show preview
	if !setConfirm {
		return previewSetBulk(vaultPath, ids, updates, warnings, sch, vaultCfg)
	}

	// Apply the changes
	return applySetBulk(vaultPath, ids, updates, warnings, sch, vaultCfg)
}

// previewSetBulk shows a preview of bulk set operations.
func previewSetBulk(vaultPath string, ids []string, updates map[string]string, warnings []Warning, sch *schema.Schema, vaultCfg *config.VaultConfig) error {
	var previewItems []BulkPreviewItem
	var skipped []BulkResult

	// Get parse options from vault config
	var parseOpts *parser.ParseOptions
	if vaultCfg != nil && vaultCfg.HasDirectoriesConfig() {
		dirs := vaultCfg.GetDirectoriesConfig()
		if dirs != nil {
			parseOpts = &parser.ParseOptions{
				ObjectsRoot: dirs.Objects,
				PagesRoot:   dirs.Pages,
			}
		}
	}

	for _, id := range ids {
		// Check if this is an embedded object
		if IsEmbeddedID(id) {
			item, skip := previewSetEmbedded(vaultPath, id, updates, vaultCfg, parseOpts)
			if skip != nil {
				skipped = append(skipped, *skip)
			} else if item != nil {
				previewItems = append(previewItems, *item)
			}
			continue
		}

		filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
		if err != nil {
			skipped = append(skipped, BulkResult{
				ID:     id,
				Status: "skipped",
				Reason: "object not found",
			})
			continue
		}

		// Read current values to show diff
		content, err := os.ReadFile(filePath)
		if err != nil {
			skipped = append(skipped, BulkResult{
				ID:     id,
				Status: "skipped",
				Reason: fmt.Sprintf("read error: %v", err),
			})
			continue
		}

		fm, err := parser.ParseFrontmatter(string(content))
		if err != nil || fm == nil {
			skipped = append(skipped, BulkResult{
				ID:     id,
				Status: "skipped",
				Reason: "no frontmatter",
			})
			continue
		}

		// Build change summary
		changes := make(map[string]string)
		for field, newVal := range updates {
			oldVal := "<unset>"
			if fm.Fields != nil {
				if v, ok := fm.Fields[field]; ok {
					oldVal = fmt.Sprintf("%v", v)
				}
			}
			changes[field] = fmt.Sprintf("%s (was: %s)", newVal, oldVal)
		}

		previewItems = append(previewItems, BulkPreviewItem{
			ID:      id,
			Action:  "set",
			Changes: changes,
		})
	}

	preview := &BulkPreview{
		Action:   "set",
		Items:    previewItems,
		Skipped:  skipped,
		Total:    len(ids),
		Warnings: warnings,
	}

	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"preview":  true,
			"action":   "set",
			"fields":   updates,
			"items":    previewItems,
			"skipped":  skipped,
			"total":    len(ids),
			"warnings": warnings,
		}, &Meta{Count: len(previewItems)})
		return nil
	}

	PrintBulkPreview(preview)
	return nil
}

// applySetBulk applies bulk set operations.
func applySetBulk(vaultPath string, ids []string, updates map[string]string, warnings []Warning, sch *schema.Schema, vaultCfg *config.VaultConfig) error {
	var results []BulkResult
	modified := 0
	skipped := 0
	errors := 0

	// Get parse options from vault config
	var parseOpts *parser.ParseOptions
	if vaultCfg != nil && vaultCfg.HasDirectoriesConfig() {
		dirs := vaultCfg.GetDirectoriesConfig()
		if dirs != nil {
			parseOpts = &parser.ParseOptions{
				ObjectsRoot: dirs.Objects,
				PagesRoot:   dirs.Pages,
			}
		}
	}

	for _, id := range ids {
		result := BulkResult{ID: id}

		// Check if this is an embedded object
		if IsEmbeddedID(id) {
			err := applySetEmbedded(vaultPath, id, updates, sch, vaultCfg, parseOpts)
			if err != nil {
				result.Status = "error"
				result.Reason = err.Error()
				errors++
			} else {
				result.Status = "modified"
				modified++
			}
			results = append(results, result)
			continue
		}

		filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
		if err != nil {
			result.Status = "skipped"
			result.Reason = "object not found"
			skipped++
			results = append(results, result)
			continue
		}

		content, err := os.ReadFile(filePath)
		if err != nil {
			result.Status = "error"
			result.Reason = fmt.Sprintf("read error: %v", err)
			errors++
			results = append(results, result)
			continue
		}

		fm, err := parser.ParseFrontmatter(string(content))
		if err != nil || fm == nil {
			result.Status = "skipped"
			result.Reason = "no frontmatter"
			skipped++
			results = append(results, result)
			continue
		}

		// Build updated frontmatter
		newContent, err := updateFrontmatter(string(content), fm, updates)
		if err != nil {
			result.Status = "error"
			result.Reason = fmt.Sprintf("update error: %v", err)
			errors++
			results = append(results, result)
			continue
		}

		// Write the file back
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			result.Status = "error"
			result.Reason = fmt.Sprintf("write error: %v", err)
			errors++
			results = append(results, result)
			continue
		}

		// Auto-reindex if configured
		if vaultCfg.IsAutoReindexEnabled() {
			reindexFile(vaultPath, filePath)
		}

		result.Status = "modified"
		modified++
		results = append(results, result)
	}

	summary := &BulkSummary{
		Action:   "set",
		Results:  results,
		Total:    len(ids),
		Modified: modified,
		Skipped:  skipped,
		Errors:   errors,
	}

	if isJSONOutput() {
		data := map[string]interface{}{
			"ok":       errors == 0,
			"action":   "set",
			"fields":   updates,
			"results":  results,
			"total":    len(ids),
			"modified": modified,
			"skipped":  skipped,
			"errors":   errors,
		}
		if len(warnings) > 0 {
			outputSuccessWithWarnings(data, warnings, &Meta{Count: modified})
		} else {
			outputSuccess(data, &Meta{Count: modified})
		}
		return nil
	}

	PrintBulkSummary(summary)
	for _, w := range warnings {
		fmt.Printf("⚠ %s\n", w.Message)
	}
	return nil
}

// setSingleObject sets fields on a single object (non-bulk mode).
func setSingleObject(vaultPath, reference string, updates map[string]string) error {
	// Load schema for validation
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	// Load vault config
	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil || vaultCfg == nil {
		vaultCfg = &config.VaultConfig{}
	}

	// Resolve the reference using unified resolver
	result, err := ResolveReference(reference, ResolveOptions{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
	})
	if err != nil {
		return handleResolveError(err, reference)
	}

	// Check if this is an embedded object (section)
	if result.IsSection {
		return setEmbeddedObject(vaultPath, result.ObjectID, updates, sch, vaultCfg)
	}

	objectID := result.ObjectID
	filePath := result.FilePath

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

			if !isField && fieldName != "alias" {
				// Unknown field - warn but allow (for flexibility)
				validationWarnings = append(validationWarnings,
					fmt.Sprintf("'%s' is not a declared field for type '%s'", fieldName, objectType))
			}

			// Validate enum values
			if isField && fieldDef != nil && fieldDef.Type == schema.FieldTypeEnum && len(fieldDef.Values) > 0 {
				if !contains(fieldDef.Values, value) {
					return handleErrorMsg(ErrValidationFailed,
						fmt.Sprintf("invalid value '%s' for field '%s'", value, fieldName),
						fmt.Sprintf("Allowed values: %s", strings.Join(fieldDef.Values, ", ")))
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

	// Auto-reindex if configured
	if vaultCfg.IsAutoReindexEnabled() {
		if err := reindexFile(vaultPath, filePath); err != nil {
			if !isJSONOutput() {
				fmt.Printf("  (reindex failed: %v)\n", err)
			}
		}
	}

	relPath, _ := filepath.Rel(vaultPath, filePath)

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

// setEmbeddedObject sets fields on an embedded object.
func setEmbeddedObject(vaultPath, objectID string, updates map[string]string, sch *schema.Schema, vaultCfg *config.VaultConfig) error {
	// Parse the embedded ID: fileID#slug
	parts := strings.SplitN(objectID, "#", 2)
	if len(parts) != 2 {
		return handleErrorMsg(ErrInvalidInput, "invalid embedded object ID", "Expected format: file-id#embedded-id")
	}
	fileID := parts[0]
	slug := parts[1]

	// Resolve file ID to file path
	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileID, vaultCfg)
	if err != nil {
		return handleError(ErrFileDoesNotExist, err, "")
	}

	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	// Get parse options from vault config
	var parseOpts *parser.ParseOptions
	if vaultCfg != nil && vaultCfg.HasDirectoriesConfig() {
		dirs := vaultCfg.GetDirectoriesConfig()
		if dirs != nil {
			parseOpts = &parser.ParseOptions{
				ObjectsRoot: dirs.Objects,
				PagesRoot:   dirs.Pages,
			}
		}
	}

	// Parse the document to find the embedded object
	doc, err := parser.ParseDocumentWithOptions(string(content), filePath, vaultPath, parseOpts)
	if err != nil {
		return handleError(ErrInvalidInput, err, "Failed to parse document")
	}

	// Find the embedded object by ID
	var targetObj *parser.ParsedObject
	for _, obj := range doc.Objects {
		if obj.ID == objectID {
			targetObj = obj
			break
		}
	}

	if targetObj == nil {
		return handleErrorMsg(ErrFileDoesNotExist,
			fmt.Sprintf("embedded object '%s' not found in file", slug),
			"Check that the embedded ID exists in the file")
	}

	// Verify this is actually an embedded object (not the file-level object)
	if targetObj.ParentID == nil {
		return handleErrorMsg(ErrInvalidInput,
			"cannot use embedded set on file-level object",
			fmt.Sprintf("Use 'rvn set %s field=value' instead", fileID))
	}

	// Get the object type for validation
	objectType := targetObj.ObjectType

	// Validate fields against schema
	typeDef, hasType := sch.Types[objectType]
	var validationWarnings []string

	for fieldName, value := range updates {
		if hasType && typeDef != nil {
			fieldDef, isField := typeDef.Fields[fieldName]

			if !isField && fieldName != "alias" && fieldName != "id" {
				validationWarnings = append(validationWarnings,
					fmt.Sprintf("'%s' is not a declared field for type '%s'", fieldName, objectType))
			}

			// Validate enum values
			if isField && fieldDef != nil && fieldDef.Type == schema.FieldTypeEnum && len(fieldDef.Values) > 0 {
				if !contains(fieldDef.Values, value) {
					return handleErrorMsg(ErrValidationFailed,
						fmt.Sprintf("invalid value '%s' for field '%s'", value, fieldName),
						fmt.Sprintf("Allowed values: %s", strings.Join(fieldDef.Values, ", ")))
				}
			}
		}
	}

	// Find the type declaration line (line after the heading)
	typeDeclLine := targetObj.LineStart + 1

	// Update the file content
	lines := strings.Split(string(content), "\n")

	// Verify the line is a type declaration
	if typeDeclLine-1 >= len(lines) {
		return handleErrorMsg(ErrInvalidInput, "type declaration line not found", "")
	}

	declLine := lines[typeDeclLine-1] // Convert to 0-indexed
	trimmedDecl := strings.TrimSpace(declLine)
	if !strings.HasPrefix(trimmedDecl, "::") {
		return handleErrorMsg(ErrInvalidInput,
			fmt.Sprintf("expected type declaration at line %d, found: %s", typeDeclLine, trimmedDecl),
			"The embedded object may have been modified or is in an unexpected format")
	}

	// Merge existing fields with updates
	newFields := make(map[string]schema.FieldValue)
	for k, v := range targetObj.Fields {
		newFields[k] = v
	}

	// Apply updates (parse the string values into FieldValues)
	for fieldName, value := range updates {
		newFields[fieldName] = parseFieldValueToSchema(value)
	}

	// Serialize the updated type declaration
	// Preserve leading whitespace from original line
	leadingSpace := ""
	for _, c := range declLine {
		if c == ' ' || c == '\t' {
			leadingSpace += string(c)
		} else {
			break
		}
	}

	newDeclLine := leadingSpace + parser.SerializeTypeDeclaration(objectType, newFields)

	// Replace the line
	lines[typeDeclLine-1] = newDeclLine

	// Write the file back
	newContent := strings.Join(lines, "\n")
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	// Auto-reindex if configured
	if vaultCfg.IsAutoReindexEnabled() {
		if err := reindexFile(vaultPath, filePath); err != nil {
			if !isJSONOutput() {
				fmt.Printf("  (reindex failed: %v)\n", err)
			}
		}
	}

	relPath, _ := filepath.Rel(vaultPath, filePath)

	// Output
	if isJSONOutput() {
		result := map[string]interface{}{
			"file":           relPath,
			"object_id":      objectID,
			"type":           objectType,
			"embedded":       true,
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
	fmt.Printf("Updated %s (embedded: %s):\n", relPath, slug)
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

// previewSetEmbedded generates a preview for an embedded object.
func previewSetEmbedded(vaultPath, id string, updates map[string]string, vaultCfg *config.VaultConfig, parseOpts *parser.ParseOptions) (*BulkPreviewItem, *BulkResult) {
	// Parse the embedded ID
	parts := strings.SplitN(id, "#", 2)
	if len(parts) != 2 {
		return nil, &BulkResult{ID: id, Status: "skipped", Reason: "invalid embedded ID format"}
	}
	fileID := parts[0]

	// Resolve file ID to file path
	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileID, vaultCfg)
	if err != nil {
		return nil, &BulkResult{ID: id, Status: "skipped", Reason: "parent file not found"}
	}

	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, &BulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("read error: %v", err)}
	}

	// Parse the document
	doc, err := parser.ParseDocumentWithOptions(string(content), filePath, vaultPath, parseOpts)
	if err != nil {
		return nil, &BulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("parse error: %v", err)}
	}

	// Find the embedded object
	var targetObj *parser.ParsedObject
	for _, obj := range doc.Objects {
		if obj.ID == id {
			targetObj = obj
			break
		}
	}

	if targetObj == nil {
		return nil, &BulkResult{ID: id, Status: "skipped", Reason: "embedded object not found"}
	}

	// Build change summary
	changes := make(map[string]string)
	for field, newVal := range updates {
		oldVal := "<unset>"
		if targetObj.Fields != nil {
			if v, ok := targetObj.Fields[field]; ok {
				if s, ok := v.AsString(); ok {
					oldVal = s
				} else if n, ok := v.AsNumber(); ok {
					oldVal = fmt.Sprintf("%v", n)
				} else if b, ok := v.AsBool(); ok {
					oldVal = fmt.Sprintf("%v", b)
				} else {
					oldVal = fmt.Sprintf("%v", v.Raw())
				}
			}
		}
		changes[field] = fmt.Sprintf("%s (was: %s)", newVal, oldVal)
	}

	return &BulkPreviewItem{
		ID:      id,
		Action:  "set",
		Changes: changes,
	}, nil
}

// applySetEmbedded applies a set operation to an embedded object.
func applySetEmbedded(vaultPath, id string, updates map[string]string, sch *schema.Schema, vaultCfg *config.VaultConfig, parseOpts *parser.ParseOptions) error {
	// Parse the embedded ID
	parts := strings.SplitN(id, "#", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid embedded ID format")
	}
	fileID := parts[0]
	slug := parts[1]

	// Resolve file ID to file path
	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileID, vaultCfg)
	if err != nil {
		return fmt.Errorf("parent file not found: %w", err)
	}

	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	// Parse the document
	doc, err := parser.ParseDocumentWithOptions(string(content), filePath, vaultPath, parseOpts)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	// Find the embedded object
	var targetObj *parser.ParsedObject
	for _, obj := range doc.Objects {
		if obj.ID == id {
			targetObj = obj
			break
		}
	}

	if targetObj == nil {
		return fmt.Errorf("embedded object '%s' not found", slug)
	}

	// Verify this is an embedded object
	if targetObj.ParentID == nil {
		return fmt.Errorf("cannot modify file-level object as embedded")
	}

	// Find the type declaration line
	typeDeclLine := targetObj.LineStart + 1
	lines := strings.Split(string(content), "\n")

	if typeDeclLine-1 >= len(lines) {
		return fmt.Errorf("type declaration line not found")
	}

	declLine := lines[typeDeclLine-1]
	trimmedDecl := strings.TrimSpace(declLine)
	if !strings.HasPrefix(trimmedDecl, "::") {
		return fmt.Errorf("expected type declaration at line %d", typeDeclLine)
	}

	// Merge existing fields with updates
	newFields := make(map[string]schema.FieldValue)
	for k, v := range targetObj.Fields {
		newFields[k] = v
	}

	for fieldName, value := range updates {
		newFields[fieldName] = parseFieldValueToSchema(value)
	}

	// Preserve leading whitespace
	leadingSpace := ""
	for _, c := range declLine {
		if c == ' ' || c == '\t' {
			leadingSpace += string(c)
		} else {
			break
		}
	}

	// Serialize and replace
	newDeclLine := leadingSpace + parser.SerializeTypeDeclaration(targetObj.ObjectType, newFields)
	lines[typeDeclLine-1] = newDeclLine

	// Write the file back
	newContent := strings.Join(lines, "\n")
	if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("write error: %w", err)
	}

	// Auto-reindex if configured
	if vaultCfg.IsAutoReindexEnabled() {
		reindexFile(vaultPath, filePath)
	}

	return nil
}

// parseFieldValueToSchema converts a string value to schema.FieldValue.
func parseFieldValueToSchema(value string) schema.FieldValue {
	parsed := parseFieldValue(value)
	return convertToSchemaFieldValue(parsed)
}

// convertToSchemaFieldValue converts an interface{} to schema.FieldValue.
func convertToSchemaFieldValue(v interface{}) schema.FieldValue {
	switch val := v.(type) {
	case bool:
		return schema.Bool(val)
	case string:
		// Check if it's a reference
		if strings.HasPrefix(val, "[[") && strings.HasSuffix(val, "]]") {
			return schema.Ref(val[2 : len(val)-2])
		}
		return schema.String(val)
	case []interface{}:
		var items []schema.FieldValue
		for _, item := range val {
			items = append(items, convertToSchemaFieldValue(item))
		}
		return schema.Array(items)
	default:
		return schema.String(fmt.Sprintf("%v", v))
	}
}

func init() {
	setCmd.Flags().BoolVar(&setStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	setCmd.Flags().BoolVar(&setConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
	rootCmd.AddCommand(setCmd)
}
