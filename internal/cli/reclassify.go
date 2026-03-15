package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/objectsvc"
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
	ValidArgsFunction: completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    false,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	}),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		objectRef := args[0]
		newTypeName := args[1]

		return runReclassify(vaultPath, objectRef, newTypeName)
	},
}

type ReclassifyResult = objectsvc.ReclassifyResult

func runReclassify(vaultPath, objectRef, newTypeName string) error {
	start := time.Now()

	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	resolved, err := ResolveReference(objectRef, ResolveOptions{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
	})
	if err != nil {
		return handleResolveError(err, objectRef)
	}

	fieldValues := parseReclassifyFieldFlags(reclassifyFieldFlags)
	force := reclassifyForce

	for {
		result, err := objectsvc.Reclassify(objectsvc.ReclassifyRequest{
			VaultPath:    vaultPath,
			VaultConfig:  vaultCfg,
			Schema:       sch,
			ObjectRef:    objectRef,
			ObjectID:     resolved.ObjectID,
			FilePath:     resolved.FilePath,
			NewTypeName:  newTypeName,
			FieldValues:  fieldValues,
			NoMove:       reclassifyNoMove,
			UpdateRefs:   reclassifyUpdateRefs,
			Force:        force,
			ParseOptions: buildParseOptions(vaultCfg),
		})
		if err != nil {
			var svcErr *objectsvc.Error
			if errors.As(err, &svcErr) && svcErr.Code == objectsvc.ErrorRequiredField && !isJSONOutput() {
				prompted, promptErr := promptMissingReclassifyFields(newTypeName, svcErr.Details)
				if promptErr != nil {
					return promptErr
				}
				for k, v := range prompted {
					fieldValues[k] = v
				}
				continue
			}
			return mapReclassifyServiceError(err)
		}

		if result.NeedsConfirm && !force {
			if isJSONOutput() {
				outputSuccess(result, nil)
				return nil
			}

			fmt.Fprintf(os.Stderr, "The following fields are not defined on type '%s' and will be dropped:\n", newTypeName)
			for _, f := range result.DroppedFields {
				fmt.Fprintf(os.Stderr, "  - %s\n", f)
			}
			fmt.Fprint(os.Stderr, "\nProceed? [y/N]: ")

			reader := bufio.NewReader(os.Stdin)
			response, readErr := reader.ReadString('\n')
			if readErr != nil {
				return handleError(ErrInternal, readErr, "")
			}
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				fmt.Fprintln(os.Stderr, "Cancelled.")
				return nil
			}
			force = true
			continue
		}

		if result.ChangedFilePath != "" {
			maybeReindex(vaultPath, result.ChangedFilePath, vaultCfg)
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(result, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		fmt.Println(ui.Checkf("Reclassified %s: %s → %s", ui.FilePath(result.File), result.OldType, result.NewType))
		if len(result.AddedFields) > 0 {
			fmt.Printf("  Added fields: %s\n", strings.Join(result.AddedFields, ", "))
		}
		if len(result.DroppedFields) > 0 {
			fmt.Printf("  Dropped fields: %s\n", strings.Join(result.DroppedFields, ", "))
		}
		if result.Moved {
			fmt.Printf("  Moved: %s → %s\n", result.OldPath, result.NewPath)
		}
		if len(result.UpdatedRefs) > 0 {
			fmt.Printf("  Updated %d references\n", len(result.UpdatedRefs))
		}
		for _, warning := range result.WarningMessages {
			fmt.Printf("  %s\n", ui.Warning(warning))
		}

		return nil
	}
}

func mapReclassifyServiceError(err error) error {
	var svcErr *objectsvc.Error
	if !errors.As(err, &svcErr) {
		return handleError(ErrInternal, err, "")
	}

	suggestion := svcErr.Suggestion
	svcCause := svcErr.Cause

	switch svcErr.Code {
	case objectsvc.ErrorTypeNotFound:
		if suggestion == "" {
			suggestion = "Check schema.yaml for available types"
		}
		return handleErrorMsg(ErrTypeNotFound, svcErr.Message, suggestion)
	case objectsvc.ErrorRequiredField:
		if isJSONOutput() {
			outputError(ErrRequiredField, svcErr.Message, svcErr.Details, suggestion)
			return nil
		}
		return handleErrorMsg(ErrRequiredField, svcErr.Message, suggestion)
	case objectsvc.ErrorInvalidInput:
		return handleErrorMsg(ErrInvalidInput, svcErr.Message, suggestion)
	case objectsvc.ErrorValidationFailed:
		if svcCause != nil {
			return handleError(ErrValidationFailed, svcCause, suggestion)
		}
		return handleErrorMsg(ErrValidationFailed, svcErr.Message, suggestion)
	case objectsvc.ErrorFileRead:
		if svcCause != nil {
			return handleError(ErrFileReadError, svcCause, suggestion)
		}
		return handleErrorMsg(ErrFileReadError, svcErr.Message, suggestion)
	case objectsvc.ErrorFileWrite:
		if svcCause != nil {
			return handleError(ErrFileWriteError, svcCause, suggestion)
		}
		return handleErrorMsg(ErrFileWriteError, svcErr.Message, suggestion)
	default:
		if svcCause != nil {
			return handleError(ErrInternal, svcCause, suggestion)
		}
		return handleErrorMsg(ErrInternal, svcErr.Message, suggestion)
	}
}

func parseReclassifyFieldFlags(flags []string) map[string]string {
	values := make(map[string]string)
	for _, f := range flags {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}
		values[key] = value
	}
	return values
}

