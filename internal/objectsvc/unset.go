package objectsvc

import (
	"os"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

type UnsetObjectFileRequest struct {
	VaultPath    string
	VaultConfig  *config.VaultConfig
	FilePath     string
	ObjectID     string
	Fields       []string
	Schema       *schema.Schema
	ParseOptions *parser.ParseOptions
}

type UnsetObjectFileResult struct {
	ObjectID       string
	ObjectType     string
	RemovedFields  map[string]schema.FieldValue
	MissingFields  []string
	Modified       bool
	PreviousFields map[string]schema.FieldValue
}

func UnsetObjectFile(req UnsetObjectFileRequest) (*UnsetObjectFileResult, error) {
	if req.Schema == nil {
		return nil, newError(ErrorValidationFailed, "schema is required", "Fix schema.yaml and try again", nil, nil)
	}
	if err := ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, req.FilePath); err != nil {
		return nil, err
	}

	content, err := os.ReadFile(req.FilePath)
	if err != nil {
		return nil, newError(ErrorFileRead, "failed to read file", "", nil, err)
	}

	fm, err := parser.ParseFrontmatter(string(content))
	if err != nil {
		return nil, newError(ErrorInvalidInput, "failed to parse frontmatter", "Failed to parse frontmatter", nil, err)
	}
	if fm == nil {
		return nil, newError(ErrorInvalidInput, "file has no frontmatter", "The file must have YAML frontmatter (---) to unset fields", nil, nil)
	}

	objectType := fm.ObjectType
	if objectType == "" {
		objectType = "page"
	}

	newContent, removedFields, missingFields, err := fieldmutation.PrepareFrontmatterUnset(string(content), req.Fields, req.Schema)
	if err != nil {
		return nil, newError(ErrorInvalidInput, err.Error(), "", nil, err)
	}

	modified := len(removedFields) > 0
	if modified {
		if err := atomicfile.WriteFile(req.FilePath, []byte(newContent), 0o644); err != nil {
			return nil, newError(ErrorFileWrite, "failed to write file", "", nil, err)
		}
	}

	previousFields := make(map[string]schema.FieldValue, len(fm.Fields))
	for key, value := range fm.Fields {
		previousFields[key] = value
	}

	return &UnsetObjectFileResult{
		ObjectID:       req.ObjectID,
		ObjectType:     objectType,
		RemovedFields:  removedFields,
		MissingFields:  missingFields,
		Modified:       modified,
		PreviousFields: previousFields,
	}, nil
}
