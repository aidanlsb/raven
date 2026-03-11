package objectsvc

import (
	"os"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

type SetObjectFileRequest struct {
	FilePath      string
	ObjectID      string
	Updates       map[string]string
	TypedUpdates  map[string]schema.FieldValue
	Schema        *schema.Schema
	AllowedFields map[string]bool
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

	mergedUpdates := make(map[string]string, len(req.Updates)+len(req.TypedUpdates))
	for key, value := range req.Updates {
		mergedUpdates[key] = value
	}
	for key, value := range req.TypedUpdates {
		mergedUpdates[key] = fieldmutation.SerializeFieldValueLiteral(value)
	}

	fieldNames := make([]string, 0, len(mergedUpdates))
	for key := range mergedUpdates {
		fieldNames = append(fieldNames, key)
	}
	if unknownErr := fieldmutation.DetectUnknownFieldMutationByNames(objectType, req.Schema, fieldNames, req.AllowedFields); unknownErr != nil {
		return nil, unknownErr
	}

	newContent, resolvedUpdates, warningMessages, err := fieldmutation.PrepareValidatedFrontmatterMutation(
		string(content),
		fm,
		objectType,
		mergedUpdates,
		req.Schema,
		req.AllowedFields,
	)
	if err != nil {
		return nil, err
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
