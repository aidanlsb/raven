package objectsvc

import (
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
)

type SetByReferenceRequest struct {
	VaultPath    string
	VaultConfig  *config.VaultConfig
	Schema       *schema.Schema
	Reference    string
	Updates      map[string]string
	TypedUpdates map[string]schema.FieldValue
	ParseOptions *parser.ParseOptions
}

type SetByReferenceResult struct {
	FilePath        string
	RelativePath    string
	ObjectID        string
	ObjectType      string
	Embedded        bool
	EmbeddedSlug    string
	ResolvedUpdates map[string]string
	WarningMessages []string
	PreviousFields  map[string]schema.FieldValue
}

func SetByReference(req SetByReferenceRequest) (*SetByReferenceResult, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(ErrorInvalidInput, "vault path is required", "", nil, nil)
	}
	if req.VaultConfig == nil {
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}
	if req.Schema == nil {
		return nil, newError(ErrorValidationFailed, "schema is required", "Fix schema.yaml and try again", nil, nil)
	}
	if strings.TrimSpace(req.Reference) == "" {
		return nil, newError(ErrorInvalidInput, "reference is required", "Usage: rvn set <object-id> field=value...", nil, nil)
	}

	resolved, err := resolveReferenceForMutation(req.VaultPath, req.VaultConfig, req.Schema, req.Reference)
	if err != nil {
		return nil, err
	}

	if resolved.IsSection {
		result, err := SetEmbeddedObject(SetEmbeddedObjectRequest{
			VaultPath:      req.VaultPath,
			FilePath:       resolved.FilePath,
			ObjectID:       resolved.ObjectID,
			Updates:        req.Updates,
			TypedUpdates:   req.TypedUpdates,
			Schema:         req.Schema,
			AllowedFields:  map[string]bool{"alias": true, "id": true},
			DocumentParser: req.ParseOptions,
		})
		if err != nil {
			return nil, err
		}
		relPath, _ := filepath.Rel(req.VaultPath, resolved.FilePath)
		_, slug, _ := paths.ParseEmbeddedID(resolved.ObjectID)
		return &SetByReferenceResult{
			FilePath:        resolved.FilePath,
			RelativePath:    relPath,
			ObjectID:        resolved.ObjectID,
			ObjectType:      result.ObjectType,
			Embedded:        true,
			EmbeddedSlug:    slug,
			ResolvedUpdates: result.ResolvedUpdates,
			WarningMessages: result.WarningMessages,
			PreviousFields:  result.PreviousFields,
		}, nil
	}

	result, err := SetObjectFile(SetObjectFileRequest{
		FilePath:      resolved.FilePath,
		ObjectID:      resolved.ObjectID,
		Updates:       req.Updates,
		TypedUpdates:  req.TypedUpdates,
		Schema:        req.Schema,
		AllowedFields: map[string]bool{"alias": true},
	})
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(req.VaultPath, resolved.FilePath)
	return &SetByReferenceResult{
		FilePath:        resolved.FilePath,
		RelativePath:    relPath,
		ObjectID:        resolved.ObjectID,
		ObjectType:      result.ObjectType,
		Embedded:        false,
		ResolvedUpdates: result.ResolvedUpdates,
		WarningMessages: result.WarningMessages,
		PreviousFields:  result.PreviousFields,
	}, nil
}
