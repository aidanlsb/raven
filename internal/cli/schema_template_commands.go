package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/template"
)

var (
	schemaTemplateFileFlag        string
	schemaTemplateDescriptionFlag string
	schemaTypeTemplateClearFlag   bool
)

func runSchemaTemplateCommand(vaultPath string, args []string, start time.Time) error {
	if len(args) == 0 {
		return handleErrorMsg(ErrMissingArgument, "missing schema template subcommand", "Use: rvn schema template list|get|set|remove ...")
	}

	switch args[0] {
	case "list":
		return schemaTemplateList(vaultPath, start)
	case "get":
		if len(args) != 2 {
			return handleErrorMsg(ErrInvalidInput, "template get requires template_id", "Use: rvn schema template get <template_id>")
		}
		return schemaTemplateGet(vaultPath, args[1], start)
	case "set":
		if len(args) != 2 {
			return handleErrorMsg(ErrInvalidInput, "template set requires template_id", "Use: rvn schema template set <template_id> --file <path>")
		}
		return schemaTemplateSet(vaultPath, args[1], start)
	case "remove":
		if len(args) != 2 {
			return handleErrorMsg(ErrInvalidInput, "template remove requires template_id", "Use: rvn schema template remove <template_id>")
		}
		return schemaTemplateRemove(vaultPath, args[1], start)
	default:
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown schema template subcommand: %s", args[0]), "Use: list, get, set, or remove")
	}
}

func schemaTemplateList(vaultPath string, start time.Time) error {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}

	type item struct {
		ID          string `json:"id"`
		File        string `json:"file"`
		Description string `json:"description,omitempty"`
	}

	var items []item
	for id, def := range sch.Templates {
		if def == nil {
			continue
		}
		items = append(items, item{
			ID:          id,
			File:        def.File,
			Description: def.Description,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })

	elapsed := time.Since(start).Milliseconds()
	if isJSONOutput() {
		outputSuccess(map[string]interface{}{"templates": items}, &Meta{Count: len(items), QueryTimeMs: elapsed})
		return nil
	}

	if len(items) == 0 {
		fmt.Println("No schema templates configured.")
		return nil
	}
	fmt.Println("Schema templates:")
	for _, it := range items {
		if it.Description != "" {
			fmt.Printf("  %s -> %s (%s)\n", it.ID, it.File, it.Description)
		} else {
			fmt.Printf("  %s -> %s\n", it.ID, it.File)
		}
	}
	return nil
}

func schemaTemplateGet(vaultPath, templateID string, start time.Time) error {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return handleErrorMsg(ErrInvalidInput, "template_id cannot be empty", "")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}
	templateDef, ok := sch.Templates[templateID]
	if !ok || templateDef == nil {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("template '%s' not found", templateID), "Use `rvn schema template list` to see available template IDs")
	}

	elapsed := time.Since(start).Milliseconds()
	result := map[string]interface{}{
		"id":          templateID,
		"file":        templateDef.File,
		"description": templateDef.Description,
	}
	if isJSONOutput() {
		outputSuccess(result, &Meta{QueryTimeMs: elapsed})
		return nil
	}
	fmt.Printf("Template: %s\n", templateID)
	fmt.Printf("  File: %s\n", templateDef.File)
	if templateDef.Description != "" {
		fmt.Printf("  Description: %s\n", templateDef.Description)
	}
	return nil
}

func schemaTemplateSet(vaultPath, templateID string, start time.Time) error {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return handleErrorMsg(ErrInvalidInput, "template_id cannot be empty", "")
	}
	if strings.TrimSpace(schemaTemplateFileFlag) == "" {
		return handleErrorMsg(ErrMissingArgument, "--file is required", "Use --file <path-under-directories.template>")
	}

	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}
	templateDir := vaultCfg.GetTemplateDirectory()
	fileRef, err := template.ResolveFileRef(schemaTemplateFileFlag, templateDir)
	if err != nil {
		return handleErrorMsg(ErrInvalidInput, err.Error(), fmt.Sprintf("Use a file path under %s", templateDir))
	}

	fullPath := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
	if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
		return handleError(ErrFileOutsideVault, err, "Template files must be within the vault")
	}
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return handleErrorMsg(ErrFileNotFound, fmt.Sprintf("template file not found: %s", fileRef), "Create the file first under directories.template (for example: templates/...)")
	}

	schemaDoc, _, err := readSchemaDoc(vaultPath)
	if err != nil {
		return err
	}
	templatesNode := ensureMapNode(schemaDoc, "templates")
	templateNode := ensureMapNode(templatesNode, templateID)
	templateNode["file"] = fileRef
	if schemaTemplateDescriptionFlag == "-" {
		delete(templateNode, "description")
	} else if strings.TrimSpace(schemaTemplateDescriptionFlag) != "" {
		templateNode["description"] = strings.TrimSpace(schemaTemplateDescriptionFlag)
	}

	if err := writeSchemaDoc(vaultPath, schemaDoc); err != nil {
		return err
	}

	elapsed := time.Since(start).Milliseconds()
	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"id":          templateID,
			"file":        fileRef,
			"description": strings.TrimSpace(schemaTemplateDescriptionFlag),
		}, &Meta{QueryTimeMs: elapsed})
		return nil
	}
	fmt.Printf("Set schema template %s -> %s\n", templateID, fileRef)
	return nil
}

