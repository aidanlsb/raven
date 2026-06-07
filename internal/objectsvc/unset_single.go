package objectsvc

import (
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

type UnsetByReferenceRequest struct {
	VaultPath    string
	VaultConfig  *config.VaultConfig
	Schema       *schema.Schema
	Reference    string
	Fields       []string
	ParseOptions *parser.ParseOptions
}

type UnsetByReferenceResult struct {
	FilePath       string
	RelativePath   string
	ObjectID       string
	ObjectType     string
	RemovedFields  map[string]schema.FieldValue
	MissingFields  []string
	Modified       bool
	PreviousFields map[string]schema.FieldValue
}

func UnsetByReference(req UnsetByReferenceRequest) (*UnsetByReferenceResult, error) {
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
		return nil, newError(ErrorInvalidInput, "reference is required", "Usage: rvn unset <object-id> <field>...", nil, nil)
	}

	resolved, err := resolveReferenceForMutation(req.VaultPath, req.VaultConfig, req.Schema, req.Reference)
	if err != nil {
		return nil, err
	}
	if resolved.IsSection {
		return nil, newError(ErrorInvalidInput, "unset only supports file-level object frontmatter", "Use a file-level object ID without a section fragment", nil, nil)
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
	return &UnsetByReferenceResult{
		FilePath:       resolved.FilePath,
		RelativePath:   relPath,
		ObjectID:       resolved.ObjectID,
		ObjectType:     result.ObjectType,
		RemovedFields:  result.RemovedFields,
		MissingFields:  result.MissingFields,
		Modified:       result.Modified,
		PreviousFields: result.PreviousFields,
	}, nil
}
