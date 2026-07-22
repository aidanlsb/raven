package schemasvc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type TemplateBindingState struct {
	Templates       []string
	DefaultTemplate string
}

type AddTemplateBindingResult struct {
	AlreadySet   bool
	DefaultMatch bool
}

func ListTypeTemplates(rt *vaultruntime.Runtime, typeName string) (*TemplateBindingState, error) {
	_, typeDef, err := loadTypeForTemplateConfig(rt, typeName)
	if err != nil {
		return nil, err
	}

	return &TemplateBindingState{
		Templates:       sortedTemplateIDs(typeDef.Templates),
		DefaultTemplate: typeDef.DefaultTemplate,
	}, nil
}

func AddTypeTemplate(rt *vaultruntime.Runtime, typeName, templateID string) (*AddTemplateBindingResult, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil, newError(ErrorInvalidInput, "template_id cannot be empty", "", nil, nil)
	}
	normalizedTypeName, err := validateTemplateTypeName(typeName)
	if err != nil {
		return nil, err
	}
	typeName = normalizedTypeName

	result := &AddTemplateBindingResult{}
	err = editSchema(rt.VaultPath, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		sch := doc.Schema()
		typeDef, err := typeForTemplateConfig(sch, typeName)
		if err != nil {
			return err
		}
		if _, exists := sch.Templates[templateID]; !exists {
			return newError(
				ErrorInvalidInput,
				fmt.Sprintf("unknown template '%s'", templateID),
				"Use `rvn schema template list` to see available template IDs",
				nil,
				nil,
			)
		}
		if containsTemplateID(typeDef.Templates, templateID) {
			result.AlreadySet = true
			result.DefaultMatch = typeDef.DefaultTemplate == templateID
			return schemadoc.ErrNoChange
		}

		typesNode, ok := doc.Root()["types"].(map[string]interface{})
		if !ok {
			return newError(ErrorSchemaInvalid, "types section not found", "", nil, nil)
		}
		typeNode := schemadoc.EnsureMap(typesNode, typeName)
		typeNode["templates"] = toInterfaceSlice(append(append([]string(nil), typeDef.Templates...), templateID))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func RemoveTypeTemplate(rt *vaultruntime.Runtime, typeName, templateID string, clearDefault bool) error {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return newError(ErrorInvalidInput, "template_id cannot be empty", "", nil, nil)
	}
	normalizedTypeName, err := validateTemplateTypeName(typeName)
	if err != nil {
		return err
	}
	typeName = normalizedTypeName

	return editSchema(rt.VaultPath, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		typeDef, err := typeForTemplateConfig(doc.Schema(), typeName)
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

		typesNode, ok := doc.Root()["types"].(map[string]interface{})
		if !ok {
			return newError(ErrorSchemaInvalid, "types section not found", "", nil, nil)
		}
		typeNode := schemadoc.EnsureMap(typesNode, typeName)
		newTemplateIDs := removeTemplateID(typeDef.Templates, templateID)
		if len(newTemplateIDs) == 0 {
			delete(typeNode, "templates")
		} else {
			typeNode["templates"] = toInterfaceSlice(newTemplateIDs)
		}
		if currentDefault, ok := typeNode["default_template"].(string); ok && currentDefault == templateID {
			if !clearDefault {
				return newError(
					ErrorInvalidInput,
					fmt.Sprintf("template '%s' is the default for type '%s'", templateID, typeName),
					"Re-run with --clear-default, or change the default first with `rvn schema template default <template_id> --type "+typeName+"`",
					nil,
					nil,
				)
			}
			delete(typeNode, "default_template")
		}
		return nil
	})
}

func SetTypeDefaultTemplate(rt *vaultruntime.Runtime, typeName, templateID string, clearDefault bool) (string, error) {
	templateID = strings.TrimSpace(templateID)
	normalizedTypeName, err := validateTemplateTypeName(typeName)
	if err != nil {
		return "", err
	}
	typeName = normalizedTypeName

	newDefault := templateID
	err = editSchema(rt.VaultPath, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		typeDef, err := typeForTemplateConfig(doc.Schema(), typeName)
		if err != nil {
			return err
		}

		typesNode, ok := doc.Root()["types"].(map[string]interface{})
		if !ok {
			return newError(ErrorSchemaInvalid, "types section not found", "", nil, nil)
		}
		typeNode := schemadoc.EnsureMap(typesNode, typeName)

		if clearDefault {
			delete(typeNode, "default_template")
			newDefault = ""
			return nil
		}
		if templateID == "" {
			return newError(
				ErrorInvalidInput,
				"default requires template_id or --clear",
				"Use: rvn schema template default <template_id> --type "+typeName+" OR --clear",
				nil,
				nil,
			)
		}
		if !containsTemplateID(typeDef.Templates, templateID) {
			return newError(
				ErrorInvalidInput,
				fmt.Sprintf("type '%s' does not include template '%s'", typeName, templateID),
				"Use `rvn schema template list --type "+typeName+"` to see available template IDs",
				nil,
				nil,
			)
		}
		typeNode["default_template"] = templateID
		return nil
	})
	if err != nil {
		return "", err
	}

	return newDefault, nil
}

