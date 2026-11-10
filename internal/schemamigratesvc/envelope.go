package schemamigratesvc

import "github.com/aidanlsb/raven/internal/schemasvc"

// RenameFieldPreviewResult is the preview payload for a field rename.
type RenameFieldPreviewResult struct {
	Preview      bool                     `json:"preview"`
	Type         string                   `json:"type"`
	OldField     string                   `json:"old_field"`
	NewField     string                   `json:"new_field"`
	TotalChanges int                      `json:"total_changes"`
	Changes      []schemasvc.SchemaChange `json:"changes"`
	Hint         string                   `json:"hint"`
}

// RenameFieldAppliedResult is the applied field-rename payload.
type RenameFieldAppliedResult struct {
	Renamed        bool   `json:"renamed"`
	Type           string `json:"type"`
	OldField       string `json:"old_field"`
	NewField       string `json:"new_field"`
	ChangesApplied int    `json:"changes_applied"`
	Hint           string `json:"hint"`
}

// RenameTypePreviewResult is the preview payload for a type rename.
type RenameTypePreviewResult struct {
	Preview                    bool                      `json:"preview"`
	OldName                    string                    `json:"old_name"`
	NewName                    string                    `json:"new_name"`
	TotalChanges               int                       `json:"total_changes"`
	Changes                    []schemasvc.SchemaChange  `json:"changes"`
	Hint                       string                    `json:"hint"`
	DefaultPathRenameAvailable *bool                     `json:"default_path_rename_available,omitempty"`
	DefaultPathOld             *string                   `json:"default_path_old,omitempty"`
	DefaultPathNew             *string                   `json:"default_path_new,omitempty"`
	OptionalTotalChanges       *int                      `json:"optional_total_changes,omitempty"`
	OptionalChanges            *[]schemasvc.SchemaChange `json:"optional_changes,omitempty"`
	FilesToMove                *int                      `json:"files_to_move,omitempty"`
}

// RenameTypeAppliedResult is the applied type-rename payload.
type RenameTypeAppliedResult struct {
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

// ConvertPreviewResult is the preview payload for trait/field conversion.
type ConvertPreviewResult struct {
	Kind         string                   `json:"kind"`
	Name         string                   `json:"name"`
	SourceType   string                   `json:"source_type"`
	TargetType   string                   `json:"target_type"`
	Hint         string                   `json:"hint"`
	Type         string                   `json:"type,omitempty"`
	Preview      bool                     `json:"preview"`
	TotalChanges int                      `json:"total_changes"`
	Changes      []schemasvc.SchemaChange `json:"changes"`
}

// ConvertAppliedResult is the applied trait/field conversion payload.
type ConvertAppliedResult struct {
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	SourceType     string `json:"source_type"`
	TargetType     string `json:"target_type"`
	Hint           string `json:"hint"`
	Type           string `json:"type,omitempty"`
	Converted      bool   `json:"converted"`
	ChangesApplied int    `json:"changes_applied"`
}

func (r *ConvertResult) Payload() any {
	if r.Preview {
		return ConvertPreviewResult{
			Kind: r.Kind, Name: r.Name, Type: r.TypeName, SourceType: r.SourceType,
			TargetType: r.TargetType, Hint: r.Hint, Preview: true,
			TotalChanges: r.TotalChanges, Changes: r.Changes,
		}
	}
	return ConvertAppliedResult{
		Kind: r.Kind, Name: r.Name, Type: r.TypeName, SourceType: r.SourceType,
		TargetType: r.TargetType, Hint: r.Hint, Converted: true, ChangesApplied: r.ChangesApplied,
	}
}

func (r *RenameFieldResult) Payload() any {
	if r.Preview {
		return RenameFieldPreviewResult{
			Preview: true, Type: r.TypeName, OldField: r.OldField, NewField: r.NewField,
			TotalChanges: r.TotalChanges, Changes: r.Changes, Hint: r.Hint,
		}
	}
	return RenameFieldAppliedResult{
		Renamed: true, Type: r.TypeName, OldField: r.OldField, NewField: r.NewField,
		ChangesApplied: r.ChangesApplied, Hint: r.Hint,
	}
}

func (r *RenameTypeResult) Payload() any {
	if r.Preview {
		data := RenameTypePreviewResult{
			Preview: true, OldName: r.OldName, NewName: r.NewName,
			TotalChanges: r.TotalChanges, Changes: r.Changes, Hint: r.Hint,
		}
		if r.DefaultPathRenameAvailable {
			data.DefaultPathRenameAvailable = &r.DefaultPathRenameAvailable
			data.DefaultPathOld = &r.DefaultPathOld
			data.DefaultPathNew = &r.DefaultPathNew
			data.OptionalTotalChanges = &r.OptionalTotalChanges
			data.OptionalChanges = &r.OptionalChanges
			data.FilesToMove = &r.FilesToMove
		}
		return data
	}
	data := RenameTypeAppliedResult{
		Renamed: true, OldName: r.OldName, NewName: r.NewName,
		ChangesApplied: r.ChangesApplied, Hint: r.Hint,
	}
	if r.DefaultPathRenameAvailable {
		data.DefaultPathRenameAvailable = &r.DefaultPathRenameAvailable
		data.DefaultPathRenamed = &r.DefaultPathRenamed
		data.DefaultPathOld = &r.DefaultPathOld
		data.DefaultPathNew = &r.DefaultPathNew
		data.FilesMoved = &r.FilesMoved
		data.ReferenceFilesUpdated = &r.ReferenceFilesUpdated
	}
	return data
}
