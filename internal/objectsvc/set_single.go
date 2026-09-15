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

type SetByReferenceRequest struct {
	Reference    string
	TypedUpdates map[string]fieldvalue.FieldValue
	// Preview validates and computes the resulting fields without writing the
	// file, for dry-run callers.
	Preview bool
}

type SetByReferenceResult struct {
	FilePath        string
	RelativePath    string
	ObjectID        string
	ObjectType      string
	ResolvedUpdates map[string]string
	WarningMessages []string
	PreviousFields  map[string]fieldvalue.FieldValue
	ChangeSet       mutation.ChangeSet
}

func SetByReference(rt *vaultruntime.Runtime, req SetByReferenceRequest) (*SetByReferenceResult, error) {
	if err := requireVaultConfig(rt); err != nil {
		return nil, err
	}
	if err := requireSchema(rt); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Reference) == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "reference is required").WithSuggestion("Usage: rvn set <reference> field=value...")
	}

	resolved, err := resolveReferenceForMutation(rt, req.Reference)
	if err != nil {
		return nil, err
	}

	if resolved.IsSection {
		return nil, svcerr.New(codes.ErrInvalidInput, "set only supports file-level object frontmatter").WithSuggestion("Use a file-level object ID without a section fragment")
	}

	result, err := SetObjectFile(rt, SetObjectFileRequest{
		FilePath:      resolved.FilePath,
		ObjectID:      resolved.ObjectID,
		TypedUpdates:  req.TypedUpdates,
		AllowedFields: map[string]bool{"alias": true},
		Preview:       req.Preview,
	})
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(rt.VaultPath, resolved.FilePath)
	relPath = filepath.ToSlash(relPath)
	changes := mutation.NewChangeSet()
	if !req.Preview {
		changes.AddChanged(relPath)
	}
	return &SetByReferenceResult{
		FilePath:        resolved.FilePath,
		RelativePath:    relPath,
		ObjectID:        resolved.ObjectID,
		ObjectType:      result.ObjectType,
		ResolvedUpdates: result.ResolvedUpdates,
		WarningMessages: result.WarningMessages,
		PreviousFields:  result.PreviousFields,
		ChangeSet:       changes,
	}, nil
}
