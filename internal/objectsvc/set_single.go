package objectsvc

import (
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
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
	File           string                           `json:"file"`
	ObjectID       string                           `json:"object_id"`
	Type           string                           `json:"type"`
	UpdatedFields  map[string]string                `json:"updated_fields"`
	PreviousFields map[string]fieldvalue.FieldValue `json:"previous_fields,omitempty"`
	Preview        bool                             `json:"preview,omitempty"`
	check.MissingReferences
	FilePath        string             `json:"-"`
	RelativePath    string             `json:"-"`
	ObjectType      string             `json:"-"`
	ResolvedUpdates map[string]string  `json:"-"`
	WarningMessages []string           `json:"-"`
	ChangeSet       mutation.ChangeSet `json:"-"`
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
		File:            relPath,
		ObjectID:        resolved.ObjectID,
		Type:            result.ObjectType,
		UpdatedFields:   result.ResolvedUpdates,
		PreviousFields:  result.PreviousFields,
		Preview:         req.Preview,
		FilePath:        resolved.FilePath,
		RelativePath:    relPath,
		ObjectType:      result.ObjectType,
		ResolvedUpdates: result.ResolvedUpdates,
		WarningMessages: result.WarningMessages,
		ChangeSet:       changes,
	}, nil
}
