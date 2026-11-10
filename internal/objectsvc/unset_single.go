package objectsvc

import (
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type UnsetByReferenceRequest struct {
	Reference string
	Fields    []string
}

type UnsetByReferenceResult struct {
	File           string                           `json:"file"`
	ObjectID       string                           `json:"object_id"`
	Type           string                           `json:"type"`
	RemovedFields  map[string]string                `json:"removed_fields"`
	MissingFields  []string                         `json:"missing_fields"`
	Modified       bool                             `json:"modified"`
	PreviousFields map[string]fieldvalue.FieldValue `json:"previous_fields"`
	check.MissingReferences
	FilePath         string                           `json:"-"`
	RelativePath     string                           `json:"-"`
	ObjectType       string                           `json:"-"`
	RemovedFieldVals map[string]fieldvalue.FieldValue `json:"-"`
	ChangeSet        mutation.ChangeSet               `json:"-"`
}

func UnsetByReference(rt *vaultruntime.Runtime, req UnsetByReferenceRequest) (*UnsetByReferenceResult, error) {
	if err := requireVaultConfig(rt); err != nil {
		return nil, err
	}
	if err := requireSchema(rt); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Reference) == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "reference is required").WithSuggestion("Usage: rvn unset <reference> <field>...")
	}

	resolved, err := resolveReferenceForMutation(rt, req.Reference)
	if err != nil {
		return nil, err
	}
	if resolved.IsSection {
		return nil, svcerr.New(codes.ErrInvalidInput, "unset only supports file-level object frontmatter").WithSuggestion("Use a file-level object ID without a section fragment")
	}

	result, err := UnsetObjectFile(rt, UnsetObjectFileRequest{
		FilePath: resolved.FilePath,
		ObjectID: resolved.ObjectID,
		Fields:   req.Fields,
	})
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(rt.VaultPath, resolved.FilePath)
	relPath = filepath.ToSlash(relPath)
	changes := mutation.NewChangeSet()
	if result.Modified {
		changes.AddChanged(relPath)
	}
	return &UnsetByReferenceResult{
		File:             relPath,
		ObjectID:         resolved.ObjectID,
		Type:             result.ObjectType,
		RemovedFields:    fieldmutation.SerializeFieldValueMap(result.RemovedFields),
		MissingFields:    result.MissingFields,
		Modified:         result.Modified,
		PreviousFields:   result.PreviousFields,
		FilePath:         resolved.FilePath,
		RelativePath:     relPath,
		ObjectType:       result.ObjectType,
		RemovedFieldVals: result.RemovedFields,
		ChangeSet:        changes,
	}, nil
}
