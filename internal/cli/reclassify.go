package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	reclassifyFieldFlags []string
	reclassifyNoMove     bool
	reclassifyUpdateRefs bool
	reclassifyForce      bool
)

var reclassifyCmd = &cobra.Command{
	Use:   "reclassify <object> <new-type>",
	Short: "Change an object's type",
	Long: `Change an object's type, updating frontmatter, applying defaults,
and optionally moving the file to the new type's default directory.

Required fields on the new type are handled as follows:
- If a required field has a default, it is applied automatically
- Missing required fields can be supplied via --field flags
- Interactive mode prompts for missing required fields
- JSON mode returns an error with retry_with template

Fields present on the old type but not defined on the new type are
identified as "dropped fields" and require confirmation before removal.
Use --force to skip this confirmation.

Examples:
  rvn reclassify inbox/note book --json
  rvn reclassify people/freya company --field industry=tech --json
  rvn reclassify pages/draft project --no-move --json
  rvn reclassify inbox/note book --force --json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		objectRef := args[0]
		newTypeName := args[1]

		return runReclassify(vaultPath, objectRef, newTypeName)
	},
}

// ReclassifyResult represents the result of a reclassify operation.
type ReclassifyResult struct {
	ObjectID      string   `json:"object_id"`
	OldType       string   `json:"old_type"`
	NewType       string   `json:"new_type"`
	File          string   `json:"file"`
	Moved         bool     `json:"moved,omitempty"`
	OldPath       string   `json:"old_path,omitempty"`
	NewPath       string   `json:"new_path,omitempty"`
	UpdatedRefs   []string `json:"updated_refs,omitempty"`
	AddedFields   []string `json:"added_fields,omitempty"`
	DroppedFields []string `json:"dropped_fields,omitempty"`
	NeedsConfirm  bool     `json:"needs_confirm,omitempty"`
	Reason        string   `json:"reason,omitempty"`
}

func runReclassify(vaultPath, objectRef, newTypeName string) error {
	start := time.Now()

	// Load vault config + schema
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	// Resolve the object reference
	resolved, err := ResolveReference(objectRef, ResolveOptions{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
	})
	if err != nil {
		return handleResolveError(err, objectRef)
	}

	filePath := resolved.FilePath
	objectID := resolved.ObjectID

	// Read and parse file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}

	fm, err := parser.ParseFrontmatter(string(content))
	if err != nil {
		return handleError(ErrInvalidInput, err, "Failed to parse frontmatter")
	}
	if fm == nil {
		return handleErrorMsg(ErrInvalidInput, "file has no frontmatter", "The file must have YAML frontmatter (---) to reclassify")
	}

	// Get current type
	oldType := fm.ObjectType
	if oldType == "" {
		oldType = "page"
	}

	// Validate new type
	if newTypeName == oldType {
		return handleErrorMsg(ErrInvalidInput,
			fmt.Sprintf("object is already type '%s'", oldType),
			"Specify a different target type")
	}

	if schema.IsBuiltinType(newTypeName) {
		return handleErrorMsg(ErrInvalidInput,
			fmt.Sprintf("cannot reclassify to built-in type '%s'", newTypeName),
			"Built-in types (page, section, date) cannot be used as reclassify targets")
	}

	newTypeDef, typeExists := sch.Types[newTypeName]
	if !typeExists {
		var typeNames []string
		for name := range sch.Types {
			typeNames = append(typeNames, name)
		}
		sort.Strings(typeNames)
		return handleErrorMsg(ErrTypeNotFound,
			fmt.Sprintf("type '%s' not found", newTypeName),
			fmt.Sprintf("Available types: %s", strings.Join(typeNames, ", ")))
	}

	// Parse --field flags
	fieldValues := make(map[string]string)
	for _, f := range reclassifyFieldFlags {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) == 2 {
			fieldValues[parts[0]] = parts[1]
		}
	}

	// Collect missing required fields on the new type
	var missingFields []string
	var fieldDetails []map[string]interface{}
	var addedFields []string

	if newTypeDef != nil {
		var fieldNames []string
		for name := range newTypeDef.Fields {
			fieldNames = append(fieldNames, name)
		}
		sort.Strings(fieldNames)

		reader := bufio.NewReader(os.Stdin)

		for _, fieldName := range fieldNames {
			fieldDef := newTypeDef.Fields[fieldName]
			if fieldDef == nil || !fieldDef.Required {
				continue
			}

			// Check if already present in the file's frontmatter
			if fm.Fields != nil {
				if _, ok := fm.Fields[fieldName]; ok {
					continue
				}
			}

			// Check if provided via --field
			if _, ok := fieldValues[fieldName]; ok {
				addedFields = append(addedFields, fieldName)
				continue
			}

			// Check if there's a default
			if fieldDef.Default != nil {
				fieldValues[fieldName] = fmt.Sprintf("%v", fieldDef.Default)
				addedFields = append(addedFields, fieldName)
				continue
			}

			// Missing required field
			if isJSONOutput() {
				missingFields = append(missingFields, fieldName)
				detail := map[string]interface{}{
					"name":     fieldName,
					"type":     string(fieldDef.Type),
					"required": true,
				}
				if len(fieldDef.Values) > 0 {
					detail["values"] = fieldDef.Values
				}
				fieldDetails = append(fieldDetails, detail)
			} else {
				// Interactive: prompt for value
				fmt.Fprintf(os.Stderr, "%s (required for type '%s'): ", fieldName, newTypeName)
				value, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read input: %w", err)
				}
				value = strings.TrimSpace(value)
				if value == "" {
					return fmt.Errorf("required field '%s' cannot be empty", fieldName)
				}
				fieldValues[fieldName] = value
				addedFields = append(addedFields, fieldName)
			}
		}
	}

	// In JSON mode, error if required fields are missing
	if isJSONOutput() && len(missingFields) > 0 {
		var exampleParts []string
		for _, f := range missingFields {
			exampleParts = append(exampleParts, fmt.Sprintf(`"%s": "<value>"`, f))
		}
		example := fmt.Sprintf(`field: {%s}`, strings.Join(exampleParts, ", "))

		details := map[string]interface{}{
			"missing_fields": fieldDetails,
			"object_id":      objectID,
			"old_type":       oldType,
			"new_type":       newTypeName,
			"retry_with": map[string]interface{}{
				"object":   objectRef,
				"new_type": newTypeName,
				"field":    buildFieldTemplate(missingFields),
			},
		}

		outputError(ErrRequiredField,
			fmt.Sprintf("Missing required fields for type '%s': %s", newTypeName, strings.Join(missingFields, ", ")),
			details,
			fmt.Sprintf("Retry with: %s", example))
		return nil
	}

	// Identify dropped fields (fields on file not in new type's schema)
	var droppedFields []string
	if fm.Fields != nil && newTypeDef != nil {
		for fieldName := range fm.Fields {
			// Skip the "type" meta-field
			if fieldName == "type" {
				continue
			}
			if _, inNewType := newTypeDef.Fields[fieldName]; !inNewType {
				droppedFields = append(droppedFields, fieldName)
			}
		}
		sort.Strings(droppedFields)
	}

	// Handle dropped fields confirmation
	if len(droppedFields) > 0 && !reclassifyForce {
		if isJSONOutput() {
			result := ReclassifyResult{
				ObjectID:      objectID,
				OldType:       oldType,
				NewType:       newTypeName,
				File:          resolved.FilePath,
				DroppedFields: droppedFields,
				NeedsConfirm:  true,
				Reason:        fmt.Sprintf("Fields not defined on type '%s' will be dropped: %s", newTypeName, strings.Join(droppedFields, ", ")),
			}
			relPath, _ := filepath.Rel(vaultPath, filePath)
			result.File = relPath
			outputSuccess(result, nil)
			return nil
		}

		// Interactive confirmation
		fmt.Fprintf(os.Stderr, "The following fields are not defined on type '%s' and will be dropped:\n", newTypeName)
		for _, f := range droppedFields {
			val := ""
			if fm.Fields != nil {
				if v, ok := fm.Fields[f]; ok {
					val = fmt.Sprintf(" (current value: %v)", v.Raw())
				}
			}
			fmt.Fprintf(os.Stderr, "  - %s%s\n", f, val)
		}
		fmt.Fprint(os.Stderr, "\nProceed? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Fprintln(os.Stderr, "Cancelled.")
			return nil
		}
	}

	// Update frontmatter: change type, apply new field values, remove dropped fields
	newContent, err := updateFrontmatterForReclassify(string(content), newTypeName, fieldValues, droppedFields)
	if err != nil {
		return handleError(ErrFileWriteError, err, "Failed to update frontmatter")
	}

	// Write the updated file
	if err := atomicfile.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}

	relPath, _ := filepath.Rel(vaultPath, filePath)
	result := ReclassifyResult{
		ObjectID:      objectID,
		OldType:       oldType,
		NewType:       newTypeName,
		File:          relPath,
		AddedFields:   addedFields,
		DroppedFields: droppedFields,
	}

	// Auto-move to new type's default_path if applicable
	var updatedRefs []string
	if !reclassifyNoMove && newTypeDef.DefaultPath != "" {
		defaultDir := strings.TrimSuffix(newTypeDef.DefaultPath, "/")
		currentDir := filepath.Dir(relPath)

		// Resolve currentDir through directory roots for comparison
		currentObjDir := vaultCfg.FilePathToObjectID(currentDir)
		if currentObjDir == "" {
			currentObjDir = currentDir
		}

		if currentObjDir != defaultDir {
			// Compute destination path
			filename := filepath.Base(relPath)
			destRelPath := vaultCfg.ResolveReferenceToFilePath(
				strings.TrimSuffix(filepath.Join(defaultDir, strings.TrimSuffix(filename, ".md")), ".md"),
			)
			destRelPath = paths.EnsureMDExtension(destRelPath)
			destAbsPath := filepath.Join(vaultPath, destRelPath)

			// Only move if destination doesn't already exist
			if _, err := os.Stat(destAbsPath); os.IsNotExist(err) {
				// Create destination directory
				if err := os.MkdirAll(filepath.Dir(destAbsPath), 0755); err != nil {
					return handleError(ErrFileWriteError, err, "Failed to create destination directory")
				}

				// Update references before moving
				if reclassifyUpdateRefs {
					updatedRefs = updateRefsForMove(vaultPath, vaultCfg, objectID, relPath, destRelPath)
				}

				// Perform the move
				if err := os.Rename(filePath, destAbsPath); err != nil {
					return handleError(ErrFileWriteError, err, "Failed to move file")
				}

				result.Moved = true
				result.OldPath = relPath
				result.NewPath = destRelPath
				result.UpdatedRefs = updatedRefs

				// Update filePath for reindexing
				filePath = destAbsPath
				relPath = destRelPath
				result.File = relPath

				// Update object ID
				newObjectID := vaultCfg.FilePathToObjectID(destRelPath)
				result.ObjectID = newObjectID

				// Remove old index entry
				reindexAfterMove(vaultPath, vaultCfg, sch, objectID, filePath)
			}
		}
	}

	// Reindex the file
	maybeReindex(vaultPath, filePath, vaultCfg)

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	// Human-readable output
	fmt.Println(ui.Checkf("Reclassified %s: %s → %s", ui.FilePath(relPath), oldType, newTypeName))
	if len(addedFields) > 0 {
		fmt.Printf("  Added fields: %s\n", strings.Join(addedFields, ", "))
	}
	if len(droppedFields) > 0 {
		fmt.Printf("  Dropped fields: %s\n", strings.Join(droppedFields, ", "))
	}
	if result.Moved {
		fmt.Printf("  Moved: %s → %s\n", result.OldPath, result.NewPath)
	}
	if len(updatedRefs) > 0 {
		fmt.Printf("  Updated %d references\n", len(updatedRefs))
	}

	return nil
}

// updateFrontmatterForReclassify updates the frontmatter to change the type,
// add new field values, and remove dropped fields.
func updateFrontmatterForReclassify(content, newType string, fieldValues map[string]string, droppedFields []string) (string, error) {
	lines := strings.Split(content, "\n")

	startLine, endLine, ok := parser.FrontmatterBounds(lines)
	if !ok {
		return "", fmt.Errorf("no frontmatter found")
	}
	if endLine == -1 {
		return "", fmt.Errorf("unclosed frontmatter")
	}

	// Parse existing frontmatter
	frontmatterContent := strings.Join(lines[startLine+1:endLine], "\n")
	var yamlData map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterContent), &yamlData); err != nil {
		return "", fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	if yamlData == nil {
		yamlData = make(map[string]interface{})
	}

	// Change type
	yamlData["type"] = newType

	// Apply new field values
	for key, value := range fieldValues {
		yamlData[key] = value
	}

	// Remove dropped fields
	droppedSet := make(map[string]bool, len(droppedFields))
	for _, f := range droppedFields {
		droppedSet[f] = true
	}
	for key := range yamlData {
		if droppedSet[key] {
			delete(yamlData, key)
		}
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

	if endLine+1 < len(lines) {
		result.WriteString("\n")
		result.WriteString(strings.Join(lines[endLine+1:], "\n"))
	}

	return result.String(), nil
}

// updateRefsForMove updates all references pointing to the old object ID to point to the new location.
// Returns the list of source IDs that were updated.
func updateRefsForMove(vaultPath string, vaultCfg *config.VaultConfig, sourceID, oldRelPath, newRelPath string) []string {
	db, err := index.Open(vaultPath)
	if err != nil {
		return nil
	}
	defer db.Close()
	db.SetDailyDirectory(vaultCfg.GetDailyDirectory())

	destID := vaultCfg.FilePathToObjectID(newRelPath)
	objectRoot := vaultCfg.GetObjectsRoot()
	pageRoot := vaultCfg.GetPagesRoot()

	backlinks, _ := db.BacklinksWithRoots(sourceID, objectRoot, pageRoot)

	aliases, _ := db.AllAliases()
	res, _ := db.Resolver(index.ResolverOptions{DailyDirectory: vaultCfg.GetDailyDirectory(), ExtraIDs: []string{destID}})
	aliasSlugToID := make(map[string]string, len(aliases))
	for a, oid := range aliases {
		aliasSlugToID[pages.SlugifyPath(a)] = oid
	}

	var updatedRefs []string
	for _, bl := range backlinks {
		oldRaw := strings.TrimSpace(bl.TargetRaw)
		oldRaw = strings.TrimPrefix(strings.TrimSuffix(oldRaw, "]]"), "[[")
		base := oldRaw
		if i := strings.Index(base, "#"); i >= 0 {
			base = base[:i]
		}
		if base == "" {
			continue
		}
		repl := chooseReplacementRefBase(base, sourceID, destID, aliasSlugToID, res)
		line := 0
		if bl.Line != nil {
			line = *bl.Line
		}
		if err := updateAllRefVariantsAtLine(vaultPath, vaultCfg, bl.SourceID, line, sourceID, base, repl, objectRoot, pageRoot); err == nil {
			updatedRefs = append(updatedRefs, bl.SourceID)
		}
	}

	return updatedRefs
}

// reindexAfterMove removes the old index entry and indexes the new file location.
func reindexAfterMove(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, oldID, newFilePath string) {
	db, err := index.Open(vaultPath)
	if err != nil {
		return
	}
	defer db.Close()
	db.SetDailyDirectory(vaultCfg.GetDailyDirectory())

	// Remove old entry
	_ = db.RemoveDocument(oldID)

	// Index new location
	newContent, err := os.ReadFile(newFilePath)
	if err != nil {
		return
	}
	parseOpts := buildParseOptions(vaultCfg)
	newDoc, err := parser.ParseDocumentWithOptions(string(newContent), newFilePath, vaultPath, parseOpts)
	if err != nil || newDoc == nil {
		return
	}
	if sch != nil {
		_ = db.IndexDocument(newDoc, sch)
	}
}


func init() {
	reclassifyCmd.Flags().StringArrayVar(&reclassifyFieldFlags, "field", nil, "Set field value (can be repeated): --field name=value")
	reclassifyCmd.Flags().BoolVar(&reclassifyNoMove, "no-move", false, "Skip moving file to new type's default_path")
	reclassifyCmd.Flags().BoolVar(&reclassifyUpdateRefs, "update-refs", true, "Update references when file moves")
	reclassifyCmd.Flags().BoolVar(&reclassifyForce, "force", false, "Skip confirmation prompts")
	rootCmd.AddCommand(reclassifyCmd)
}
