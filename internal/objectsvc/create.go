package objectsvc

import (
	"github.com/aidanlsb/raven/internal/check"
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
	ID    string `json:"id"`
	File  string `json:"file"`
	Title string `json:"title"`
	Type  string `json:"type"`
	check.MissingReferences
	FilePath     string             `json:"-"`
	RelativePath string             `json:"-"`
	ChangeSet    mutation.ChangeSet `json:"-"`
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
		ID:           result.ID,
		File:         result.File,
		Title:        result.Title,
		Type:         result.Type,
		FilePath:     result.FilePath,
		RelativePath: result.RelativePath,
		ChangeSet:    result.ChangeSet,
	}, nil
}
