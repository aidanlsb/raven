package schemamigratesvc

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type FieldRenameConflict struct {
	FilePath      string `json:"file_path"`
	ConflictType  string `json:"conflict_type"`
	Message       string `json:"message"`
	Line          int    `json:"line,omitempty"`
	OldFieldFound bool   `json:"old_field_found,omitempty"`
	NewFieldFound bool   `json:"new_field_found,omitempty"`
}

type RenameFieldRequest struct {
	TypeName string
	OldField string
	NewField string
	Confirm  bool
}

type RenameFieldResult struct {
	Preview        bool
	TypeName       string
	OldField       string
	NewField       string
	TotalChanges   int
	Changes        []schemasvc.SchemaChange
	ChangesApplied int
	Hint           string
}

type RenameTypeRequest struct {
	OldName           string
	NewName           string
	Description       string
	Confirm           bool
	RenameDefaultPath bool
}

type RenameTypeResult struct {
	Preview                    bool
	OldName                    string
	NewName                    string
	TotalChanges               int
	Changes                    []schemasvc.SchemaChange
	Hint                       string
	DefaultPathRenameAvailable bool
	DefaultPathRenamed         bool
	DefaultPathOld             string
	DefaultPathNew             string
	OptionalTotalChanges       int
	OptionalChanges            []schemasvc.SchemaChange
	FilesToMove                int
	ChangesApplied             int
	FilesMoved                 int
	ReferenceFilesUpdated      int
}

type typeDirectoryMove struct {
	SourceRelPath      string
	DestinationRelPath string
	SourceID           string
	DestinationID      string
}

type typeDefaultPathRenamePlan struct {
	OldDefaultPath string
	NewDefaultPath string
	Moves          []typeDirectoryMove
}

type fieldRenamePlan struct {
	Changes       []schemasvc.SchemaChange
	SchemaYAML    []byte
	TemplateFiles map[string][]byte
	RavenYAML     []byte
	MarkdownFiles map[string][]byte
	Conflicts     []FieldRenameConflict
}

// typeRenamePlan combines the schema-only plan with every staged vault
// mutation. Preview and apply consume this same plan so their behavior cannot
// drift.
type typeRenamePlan struct {
	SchemaPlan      *schemasvc.TypeRenamePlan
	MarkdownFiles   map[string][]byte
	DefaultPathPlan *typeDefaultPathRenamePlan
	ReferenceFiles  map[string][]byte
	Changes         []schemasvc.SchemaChange
	OptionalChanges []schemasvc.SchemaChange
}

func RenameField(rt *vaultruntime.Runtime, req RenameFieldRequest) (*RenameFieldResult, error) {
	typeName := strings.TrimSpace(req.TypeName)
	oldField := strings.TrimSpace(req.OldField)
	newField := strings.TrimSpace(req.NewField)
	if typeName == "" || oldField == "" || newField == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "type and field names cannot be empty").WithSuggestion("Usage: rvn schema rename field <type> <old_field> <new_field>")
	}
	if oldField == newField {
		return nil, svcerr.New(codes.ErrInvalidInput, "old and new field names are the same")
	}
	if schema.IsBuiltinType(typeName) {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("cannot rename fields on built-in type '%s'", typeName))
	}

	schemaDoc, err := loadSchemaDocument(rt.VaultPath)
	if err != nil {
		return nil, err
	}
	sch := schemaDoc.Schema()
	typeDef, exists := sch.Types[typeName]
	if !exists {
		return nil, svcerr.New(codes.ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName))
	}
	if typeDef == nil || typeDef.Fields == nil {
		return nil, svcerr.New(codes.ErrFieldNotFound, fmt.Sprintf("type '%s' has no fields", typeName))
	}
	if _, ok := typeDef.Fields[oldField]; !ok {
		return nil, svcerr.New(codes.ErrFieldNotFound, fmt.Sprintf("field '%s' not found on type '%s'", oldField, typeName))
	}
	if _, ok := typeDef.Fields[newField]; ok {
		return nil, svcerr.New(codes.ErrObjectExists, fmt.Sprintf("field '%s' already exists on type '%s'", newField, typeName))
	}

	plan, err := buildFieldRenamePlan(rt.VaultPath, schemaDoc, typeName, oldField, newField, rt.VaultCfg)
	if err != nil {
		return nil, err
	}
	if len(plan.Conflicts) > 0 {
		return nil, svcerr.New(codes.ErrDataIntegrityBlock, fmt.Sprintf("field rename blocked by %d conflicts", len(plan.Conflicts))).WithSuggestion("Resolve conflicts (remove one of the duplicate keys) and retry").WithDetails(map[string]interface{}{
			"type":       typeName,
			"old_field":  oldField,
			"new_field":  newField,
			"conflicts":  plan.Conflicts,
			"hint":       "Conflicts occur when both old and new field keys are present in the same object/declaration.",
			"next_steps": "Fix conflicts, then re-run the command (preview first).",
		})
	}

	if !req.Confirm {
		return &RenameFieldResult{
			Preview:      true,
			TypeName:     typeName,
			OldField:     oldField,
			NewField:     newField,
			TotalChanges: len(plan.Changes),
			Changes:      plan.Changes,
			Hint:         "Run with --confirm to apply changes",
		}, nil
	}

	appliedChanges, err := applyFieldRenamePlan(rt, plan)
	if err != nil {
		return nil, err
	}
	return &RenameFieldResult{
		Preview:        false,
		TypeName:       typeName,
		OldField:       oldField,
		NewField:       newField,
		ChangesApplied: appliedChanges,
		Hint:           "Run 'rvn reindex --full' to update the index",
	}, nil
}

