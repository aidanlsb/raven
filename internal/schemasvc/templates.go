package schemasvc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/template"
)

type TemplateDefinition struct {
	ID          string `json:"id"`
	File        string `json:"file"`
	Description string `json:"description,omitempty"`
}

type TemplateBindingState struct {
	Templates       []string
	DefaultTemplate string
}

type AddTemplateBindingResult struct {
	AlreadySet   bool
	DefaultMatch bool
}

type SetTemplateRequest struct {
	VaultPath   string
	TemplateID  string
	File        string
	Description string
}

func ListTemplates(vaultPath string) ([]TemplateDefinition, error) {
	sch, err := loadSchema(vaultPath, "Run 'rvn init' to create a schema")
	if err != nil {
		return nil, err
	}

	items := make([]TemplateDefinition, 0, len(sch.Templates))
	for id, def := range sch.Templates {
		if def == nil {
			continue
		}
		items = append(items, TemplateDefinition{
			ID:          id,
			File:        def.File,
			Description: def.Description,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func GetTemplate(vaultPath, templateID string) (*TemplateDefinition, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil, newError(ErrorInvalidInput, "template_id cannot be empty", "", nil, nil)
	}

	sch, err := loadSchema(vaultPath, "Run 'rvn init' to create a schema")
	if err != nil {
		return nil, err
	}

	templateDef, ok := sch.Templates[templateID]
	if !ok || templateDef == nil {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("template '%s' not found", templateID),
			"Use `rvn schema template list` to see available template IDs",
			nil,
			nil,
		)
	}

	return &TemplateDefinition{
		ID:          templateID,
		File:        templateDef.File,
		Description: templateDef.Description,
	}, nil
}

func SetTemplate(req SetTemplateRequest) (*TemplateDefinition, error) {
	templateID := strings.TrimSpace(req.TemplateID)
	if templateID == "" {
		return nil, newError(ErrorInvalidInput, "template_id cannot be empty", "", nil, nil)
	}
	if strings.TrimSpace(req.File) == "" {
		return nil, newError(ErrorInvalidInput, "--file is required", "Use --file <path-under-directories.template>", nil, nil)
	}

	vaultCfg, err := config.LoadVaultConfig(req.VaultPath)
	if err != nil {
		return nil, newError(ErrorConfigInvalid, "failed to load vault config", "Fix raven.yaml and try again", nil, err)
	}

	templateDir := vaultCfg.GetTemplateDirectory()
	fileRef, err := template.ResolveFileRef(req.File, templateDir)
	if err != nil {
		return nil, newError(ErrorInvalidInput, err.Error(), fmt.Sprintf("Use a file path under %s", templateDir), nil, err)
	}

	fullPath := filepath.Join(req.VaultPath, filepath.FromSlash(fileRef))
	if err := paths.ValidateWithinVault(req.VaultPath, fullPath); err != nil {
		return nil, newError(ErrorFileOutside, "template files must be within the vault", "Template files must be within the vault", nil, err)
	}
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return nil, newError(
			ErrorFileNotFound,
			fmt.Sprintf("template file not found: %s", fileRef),
			"Create the file first under directories.template (for example: templates/...)",
			nil,
			err,
		)
	}

	schemaDoc, err := readSchemaDoc(req.VaultPath)
	if err != nil {
		return nil, err
	}
	templatesNode := ensureMapNode(schemaDoc, "templates")
	templateNode := ensureMapNode(templatesNode, templateID)
	templateNode["file"] = fileRef

	description := strings.TrimSpace(req.Description)
	if req.Description == "-" {
		delete(templateNode, "description")
		description = ""
	} else if description != "" {
		templateNode["description"] = description
	}

	if err := writeSchemaDoc(req.VaultPath, schemaDoc); err != nil {
		return nil, err
	}

	return &TemplateDefinition{
		ID:          templateID,
		File:        fileRef,
		Description: description,
	}, nil
}

func RemoveTemplate(vaultPath, templateID string) error {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return newError(ErrorInvalidInput, "template_id cannot be empty", "", nil, nil)
	}

	sch, err := loadSchema(vaultPath, "Run 'rvn init' to create a schema")
	if err != nil {
		return err
	}
	if _, ok := sch.Templates[templateID]; !ok {
		return newError(ErrorInvalidInput, fmt.Sprintf("template '%s' not found", templateID), "Nothing to remove", nil, nil)
	}

	refs := templateRefs(sch, templateID)
	if len(refs) > 0 {
		return newError(
			ErrorInvalidInput,
			fmt.Sprintf("template '%s' is still referenced by: %s", templateID, strings.Join(refs, ", ")),
			"Remove those bindings first with `rvn schema type <type_name> template remove <template_id>` or `rvn schema core <core_type> template remove <template_id>`",
			nil,
			nil,
		)
	}

	schemaDoc, err := readSchemaDoc(vaultPath)
	if err != nil {
		return err
	}
	templatesNode, ok := schemaDoc["templates"].(map[string]interface{})
	if !ok {
		return newError(ErrorInvalidInput, "schema has no templates section", "Nothing to remove", nil, nil)
	}
	delete(templatesNode, templateID)
	if len(templatesNode) == 0 {
		delete(schemaDoc, "templates")
	}

	return writeSchemaDoc(vaultPath, schemaDoc)
}

