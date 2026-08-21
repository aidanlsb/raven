package commandpayload

import "github.com/aidanlsb/raven/internal/schemasvc"

// SchemaValidateResult is the success payload for `schema validate`.
type SchemaValidateResult struct {
	Valid  bool     `json:"valid"`
	Issues []string `json:"issues"`
	Types  int      `json:"types"`
	Traits int      `json:"traits"`
}

// SchemaAddTypeResult is the success payload for `schema add type`.
type SchemaAddTypeResult struct {
	Added            string `json:"added"`
	Name             string `json:"name"`
	DefaultPath      string `json:"default_path"`
	Description      string `json:"description,omitempty"`
	NameField        string `json:"name_field,omitempty"`
	AutoCreatedField *bool  `json:"auto_created_field,omitempty"`
}

// SchemaAddTraitResult is the success payload for `schema add trait`.
type SchemaAddTraitResult struct {
	Added  string   `json:"added"`
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Values []string `json:"values,omitempty"`
}

// SchemaAddFieldResult is the success payload for `schema add field`.
type SchemaAddFieldResult struct {
	Added       string `json:"added"`
	Type        string `json:"type"`
	Field       string `json:"field"`
	FieldType   string `json:"field_type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

// SchemaUpdateResult is shared by type, trait, and field updates.
type SchemaUpdateResult struct {
	Updated string   `json:"updated"`
	Changes []string `json:"changes"`
	Name    string   `json:"name,omitempty"`
	Type    string   `json:"type,omitempty"`
	Field   string   `json:"field,omitempty"`
}

// SchemaRemoveResult is shared by type, trait, and field removals.
type SchemaRemoveResult struct {
	Removed string `json:"removed"`
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	Field   string `json:"field,omitempty"`
}

// SchemaRenameFieldPreviewResult is the preview payload for a field rename.
type SchemaRenameFieldPreviewResult struct {
	Preview      bool                          `json:"preview"`
	Type         string                        `json:"type"`
	OldField     string                        `json:"old_field"`
	NewField     string                        `json:"new_field"`
	TotalChanges int                           `json:"total_changes"`
	Changes      []schemasvc.FieldRenameChange `json:"changes"`
	Hint         string                        `json:"hint"`
}

// SchemaRenameFieldResult is the applied field-rename payload.
type SchemaRenameFieldResult struct {
	Renamed        bool   `json:"renamed"`
	Type           string `json:"type"`
	OldField       string `json:"old_field"`
	NewField       string `json:"new_field"`
	ChangesApplied int    `json:"changes_applied"`
	Hint           string `json:"hint"`
}

// SchemaRenameTypePreviewResult is the preview payload for a type rename.
// Optional default-path fields are pointers so an available zero/false value is
// still serialized while unavailable fields remain absent.
type SchemaRenameTypePreviewResult struct {
	Preview                    bool                          `json:"preview"`
	OldName                    string                        `json:"old_name"`
	NewName                    string                        `json:"new_name"`
	TotalChanges               int                           `json:"total_changes"`
	Changes                    []schemasvc.TypeRenameChange  `json:"changes"`
	Hint                       string                        `json:"hint"`
	DefaultPathRenameAvailable *bool                         `json:"default_path_rename_available,omitempty"`
	DefaultPathOld             *string                       `json:"default_path_old,omitempty"`
	DefaultPathNew             *string                       `json:"default_path_new,omitempty"`
	OptionalTotalChanges       *int                          `json:"optional_total_changes,omitempty"`
	OptionalChanges            *[]schemasvc.TypeRenameChange `json:"optional_changes,omitempty"`
	FilesToMove                *int                          `json:"files_to_move,omitempty"`
}

// SchemaRenameTypeResult is the applied type-rename payload.
type SchemaRenameTypeResult struct {
	Renamed                    bool    `json:"renamed"`
	OldName                    string  `json:"old_name"`
	NewName                    string  `json:"new_name"`
	ChangesApplied             int     `json:"changes_applied"`
	Hint                       string  `json:"hint"`
	DefaultPathRenameAvailable *bool   `json:"default_path_rename_available,omitempty"`
	DefaultPathRenamed         *bool   `json:"default_path_renamed,omitempty"`
	DefaultPathOld             *string `json:"default_path_old,omitempty"`
	DefaultPathNew             *string `json:"default_path_new,omitempty"`
	FilesMoved                 *int    `json:"files_moved,omitempty"`
	ReferenceFilesUpdated      *int    `json:"reference_files_updated,omitempty"`
}

// SchemaConvertPreviewResult is the preview payload for trait/field conversion.
type SchemaConvertPreviewResult struct {
	Kind         string                         `json:"kind"`
	Name         string                         `json:"name"`
	SourceType   string                         `json:"source_type"`
	TargetType   string                         `json:"target_type"`
	Hint         string                         `json:"hint"`
	Type         string                         `json:"type,omitempty"`
	Preview      bool                           `json:"preview"`
	TotalChanges int                            `json:"total_changes"`
	Changes      []schemasvc.ValueConvertChange `json:"changes"`
}

// SchemaConvertResult is the applied trait/field conversion payload.
type SchemaConvertResult struct {
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	SourceType     string `json:"source_type"`
	TargetType     string `json:"target_type"`
	Hint           string `json:"hint"`
	Type           string `json:"type,omitempty"`
	Converted      bool   `json:"converted"`
	ChangesApplied int    `json:"changes_applied"`
}

// SchemaTemplateDefinitionResult is shared by template get/set.
type SchemaTemplateDefinitionResult struct {
	ID          string `json:"id"`
	File        string `json:"file"`
	Description string `json:"description"`
}

// SchemaTemplateRemoveResult is the template-definition removal payload.
type SchemaTemplateRemoveResult struct {
	Removed bool   `json:"removed"`
	ID      string `json:"id"`
}

// SchemaTemplateBindResult is the template binding payload.
type SchemaTemplateBindResult struct {
	Type            string `json:"type,omitempty"`
	Core            string `json:"core,omitempty"`
	TemplateID      string `json:"template_id"`
	AlreadySet      bool   `json:"already_set,omitempty"`
	DefaultMatch    *bool  `json:"default_match,omitempty"`
	DefaultTemplate string `json:"default_template,omitempty"`
}

// SchemaTemplateUnbindResult is the template unbinding payload.
type SchemaTemplateUnbindResult struct {
	Type           string `json:"type,omitempty"`
	Core           string `json:"core,omitempty"`
	TemplateID     string `json:"template_id"`
	Removed        bool   `json:"removed"`
	DefaultCleared bool   `json:"default_cleared,omitempty"`
}

// TemplateWriteResult is the success payload for `template write`.
type TemplateWriteResult struct {
	Path        string `json:"path"`
	Status      string `json:"status"`
	TemplateDir string `json:"template_dir"`
}

// TemplateDeleteResult is the success payload for `template delete`.
type TemplateDeleteResult struct {
	Deleted     string   `json:"deleted"`
	TrashPath   string   `json:"trash_path"`
	Forced      bool     `json:"forced"`
	TemplateIDs []string `json:"template_ids"`
}
