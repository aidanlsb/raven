package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	moveForce         bool
	moveUpdateRefs    bool
	moveSkipTypeCheck bool
	moveStdin         bool
	moveConfirm       bool
)

var moveCmd = &cobra.Command{
	Use:   "move <source> <destination>",
	Short: "Move or rename an object within the vault",
	Long: `Move or rename a file/object within the vault.

Both source and destination must be within the vault. This command:
- Validates paths are within the vault (security constraint)
- Updates all references to the moved file if --update-refs is set
- Warns if moving to a type's default directory with mismatched type
- Creates destination directories if needed

Bulk operations:
  Use --stdin to read object IDs from stdin (one per line).
  Destination must be a directory (ending with /).
  Bulk operations preview changes by default; use --confirm to apply.

Examples:
  rvn move people/loki people/loki-archived      # Rename
  rvn move inbox/task.md projects/website/task.md # Move to subdirectory
  rvn move drafts/person.md people/freya.md --update-refs

Bulk examples:
  rvn query "object:project .status==archived" --ids | rvn move --stdin archive/projects/
  rvn query "object:project .status==archived" --ids | rvn move --stdin archive/projects/ --confirm`,
	Args: cobra.MaximumNArgs(2),
	ValidArgsFunction: completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    true,
		NonTargetDirective:  cobra.ShellCompDirectiveDefault,
	}),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		// Handle --stdin mode for bulk operations
		if moveStdin {
			return runMoveBulk(args, vaultPath)
		}

		// Single object mode - requires source and destination
		if len(args) < 2 {
			return handleErrorMsg(ErrMissingArgument, "requires source and destination arguments", "Usage: rvn move <source> <destination>")
		}

		return moveSingleObject(vaultPath, args[0], args[1])
	},
}

// runMoveBulk handles bulk move operations from stdin.
func runMoveBulk(args []string, vaultPath string) error {
	// Destination is provided as argument
	if len(args) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no destination provided", "Usage: rvn move --stdin <destination-directory/>")
	}
	destination := args[0]

	// Destination must be a directory (end with /)
	if !strings.HasSuffix(destination, "/") {
		return handleErrorMsg(ErrInvalidInput,
			"destination must be a directory (end with /)",
			"Example: rvn move --stdin archive/projects/")
	}

	// Read IDs from stdin
	ids, embedded, err := ReadIDsFromStdin()
	if err != nil {
		return handleError(ErrInternal, err, "")
	}

	if len(ids) == 0 && len(embedded) == 0 {
		return handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
	}

	// Build warnings for embedded objects
	var warnings []Warning
	if w := BuildEmbeddedSkipWarning(embedded); w != nil {
		warnings = append(warnings, *w)
	}

	// Load vault config
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}

	// If not confirming, show preview
	if !moveConfirm {
		return previewMoveBulk(vaultPath, ids, destination, warnings, vaultCfg)
	}

	// Apply the moves
	return applyMoveBulk(vaultPath, ids, destination, warnings, vaultCfg)
}

// previewMoveBulk shows a preview of bulk move operations.
func previewMoveBulk(vaultPath string, ids []string, destDir string, warnings []Warning, vaultCfg *config.VaultConfig) error {
	previewResult, err := objectsvc.PreviewMoveBulk(objectsvc.MoveBulkRequest{
		VaultPath:      vaultPath,
		VaultConfig:    vaultCfg,
		ObjectIDs:      ids,
		DestinationDir: destDir,
	})
	if err != nil {
		return handleError(ErrInvalidInput, err, "Example: rvn move --stdin archive/projects/")
	}

	preview := &BulkPreview{
		Action:   "move",
		Items:    make([]BulkPreviewItem, 0, len(previewResult.Items)),
		Skipped:  make([]BulkResult, 0, len(previewResult.Skipped)),
		Total:    previewResult.Total,
		Warnings: warnings,
	}
	for _, item := range previewResult.Items {
		preview.Items = append(preview.Items, BulkPreviewItem{
			ID:      item.ID,
			Action:  item.Action,
			Details: item.Details,
		})
	}
	for _, skip := range previewResult.Skipped {
		preview.Skipped = append(preview.Skipped, BulkResult{
			ID:     skip.ID,
			Status: skip.Status,
			Reason: skip.Reason,
		})
	}

	return outputBulkPreview(preview, map[string]interface{}{
		"destination": destDir,
	})
}

