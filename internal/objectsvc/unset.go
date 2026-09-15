package objectsvc

import (
	"os"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type UnsetObjectFileRequest struct {
	FilePath string
	ObjectID string
	Fields   []string
}

type UnsetObjectFileResult struct {
	ObjectID       string
	ObjectType     string
	RemovedFields  map[string]fieldvalue.FieldValue
	MissingFields  []string
	Modified       bool
	PreviousFields map[string]fieldvalue.FieldValue
}

func UnsetObjectFile(rt *vaultruntime.Runtime, req UnsetObjectFileRequest) (*UnsetObjectFileResult, error) {
	if err := requireVaultConfig(rt); err != nil {
		return nil, err
	}
	if err := requireSchema(rt); err != nil {
		return nil, err
	}
	if err := mutationguard.ValidateContentMutationFilePath(rt.VaultPath, rt.VaultCfg, req.FilePath); err != nil {
		return nil, err
	}

	content, err := os.ReadFile(req.FilePath)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to read file", err)
	}

	fm, err := parser.ParseFrontmatter(string(content))
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "failed to parse frontmatter", err).WithSuggestion("Failed to parse frontmatter")
	}
	if fm == nil {
		return nil, svcerr.New(codes.ErrInvalidInput, "file has no frontmatter").WithSuggestion("The file must have YAML frontmatter (---) to unset fields")
	}

	objectType := fm.ObjectType
	if objectType == "" {
		objectType = "page"
	}

	newContent, removedFields, missingFields, err := fieldmutation.PrepareFrontmatterUnset(string(content), req.Fields, rt.Schema)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err)
	}

	modified := len(removedFields) > 0
	if modified {
		if err := atomicfile.WriteFile(req.FilePath, []byte(newContent), 0o644); err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to write file", err)
		}
	}

	previousFields := make(map[string]fieldvalue.FieldValue, len(fm.Fields))
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