func schemaTemplateRemove(vaultPath, templateID string, start time.Time) error {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return handleErrorMsg(ErrInvalidInput, "template_id cannot be empty", "")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' to create a schema")
	}
	if _, ok := sch.Templates[templateID]; !ok {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("template '%s' not found", templateID), "Nothing to remove")
	}

	var refs []string
	for typeName, typeDef := range sch.Types {
		if typeDef == nil {
			continue
		}
		for _, refID := range typeDef.Templates {
			if refID == templateID {
				refs = append(refs, typeName)
				break
			}
		}
	}
	if len(refs) > 0 {
		sort.Strings(refs)
		return handleErrorMsg(
			ErrInvalidInput,
			fmt.Sprintf("template '%s' is still referenced by: %s", templateID, strings.Join(refs, ", ")),
			"Remove those type template bindings first with `rvn schema type <type_name> template remove <template_id>`",
		)
	}

	schemaDoc, _, err := readSchemaDoc(vaultPath)
	if err != nil {
		return err
	}
	templatesNode, ok := schemaDoc["templates"].(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrInvalidInput, "schema has no templates section", "Nothing to remove")
	}
	delete(templatesNode, templateID)
	if len(templatesNode) == 0 {
		delete(schemaDoc, "templates")
	}
	if err := writeSchemaDoc(vaultPath, schemaDoc); err != nil {
		return err
	}

	elapsed := time.Since(start).Milliseconds()
	if isJSONOutput() {
		outputSuccess(map[string]interface{}{"removed": true, "id": templateID}, &Meta{QueryTimeMs: elapsed})
		return nil
	}
	fmt.Printf("Removed schema template %s\n", templateID)
	return nil
}