// applyMoveBulk applies bulk move operations.
func applyMoveBulk(vaultPath string, ids []string, destDir string, warnings []Warning, vaultCfg *config.VaultConfig) error {
	// Load schema for type checking
	sch, _ := schema.Load(vaultPath)
	parseOpts := buildParseOptions(vaultCfg)
	summaryResult, err := objectsvc.ApplyMoveBulk(objectsvc.MoveBulkRequest{
		VaultPath:      vaultPath,
		VaultConfig:    vaultCfg,
		Schema:         sch,
		ObjectIDs:      ids,
		DestinationDir: destDir,
		UpdateRefs:     moveUpdateRefs,
		ParseOptions:   parseOpts,
	})
	if err != nil {
		return handleError(ErrInvalidInput, err, "Example: rvn move --stdin archive/projects/")
	}

	results := make([]BulkResult, 0, len(summaryResult.Results))
	for _, result := range summaryResult.Results {
		results = append(results, BulkResult{
			ID:      result.ID,
			Status:  result.Status,
			Reason:  result.Reason,
			Details: result.Details,
		})
	}

	combinedWarnings := append([]Warning{}, warnings...)
	for _, warningMessage := range summaryResult.WarningMessages {
		combinedWarnings = append(combinedWarnings, Warning{
			Code:    WarnIndexUpdateFailed,
			Message: warningMessage,
			Ref:     "Run 'rvn reindex' to rebuild the database",
		})
	}

	summary := buildBulkSummary("move", results, combinedWarnings)
	return outputBulkSummary(summary, combinedWarnings, map[string]interface{}{
		"destination": destDir,
	})
}

