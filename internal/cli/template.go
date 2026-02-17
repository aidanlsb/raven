package cli

import (
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
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/slugs"
	"github.com/aidanlsb/raven/internal/template"
	"github.com/aidanlsb/raven/internal/vault"
)

var (
	templateFileFlag      string
	templateContentFlag   string
	templateTitleFlag     string
	templateFieldsFlag    []string
	templateDeleteFile    bool
	templateForce         bool
	templateScaffoldFile  string
	templateScaffoldForce bool
	templateDateFlag      string
)

type templateTarget struct {
	Kind     string // "type" | "daily"
	TypeName string
}

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage template lifecycle",
	Long: `Manage template lifecycle for type and daily templates.

Templates are file-backed only and must live under directories.template (default: templates/).`,
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured templates",
	Long:  "List all configured type templates and daily template binding.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		templateDir := vaultCfg.GetTemplateDirectory()

		sch, err := schema.Load(vaultPath)
		if err != nil {
			return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
		}

		type templateItem struct {
			Target string `json:"target"`
			File   string `json:"file,omitempty"`
			Error  string `json:"error,omitempty"`
		}

		items := make([]templateItem, 0)
		if vaultCfg.DailyTemplate != "" {
			ref, refErr := template.ResolveFileRef(vaultCfg.DailyTemplate, templateDir)
			item := templateItem{Target: "daily"}
			if refErr != nil {
				item.Error = refErr.Error()
			} else {
				item.File = ref
			}
			items = append(items, item)
		}

		var typeNames []string
		for typeName := range sch.Types {
			if schema.IsBuiltinType(typeName) {
				continue
			}
			typeNames = append(typeNames, typeName)
		}
		sort.Strings(typeNames)

		for _, typeName := range typeNames {
			typeDef := sch.Types[typeName]
			if typeDef == nil || strings.TrimSpace(typeDef.Template) == "" {
				continue
			}
			ref, refErr := template.ResolveFileRef(typeDef.Template, templateDir)
			item := templateItem{Target: "type:" + typeName}
			if refErr != nil {
				item.Error = refErr.Error()
			} else {
				item.File = ref
			}
			items = append(items, item)
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"template_directory": templateDir,
				"items":              items,
			}, &Meta{Count: len(items), QueryTimeMs: elapsed})
			return nil
		}

		fmt.Printf("Template directory: %s\n", templateDir)
		if len(items) == 0 {
			fmt.Println("No templates configured.")
			return nil
		}
		for _, item := range items {
			if item.Error != "" {
				fmt.Printf("- %s (invalid: %s)\n", item.Target, item.Error)
				continue
			}
			fmt.Printf("- %s -> %s\n", item.Target, item.File)
		}
		return nil
	},
}

var templateGetCmd = &cobra.Command{
	Use:   "get <type|daily> [type_name]",
	Short: "Show template binding and content",
	Long:  "Show template file binding and loaded content for a type or daily template.",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		target, err := parseTemplateTarget(args)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Use: rvn template get type <type_name> OR rvn template get daily")
		}

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		templateDir := vaultCfg.GetTemplateDirectory()

		spec, err := getTemplateSpec(vaultPath, target, vaultCfg)
		if err != nil {
			return err
		}

		elapsed := time.Since(start).Milliseconds()
		result := map[string]interface{}{
			"target":             targetLabel(target),
			"template_directory": templateDir,
		}

		if spec == "" {
			result["configured"] = false
			if isJSONOutput() {
				outputSuccess(result, &Meta{QueryTimeMs: elapsed})
				return nil
			}
			fmt.Printf("%s has no template configured.\n", targetLabel(target))
			return nil
		}

		ref, err := template.ResolveFileRef(spec, templateDir)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Template must be a file path under directories.template")
		}
		content, err := template.Load(vaultPath, ref, templateDir)
		if err != nil {
			return handleError(ErrFileReadError, err, "")
		}

		result["configured"] = true
		result["file"] = ref
		result["content"] = content

		if isJSONOutput() {
			outputSuccess(result, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		fmt.Printf("Target: %s\n", targetLabel(target))
		fmt.Printf("File: %s\n\n", ref)
		fmt.Println(content)
		return nil
	},
}

