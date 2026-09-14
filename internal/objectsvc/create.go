package objectsvc

import (
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type CreateRequest struct {
	VaultPath   string
	TypeName    string
	Title       string
	TargetPath  string
	FieldValues map[string]fieldvalue.FieldValue
	VaultConfig *config.VaultConfig
	Schema      *schema.Schema
	ObjectsRoot string
	PagesRoot   string
	TemplateDir string
	TemplateID  string
	Runtime     *vaultruntime.Runtime
}

type CreateResult struct {
	FilePath     string
	RelativePath string
	ChangeSet    mutation.ChangeSet
}

// Create is the create-only path used by tests and callers that want Write's
// create mutation without replace semantics. It fails if the target exists.
func Create(req CreateRequest) (*CreateResult, error) {
	result, err := Write(WriteRequest{
		VaultPath:   req.VaultPath,
		TypeName:    req.TypeName,
		Title:       req.Title,
		TargetPath:  req.TargetPath,
		FieldValues: req.FieldValues,
		VaultConfig: req.VaultConfig,
		Schema:      req.Schema,
		ObjectsRoot: req.ObjectsRoot,
		PagesRoot:   req.PagesRoot,
		TemplateDir: req.TemplateDir,
		TemplateID:  req.TemplateID,
		CreateOnly:  true,
		Runtime:     req.Runtime,
	})
	if err != nil {
		return nil, err
	}
	return &CreateResult{
		FilePath:     result.FilePath,
		RelativePath: result.RelativePath,
		ChangeSet:    result.ChangeSet,
	}, nil
}