func RenameType(rt *vaultruntime.Runtime, req RenameTypeRequest) (*RenameTypeResult, error) {
	oldName := strings.TrimSpace(req.OldName)
	newName := strings.TrimSpace(req.NewName)
	if oldName == "" || newName == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "type names cannot be empty")
	}
	if oldName == newName {
		return nil, svcerr.New(codes.ErrInvalidInput, "old and new names are the same")
	}
	if schema.IsBuiltinType(oldName) {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("'%s' is a built-in type and cannot be renamed", oldName))
	}
	if schema.IsBuiltinType(newName) {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("cannot rename to '%s' - it's a built-in type", newName))
	}

	schemaDoc, err := loadSchemaDocument(rt.VaultPath)
	if err != nil {
		return nil, err
	}
	sch := schemaDoc.Schema()
	if rt.VaultCfg == nil {
		return nil, svcerr.New(codes.ErrConfigInvalid, "failed to load raven.yaml").WithSuggestion("Fix raven.yaml and try again")
	}
	vaultCfg := rt.VaultCfg
	oldTypeDef, exists := sch.Types[oldName]
	if !exists {
		return nil, svcerr.New(codes.ErrTypeNotFound, fmt.Sprintf("type '%s' not found", oldName))
	}
	if _, exists := sch.Types[newName]; exists {
		return nil, svcerr.New(codes.ErrObjectExists, fmt.Sprintf("type '%s' already exists", newName)).WithSuggestion("Choose a different name")
	}

	plan, err := buildTypeRenamePlan(rt.VaultPath, schemaDoc, req.Description, oldName, newName, oldTypeDef, vaultCfg)
	if err != nil {
		return nil, err
	}
	defaultPathPlan := plan.DefaultPathPlan

	result := &RenameTypeResult{
		OldName:      oldName,
		NewName:      newName,
		TotalChanges: len(plan.Changes),
		Changes:      plan.Changes,
		Hint:         "Run with --confirm to apply changes",
	}
	if defaultPathPlan != nil {
		result.DefaultPathRenameAvailable = true
		result.DefaultPathOld = defaultPathPlan.OldDefaultPath
		result.DefaultPathNew = defaultPathPlan.NewDefaultPath
		result.OptionalTotalChanges = len(plan.OptionalChanges)
		result.OptionalChanges = plan.OptionalChanges
		result.FilesToMove = len(defaultPathPlan.Moves)
		result.Hint = "Run with --confirm to apply changes. Add --rename-default-path to also rename the default directory and move matching files."
	}

	if !req.Confirm {
		result.Preview = true
		return result, nil
	}

	applyDefaultPathRename := defaultPathPlan != nil && req.RenameDefaultPath
	if applyDefaultPathRename {
		if err := validateTypeDirectoryMoves(rt.VaultPath, defaultPathPlan.Moves); err != nil {
			return nil, svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("cannot rename default directory: %v", err)).WithSuggestion("Use --confirm without --rename-default-path, or resolve destination conflicts and try again")
		}
	}

	appliedChanges, movedFiles, referenceFilesUpdated, err := applyTypeRenamePlan(rt, plan, applyDefaultPathRename)
	if err != nil {
		return nil, err
	}

	return &RenameTypeResult{
		Preview:                    false,
		OldName:                    oldName,
		NewName:                    newName,
		ChangesApplied:             appliedChanges,
		Hint:                       hintForTypeApply(defaultPathPlan != nil, applyDefaultPathRename),
		DefaultPathRenameAvailable: defaultPathPlan != nil,
		DefaultPathRenamed:         applyDefaultPathRename,
		DefaultPathOld:             defaultPathValue(defaultPathPlan, true),
		DefaultPathNew:             defaultPathValue(defaultPathPlan, false),
		FilesMoved:                 movedFiles,
		ReferenceFilesUpdated:      referenceFilesUpdated,
	}, nil
}
