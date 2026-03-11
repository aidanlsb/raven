package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/vault"
)

var (
	setStdin      bool
	setConfirm    bool
	setFieldsJSON string
)

var setCmd = &cobra.Command{
	Use:   "set <object-id> <field=value>...",
	Short: "Set frontmatter fields on an object",
	Long: `Set one or more frontmatter fields on an existing object.

The object ID can be a full path (e.g., "people/freya") or a short reference
that uniquely identifies an object. Field values are validated against the
schema if the object has a known type. Unknown fields are rejected.

Bulk operations:
  Use --stdin to read object IDs from stdin (one per line).
  Bulk operations preview changes by default; use --confirm to apply.

Examples:
  rvn set people/freya email=freya@asgard.realm
  rvn set people/freya name="Freya" email=freya@vanaheim.realm
  rvn set projects/website status=active priority=high
  rvn set projects/website --json

Bulk examples:
  rvn query "object:project .status==active" --ids | rvn set --stdin status=archived
  rvn query "object:project .status==active" --ids | rvn set --stdin status=archived --confirm`,
	Args: cobra.ArbitraryArgs,
	ValidArgsFunction: completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    true,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	}),
	RunE: runSet,
}

func runSet(cmd *cobra.Command, args []string) error {
	vaultPath := getVaultPath()

	// Handle --stdin mode for bulk operations
	if setStdin {
		if strings.TrimSpace(setFieldsJSON) != "" {
			return handleErrorMsg(ErrInvalidInput,
				"--fields-json is not supported with --stdin",
				"Use positional field=value updates when using --stdin")
		}
		return runSetBulk(cmd, args, vaultPath)
	}

	// Single object mode - requires object-id and at least one update source.
	if len(args) < 1 {
		return handleErrorMsg(ErrMissingArgument, "requires object-id", "Usage: rvn set <object-id> field=value...")
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

	typedUpdates, err := parseFieldValuesJSON(setFieldsJSON)
	if err != nil {
		return handleErrorMsg(ErrInvalidInput, "invalid --fields-json payload", "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
	}

	if len(updates) == 0 && len(typedUpdates) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no fields to set", "Usage: rvn set <object-id> field=value... or --fields-json '{...}'")
	}

	return setSingleObject(vaultPath, objectID, updates, typedUpdates)
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
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaInvalid, err, "Fix schema.yaml and try again")
	}

	// Load vault config (optional, used for roots + auto-reindex)
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
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
	parseOpts := buildParseOptions(vaultCfg)

	preview := buildBulkPreview("set", ids, warnings, func(id string) (*BulkPreviewItem, *BulkResult) {
		// Check if this is an embedded object
		if IsEmbeddedID(id) {
			return previewSetEmbedded(vaultPath, id, updates, sch, vaultCfg, parseOpts)
		}

		filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
		if err != nil {
			return nil, &BulkResult{ID: id, Status: "skipped", Reason: "object not found"}
		}

		// Read current values to show diff
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, &BulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("read error: %v", err)}
		}

		fm, err := parser.ParseFrontmatter(string(content))
		if err != nil || fm == nil {
			return nil, &BulkResult{ID: id, Status: "skipped", Reason: "no frontmatter"}
		}

		objectType := fm.ObjectType
		if objectType == "" {
			objectType = "page"
		}
		if ufErr := detectUnknownSetFields(objectType, updates, sch, map[string]bool{"alias": true}); ufErr != nil {
			return nil, &BulkResult{ID: id, Status: "skipped", Reason: ufErr.Error()}
		}

		_, resolvedUpdates, _, err := prepareValidatedFrontmatterMutation(
			string(content),
			fm,
			objectType,
			updates,
			sch,
			map[string]bool{"alias": true},
		)
		if err != nil {
			var validationErr *fieldValidationError
			if errors.As(err, &validationErr) {
				return nil, &BulkResult{ID: id, Status: "skipped", Reason: validationErr.Error()}
			}
			return nil, &BulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("validation error: %v", err)}
		}

		// Build change summary
		changes := make(map[string]string)
		for field, resolvedVal := range resolvedUpdates {
			oldVal := "<unset>"
			if fm.Fields != nil {
				if v, ok := fm.Fields[field]; ok {
					oldVal = fmt.Sprintf("%v", v)
				}
			}
			changes[field] = fmt.Sprintf("%s (was: %s)", resolvedVal, oldVal)
		}

		return &BulkPreviewItem{
			ID:      id,
			Action:  "set",
			Changes: changes,
		}, nil
	})

	return outputBulkPreview(preview, map[string]interface{}{
		"fields": updates,
	})
}

