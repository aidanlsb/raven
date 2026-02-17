package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/parser"
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
			return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
		}

		typeDef, typeExists := s.Types[typeName]
		if !typeExists && !schema.IsBuiltinType(typeName) {
			var typeNames []string
			for name := range s.Types {
				typeNames = append(typeNames, name)
			}
			sort.Strings(typeNames)
			return handleErrorMsg(
				ErrTypeNotFound,
				fmt.Sprintf("type '%s' not found", typeName),
				fmt.Sprintf("Available types: %s", strings.Join(typeNames, ", ")),
			)
		}

		fieldValues, err := parseFieldFlags(upsertFieldFlags)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Use format: --field name=value")
		}
		typedFieldValues, err := parseFieldValuesJSON(upsertFieldJSON)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, "invalid --field-json payload", "Provide a JSON object, e.g. --field-json '{\"status\":\"active\"}'")
		}

		// Keep parity with `new`: auto-fill name_field from title if not explicitly provided.
		if typeDef != nil && typeDef.NameField != "" {
			if _, provided := fieldValues[typeDef.NameField]; !provided {
				if _, typedProvided := typedFieldValues[typeDef.NameField]; !typedProvided && title != "" {
					fieldValues[typeDef.NameField] = title
				}
			}
		}

		// Merge typed JSON fields into the same canonical field pipeline.
		// Typed JSON values win over --field key=value collisions.
		for key, value := range typedFieldValues {
			fieldValues[key] = serializeFieldValueLiteral(value)
		}

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		objectsRoot := vaultCfg.GetObjectsRoot()
		pagesRoot := vaultCfg.GetPagesRoot()
		templateDir := vaultCfg.GetTemplateDirectory()
		creator := newObjectCreationContext(vaultPath, s, objectsRoot, pagesRoot, templateDir)
		slugified := creator.resolveAndSlugifyTargetPath(targetPath, typeName)
		if !strings.HasSuffix(slugified, ".md") {
			slugified += ".md"
		}
		filePath := filepath.Join(vaultPath, slugified)
		relPath := slugified

		status := "unchanged"
		var cliWarnings []Warning
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			missingRequired := requiredFieldGaps(typeDef, fieldValues)
			if len(missingRequired) > 0 {
				msg := fmt.Sprintf("Missing required fields: %s", strings.Join(missingRequired, ", "))
				if isJSONOutput() {
					outputError(ErrRequiredField, msg, map[string]interface{}{
						"type":           typeName,
						"title":          title,
						"missing_fields": missingRequired,
						"retry_with": map[string]interface{}{
							"type":  typeName,
							"title": title,
							"field": buildFieldTemplate(missingRequired),
						},
					}, "")
					return nil
				}
				return handleErrorMsg(ErrRequiredField, msg, "Provide missing fields with --field")
			}

			_, resolvedCreateFields, warningMessages, err := prepareValidatedFieldMutation(
				typeName,
				nil,
				fieldValues,
				s,
				map[string]bool{"type": true},
			)
			if err != nil {
				var validationErr *fieldValidationError
				if errors.As(err, &validationErr) {
					return handleErrorMsg(ErrValidationFailed, validationErr.Error(), validationErr.Suggestion())
				}
				return handleError(ErrValidationFailed, err, "Failed to validate field values")
			}
			fieldValues = resolvedCreateFields
			cliWarnings = append(cliWarnings, warningMessagesToWarnings(warningMessages)...)

			createResult, err := creator.create(objectCreateParams{
				typeName:   typeName,
				title:      title,
				targetPath: targetPath,
				fields:     fieldValues,
			})
			if err != nil {
				return handleError(ErrFileWriteError, err, "")
			}
			filePath = createResult.FilePath
			relPath = createResult.RelativePath

			createdBytes, err := os.ReadFile(filePath)
			if err != nil {
				return handleError(ErrFileReadError, err, "")
			}
			createdContent := string(createdBytes)

			if len(resolvedCreateFields) > 0 {
				createdFM, err := parser.ParseFrontmatter(createdContent)
				if err != nil {
					return handleError(ErrInvalidInput, err, "Failed to parse frontmatter")
				}
				if createdFM == nil {
					return handleErrorMsg(ErrInvalidInput, "file has no frontmatter", "The file must have YAML frontmatter (---) for upsert")
				}

				updatedContent, _, warningMessages, err := prepareValidatedFrontmatterMutation(
					createdContent,
					createdFM,
					typeName,
					resolvedCreateFields,
					s,
					map[string]bool{"type": true, "alias": true},
				)
				if err != nil {
					var validationErr *fieldValidationError
					if errors.As(err, &validationErr) {
						return handleErrorMsg(ErrValidationFailed, validationErr.Error(), validationErr.Suggestion())
					}
					return handleError(ErrFileWriteError, err, "Failed to update frontmatter")
				}
				cliWarnings = append(cliWarnings, warningMessagesToWarnings(warningMessages)...)
				createdContent = updatedContent
			}

			if replaceBody {
				createdContent = replaceBodyContent(createdContent, upsertContent)
			}

			if createdContent != string(createdBytes) {
				if err := atomicfile.WriteFile(filePath, []byte(createdContent), 0o644); err != nil {
					return handleError(ErrFileWriteError, err, "")
				}
			}

			maybeReindex(vaultPath, filePath, vaultCfg)
			status = "created"
		} else if err != nil {
			return handleError(ErrFileReadError, err, "")
		} else {
			originalBytes, err := os.ReadFile(filePath)
			if err != nil {
				return handleError(ErrFileReadError, err, "")
			}
			original := string(originalBytes)

			fm, err := parser.ParseFrontmatter(original)
			if err != nil {
				return handleError(ErrInvalidInput, err, "Failed to parse frontmatter")
			}
			if fm == nil {
				return handleErrorMsg(ErrInvalidInput, "file has no frontmatter", "The file must have YAML frontmatter (---) for upsert")
			}
			if fm.ObjectType != "" && fm.ObjectType != typeName {
				return handleErrorMsg(
					ErrValidationFailed,
					fmt.Sprintf("existing object type is '%s', cannot upsert as '%s'", fm.ObjectType, typeName),
					"Choose a different title/path, or update the existing type first",
				)
			}

			updates := make(map[string]string, len(fieldValues)+1)
			if fm.ObjectType == "" {
				updates["type"] = typeName
			}
			for k, v := range fieldValues {
				if fm.Fields != nil {
					if existing, ok := fm.Fields[k]; ok && fieldValueMatchesInput(existing, v) {
						continue
					}
				}
				updates[k] = v
			}

			nextContent := original
			if len(updates) > 0 {
				var warningMessages []string
				nextContent, _, warningMessages, err = prepareValidatedFrontmatterMutation(
					original,
					fm,
					typeName,
					updates,
					s,
					map[string]bool{"type": true, "alias": true},
				)
				if err != nil {
					var validationErr *fieldValidationError
					if errors.As(err, &validationErr) {
						return handleErrorMsg(ErrValidationFailed, validationErr.Error(), validationErr.Suggestion())
					}
					return handleError(ErrFileWriteError, err, "Failed to update frontmatter")
				}
				cliWarnings = append(cliWarnings, warningMessagesToWarnings(warningMessages)...)
			}

			if replaceBody {
				nextContent = replaceBodyContent(nextContent, upsertContent)
			}

			if nextContent != original {
				if err := atomicfile.WriteFile(filePath, []byte(nextContent), 0o644); err != nil {
					return handleError(ErrFileWriteError, err, "")
				}
				maybeReindex(vaultPath, filePath, vaultCfg)
				status = "updated"
			}
		}

		objectID := vaultCfg.FilePathToObjectID(relPath)
		if isJSONOutput() {
			data := map[string]interface{}{
				"status": status,
				"id":     objectID,
				"file":   relPath,
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

		switch status {
		case "created":
			fmt.Println(ui.Checkf("Created %s", ui.FilePath(relPath)))
		case "updated":
			fmt.Println(ui.Checkf("Updated %s", ui.FilePath(relPath)))
		default:
			fmt.Println(ui.Checkf("Unchanged %s", ui.FilePath(relPath)))
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

func requiredFieldGaps(typeDef *schema.TypeDefinition, fields map[string]string) []string {
	if typeDef == nil {
		return nil
	}

	var missing []string
	for fieldName, fieldDef := range typeDef.Fields {
		if fieldDef == nil || !fieldDef.Required {
			continue
		}
		if _, ok := fields[fieldName]; ok {
			continue
		}
		if fieldDef.Default != nil {
			fields[fieldName] = fmt.Sprintf("%v", fieldDef.Default)
			continue
		}
		missing = append(missing, fieldName)
	}
	sort.Strings(missing)
	return missing
}

func fieldValueMatchesInput(v schema.FieldValue, input string) bool {
	if s, ok := v.AsString(); ok {
		return s == input
	}
	if n, ok := v.AsNumber(); ok {
		return fmt.Sprintf("%v", n) == input
	}
	if b, ok := v.AsBool(); ok {
		return fmt.Sprintf("%v", b) == input
	}
	return fmt.Sprintf("%v", v.Raw()) == input
}

func init() {
	upsertCmd.Flags().StringArrayVar(&upsertFieldFlags, "field", nil, "Set field value (can be repeated): --field name=value")
	upsertCmd.Flags().StringVar(&upsertFieldJSON, "field-json", "", "Set/update frontmatter fields as a JSON object")
	upsertCmd.Flags().StringVar(&upsertContent, "content", "", "Replace body content (idempotent full-body mode)")
	upsertCmd.Flags().StringVar(&upsertPathFlag, "path", "", "Explicit target path (overrides title-derived path)")
	rootCmd.AddCommand(upsertCmd)
}