var templateSetCmd = &cobra.Command{
	Use:   "set <type|daily> [type_name]",
	Short: "Set template file binding",
	Long:  "Set the template file binding for a type or daily template.",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		target, err := parseTemplateTarget(args)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Use: rvn template set type <type_name> --file <path> OR rvn template set daily --file <path>")
		}
		if strings.TrimSpace(templateFileFlag) == "" {
			return handleErrorMsg(ErrMissingArgument, "--file is required", "Use --file <template-file-path>")
		}

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		templateDir := vaultCfg.GetTemplateDirectory()

		fileRef, err := template.ResolveFileRef(templateFileFlag, templateDir)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), fmt.Sprintf("Use a file path under %s", templateDir))
		}
		fullPath := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
		if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
			return handleError(ErrFileOutsideVault, err, "Template files must be within the vault")
		}
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			return handleErrorMsg(ErrFileNotFound, fmt.Sprintf("template file not found: %s", fileRef), "Create it first with `rvn template scaffold ...`")
		}

		if err := setTemplateSpec(vaultPath, target, fileRef, vaultCfg); err != nil {
			return err
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"target": targetLabel(target),
				"file":   fileRef,
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		fmt.Printf("Set template for %s -> %s\n", targetLabel(target), fileRef)
		return nil
	},
}

var templateScaffoldCmd = &cobra.Command{
	Use:   "scaffold <type|daily> [type_name]",
	Short: "Create template file and register binding",
	Long:  "Create a template file (if needed) and register it for a type or daily template.",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		target, err := parseTemplateTarget(args)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Use: rvn template scaffold type <type_name> OR rvn template scaffold daily")
		}

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		templateDir := vaultCfg.GetTemplateDirectory()

		fileRef := strings.TrimSpace(templateScaffoldFile)
		if fileRef == "" {
			switch target.Kind {
			case "daily":
				fileRef = templateDir + "daily.md"
			case "type":
				fileRef = templateDir + target.TypeName + ".md"
			}
		}
		fileRef, err = template.ResolveFileRef(fileRef, templateDir)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), fmt.Sprintf("Use a file path under %s", templateDir))
		}

		fullPath := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
		if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
			return handleError(ErrFileOutsideVault, err, "Template files must be within the vault")
		}

		if _, err := os.Stat(fullPath); err == nil && !templateScaffoldForce {
			return handleErrorMsg(ErrFileExists, fmt.Sprintf("template file already exists: %s", fileRef), "Use --force to overwrite")
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		content := defaultTemplateScaffold(target)
		if err := atomicfile.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		if err := setTemplateSpec(vaultPath, target, fileRef, vaultCfg); err != nil {
			return err
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"target":     targetLabel(target),
				"file":       fileRef,
				"scaffolded": true,
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		fmt.Printf("Scaffolded template for %s at %s\n", targetLabel(target), fileRef)
		return nil
	},
}

var templateWriteCmd = &cobra.Command{
	Use:   "write <type|daily> [type_name]",
	Short: "Replace bound template file content",
	Long:  "Replace content in the currently bound template file for a type or daily template.",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		target, err := parseTemplateTarget(args)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Use: rvn template write type <type_name> --content ... OR rvn template write daily --content ...")
		}
		if templateContentFlag == "" {
			return handleErrorMsg(ErrMissingArgument, "--content is required", "Provide template content to write")
		}

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		templateDir := vaultCfg.GetTemplateDirectory()

		spec, err := getTemplateSpec(vaultPath, target, vaultCfg)
		if err != nil {
			return err
		}
		if spec == "" {
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("%s has no template configured", targetLabel(target)), "Set one first with `rvn template set ... --file ...` or scaffold one with `rvn template scaffold ...`")
		}

		fileRef, err := template.ResolveFileRef(spec, templateDir)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Template must be a file path under directories.template")
		}
		fullPath := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
		if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
			return handleError(ErrFileOutsideVault, err, "Template files must be within the vault")
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}
		if err := atomicfile.WriteFile(fullPath, []byte(templateContentFlag), 0o644); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"target":  targetLabel(target),
				"file":    fileRef,
				"written": true,
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		fmt.Printf("Updated template file for %s: %s\n", targetLabel(target), fileRef)
		return nil
	},
}