func ListTypeTemplates(vaultPath, typeName string) (*TemplateBindingState, error) {
	_, typeDef, err := loadTypeForTemplateConfig(vaultPath, typeName)
	if err != nil {
		return nil, err
	}

	return &TemplateBindingState{
		Templates:       sortedTemplateIDs(typeDef.Templates),
		DefaultTemplate: typeDef.DefaultTemplate,
	}, nil
}

func AddTypeTemplate(vaultPath, typeName, templateID string) (*AddTemplateBindingResult, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil, newError(ErrorInvalidInput, "template_id cannot be empty", "", nil, nil)
	}

	sch, typeDef, err := loadTypeForTemplateConfig(vaultPath, typeName)
	if err != nil {
		return nil, err
	}
	if _, exists := sch.Templates[templateID]; !exists {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("unknown template '%s'", templateID),
			"Use `rvn schema template list` to see available template IDs",
			nil,
			nil,
		)
	}
	if containsTemplateID(typeDef.Templates, templateID) {
		return &AddTemplateBindingResult{
			AlreadySet:   true,
			DefaultMatch: typeDef.DefaultTemplate == templateID,
		}, nil
	}

	newTemplateIDs := append(append([]string(nil), typeDef.Templates...), templateID)
	schemaDoc, typesNode, err := readSchemaDocWithTypes(vaultPath)
	if err != nil {
		return nil, err
	}
	typeNode := ensureMapNode(typesNode, typeName)
	typeNode["templates"] = toInterfaceSlice(newTemplateIDs)
	if err := writeSchemaDoc(vaultPath, schemaDoc); err != nil {
		return nil, err
	}

	return &AddTemplateBindingResult{}, nil
}

func RemoveTypeTemplate(vaultPath, typeName, templateID string) error {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return newError(ErrorInvalidInput, "template_id cannot be empty", "", nil, nil)
	}

	_, typeDef, err := loadTypeForTemplateConfig(vaultPath, typeName)
	if err != nil {
		return err
	}
	if !containsTemplateID(typeDef.Templates, templateID) {
		return newError(
			ErrorInvalidInput,
			fmt.Sprintf("type '%s' does not include template '%s'", typeName, templateID),
			"Nothing to remove",
			nil,
			nil,
		)
	}

	newTemplateIDs := removeTemplateID(typeDef.Templates, templateID)
	schemaDoc, typesNode, err := readSchemaDocWithTypes(vaultPath)
	if err != nil {
		return err
	}
	typeNode := ensureMapNode(typesNode, typeName)
	if len(newTemplateIDs) == 0 {
		delete(typeNode, "templates")
	} else {
		typeNode["templates"] = toInterfaceSlice(newTemplateIDs)
	}
	if currentDefault, ok := typeNode["default_template"].(string); ok && currentDefault == templateID {
		delete(typeNode, "default_template")
	}

	return writeSchemaDoc(vaultPath, schemaDoc)
}

func SetTypeDefaultTemplate(vaultPath, typeName, templateID string, clearDefault bool) (string, error) {
	typeName = strings.TrimSpace(typeName)
	templateID = strings.TrimSpace(templateID)
	if typeName == "" {
		return "", newError(ErrorInvalidInput, "type_name cannot be empty", "", nil, nil)
	}

	_, typeDef, err := loadTypeForTemplateConfig(vaultPath, typeName)
	if err != nil {
		return "", err
	}

	if clearDefault {
		schemaDoc, typesNode, err := readSchemaDocWithTypes(vaultPath)
		if err != nil {
			return "", err
		}
		typeNode := ensureMapNode(typesNode, typeName)
		delete(typeNode, "default_template")
		if err := writeSchemaDoc(vaultPath, schemaDoc); err != nil {
			return "", err
		}
		return "", nil
	}

	if templateID == "" {
		return "", newError(
			ErrorInvalidInput,
			"default requires template_id or --clear",
			"Use: rvn schema type <type_name> template default <template_id> OR --clear",
			nil,
			nil,
		)
	}
	if !containsTemplateID(typeDef.Templates, templateID) {
		return "", newError(
			ErrorInvalidInput,
			fmt.Sprintf("type '%s' does not include template '%s'", typeName, templateID),
			"Use `rvn schema type <type_name> template list` to see available template IDs",
			nil,
			nil,
		)
	}

	schemaDoc, typesNode, err := readSchemaDocWithTypes(vaultPath)
	if err != nil {
		return "", err
	}
	typeNode := ensureMapNode(typesNode, typeName)
	typeNode["default_template"] = templateID
	if err := writeSchemaDoc(vaultPath, schemaDoc); err != nil {
		return "", err
	}

	return templateID, nil
}

