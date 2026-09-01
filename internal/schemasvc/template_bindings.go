package schemasvc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/svcerr"
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

type bindingTarget struct {
	kind       string
	name       string
	definition bindingDefinition
}

type bindingDefinition interface {
	getTemplates() []string
	getDefaultTemplate() string
}

type typeBindingDef struct{ *schema.TypeDefinition }

func (t typeBindingDef) getTemplates() []string     { return t.Templates }
func (t typeBindingDef) getDefaultTemplate() string { return t.DefaultTemplate }

type coreBindingDef struct{ *schema.CoreTypeDefinition }

func (c coreBindingDef) getTemplates() []string     { return c.Templates }
func (c coreBindingDef) getDefaultTemplate() string { return c.DefaultTemplate }

func listTemplates(rt *vaultruntime.Runtime, target bindingTarget) (*TemplateBindingState, error) {
	return &TemplateBindingState{
		Templates:       sortedTemplateIDs(target.definition.getTemplates()),
		DefaultTemplate: target.definition.getDefaultTemplate(),
	}, nil
}

func ListTypeTemplates(rt *vaultruntime.Runtime, typeName string) (*TemplateBindingState, error) {
	target, err := loadTypeTarget(rt, typeName)
	if err != nil {
		return nil, err
	}
	return listTemplates(rt, target)
}

func ListCoreTemplates(rt *vaultruntime.Runtime, coreTypeName string) (*TemplateBindingState, error) {
	target, err := loadCoreTarget(rt, coreTypeName)
	if err != nil {
		return nil, err
	}
	return listTemplates(rt, target)
}

func addTemplate(rt *vaultruntime.Runtime, target bindingTarget, templateID string) (*AddTemplateBindingResult, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "template_id cannot be empty")
	}

	result := &AddTemplateBindingResult{}
	err := editRuntimeSchema(rt, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		sch := doc.Schema()
		if _, exists := sch.Templates[templateID]; !exists {
			return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("unknown template '%s'", templateID)).WithSuggestion("Use `rvn schema template list` to see available template IDs")
		}
		templates := target.definition.getTemplates()
		if containsTemplateID(templates, templateID) {
			result.AlreadySet = true
			result.DefaultMatch = target.definition.getDefaultTemplate() == templateID
			return schemadoc.ErrNoChange
		}

		targetNode := ensureTargetNode(doc, target)
		targetNode["templates"] = toInterfaceSlice(append(append([]string(nil), templates...), templateID))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func AddTypeTemplate(rt *vaultruntime.Runtime, typeName, templateID string) (*AddTemplateBindingResult, error) {
	target, err := loadTypeTarget(rt, typeName)
	if err != nil {
		return nil, err
	}
	return addTemplate(rt, target, templateID)
}

func AddCoreTemplate(rt *vaultruntime.Runtime, coreTypeName, templateID string) (*AddTemplateBindingResult, error) {
	target, err := loadCoreTarget(rt, coreTypeName)
	if err != nil {
		return nil, err
	}
	return addTemplate(rt, target, templateID)
}

func removeTemplate(rt *vaultruntime.Runtime, target bindingTarget, templateID string, clearDefault bool) error {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return svcerr.New(codes.ErrInvalidInput, "template_id cannot be empty")
	}

	return editRuntimeSchema(rt, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		templates := target.definition.getTemplates()
		if !containsTemplateID(templates, templateID) {
			return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("%s '%s' does not include template '%s'", target.kind, target.name, templateID)).WithSuggestion("Nothing to remove")
		}

		targetNode := ensureTargetNode(doc, target)
		newTemplateIDs := removeTemplateID(templates, templateID)
		if len(newTemplateIDs) == 0 {
			delete(targetNode, "templates")
		} else {
			targetNode["templates"] = toInterfaceSlice(newTemplateIDs)
		}
		if currentDefault, ok := targetNode["default_template"].(string); ok && currentDefault == templateID {
			if !clearDefault {
				return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("template '%s' is the default for %s '%s'", templateID, target.kind, target.name)).WithSuggestion(fmt.Sprintf("Re-run with --clear-default, or change the default with `rvn schema template bind <template_id> --%s %s --default`", target.kind, target.name))
			}
			delete(targetNode, "default_template")
		}
		return nil
	})
}

func RemoveTypeTemplate(rt *vaultruntime.Runtime, typeName, templateID string, clearDefault bool) error {
	target, err := loadTypeTarget(rt, typeName)
	if err != nil {
		return err
	}
	return removeTemplate(rt, target, templateID, clearDefault)
}

func RemoveCoreTemplate(rt *vaultruntime.Runtime, coreTypeName, templateID string, clearDefault bool) error {
	target, err := loadCoreTarget(rt, coreTypeName)
	if err != nil {
		return err
	}
	return removeTemplate(rt, target, templateID, clearDefault)
}