// applySetBulk applies bulk set operations.
func applySetBulk(vaultPath string, ids []string, updates map[string]string, warnings []Warning, sch *schema.Schema, vaultCfg *config.VaultConfig) error {
	parseOpts := buildParseOptions(vaultCfg)

	results := applyBulk(ids, func(id string) BulkResult {
		result := BulkResult{ID: id}
		// Check if this is an embedded object
		if IsEmbeddedID(id) {
			err := applySetEmbedded(vaultPath, id, updates, sch, vaultCfg, parseOpts)
			if err != nil {
				result.Status = "error"
				result.Reason = err.Error()
			} else {
				result.Status = "modified"
			}
			return result
		}

		filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, id, vaultCfg)
		if err != nil {
			result.Status = "skipped"
			result.Reason = "object not found"
			return result
		}

		_, err = objectsvc.SetObjectFile(objectsvc.SetObjectFileRequest{
			FilePath:      filePath,
			ObjectID:      id,
			Updates:       updates,
			Schema:        sch,
			AllowedFields: map[string]bool{"alias": true},
		})
		if err != nil {
			result.Status = "error"
			var svcErr *objectsvc.Error
			var unknownErr *unknownFieldMutationError
			var validationErr *fieldValidationError
			switch {
			case errors.As(err, &svcErr):
				result.Reason = svcErr.Message
			case errors.As(err, &unknownErr):
				result.Reason = unknownErr.Error()
			case errors.As(err, &validationErr):
				result.Reason = validationErr.Error()
			default:
				result.Reason = fmt.Sprintf("update error: %v", err)
			}
			return result
		}

		// Auto-reindex if configured
		maybeReindex(vaultPath, filePath, vaultCfg)

		result.Status = "modified"
		return result
	})

	summary := buildBulkSummary("set", results, warnings)
	return outputBulkSummary(summary, warnings, map[string]interface{}{
		"fields": updates,
	})
}

// setSingleObject sets fields on a single object (non-bulk mode).
func setSingleObject(vaultPath, reference string, updates map[string]string, typedUpdates map[string]schema.FieldValue) error {
	// Load schema for validation
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaInvalid, err, "Fix schema.yaml and try again")
	}

	// Load vault config
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
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
		return setEmbeddedObject(vaultPath, result.ObjectID, updates, typedUpdates, sch, vaultCfg)
	}

	objectID := result.ObjectID
	filePath := result.FilePath

	serviceResult, err := objectsvc.SetObjectFile(objectsvc.SetObjectFileRequest{
		FilePath:      filePath,
		ObjectID:      objectID,
		Updates:       updates,
		TypedUpdates:  typedUpdates,
		Schema:        sch,
		AllowedFields: map[string]bool{"alias": true},
	})
	if err != nil {
		var svcErr *objectsvc.Error
		if errors.As(err, &svcErr) {
			switch svcErr.Code {
			case objectsvc.ErrorInvalidInput:
				return handleErrorMsg(ErrInvalidInput, svcErr.Message, svcErr.Suggestion)
			case objectsvc.ErrorValidationFailed:
				return handleErrorMsg(ErrValidationFailed, svcErr.Message, svcErr.Suggestion)
			case objectsvc.ErrorFileRead:
				return handleError(ErrFileReadError, svcErr, svcErr.Suggestion)
			case objectsvc.ErrorFileWrite:
				return handleError(ErrFileWriteError, svcErr, svcErr.Suggestion)
			default:
				return handleError(ErrInternal, svcErr, svcErr.Suggestion)
			}
		}
		var unknownErr *unknownFieldMutationError
		var validationErr *fieldValidationError
		if errors.As(err, &unknownErr) {
			return handleErrorWithDetails(ErrUnknownField, unknownErr.Error(), unknownErr.Suggestion(), unknownErr.Details())
		}
		if errors.As(err, &validationErr) {
			return handleErrorMsg(ErrValidationFailed, validationErr.Error(), validationErr.Suggestion())
		}
		return handleError(ErrFileWriteError, err, "Failed to update frontmatter")
	}

	// Auto-reindex if configured
	maybeReindex(vaultPath, filePath, vaultCfg)

	relPath, _ := filepath.Rel(vaultPath, filePath)
	objectType := serviceResult.ObjectType
	resolvedUpdates := serviceResult.ResolvedUpdates
	validationWarnings := serviceResult.WarningMessages

	// Output
	if isJSONOutput() {
		result := map[string]interface{}{
			"file":           relPath,
			"object_id":      objectID,
			"type":           objectType,
			"updated_fields": resolvedUpdates,
		}
		if len(validationWarnings) > 0 {
			warnings := warningMessagesToWarnings(validationWarnings)
			outputSuccessWithWarnings(result, warnings, nil)
		} else {
			outputSuccess(result, nil)
		}
		return nil
	}

	// Human-readable output with diff-style changes
	fmt.Println(ui.Checkf("Updated %s", ui.FilePath(relPath)))
	var fieldNames []string
	for name := range resolvedUpdates {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)
	for _, name := range fieldNames {
		oldVal := ""
		if serviceResult.PreviousFields != nil {
			if v, ok := serviceResult.PreviousFields[name]; ok {
				oldVal = fmt.Sprintf("%v", v)
			}
		}
		if oldVal != "" && oldVal != resolvedUpdates[name] {
			fmt.Printf("  %s\n", ui.FieldChange(name, oldVal, resolvedUpdates[name]))
		} else if oldVal == "" {
			fmt.Printf("  %s\n", ui.FieldAdd(name, resolvedUpdates[name]))
		} else {
			fmt.Printf("  %s\n", ui.FieldSet(name, resolvedUpdates[name]))
		}
	}
	for _, warning := range validationWarnings {
		fmt.Printf("  %s\n", ui.Warning(warning))
	}

	return nil
}