var templateRemoveCmd = &cobra.Command{
	Use:   "remove <type|daily> [type_name]",
	Short: "Remove template binding (optionally delete file)",
	Long:  "Remove a template binding from a type or daily template. Optionally delete the template file.",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		target, err := parseTemplateTarget(args)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Use: rvn template remove type <type_name> OR rvn template remove daily")
		}

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		templateDir := vaultCfg.GetTemplateDirectory()

		spec, err := getTemplateSpec(vaultPath, target, vaultCfg)
		if err != nil {
			return err
		}
		if spec == "" {
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("%s has no template configured", targetLabel(target)), "Nothing to remove")
		}

		fileRef, err := template.ResolveFileRef(spec, templateDir)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Template must be a file path under directories.template")
		}

		if templateDeleteFile {
			refs, refErr := collectTemplateReferences(vaultPath, vaultCfg)
			if refErr != nil {
				return refErr
			}
			var remaining []string
			for _, refTarget := range refs[fileRef] {
				if refTarget != targetLabel(target) {
					remaining = append(remaining, refTarget)
				}
			}
			if len(remaining) > 0 && !templateForce {
				sort.Strings(remaining)
				return handleErrorMsg(
					ErrInvalidInput,
					fmt.Sprintf("cannot delete template file %s; still referenced by: %s", fileRef, strings.Join(remaining, ", ")),
					"Unlink those references first, or rerun with --force",
				)
			}
		}

		if err := removeTemplateSpec(vaultPath, target, vaultCfg); err != nil {
			return err
		}

		deleted := false
		if templateDeleteFile {
			fullPath := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
			if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
				return handleError(ErrFileOutsideVault, err, "Template files must be within the vault")
			}
			if err := os.Remove(fullPath); err == nil {
				deleted = true
			} else if !os.IsNotExist(err) {
				return handleError(ErrFileWriteError, err, "")
			}
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"target":       targetLabel(target),
				"removed":      true,
				"file":         fileRef,
				"deleted_file": deleted,
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		fmt.Printf("Removed template binding for %s\n", targetLabel(target))
		if templateDeleteFile {
			if deleted {
				fmt.Printf("Deleted template file: %s\n", fileRef)
			} else {
				fmt.Printf("Template file did not exist: %s\n", fileRef)
			}
		}
		return nil
	},
}

var templateRenderCmd = &cobra.Command{
	Use:   "render <type|daily> [type_name]",
	Short: "Preview rendered template content",
	Long:  "Render a template with variable substitution for type or daily targets.",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()
		target, err := parseTemplateTarget(args)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Use: rvn template render type <type_name> OR rvn template render daily")
		}

		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		templateDir := vaultCfg.GetTemplateDirectory()

		spec, err := getTemplateSpec(vaultPath, target, vaultCfg)
		if err != nil {
			return err
		}
		if spec == "" {
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("%s has no template configured", targetLabel(target)), "Set one first with `rvn template set ...`")
		}

		fileRef, err := template.ResolveFileRef(spec, templateDir)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Template must be a file path under directories.template")
		}
		templateContent, err := template.Load(vaultPath, fileRef, templateDir)
		if err != nil {
			return handleError(ErrFileReadError, err, "Failed to load template")
		}

		var rendered string
		switch target.Kind {
		case "type":
			title := templateTitleFlag
			if title == "" {
				title = "Sample " + target.TypeName
			}
			fields, err := parseFieldFlags(templateFieldsFlag)
			if err != nil {
				return handleErrorMsg(ErrInvalidInput, err.Error(), "Use --field name=value")
			}
			slug := slugs.ComponentSlug(title)
			vars := template.NewVariables(title, target.TypeName, slug, fields)
			rendered = template.Apply(templateContent, vars)
		case "daily":
			targetDate, err := vault.ParseDateArg(templateDateFlag)
			if err != nil {
				return handleErrorMsg(ErrInvalidInput, err.Error(), "Use YYYY-MM-DD or today/yesterday/tomorrow")
			}
			vars := template.NewDailyVariables(targetDate)
			rendered = template.Apply(templateContent, vars)
		default:
			return handleErrorMsg(ErrInvalidInput, "unknown target kind", "")
		}

		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"target":   targetLabel(target),
				"file":     fileRef,
				"rendered": rendered,
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		fmt.Printf("Rendered template for %s:\n\n", targetLabel(target))
		fmt.Println(rendered)
		return nil
	},
}

