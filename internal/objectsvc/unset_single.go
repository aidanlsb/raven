package objectsvc

import (
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type UnsetByReferenceRequest struct {
	VaultPath    string
	VaultConfig  *config.VaultConfig
	Schema       *schema.Schema
	Reference    string
	Fields       []string
	ParseOptions *parser.ParseOptions
	Runtime      *vaultruntime.Runtime
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

func UnsetByReference(req UnsetByReferenceRequest) (*UnsetByReferenceResult, error) {
	if err := vaultruntime.RequirePath(req.VaultPath); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if req.VaultConfig == nil {
		return nil, svcerr.New(codes.ErrValidationFailed, "vault config is required").WithSuggestion("Fix raven.yaml and try again")
	}
	if req.Schema == nil {
		return nil, svcerr.New(codes.ErrValidationFailed, "schema is required").WithSuggestion("Fix schema.yaml and try again")
	}
	if strings.TrimSpace(req.Reference) == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "reference is required").WithSuggestion("Usage: rvn unset <reference> <field>...")
	}
	rt, owned := vaultruntime.FromRequest(req.Runtime, req.VaultPath, req.VaultConfig, req.Schema, req.ParseOptions)
	if owned {
		defer rt.Close()
	}

	resolved, err := resolveReferenceForMutation(rt, req.Reference)
	if err != nil {
		return nil, err
	}
	if resolved.IsSection {
		return nil, svcerr.New(codes.ErrInvalidInput, "unset only supports file-level object frontmatter").WithSuggestion("Use a file-level object ID without a section fragment")
	}

	result, err := UnsetObjectFile(UnsetObjectFileRequest{
		VaultPath:    req.VaultPath,
		VaultConfig:  req.VaultConfig,
		FilePath:     resolved.FilePath,
		ObjectID:     resolved.ObjectID,
		Fields:       req.Fields,
		Schema:       req.Schema,
		ParseOptions: req.ParseOptions,
	})
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(req.VaultPath, resolved.FilePath)
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
