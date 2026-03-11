package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	upsertFieldFlags []string
	upsertFieldJSON  string
	upsertContent    string
	upsertPathFlag   string
)

var upsertCmd = &cobra.Command{
	Use:   "upsert <type> <title>",
	Short: commands.Registry["upsert"].Description,
	Long:  commands.Registry["upsert"].LongDesc,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		typeName := args[0]
		title := args[1]
		replaceBody := cmd.Flags().Changed("content")

		if err := validateObjectTitle(title); err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Provide a plain title without path separators")
		}

		targetPath := title
		if cmd.Flags().Changed("path") {
			targetPath = strings.TrimSpace(upsertPathFlag)
			if err := validateObjectTargetPath(targetPath); err != nil {
				return handleErrorMsg(ErrInvalidInput, err.Error(), "Use --path with an explicit object path like note/raven-friction")
			}
		}

		s, err := schema.Load(vaultPath)
		if err != nil {
			return handleError(ErrSchemaInvalid, err, "Fix schema.yaml and try again")
		}

		fieldValues, err := parseFieldFlags(upsertFieldFlags)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Use format: --field name=value")
		}

		typedFieldValues, err := parseFieldValuesJSON(upsertFieldJSON)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, "invalid --field-json payload", "Provide a JSON object, e.g. --field-json '{\"status\":\"active\"}'")
		}

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}

		result, err := objectsvc.Upsert(objectsvc.UpsertRequest{
			VaultPath:        vaultPath,
			TypeName:         typeName,
			Title:            title,
			TargetPath:       targetPath,
			ReplaceBody:      replaceBody,
			Content:          upsertContent,
			FieldValues:      fieldValues,
			TypedFieldValues: typedFieldValues,
			Schema:           s,
			ObjectsRoot:      vaultCfg.GetObjectsRoot(),
			PagesRoot:        vaultCfg.GetPagesRoot(),
			TemplateDir:      vaultCfg.GetTemplateDirectory(),
		})
		if err != nil {
			var svcErr *objectsvc.Error
			if errors.As(err, &svcErr) {
				switch svcErr.Code {
				case objectsvc.ErrorTypeNotFound:
					return handleErrorMsg(ErrTypeNotFound, svcErr.Message, svcErr.Suggestion)
				case objectsvc.ErrorRequiredField:
					if isJSONOutput() {
						outputError(ErrRequiredField, svcErr.Message, svcErr.Details, svcErr.Suggestion)
						return nil
					}
					return handleErrorMsg(ErrRequiredField, svcErr.Message, svcErr.Suggestion)
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
			return handleError(ErrFileWriteError, err, "")
		}

		if result.Status == "created" || result.Status == "updated" {
			maybeReindex(vaultPath, result.FilePath, vaultCfg)
		}

		cliWarnings := warningMessagesToWarnings(result.WarningMessages)
		objectID := vaultCfg.FilePathToObjectID(result.RelativePath)
		if isJSONOutput() {
			data := map[string]interface{}{
				"status": result.Status,
				"id":     objectID,
				"file":   result.RelativePath,
				"type":   typeName,
				"title":  title,
			}
			if len(cliWarnings) > 0 {
				outputSuccessWithWarnings(data, cliWarnings, nil)
			} else {
				outputSuccess(data, nil)
			}
			return nil
		}

		switch result.Status {
		case "created":
			fmt.Println(ui.Checkf("Created %s", ui.FilePath(result.RelativePath)))
		case "updated":
			fmt.Println(ui.Checkf("Updated %s", ui.FilePath(result.RelativePath)))
		default:
			fmt.Println(ui.Checkf("Unchanged %s", ui.FilePath(result.RelativePath)))
		}
		for _, warning := range cliWarnings {
			fmt.Println(ui.Warning(warning.Message))
		}
		return nil
	},
}

func parseFieldFlags(flags []string) (map[string]string, error) {
	fields := make(map[string]string, len(flags))
	for _, f := range flags {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid field format: %s", f)
		}
		fields[parts[0]] = parts[1]
	}
	return fields, nil
}

func init() {
	upsertCmd.Flags().StringArrayVar(&upsertFieldFlags, "field", nil, "Set field value (can be repeated): --field name=value")
	upsertCmd.Flags().StringVar(&upsertFieldJSON, "field-json", "", "Set/update frontmatter fields as a JSON object")
	upsertCmd.Flags().StringVar(&upsertContent, "content", "", "Replace body content (idempotent full-body mode)")
	upsertCmd.Flags().StringVar(&upsertPathFlag, "path", "", "Explicit target path (overrides title-derived path)")
	rootCmd.AddCommand(upsertCmd)
}