func parseTemplateTarget(args []string) (templateTarget, error) {
	if len(args) == 0 {
		return templateTarget{}, fmt.Errorf("missing target")
	}
	kind := strings.ToLower(strings.TrimSpace(args[0]))
	switch kind {
	case "daily":
		if len(args) != 1 {
			return templateTarget{}, fmt.Errorf("daily target does not take a type_name")
		}
		return templateTarget{Kind: "daily"}, nil
	case "type":
		if len(args) != 2 {
			return templateTarget{}, fmt.Errorf("type target requires type_name")
		}
		typeName := strings.TrimSpace(args[1])
		if typeName == "" {
			return templateTarget{}, fmt.Errorf("type_name cannot be empty")
		}
		return templateTarget{Kind: "type", TypeName: typeName}, nil
	default:
		return templateTarget{}, fmt.Errorf("unknown target %q (expected 'type' or 'daily')", args[0])
	}
}

func targetLabel(target templateTarget) string {
	if target.Kind == "daily" {
		return "daily"
	}
	return "type:" + target.TypeName
}

func getTemplateSpec(vaultPath string, target templateTarget, vaultCfg *config.VaultConfig) (string, error) {
	switch target.Kind {
	case "daily":
		return strings.TrimSpace(vaultCfg.DailyTemplate), nil
	case "type":
		if schema.IsBuiltinType(target.TypeName) {
			return "", handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type and cannot use custom templates", target.TypeName), "")
		}
		sch, err := schema.Load(vaultPath)
		if err != nil {
			return "", handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
		}
		typeDef, exists := sch.Types[target.TypeName]
		if !exists {
			return "", handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", target.TypeName), "Run 'rvn schema types' to see available types")
		}
		if typeDef == nil {
			return "", nil
		}
		return strings.TrimSpace(typeDef.Template), nil
	default:
		return "", handleErrorMsg(ErrInvalidInput, "unknown target kind", "")
	}
}

func readSchemaDoc(vaultPath string) (map[string]interface{}, map[string]interface{}, error) {
	schemaPath := paths.SchemaPath(vaultPath)
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, nil, handleError(ErrFileReadError, err, "")
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return nil, nil, handleError(ErrSchemaInvalid, err, "")
	}
	types, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		return nil, nil, handleErrorMsg(ErrSchemaInvalid, "types section not found", "")
	}
	return schemaDoc, types, nil
}

func writeSchemaDoc(vaultPath string, schemaDoc map[string]interface{}) error {
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return handleError(ErrInternal, err, "")
	}
	schemaPath := paths.SchemaPath(vaultPath)
	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
		return handleError(ErrFileWriteError, err, "")
	}
	return nil
}

func setTemplateSpec(vaultPath string, target templateTarget, fileRef string, vaultCfg *config.VaultConfig) error {
	switch target.Kind {
	case "daily":
		vaultCfg.DailyTemplate = fileRef
		if err := config.SaveVaultConfig(vaultPath, vaultCfg); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}
		return nil
	case "type":
		if schema.IsBuiltinType(target.TypeName) {
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type and cannot use custom templates", target.TypeName), "")
		}
		sch, err := schema.Load(vaultPath)
		if err != nil {
			return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
		}
		if _, exists := sch.Types[target.TypeName]; !exists {
			return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", target.TypeName), "Use 'rvn schema add type' to create it")
		}

		schemaDoc, types, err := readSchemaDoc(vaultPath)
		if err != nil {
			return err
		}
		typeNode, ok := types[target.TypeName].(map[string]interface{})
		if !ok {
			typeNode = make(map[string]interface{})
			types[target.TypeName] = typeNode
		}
		typeNode["template"] = fileRef
		return writeSchemaDoc(vaultPath, schemaDoc)
	default:
		return handleErrorMsg(ErrInvalidInput, "unknown target kind", "")
	}
}