// moveSingleObject handles single move operation (non-bulk mode).
func moveSingleObject(vaultPath, source, destination string) error {
	start := time.Now()
	originalDestination := destination
	destinationIsDirectory := strings.HasSuffix(originalDestination, "/") || strings.HasSuffix(originalDestination, "\\")

	// Load vault config for directory roots
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}

	// Resolve source using unified resolver (supports short names, aliases, etc.)
	sourceResult, err := ResolveReference(source, ResolveOptions{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
	})
	if err != nil {
		return handleResolveError(err, source)
	}
	sourceFile := sourceResult.FilePath

	// Security: Validate source is within vault
	if err := paths.ValidateWithinVault(vaultPath, sourceFile); err != nil {
		return handleErrorMsg(ErrValidationFailed,
			"Source path is outside vault",
			"Files can only be moved within the vault")
	}
	sourceRelPath, err := filepath.Rel(vaultPath, sourceFile)
	if err != nil {
		return handleError(ErrInternal, err, "Failed to resolve source path")
	}
	sourceID := vaultCfg.FilePathToObjectID(sourceRelPath)

	// If destination is a directory (trailing slash), keep the source filename.
	if destinationIsDirectory {
		sourceBase := strings.TrimSuffix(filepath.Base(sourceRelPath), ".md")
		if strings.TrimSpace(sourceBase) == "" {
			return handleErrorMsg(ErrInvalidInput, "source has an invalid filename", "Use an explicit destination file path")
		}
		destination = filepath.ToSlash(filepath.Join(originalDestination, sourceBase))
	}

	// Normalize destination path (add .md if missing)
	destination = paths.EnsureMDExtension(destination)
	destinationBase := strings.TrimSuffix(filepath.Base(destination), ".md")
	if strings.TrimSpace(destinationBase) == "" {
		return handleErrorMsg(ErrInvalidInput, "destination has an empty filename", "Use a non-empty destination filename or a directory ending with /")
	}

	// Build destination path - apply directory roots if configured
	destPath := destination
	if vaultCfg.HasDirectoriesConfig() {
		// If destination is an object ID (like "people/freya.md"), resolve to file path
		destPath = vaultCfg.ResolveReferenceToFilePath(strings.TrimSuffix(destination, ".md"))
	}
	destFile := filepath.Join(vaultPath, destPath)

	// Security: Validate destination is within vault
	if err := paths.ValidateWithinVault(vaultPath, destFile); err != nil {
		return handleErrorMsg(ErrValidationFailed,
			"Destination path is outside vault",
			"Files can only be moved within the vault")
	}

	// Security: Ensure destination is not in .raven directory
	relDest, _ := filepath.Rel(vaultPath, destFile)
	if strings.HasPrefix(relDest, ".raven") || strings.HasPrefix(relDest, ".trash") {
		return handleErrorMsg(ErrValidationFailed,
			"Cannot move to system directory",
			"Destination cannot be in .raven or .trash directories")
	}

	// Check if destination already exists
	if _, err := os.Stat(destFile); err == nil {
		return handleErrorMsg(ErrValidationFailed,
			fmt.Sprintf("Destination '%s' already exists", destination),
			"Choose a different destination or delete the existing file first")
	}

	// Load schema for type checking
	sch, err := schema.Load(vaultPath)
	if err != nil {
		sch = schema.NewSchema()
	}

	// Build parse options from vault config
	parseOpts := buildParseOptions(vaultCfg)

	// Parse source file to get its type
	content, err := os.ReadFile(sourceFile)
	if err != nil {
		return handleError(ErrFileReadError, err, "")
	}
	doc, err := parser.ParseDocumentWithOptions(string(content), sourceFile, vaultPath, parseOpts)
	if err != nil {
		return handleError(ErrInternal, err, "Failed to parse source file")
	}

	// Get file type from first object
	var fileType string
	if len(doc.Objects) > 0 {
		fileType = doc.Objects[0].ObjectType
	}

	// Check for type-directory mismatch
	var warnings []Warning
	destDir := filepath.Dir(relDest)
	mismatchType := ""
	for typeName, typeDef := range sch.Types {
		if typeDef.DefaultPath != "" {
			defaultPath := strings.TrimSuffix(typeDef.DefaultPath, "/")
			if destDir == defaultPath && typeName != fileType {
				mismatchType = typeName
				break
			}
		}
	}

	if mismatchType != "" && !moveSkipTypeCheck {
		warnings = append(warnings, Warning{
			Code: WarnTypeMismatch,
			Message: fmt.Sprintf("Moving to '%s/' which is the default directory for type '%s', but file has type '%s'",
				destDir, mismatchType, fileType),
			Ref: fmt.Sprintf("Use --skip-type-check to proceed, or change the file's type to '%s'", mismatchType),
		})

		// In JSON mode, return warning for agent to handle
		if isJSONOutput() {
			result := MoveResult{
				Source:       vaultCfg.FilePathToObjectID(source),
				Destination:  vaultCfg.FilePathToObjectID(destPath),
				NeedsConfirm: true,
				Reason:       fmt.Sprintf("Type mismatch: file is '%s' but destination is default path for '%s'", fileType, mismatchType),
			}
			outputSuccessWithWarnings(result, warnings, nil)
			return nil
		}

		// Interactive confirmation
		if !moveForce {
			fmt.Fprintf(os.Stderr, "⚠ Warning: Moving to '%s/' which is the default directory for type '%s'\n", destDir, mismatchType)
			fmt.Fprintf(os.Stderr, "  But this file has type '%s'\n\n", fileType)
			fmt.Fprint(os.Stderr, "Proceed anyway? [y/N]: ")

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
	}

	// Find backlinks to update
	serviceResult, err := objectsvc.MoveFile(objectsvc.MoveFileRequest{
		VaultPath:         vaultPath,
		SourceFile:        sourceFile,
		DestinationFile:   destFile,
		SourceObjectID:    sourceID,
		DestinationObject: vaultCfg.FilePathToObjectID(destPath),
		UpdateRefs:        moveUpdateRefs,
		FailOnIndexError:  false,
		VaultConfig:       vaultCfg,
		Schema:            sch,
		ParseOptions:      parseOpts,
	})
	if err != nil {
		var svcErr *objectsvc.Error
		if errors.As(err, &svcErr) {
			switch svcErr.Code {
			case objectsvc.ErrorInvalidInput:
				return handleErrorMsg(ErrInvalidInput, svcErr.Message, svcErr.Suggestion)
			case objectsvc.ErrorFileRead:
				return handleError(ErrFileReadError, svcErr, svcErr.Suggestion)
			case objectsvc.ErrorFileWrite:
				return handleError(ErrFileWriteError, svcErr, svcErr.Suggestion)
			case objectsvc.ErrorValidationFailed:
				return handleErrorMsg(ErrValidationFailed, svcErr.Message, svcErr.Suggestion)
			default:
				return handleError(ErrInternal, svcErr, svcErr.Suggestion)
			}
		}
		return handleError(ErrFileWriteError, err, "")
	}

	updatedRefs := serviceResult.UpdatedRefs
	for _, warningMessage := range serviceResult.WarningMessages {
		warnings = append(warnings, Warning{
			Code:    WarnIndexUpdateFailed,
			Message: warningMessage,
			Ref:     "Run 'rvn reindex' to rebuild the database",
		})
	}

	elapsed := time.Since(start).Milliseconds()

	if isJSONOutput() {
		result := MoveResult{
			Source:      sourceID,
			Destination: vaultCfg.FilePathToObjectID(destPath),
			UpdatedRefs: updatedRefs,
		}
		outputSuccessWithWarnings(result, warnings, &Meta{QueryTimeMs: elapsed})
		return nil
	}

	fmt.Println(ui.Checkf("Moved %s → %s", ui.FilePath(sourceRelPath), ui.FilePath(destination)))
	if len(updatedRefs) > 0 {
		fmt.Printf("  Updated %d references\n", len(updatedRefs))
	}

	return nil
}

