package schemasvc

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