func removeTemplateSpec(vaultPath string, target templateTarget, vaultCfg *config.VaultConfig) error {
	switch target.Kind {
	case "daily":
		vaultCfg.DailyTemplate = ""
		if err := config.SaveVaultConfig(vaultPath, vaultCfg); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}
		return nil
	case "type":
		if schema.IsBuiltinType(target.TypeName) {
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type and cannot use custom templates", target.TypeName), "")
		}
		sch, err := schema.Load(vaultPath)
		if err != nil {
			return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
		}
		typeDef, exists := sch.Types[target.TypeName]
		if !exists {
			return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", target.TypeName), "")
		}
		if typeDef == nil || strings.TrimSpace(typeDef.Template) == "" {
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("type '%s' has no template configured", target.TypeName), "Nothing to remove")
		}

		schemaDoc, types, err := readSchemaDoc(vaultPath)
		if err != nil {
			return err
		}
		typeNode, ok := types[target.TypeName].(map[string]interface{})
		if !ok {
			return handleErrorMsg(ErrSchemaInvalid, fmt.Sprintf("type '%s' definition is invalid", target.TypeName), "")
		}
		delete(typeNode, "template")
		return writeSchemaDoc(vaultPath, schemaDoc)
	default:
		return handleErrorMsg(ErrInvalidInput, "unknown target kind", "")
	}
}

func defaultTemplateScaffold(target templateTarget) string {
	switch target.Kind {
	case "daily":
		return "# {{weekday}}, {{date}}\n\n## Notes\n"
	default:
		return "# {{title}}\n\n## Notes\n"
	}
}

func collectTemplateReferences(vaultPath string, vaultCfg *config.VaultConfig) (map[string][]string, error) {
	templateDir := vaultCfg.GetTemplateDirectory()
	refs := make(map[string][]string)

	if strings.TrimSpace(vaultCfg.DailyTemplate) != "" {
		fileRef, err := template.ResolveFileRef(vaultCfg.DailyTemplate, templateDir)
		if err != nil {
			return nil, handleErrorMsg(ErrInvalidInput, err.Error(), "Fix daily_template in raven.yaml")
		}
		refs[fileRef] = append(refs[fileRef], "daily")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return nil, handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}
	for typeName, typeDef := range sch.Types {
		if schema.IsBuiltinType(typeName) || typeDef == nil || strings.TrimSpace(typeDef.Template) == "" {
			continue
		}
		fileRef, err := template.ResolveFileRef(typeDef.Template, templateDir)
		if err != nil {
			return nil, handleErrorMsg(ErrInvalidInput, fmt.Sprintf("invalid template for type '%s': %v", typeName, err), "Fix schema template bindings")
		}
		refs[fileRef] = append(refs[fileRef], "type:"+typeName)
	}

	return refs, nil
}

func init() {
	templateSetCmd.Flags().StringVar(&templateFileFlag, "file", "", "Template file path under directories.template")

	templateScaffoldCmd.Flags().StringVar(&templateScaffoldFile, "file", "", "Template file path under directories.template")
	templateScaffoldCmd.Flags().BoolVar(&templateScaffoldForce, "force", false, "Overwrite scaffold file if it already exists")

	templateWriteCmd.Flags().StringVar(&templateContentFlag, "content", "", "Template file content to write")

	templateRemoveCmd.Flags().BoolVar(&templateDeleteFile, "delete-file", false, "Also delete the underlying template file")
	templateRemoveCmd.Flags().BoolVar(&templateForce, "force", false, "Skip safety checks for --delete-file")

	templateRenderCmd.Flags().StringVar(&templateTitleFlag, "title", "", "Title to use when rendering type templates")
	templateRenderCmd.Flags().StringArrayVar(&templateFieldsFlag, "field", nil, "Set field value for rendering (repeatable): --field key=value")
	templateRenderCmd.Flags().StringVar(&templateDateFlag, "date", "", "Date for daily render: today/yesterday/tomorrow/YYYY-MM-DD")

	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateGetCmd)
	templateCmd.AddCommand(templateSetCmd)
	templateCmd.AddCommand(templateScaffoldCmd)
	templateCmd.AddCommand(templateWriteCmd)
	templateCmd.AddCommand(templateRemoveCmd)
	templateCmd.AddCommand(templateRenderCmd)
	rootCmd.AddCommand(templateCmd)
}