// MoveResult represents the result of a move operation.
type MoveResult struct {
	Source       string   `json:"source"`
	Destination  string   `json:"destination"`
	UpdatedRefs  []string `json:"updated_refs,omitempty"`
	NeedsConfirm bool     `json:"needs_confirm,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

func updateReference(vaultPath string, vaultCfg *config.VaultConfig, sourceID, oldRef, newRef string) error {
	return objectsvc.UpdateReference(vaultPath, vaultCfg, sourceID, oldRef, newRef)
}

func updateReferenceAtLine(vaultPath string, vaultCfg *config.VaultConfig, sourceID string, line int, oldRef, newRef string) error {
	return objectsvc.UpdateReferenceAtLine(vaultPath, vaultCfg, sourceID, line, oldRef, newRef)
}

func updateAllRefVariantsAtLine(vaultPath string, vaultCfg *config.VaultConfig, sourceID string, line int, oldID, oldBase, newRef, objectRoot, pageRoot string) error {
	return objectsvc.UpdateAllRefVariantsAtLine(vaultPath, vaultCfg, sourceID, line, oldID, oldBase, newRef, objectRoot, pageRoot)
}

func replaceAllRefVariants(content, oldID, oldBase, newRef, objectRoot, pageRoot string) string {
	return objectsvc.ReplaceAllRefVariants(content, oldID, oldBase, newRef, objectRoot, pageRoot)
}

func chooseReplacementRefBase(oldBase, sourceID, destID string, aliasSlugToID map[string]string, res *resolver.Resolver) string {
	return objectsvc.ChooseReplacementRefBase(oldBase, sourceID, destID, aliasSlugToID, res)
}

func init() {
	moveCmd.Flags().BoolVar(&moveForce, "force", false, "Skip confirmation prompts")
	moveCmd.Flags().BoolVar(&moveUpdateRefs, "update-refs", true, "Update references to moved file")
	moveCmd.Flags().BoolVar(&moveSkipTypeCheck, "skip-type-check", false, "Skip type-directory mismatch warning")
	moveCmd.Flags().BoolVar(&moveStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	moveCmd.Flags().BoolVar(&moveConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
	rootCmd.AddCommand(moveCmd)
}