func detectUnknownSetFields(objectType string, updates map[string]string, sch *schema.Schema, allowedUnknown map[string]bool) *unknownFieldMutationError {
	fieldNames := make([]string, 0, len(updates))
	for key := range updates {
		fieldNames = append(fieldNames, key)
	}
	return detectUnknownFieldMutationByNames(
		objectType,
		sch,
		fieldNames,
		allowedUnknown,
	)
}

// setEmbeddedObject sets fields on an embedded object.
func setEmbeddedObject(vaultPath, objectID string, updates map[string]string, typedUpdates map[string]schema.FieldValue, sch *schema.Schema, vaultCfg *config.VaultConfig) error {
	// Parse the embedded ID: fileID#slug
	fileID, slug, isEmbedded := paths.ParseEmbeddedID(objectID)
	if !isEmbedded {
		return handleErrorMsg(ErrInvalidInput, "invalid embedded object ID", "Expected format: file-id#embedded-id")
	}

	// Resolve file ID to file path
	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileID, vaultCfg)
	if err != nil {
		return handleError(ErrFileDoesNotExist, err, "")
	}

	serviceResult, err := objectsvc.SetEmbeddedObject(objectsvc.SetEmbeddedObjectRequest{
		VaultPath:      vaultPath,
		FilePath:       filePath,
		ObjectID:       objectID,
		Updates:        updates,
		TypedUpdates:   typedUpdates,
		Schema:         sch,
		AllowedFields:  map[string]bool{"alias": true, "id": true},
		DocumentParser: buildParseOptions(vaultCfg),
	})
	if err != nil {
		var svcErr *objectsvc.Error
		if errors.As(err, &svcErr) {
			switch svcErr.Code {
			case objectsvc.ErrorInvalidInput:
				return handleErrorMsg(ErrInvalidInput, svcErr.Message, svcErr.Suggestion)
			case objectsvc.ErrorValidationFailed:
				return handleErrorMsg(ErrValidationFailed, svcErr.Message, svcErr.Suggestion)
			case objectsvc.ErrorFileRead:
				return handleError(ErrFileReadError, svcErr, svcErr.Suggestion)
			case objectsvc.ErrorFileWrite:
				return handleError(ErrFileWriteError, svcErr, svcErr.Suggestion)
			default:
				return handleError(ErrInternal, svcErr, svcErr.Suggestion)
			}
		}
		var unknownErr *unknownFieldMutationError
		var validationErr *fieldValidationError
		if errors.As(err, &unknownErr) {
			return handleErrorWithDetails(ErrUnknownField, unknownErr.Error(), unknownErr.Suggestion(), unknownErr.Details())
		}
		if errors.As(err, &validationErr) {
			return handleErrorMsg(ErrValidationFailed, validationErr.Error(), validationErr.Suggestion())
		}
		return handleError(ErrFileWriteError, err, "Failed to update embedded fields")
	}

	// Auto-reindex if configured
	maybeReindex(vaultPath, filePath, vaultCfg)

	relPath, _ := filepath.Rel(vaultPath, filePath)
	objectType := serviceResult.ObjectType
	resolvedUpdates := serviceResult.ResolvedUpdates
	validationWarnings := serviceResult.WarningMessages
	previousFields := serviceResult.PreviousFields

	// Output
	if isJSONOutput() {
		result := map[string]interface{}{
			"file":           relPath,
			"object_id":      objectID,
			"type":           objectType,
			"embedded":       true,
			"updated_fields": resolvedUpdates,
		}
		if len(validationWarnings) > 0 {
			warnings := warningMessagesToWarnings(validationWarnings)
			outputSuccessWithWarnings(result, warnings, nil)
		} else {
			outputSuccess(result, nil)
		}
		return nil
	}

	// Human-readable output with diff-style changes
	fmt.Println(ui.Checkf("Updated %s %s", ui.FilePath(relPath), ui.Hint("(embedded: "+slug+")")))
	var fieldNames []string
	for name := range resolvedUpdates {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)
	for _, name := range fieldNames {
		oldVal := ""
		if previousFields != nil {
			if v, ok := previousFields[name]; ok {
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
		if oldVal != "" && oldVal != resolvedUpdates[name] {
			fmt.Printf("  %s\n", ui.FieldChange(name, oldVal, resolvedUpdates[name]))
		} else if oldVal == "" {
			fmt.Printf("  %s\n", ui.FieldAdd(name, resolvedUpdates[name]))
		} else {
			fmt.Printf("  %s\n", ui.FieldSet(name, resolvedUpdates[name]))
		}
	}
	for _, warning := range validationWarnings {
		fmt.Printf("  %s\n", ui.Warning(warning))
	}

	return nil
}

// previewSetEmbedded generates a preview for an embedded object.
func previewSetEmbedded(vaultPath, id string, updates map[string]string, sch *schema.Schema, vaultCfg *config.VaultConfig, parseOpts *parser.ParseOptions) (*BulkPreviewItem, *BulkResult) {
	// Parse the embedded ID
	fileID, _, isEmbedded := paths.ParseEmbeddedID(id)
	if !isEmbedded {
		return nil, &BulkResult{ID: id, Status: "skipped", Reason: "invalid embedded ID format"}
	}

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
	if ufErr := detectUnknownSetFields(targetObj.ObjectType, updates, sch, map[string]bool{"alias": true, "id": true}); ufErr != nil {
		return nil, &BulkResult{ID: id, Status: "skipped", Reason: ufErr.Error()}
	}

	_, resolvedUpdates, _, err := prepareValidatedFieldMutation(
		targetObj.ObjectType,
		targetObj.Fields,
		updates,
		sch,
		map[string]bool{"alias": true, "id": true},
	)
	if err != nil {
		var validationErr *fieldValidationError
		if errors.As(err, &validationErr) {
			return nil, &BulkResult{ID: id, Status: "skipped", Reason: validationErr.Error()}
		}
		return nil, &BulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("validation error: %v", err)}
	}

	// Build change summary
	changes := make(map[string]string)
	for field, resolvedVal := range resolvedUpdates {
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
		changes[field] = fmt.Sprintf("%s (was: %s)", resolvedVal, oldVal)
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
	fileID, _, isEmbedded := paths.ParseEmbeddedID(id)
	if !isEmbedded {
		return fmt.Errorf("invalid embedded ID format")
	}

	// Resolve file ID to file path
	filePath, err := vault.ResolveObjectToFileWithConfig(vaultPath, fileID, vaultCfg)
	if err != nil {
		return fmt.Errorf("parent file not found: %w", err)
	}

	_, err = objectsvc.SetEmbeddedObject(objectsvc.SetEmbeddedObjectRequest{
		VaultPath:      vaultPath,
		FilePath:       filePath,
		ObjectID:       id,
		Updates:        updates,
		Schema:         sch,
		AllowedFields:  map[string]bool{"alias": true, "id": true},
		DocumentParser: parseOpts,
	})
	if err != nil {
		var svcErr *objectsvc.Error
		if errors.As(err, &svcErr) {
			return errors.New(svcErr.Message)
		}
		return err
	}

	// Auto-reindex if configured
	maybeReindex(vaultPath, filePath, vaultCfg)

	return nil
}

func init() {
	setCmd.Flags().BoolVar(&setStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	setCmd.Flags().BoolVar(&setConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
	setCmd.Flags().StringVar(&setFieldsJSON, "fields-json", "", "Set fields via JSON object (typed values)")
	rootCmd.AddCommand(setCmd)
}