func promptMissingReclassifyFields(newTypeName string, details map[string]interface{}) (map[string]string, error) {
	rawMissing, ok := details["missing_fields"]
	if !ok {
		return nil, handleErrorMsg(ErrRequiredField, fmt.Sprintf("Missing required fields for type '%s'", newTypeName), "Provide required fields with --field")
	}

	entries, ok := rawMissing.([]map[string]interface{})
	if !ok {
		if genericEntries, ok2 := rawMissing.([]interface{}); ok2 {
			entries = make([]map[string]interface{}, 0, len(genericEntries))
			for _, entry := range genericEntries {
				if m, ok3 := entry.(map[string]interface{}); ok3 {
					entries = append(entries, m)
				}
			}
		}
	}

	fieldNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		name, _ := entry["name"].(string)
		name = strings.TrimSpace(name)
		if name != "" {
			fieldNames = append(fieldNames, name)
		}
	}
	if len(fieldNames) == 0 {
		return nil, handleErrorMsg(ErrRequiredField, fmt.Sprintf("Missing required fields for type '%s'", newTypeName), "Provide required fields with --field")
	}

	sort.Strings(fieldNames)
	reader := bufio.NewReader(os.Stdin)
	values := make(map[string]string, len(fieldNames))

	for _, fieldName := range fieldNames {
		fmt.Fprintf(os.Stderr, "%s (required for type '%s'): ", fieldName, newTypeName)
		value, err := reader.ReadString('\n')
		if err != nil {
			return nil, handleError(ErrInternal, err, "")
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, handleErrorMsg(ErrRequiredField, fmt.Sprintf("required field '%s' cannot be empty", fieldName), "Provide a non-empty value")
		}
		values[fieldName] = value
	}

	return values, nil
}

func init() {
	reclassifyCmd.Flags().StringArrayVar(&reclassifyFieldFlags, "field", nil, "Set field value (can be repeated): --field name=value")
	reclassifyCmd.Flags().BoolVar(&reclassifyNoMove, "no-move", false, "Skip moving file to new type's default_path")
	reclassifyCmd.Flags().BoolVar(&reclassifyUpdateRefs, "update-refs", true, "Update references when file moves")
	reclassifyCmd.Flags().BoolVar(&reclassifyForce, "force", false, "Skip confirmation prompts")
	rootCmd.AddCommand(reclassifyCmd)
}