func ListCoreTemplates(rt *vaultruntime.Runtime, coreTypeName string) (*TemplateBindingState, error) {
	coreDef, err := loadCoreTypeForTemplateConfig(rt, coreTypeName)
	if err != nil {
		return nil, err
	}

	return &TemplateBindingState{
		Templates:       sortedTemplateIDs(coreDef.Templates),
		DefaultTemplate: coreDef.DefaultTemplate,
	}, nil
}

func AddCoreTemplate(rt *vaultruntime.Runtime, coreTypeName, templateID string) (*AddTemplateBindingResult, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil, newError(ErrorInvalidInput, "template_id cannot be empty", "", nil, nil)
	}
	normalizedCoreTypeName, err := validateTemplateCoreTypeName(coreTypeName)
	if err != nil {
		return nil, err
	}
	coreTypeName = normalizedCoreTypeName

	result := &AddTemplateBindingResult{}
	err = editSchema(rt.VaultPath, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		sch := doc.Schema()
		coreDef, err := coreTypeForTemplateConfig(sch, coreTypeName)
		if err != nil {
			return err
		}
		if _, exists := sch.Templates[templateID]; !exists {
			return newError(
				ErrorInvalidInput,
				fmt.Sprintf("unknown template '%s'", templateID),
				"Use `rvn schema template list` to see available template IDs",
				nil,
				nil,
			)
		}
		if containsTemplateID(coreDef.Templates, templateID) {
			result.AlreadySet = true
			result.DefaultMatch = coreDef.DefaultTemplate == templateID
			return schemadoc.ErrNoChange
		}

		coreNode := schemadoc.EnsureMap(doc.Root(), "core")
		typeNode := schemadoc.EnsureMap(coreNode, coreTypeName)
		typeNode["templates"] = toInterfaceSlice(append(append([]string(nil), coreDef.Templates...), templateID))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func RemoveCoreTemplate(rt *vaultruntime.Runtime, coreTypeName, templateID string, clearDefault bool) error {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return newError(ErrorInvalidInput, "template_id cannot be empty", "", nil, nil)
	}
	normalizedCoreTypeName, err := validateTemplateCoreTypeName(coreTypeName)
	if err != nil {
		return err
	}
	coreTypeName = normalizedCoreTypeName

	return editSchema(rt.VaultPath, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		coreDef, err := coreTypeForTemplateConfig(doc.Schema(), coreTypeName)
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

		coreNode := schemadoc.EnsureMap(doc.Root(), "core")
		typeNode := schemadoc.EnsureMap(coreNode, coreTypeName)
		newTemplateIDs := removeTemplateID(coreDef.Templates, templateID)
		if len(newTemplateIDs) == 0 {
			delete(typeNode, "templates")
		} else {
			typeNode["templates"] = toInterfaceSlice(newTemplateIDs)
		}
		if currentDefault, ok := typeNode["default_template"].(string); ok && currentDefault == templateID {
			if !clearDefault {
				return newError(
					ErrorInvalidInput,
					fmt.Sprintf("template '%s' is the default for core type '%s'", templateID, coreTypeName),
					"Re-run with --clear-default, or change the default first with `rvn schema template default <template_id> --core "+coreTypeName+"`",
					nil,
					nil,
				)
			}
			delete(typeNode, "default_template")
		}
		return nil
	})
}