func ListCoreTemplates(vaultPath, coreTypeName string) (*TemplateBindingState, error) {
	coreDef, err := loadCoreTypeForTemplateConfig(vaultPath, coreTypeName)
	if err != nil {
		return nil, err
	}

	return &TemplateBindingState{
		Templates:       sortedTemplateIDs(coreDef.Templates),
		DefaultTemplate: coreDef.DefaultTemplate,
	}, nil
}

func AddCoreTemplate(vaultPath, coreTypeName, templateID string) (*AddTemplateBindingResult, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil, newError(ErrorInvalidInput, "template_id cannot be empty", "", nil, nil)
	}

	sch, coreDef, err := loadSchemaAndCoreType(vaultPath, coreTypeName)
	if err != nil {
		return nil, err
	}
	if _, exists := sch.Templates[templateID]; !exists {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("unknown template '%s'", templateID),
			"Use `rvn schema template list` to see available template IDs",
			nil,
			nil,
		)
	}
	if containsTemplateID(coreDef.Templates, templateID) {
		return &AddTemplateBindingResult{
			AlreadySet:   true,
			DefaultMatch: coreDef.DefaultTemplate == templateID,
		}, nil
	}

	newTemplateIDs := append(append([]string(nil), coreDef.Templates...), templateID)
	schemaDoc, err := readSchemaDoc(vaultPath)
	if err != nil {
		return nil, err
	}
	coreNode := ensureMapNode(schemaDoc, "core")
	typeNode := ensureMapNode(coreNode, coreTypeName)
	typeNode["templates"] = toInterfaceSlice(newTemplateIDs)
	if err := writeSchemaDoc(vaultPath, schemaDoc); err != nil {
		return nil, err
	}

	return &AddTemplateBindingResult{}, nil
}

func RemoveCoreTemplate(vaultPath, coreTypeName, templateID string) error {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return newError(ErrorInvalidInput, "template_id cannot be empty", "", nil, nil)
	}

	_, coreDef, err := loadSchemaAndCoreType(vaultPath, coreTypeName)
	if err != nil {
		return err
	}
	if !containsTemplateID(coreDef.Templates, templateID) {
		return newError(
			ErrorInvalidInput,
			fmt.Sprintf("core type '%s' does not include template '%s'", coreTypeName, templateID),
			"Nothing to remove",
			nil,
			nil,
		)
	}

	newTemplateIDs := removeTemplateID(coreDef.Templates, templateID)
	schemaDoc, err := readSchemaDoc(vaultPath)
	if err != nil {
		return err
	}
	coreNode := ensureMapNode(schemaDoc, "core")
	typeNode := ensureMapNode(coreNode, coreTypeName)
	if len(newTemplateIDs) == 0 {
		delete(typeNode, "templates")
	} else {
		typeNode["templates"] = toInterfaceSlice(newTemplateIDs)
	}
	if currentDefault, ok := typeNode["default_template"].(string); ok && currentDefault == templateID {
		delete(typeNode, "default_template")
	}
	return writeSchemaDoc(vaultPath, schemaDoc)
}

func SetCoreDefaultTemplate(vaultPath, coreTypeName, templateID string, clearDefault bool) (string, error) {
	templateID = strings.TrimSpace(templateID)
	_, coreDef, err := loadSchemaAndCoreType(vaultPath, coreTypeName)
	if err != nil {
		return "", err
	}

	if clearDefault {
		schemaDoc, err := readSchemaDoc(vaultPath)
		if err != nil {
			return "", err
		}
		coreNode := ensureMapNode(schemaDoc, "core")
		typeNode := ensureMapNode(coreNode, coreTypeName)
		delete(typeNode, "default_template")
		if err := writeSchemaDoc(vaultPath, schemaDoc); err != nil {
			return "", err
		}
		return "", nil
	}

	if templateID == "" {
		return "", newError(
			ErrorInvalidInput,
			"default requires template_id or --clear",
			"Use: rvn schema core <core_type> template default <template_id> OR --clear",
			nil,
			nil,
		)
	}
	if !containsTemplateID(coreDef.Templates, templateID) {
		return "", newError(
			ErrorInvalidInput,
			fmt.Sprintf("core type '%s' does not include template '%s'", coreTypeName, templateID),
			"Use `rvn schema core <core_type> template list` to see available template IDs",
			nil,
			nil,
		)
	}

	schemaDoc, err := readSchemaDoc(vaultPath)
	if err != nil {
		return "", err
	}
	coreNode := ensureMapNode(schemaDoc, "core")
	typeNode := ensureMapNode(coreNode, coreTypeName)
	typeNode["default_template"] = templateID
	if err := writeSchemaDoc(vaultPath, schemaDoc); err != nil {
		return "", err
	}

	return templateID, nil
}

