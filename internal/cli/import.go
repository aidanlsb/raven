package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/importsvc"
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

type importMappingConfig = importsvc.MappingConfig
type importTypeMapping = importsvc.TypeMapping
type importResult = importsvc.ResultItem
type importItemConfig = importsvc.ItemConfig

func runImport(_ *cobra.Command, args []string) error {
	vaultPath := getVaultPath()

	mappingCfg, err := buildMappingConfig(args)
	if err != nil {
		return handleError(ErrInvalidInput, err, "")
	}

	items, err := readJSONInput()
	if err != nil {
		return handleError(ErrInvalidInput, err, "Expected a JSON array of objects or a single JSON object")
	}
	if len(items) == 0 {
		return handleErrorMsg(ErrInvalidInput, "no items to import", "Provide a non-empty JSON array")
	}

	serviceResult, err := importsvc.Run(importsvc.RunRequest{
		VaultPath:     vaultPath,
		MappingConfig: mappingCfg,
		Items:         items,
		DryRun:        importDryRun,
		CreateOnly:    importCreateOnly,
		UpdateOnly:    importUpdateOnly,
	})
	if err != nil {
		return handleImportServiceError(err)
	}

	if !importDryRun {
		reindexed := make(map[string]struct{}, len(serviceResult.ChangedFilePaths))
		for _, changedFile := range serviceResult.ChangedFilePaths {
			if changedFile == "" {
				continue
			}
			if _, seen := reindexed[changedFile]; seen {
				continue
			}
			reindexed[changedFile] = struct{}{}
			maybeReindex(vaultPath, changedFile, serviceResult.VaultConfig)
		}
	}

	warnings := warningMessagesToWarnings(serviceResult.WarningMessages)
	return outputImportResults(serviceResult.Results, warnings)
}

func handleImportServiceError(err error) error {
	svcErr, ok := importsvc.AsError(err)
	if !ok {
		return handleError(ErrInternal, err, "")
	}

	switch svcErr.Code {
	case importsvc.CodeInvalidInput:
		return handleError(ErrInvalidInput, err, "")
	case importsvc.CodeTypeNotFound:
		return handleError(ErrTypeNotFound, err, "Check schema.yaml for available types")
	case importsvc.CodeSchemaInvalid:
		return handleError(ErrSchemaInvalid, err, "Fix schema.yaml and try again")
	case importsvc.CodeConfigInvalid:
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	default:
		return handleError(ErrInternal, err, "")
	}
}

func buildMappingConfig(args []string) (*importMappingConfig, error) {
	cliType := ""
	if len(args) > 0 {
		cliType = args[0]
	}
	return importsvc.BuildMappingConfig(importsvc.BuildMappingConfigRequest{
		MappingFilePath: importMapping,
		CLIType:         cliType,
		MapFlags:        importMapFlags,
		Key:             importKey,
		ContentField:    importContentField,
	})
}

func validateMappingTypes(cfg *importMappingConfig, sch *schema.Schema) error {
	return importsvc.ValidateMappingTypes(cfg, sch)
}

func readJSONInput() ([]map[string]interface{}, error) {
	return importsvc.ReadJSONInput(importFile, os.Stdin)
}

func resolveItemMapping(item map[string]interface{}, cfg *importMappingConfig, sch *schema.Schema) (*importItemConfig, error) {
	return importsvc.ResolveItemMapping(item, cfg, sch)
}

func applyFieldMappings(item map[string]interface{}, fieldMap map[string]string) map[string]interface{} {
	return importsvc.ApplyFieldMappings(item, fieldMap)
}

func matchKeyValue(mapped map[string]interface{}, matchKey string) (string, bool) {
	return importsvc.MatchKeyValue(mapped, matchKey)
}

func fieldsToStringMap(fields map[string]interface{}, typeName string) map[string]string {
	return importsvc.FieldsToStringMap(fields, typeName)
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
			if r.Code != "" {
				fmt.Printf("  %s %s [%s]: %s\n", ui.Warning("error"), r.ID, r.Code, r.Reason)
			} else {
				fmt.Printf("  %s %s: %s\n", ui.Warning("error"), r.ID, r.Reason)
			}
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

func extractContentField(mapped map[string]interface{}, contentField string) string {
	return importsvc.ExtractContentField(mapped, contentField)
}

func replaceBodyContent(fileContent, newBody string) string {
	return importsvc.ReplaceBodyContent(fileContent, newBody)
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
