package schemasvc

// TemplateCatalogResult is the success payload for `schema template list`
// without a type/core target.
type TemplateCatalogResult struct {
	Templates []TemplateDefinition `json:"templates"`
}

func TemplateCatalogPayload(items []TemplateDefinition) TemplateCatalogResult {
	if items == nil {
		items = []TemplateDefinition{}
	}
	return TemplateCatalogResult{Templates: items}
}

// TypeTemplateListResult is the success payload for `schema template list --type`.
type TypeTemplateListResult struct {
	DefaultTemplate string   `json:"default_template"`
	Templates       []string `json:"templates"`
	Type            string   `json:"type"`
}

// CoreTemplateListResult is the success payload for `schema template list --core`.
type CoreTemplateListResult struct {
	Core            string   `json:"core"`
	DefaultTemplate string   `json:"default_template"`
	Templates       []string `json:"templates"`
}

func (s *TemplateBindingState) TypePayload(typeName string) TypeTemplateListResult {
	templates := s.Templates
	if templates == nil {
		templates = []string{}
	}
	return TypeTemplateListResult{DefaultTemplate: s.DefaultTemplate, Templates: templates, Type: typeName}
}

func (s *TemplateBindingState) CorePayload(coreType string) CoreTemplateListResult {
	templates := s.Templates
	if templates == nil {
		templates = []string{}
	}
	return CoreTemplateListResult{Core: coreType, DefaultTemplate: s.DefaultTemplate, Templates: templates}
}

// TemplateDefinitionResult is shared by template get/set.
type TemplateDefinitionResult struct {
	ID          string `json:"id"`
	File        string `json:"file"`
	Description string `json:"description"`
}

// TemplateRemoveResult is the template-definition removal payload.
type TemplateRemoveResult struct {
	Removed bool   `json:"removed"`
	ID      string `json:"id"`
}

// TemplateBindResult is the template binding payload.
type TemplateBindResult struct {
	Type            string `json:"type,omitempty"`
	Core            string `json:"core,omitempty"`
	TemplateID      string `json:"template_id"`
	AlreadySet      bool   `json:"already_set,omitempty"`
	DefaultMatch    *bool  `json:"default_match,omitempty"`
	DefaultTemplate string `json:"default_template,omitempty"`
}

// TemplateUnbindResult is the template unbinding payload.
type TemplateUnbindResult struct {
	Type           string `json:"type,omitempty"`
	Core           string `json:"core,omitempty"`
	TemplateID     string `json:"template_id"`
	Removed        bool   `json:"removed"`
	DefaultCleared bool   `json:"default_cleared,omitempty"`
}
