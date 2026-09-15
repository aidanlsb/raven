package objectsvc

import (
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type CreateRequest struct {
	TypeName    string
	Title       string
	TargetPath  string
	FieldValues map[string]fieldvalue.FieldValue
	TemplateID  string
}

type CreateResult struct {
	FilePath     string
	RelativePath string
	ChangeSet    mutation.ChangeSet
}

// Create is the create-only path used by tests and callers that want Write's
// create mutation without replace semantics. It fails if the target exists.
func Create(rt *vaultruntime.Runtime, req CreateRequest) (*CreateResult, error) {
	result, err := Write(rt, WriteRequest{
		TypeName:    req.TypeName,
		Title:       req.Title,
		TargetPath:  req.TargetPath,
		FieldValues: req.FieldValues,
		TemplateID:  req.TemplateID,
		CreateOnly:  true,
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
