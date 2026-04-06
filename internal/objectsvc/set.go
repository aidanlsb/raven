package objectsvc

import (
	"os"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

type SetObjectFileRequest struct {
	VaultPath     string
	VaultConfig   *config.VaultConfig
	FilePath      string
	ObjectID      string
	TypedUpdates  map[string]schema.FieldValue
	Schema        *schema.Schema
	AllowedFields map[string]bool
	ParseOptions  *parser.ParseOptions
}

type SetObjectFileResult struct {
	ObjectID        string
	ObjectType      string
	ResolvedUpdates map[string]string
	WarningMessages []string
	PreviousFields  map[string]schema.FieldValue
}

func SetObjectFile(req SetObjectFileRequest) (*SetObjectFileResult, error) {
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
		return nil, newError(ErrorInvalidInput, "file has no frontmatter", "The file must have YAML frontmatter (---) to set fields", nil, nil)
	}

	objectType := fm.ObjectType
	if objectType == "" {
		objectType = "page"
	}

	newContent, warningMessages, err := fieldmutation.PrepareValidatedFrontmatterMutationValues(
		string(content),
		fm,
		objectType,
		req.TypedUpdates,
		req.Schema,
		req.AllowedFields,
		&fieldmutation.RefValidationContext{
			VaultPath:    req.VaultPath,
			VaultConfig:  req.VaultConfig,
			ParseOptions: req.ParseOptions,
		},
	)
	if err != nil {
		return nil, err
	}

	updatedFM, err := parser.ParseFrontmatter(newContent)
	if err != nil {
		return nil, newError(ErrorInvalidInput, "failed to parse updated frontmatter", "", nil, err)
	}
	if updatedFM == nil {
		return nil, newError(ErrorInvalidInput, "file has no frontmatter after update", "", nil, nil)
	}

	resolvedUpdates := make(map[string]string, len(req.TypedUpdates))
	for key := range req.TypedUpdates {
		resolvedUpdates[key] = fieldmutation.SerializeFieldValueLiteral(updatedFM.Fields[key])
	}

	if err := atomicfile.WriteFile(req.FilePath, []byte(newContent), 0o644); err != nil {
		return nil, newError(ErrorFileWrite, "failed to write file", "", nil, err)
	}

	previousFields := make(map[string]schema.FieldValue, len(fm.Fields))
	for key, value := range fm.Fields {
		previousFields[key] = value
	}

	return &SetObjectFileResult{
		ObjectID:        req.ObjectID,
		ObjectType:      objectType,
		ResolvedUpdates: resolvedUpdates,
		WarningMessages: warningMessages,
		PreviousFields:  previousFields,
	}, nil
}