func runSchemaTypeTemplateCommand(vaultPath, typeName string, args []string, start time.Time) error {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return handleErrorMsg(ErrInvalidInput, "type_name cannot be empty", "")
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return handleError(ErrSchemaNotFound, err, "Run 'rvn init' first")
	}
	if schema.IsBuiltinType(typeName) && typeName != "date" {
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type and cannot use custom templates", typeName), "")
	}
	typeDef, ok := sch.Types[typeName]
	if !ok || typeDef == nil {
		return handleErrorMsg(ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "Run 'rvn schema types' to see available types")
	}

	if len(args) == 0 {
		return handleErrorMsg(ErrMissingArgument, "missing type template subcommand", "Use: rvn schema type <type_name> template list|set|remove|default ...")
	}

	switch args[0] {
	case "list":
		elapsed := time.Since(start).Milliseconds()
		templateIDs := sortedTemplateIDs(typeDef.Templates)
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"type":             typeName,
				"templates":        templateIDs,
				"default_template": typeDef.DefaultTemplate,
			}, &Meta{Count: len(templateIDs), QueryTimeMs: elapsed})
			return nil
		}
		fmt.Printf("Type templates for %s:\n", typeName)
		if len(templateIDs) == 0 {
			fmt.Println("  (none)")
		} else {
			for _, templateID := range templateIDs {
				fmt.Printf("  - %s\n", templateID)
			}
		}
		if typeDef.DefaultTemplate != "" {
			fmt.Printf("Default: %s\n", typeDef.DefaultTemplate)
		} else {
			fmt.Println("Default: (none)")
		}
		return nil
	case "set":
		if len(args) != 2 {
			return handleErrorMsg(ErrInvalidInput, "set requires template_id", "Use: rvn schema type <type_name> template set <template_id>")
		}
		templateID := strings.TrimSpace(args[1])
		if templateID == "" {
			return handleErrorMsg(ErrInvalidInput, "template_id cannot be empty", "")
		}
		if _, exists := sch.Templates[templateID]; !exists {
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown template '%s'", templateID), "Use `rvn schema template list` to see available template IDs")
		}
		if containsTemplateID(typeDef.Templates, templateID) {
			elapsed := time.Since(start).Milliseconds()
			if isJSONOutput() {
				outputSuccess(map[string]interface{}{
					"type":          typeName,
					"template_id":   templateID,
					"already_set":   true,
					"default_match": typeDef.DefaultTemplate == templateID,
				}, &Meta{QueryTimeMs: elapsed})
				return nil
			}
			fmt.Printf("Type %s already includes template %s\n", typeName, templateID)
			return nil
		}
		newTemplateIDs := append(append([]string(nil), typeDef.Templates...), templateID)

		schemaDoc, types, err := readSchemaDoc(vaultPath)
		if err != nil {
			return err
		}
		typeNode := ensureMapNode(types, typeName)
		typeNode["templates"] = toInterfaceSlice(newTemplateIDs)
		if err := writeSchemaDoc(vaultPath, schemaDoc); err != nil {
			return err
		}
		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"type":        typeName,
				"template_id": templateID,
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		fmt.Printf("Added template %s to type %s\n", templateID, typeName)
		return nil
	case "remove":
		if len(args) != 2 {
			return handleErrorMsg(ErrInvalidInput, "remove requires template_id", "Use: rvn schema type <type_name> template remove <template_id>")
		}
		templateID := strings.TrimSpace(args[1])
		if templateID == "" {
			return handleErrorMsg(ErrInvalidInput, "template_id cannot be empty", "")
		}
		if !containsTemplateID(typeDef.Templates, templateID) {
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("type '%s' does not include template '%s'", typeName, templateID), "Nothing to remove")
		}
		newTemplateIDs := removeTemplateID(typeDef.Templates, templateID)

		schemaDoc, types, err := readSchemaDoc(vaultPath)
		if err != nil {
			return err
		}
		typeNode := ensureMapNode(types, typeName)
		if len(newTemplateIDs) == 0 {
			delete(typeNode, "templates")
		} else {
			typeNode["templates"] = toInterfaceSlice(newTemplateIDs)
		}
		if currentDefault, ok := typeNode["default_template"].(string); ok && currentDefault == templateID {
			delete(typeNode, "default_template")
		}
		if err := writeSchemaDoc(vaultPath, schemaDoc); err != nil {
			return err
		}
		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{"type": typeName, "template_id": templateID, "removed": true}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		fmt.Printf("Removed template %s from type %s\n", templateID, typeName)
		return nil
	case "default":
		if schemaTypeTemplateClearFlag {
			schemaDoc, types, err := readSchemaDoc(vaultPath)
			if err != nil {
				return err
			}
			typeNode := ensureMapNode(types, typeName)
			delete(typeNode, "default_template")
			if err := writeSchemaDoc(vaultPath, schemaDoc); err != nil {
				return err
			}
			elapsed := time.Since(start).Milliseconds()
			if isJSONOutput() {
				outputSuccess(map[string]interface{}{"type": typeName, "default_template": ""}, &Meta{QueryTimeMs: elapsed})
				return nil
			}
			fmt.Printf("Cleared default template for type %s\n", typeName)
			return nil
		}
		if len(args) != 2 {
			return handleErrorMsg(ErrInvalidInput, "default requires template_id or --clear", "Use: rvn schema type <type_name> template default <template_id> OR --clear")
		}
		templateID := strings.TrimSpace(args[1])
		if !containsTemplateID(typeDef.Templates, templateID) {
			return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("type '%s' does not include template '%s'", typeName, templateID), "Use `rvn schema type <type_name> template list` to see available template IDs")
		}
		schemaDoc, types, err := readSchemaDoc(vaultPath)
		if err != nil {
			return err
		}
		typeNode := ensureMapNode(types, typeName)
		typeNode["default_template"] = templateID
		if err := writeSchemaDoc(vaultPath, schemaDoc); err != nil {
			return err
		}
		elapsed := time.Since(start).Milliseconds()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{"type": typeName, "default_template": templateID}, &Meta{QueryTimeMs: elapsed})
			return nil
		}
		fmt.Printf("Set default template for type %s -> %s\n", typeName, templateID)
		return nil
	default:
		return handleErrorMsg(ErrInvalidInput, fmt.Sprintf("unknown type template subcommand: %s", args[0]), "Use: list, set, remove, or default")
	}
}

func containsTemplateID(templateIDs []string, templateID string) bool {
	for _, id := range templateIDs {
		if id == templateID {
			return true
		}
	}
	return false
}

func removeTemplateID(templateIDs []string, templateID string) []string {
	out := make([]string, 0, len(templateIDs))
	for _, id := range templateIDs {
		if id == templateID {
			continue
		}
		out = append(out, id)
	}
	return out
}

func sortedTemplateIDs(templateIDs []string) []string {
	out := append([]string(nil), templateIDs...)
	sort.Strings(out)
	return out
}

func toInterfaceSlice(items []string) []interface{} {
	out := make([]interface{}, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func ensureMapNode(parent map[string]interface{}, key string) map[string]interface{} {
	node, ok := parent[key].(map[string]interface{})
	if ok {
		return node
	}
	node = make(map[string]interface{})
	parent[key] = node
	return node
}