func setDefaultTemplate(rt *vaultruntime.Runtime, target bindingTarget, templateID string) (string, error) {
	templateID = strings.TrimSpace(templateID)

	err := editRuntimeSchema(rt, "Run 'rvn init' first", func(doc *schemadoc.Document) error {
		targetNode := ensureTargetNode(doc, target)

		if templateID == "" {
			return svcerr.New(codes.ErrInvalidInput, "default requires template_id").WithSuggestion(fmt.Sprintf("Use: rvn schema template bind <template_id> --%s %s --default", target.kind, target.name))
		}
		templates := target.definition.getTemplates()
		if !containsTemplateID(templates, templateID) {
			return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("%s '%s' does not include template '%s'", target.kind, target.name, templateID)).WithSuggestion(fmt.Sprintf("Use `rvn schema template list --%s %s` to see available template IDs", target.kind, target.name))
		}
		targetNode["default_template"] = templateID
		return nil
	})
	if err != nil {
		return "", err
	}

	return templateID, nil
}

func SetTypeDefaultTemplate(rt *vaultruntime.Runtime, typeName, templateID string) (string, error) {
	target, err := loadTypeTarget(rt, typeName)
	if err != nil {
		return "", err
	}
	return setDefaultTemplate(rt, target, templateID)
}

func SetCoreDefaultTemplate(rt *vaultruntime.Runtime, coreTypeName, templateID string) (string, error) {
	target, err := loadCoreTarget(rt, coreTypeName)
	if err != nil {
		return "", err
	}
	return setDefaultTemplate(rt, target, templateID)
}

func ensureTargetNode(doc *schemadoc.Document, target bindingTarget) map[string]interface{} {
	if target.kind == "type" {
		typesNode, ok := doc.Root()["types"].(map[string]interface{})
		if !ok {
			typesNode = make(map[string]interface{})
			doc.Root()["types"] = typesNode
		}
		return schemadoc.EnsureMap(typesNode, target.name)
	}
	coreNode := schemadoc.EnsureMap(doc.Root(), "core")
	return schemadoc.EnsureMap(coreNode, target.name)
}

func loadTypeTarget(rt *vaultruntime.Runtime, typeName string) (bindingTarget, error) {
	normalizedTypeName, err := validateTemplateTypeName(typeName)
	if err != nil {
		return bindingTarget{}, err
	}
	sch, err := runtimeSchema(rt, "Run 'rvn init' first")
	if err != nil {
		return bindingTarget{}, err
	}
	typeDef, err := typeForTemplateConfig(sch, normalizedTypeName)
	if err != nil {
		return bindingTarget{}, err
	}
	return bindingTarget{
		kind:       "type",
		name:       normalizedTypeName,
		definition: typeBindingDef{typeDef},
	}, nil
}

func loadCoreTarget(rt *vaultruntime.Runtime, coreTypeName string) (bindingTarget, error) {
	normalizedCoreTypeName, err := validateTemplateCoreTypeName(coreTypeName)
	if err != nil {
		return bindingTarget{}, err
	}
	sch, err := runtimeSchema(rt, "Run 'rvn init' first")
	if err != nil {
		return bindingTarget{}, err
	}
	coreDef, err := coreTypeForTemplateConfig(sch, normalizedCoreTypeName)
	if err != nil {
		return bindingTarget{}, err
	}
	return bindingTarget{
		kind:       "core",
		name:       normalizedCoreTypeName,
		definition: coreBindingDef{coreDef},
	}, nil
}

func typeForTemplateConfig(sch *schema.Schema, typeName string) (*schema.TypeDefinition, error) {
	typeName, err := validateTemplateTypeName(typeName)
	if err != nil {
		return nil, err
	}
	typeDef, ok := sch.Types[typeName]
	if !ok || typeDef == nil {
		return nil, svcerr.New(codes.ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName)).WithSuggestion("Run 'rvn schema types' to see available types")
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
		return "", svcerr.New(codes.ErrInvalidInput, "type_name cannot be empty")
	}
	if schema.IsBuiltinType(typeName) {
		return "", svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("'%s' is a core type; configure templates with `rvn schema template ... --core %s`", typeName, typeName))
	}
	return typeName, nil
}

func validateTemplateCoreTypeName(coreTypeName string) (string, error) {
	coreTypeName = strings.TrimSpace(coreTypeName)
	if coreTypeName == "" {
		return "", svcerr.New(codes.ErrInvalidInput, "core_type cannot be empty")
	}
	if !schema.IsBuiltinType(coreTypeName) {
		return "", svcerr.New(codes.ErrTypeNotFound, fmt.Sprintf("core type '%s' not found", coreTypeName)).WithSuggestion("Available core types: date, page, section")
	}
	if coreTypeName == "section" {
		return "", svcerr.New(codes.ErrInvalidInput, "core type 'section' does not support template configuration")
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
