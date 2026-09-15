package objectsvc

import (
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
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
	FilePath       string
	RelativePath   string
	ObjectID       string
	ObjectType     string
	RemovedFields  map[string]fieldvalue.FieldValue
	MissingFields  []string
	Modified       bool
	PreviousFields map[string]fieldvalue.FieldValue
	ChangeSet      mutation.ChangeSet
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
		FilePath:       resolved.FilePath,
		RelativePath:   relPath,
		ObjectID:       resolved.ObjectID,
		ObjectType:     result.ObjectType,
		RemovedFields:  result.RemovedFields,
		MissingFields:  result.MissingFields,
		Modified:       result.Modified,
		PreviousFields: result.PreviousFields,
		ChangeSet:      changes,
	}, nil
}