func loadTypeForTemplateConfig(vaultPath, typeName string) (*schema.Schema, *schema.TypeDefinition, error) {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return nil, nil, newError(ErrorInvalidInput, "type_name cannot be empty", "", nil, nil)
	}

	sch, err := loadSchema(vaultPath, "Run 'rvn init' first")
	if err != nil {
		return nil, nil, err
	}
	if schema.IsBuiltinType(typeName) {
		return nil, nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("'%s' is a core type; configure templates with `rvn schema core %s template ...`", typeName, typeName),
			"",
			nil,
			nil,
		)
	}

	typeDef, ok := sch.Types[typeName]
	if !ok || typeDef == nil {
		return nil, nil, newError(
			ErrorTypeNotFound,
			fmt.Sprintf("type '%s' not found", typeName),
			"Run 'rvn schema types' to see available types",
			nil,
			nil,
		)
	}
	return sch, typeDef, nil
}

func loadCoreTypeForTemplateConfig(vaultPath, coreTypeName string) (*schema.CoreTypeDefinition, error) {
	_, coreDef, err := loadSchemaAndCoreType(vaultPath, coreTypeName)
	if err != nil {
		return nil, err
	}
	return coreDef, nil
}

func loadSchemaAndCoreType(vaultPath, coreTypeName string) (*schema.Schema, *schema.CoreTypeDefinition, error) {
	coreTypeName = strings.TrimSpace(coreTypeName)
	if coreTypeName == "" {
		return nil, nil, newError(ErrorInvalidInput, "core_type cannot be empty", "", nil, nil)
	}
	if !schema.IsBuiltinType(coreTypeName) {
		return nil, nil, newError(
			ErrorTypeNotFound,
			fmt.Sprintf("core type '%s' not found", coreTypeName),
			"Available core types: date, page, section",
			nil,
			nil,
		)
	}
	if coreTypeName == "section" {
		return nil, nil, newError(ErrorInvalidInput, "core type 'section' does not support template configuration", "", nil, nil)
	}

	sch, err := loadSchema(vaultPath, "Run 'rvn init' first")
	if err != nil {
		return nil, nil, err
	}
	coreDef := sch.Core[coreTypeName]
	if coreDef == nil {
		coreDef = &schema.CoreTypeDefinition{}
	}
	return sch, coreDef, nil
}

func loadSchema(vaultPath, suggestion string) (*schema.Schema, error) {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return nil, newError(ErrorSchemaNotFound, err.Error(), suggestion, nil, err)
	}
	return sch, nil
}

func templateRefs(sch *schema.Schema, templateID string) []string {
	refs := make(map[string]struct{})
	for typeName, typeDef := range sch.Types {
		if typeDef == nil {
			continue
		}
		if containsTemplateID(typeDef.Templates, templateID) {
			refs[typeName] = struct{}{}
		}
	}
	for coreType, coreDef := range sch.Core {
		if coreDef == nil {
			continue
		}
		if containsTemplateID(coreDef.Templates, templateID) {
			refs["core."+coreType] = struct{}{}
		}
	}
	out := make([]string, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func readSchemaDoc(vaultPath string) (map[string]interface{}, error) {
	schemaPath := paths.SchemaPath(vaultPath)
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return nil, newError(ErrorFileRead, err.Error(), "", nil, err)
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return nil, newError(ErrorSchemaInvalid, err.Error(), "", nil, err)
	}
	return schemaDoc, nil
}

func readSchemaDocWithTypes(vaultPath string) (map[string]interface{}, map[string]interface{}, error) {
	schemaDoc, err := readSchemaDoc(vaultPath)
	if err != nil {
		return nil, nil, err
	}
	typesNode, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		return nil, nil, newError(ErrorSchemaInvalid, "types section not found", "", nil, nil)
	}
	return schemaDoc, typesNode, nil
}

func writeSchemaDoc(vaultPath string, schemaDoc map[string]interface{}) error {
	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return newError(ErrorInternal, err.Error(), "", nil, err)
	}
	schemaPath := paths.SchemaPath(vaultPath)
	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
		return newError(ErrorFileWrite, err.Error(), "", nil, err)
	}
	return nil
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
