package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
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

var importCmd = newCanonicalLeafCommand("import", canonicalLeafOptions{
	VaultPath:       getVaultPath,
	BuildArgs:       buildImportArgs,
	Invoke:          invokeImport,
	RenderHuman:     renderImportResult,
	SkipFlagBinding: true,
})

type importMappingConfig = importsvc.MappingConfig
type importTypeMapping = importsvc.TypeMapping
type importResult = importsvc.ResultItem
type importItemConfig = importsvc.ItemConfig

func buildImportArgs(_ *cobra.Command, args []string) (map[string]interface{}, error) {
	argsMap := map[string]interface{}{
		"file":          importFile,
		"mapping":       importMapping,
		"map":           append([]string{}, importMapFlags...),
		"key":           importKey,
		"content-field": importContentField,
		"dry-run":       importDryRun,
		"create-only":   importCreateOnly,
		"update-only":   importUpdateOnly,
		"confirm":       importConfirm,
	}
	if len(args) > 0 {
		argsMap["type"] = args[0]
	}
	return argsMap, nil
}

func invokeImport(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	var stdinData []byte
	if strings.TrimSpace(importFile) == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return commandexec.Failure(ErrInvalidInput, err.Error(), nil, "Expected a JSON array of objects or a single JSON object")
		}
		stdinData = data
	}

	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
		Confirm:   importConfirm,
		Stdin:     stdinData,
	})
}

func renderImportResult(_ *cobra.Command, result commandexec.Result) error {
	return renderCanonicalImportResult(result)
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

func renderCanonicalImportResult(result commandexec.Result) error {
	data := canonicalDataMap(result)
	results := make([]importResult, 0)
	switch rawResults := data["results"].(type) {
	case []importResult:
		results = append(results, rawResults...)
	case []interface{}:
		for _, raw := range rawResults {
			itemMap, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			item := importResult{
				ID:     stringValue(itemMap["id"]),
				Action: stringValue(itemMap["action"]),
				File:   stringValue(itemMap["file"]),
				Reason: stringValue(itemMap["reason"]),
				Code:   stringValue(itemMap["code"]),
			}
			if details, ok := itemMap["details"].(map[string]interface{}); ok {
				item.Details = details
			}
			results = append(results, item)
		}
	}
	return outputImportResults(results, result.Warnings)
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