func SetCoreDefaultTemplate(rt *vaultruntime.Runtime, coreTypeName, templateID string, clearDefault bool) (string, error) {
	templateID = strings.TrimSpace(templateID)
	normalizedCoreTypeName, err := validateTemplateCoreTypeName(coreTypeName)
	if err != nil {
		return "", err
	}
	coreTypeName = normalizedCoreTypeName

	newDefault := templateID
	err = editSchema(rt.VaultPath, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		coreDef, err := coreTypeForTemplateConfig(doc.Schema(), coreTypeName)
		if err != nil {
			return err
		}

		coreNode := schemadoc.EnsureMap(doc.Root(), "core")
		typeNode := schemadoc.EnsureMap(coreNode, coreTypeName)
		if clearDefault {
			delete(typeNode, "default_template")
			newDefault = ""
			return nil
		}
		if templateID == "" {
			return newError(
				ErrorInvalidInput,
				"default requires template_id or --clear",
				"Use: rvn schema template default <template_id> --core "+coreTypeName+" OR --clear",
				nil,
				nil,
			)
		}
		if !containsTemplateID(coreDef.Templates, templateID) {
			return newError(
				ErrorInvalidInput,
				fmt.Sprintf("core type '%s' does not include template '%s'", coreTypeName, templateID),
				"Use `rvn schema template list --core "+coreTypeName+"` to see available template IDs",
				nil,
				nil,
			)
		}
		typeNode["default_template"] = templateID
		return nil
	})
	if err != nil {
		return "", err
	}

	return newDefault, nil
}

func loadTypeForTemplateConfig(rt *vaultruntime.Runtime, typeName string) (*schema.Schema, *schema.TypeDefinition, error) {
	normalizedTypeName, err := validateTemplateTypeName(typeName)
	if err != nil {
		return nil, nil, err
	}
	sch, err := runtimeSchema(rt, "Run 'rvn init' first")
	if err != nil {
		return nil, nil, err
	}
	typeDef, err := typeForTemplateConfig(sch, normalizedTypeName)
	if err != nil {
		return nil, nil, err
	}
	return sch, typeDef, nil
}

func loadCoreTypeForTemplateConfig(rt *vaultruntime.Runtime, coreTypeName string) (*schema.CoreTypeDefinition, error) {
	_, coreDef, err := loadSchemaAndCoreType(vaultPath, coreTypeName)
	if err != nil {
		return nil, err
	}
	return coreDef, nil
}

func loadSchemaAndCoreType(vaultPath, coreTypeName string) (*schema.Schema, *schema.CoreTypeDefinition, error) {
	normalizedCoreTypeName, err := validateTemplateCoreTypeName(coreTypeName)
	if err != nil {
		return nil, nil, err
	}
	sch, err := runtimeSchema(rt, "Run 'rvn init' first")
	if err != nil {
		return nil, nil, err
	}
	coreDef, err := coreTypeForTemplateConfig(sch, normalizedCoreTypeName)
	if err != nil {
		return nil, nil, err
	}
	return sch, coreDef, nil
}

func typeForTemplateConfig(sch *schema.Schema, typeName string) (*schema.TypeDefinition, error) {
	typeName, err := validateTemplateTypeName(typeName)
	if err != nil {
		return nil, err
	}
	typeDef, ok := sch.Types[typeName]
	if !ok || typeDef == nil {
		return nil, newError(
			ErrorTypeNotFound,
			fmt.Sprintf("type '%s' not found", typeName),
			"Run 'rvn schema types' to see available types",
			nil,
			nil,
		)
	}
	return typeDef, nil
}

func coreTypeForTemplateConfig(sch *schema.Schema, coreTypeName string) (*schema.CoreTypeDefinition, error) {
	coreTypeName, err := validateTemplateCoreTypeName(coreTypeName)
	if err != nil {
		return nil, err
	}
	coreDef := sch.Core[coreTypeName]
	if coreDef == nil {
		coreDef = &schema.CoreTypeDefinition{}
	}
	return coreDef, nil
}

func validateTemplateTypeName(typeName string) (string, error) {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return "", newError(ErrorInvalidInput, "type_name cannot be empty", "", nil, nil)
	}
	if schema.IsBuiltinType(typeName) {
		return "", newError(
			ErrorInvalidInput,
			fmt.Sprintf("'%s' is a core type; configure templates with `rvn schema template ... --core %s`", typeName, typeName),
			"",
			nil,
			nil,
		)
	}
	return typeName, nil
}

func validateTemplateCoreTypeName(coreTypeName string) (string, error) {
	coreTypeName = strings.TrimSpace(coreTypeName)
	if coreTypeName == "" {
		return "", newError(ErrorInvalidInput, "core_type cannot be empty", "", nil, nil)
	}
	if !schema.IsBuiltinType(coreTypeName) {
		return "", newError(
			ErrorTypeNotFound,
			fmt.Sprintf("core type '%s' not found", coreTypeName),
			"Available core types: date, page, section",
			nil,
			nil,
		)
	}
	if coreTypeName == "section" {
		return "", newError(ErrorInvalidInput, "core type 'section' does not support template configuration", "", nil, nil)
	}
	return coreTypeName, nil
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
