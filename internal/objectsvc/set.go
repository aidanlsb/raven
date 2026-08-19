package objectsvc

import (
	"os"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type SetObjectFileRequest struct {
	VaultPath     string
	VaultConfig   *config.VaultConfig
	FilePath      string
	ObjectID      string
	TypedUpdates  map[string]fieldvalue.FieldValue
	Schema        *schema.Schema
	AllowedFields map[string]bool
	ParseOptions  *parser.ParseOptions
	Runtime       *vaultruntime.Runtime
	// Preview validates and computes the resulting fields without writing the
	// file, for dry-run callers.
	Preview bool
}

type SetObjectFileResult struct {
	ObjectID        string
	ObjectType      string
	ResolvedUpdates map[string]string
	WarningMessages []string
	PreviousFields  map[string]fieldvalue.FieldValue
}

func SetObjectFile(req SetObjectFileRequest) (*SetObjectFileResult, error) {
	if req.Schema == nil {
		return nil, svcerr.New(codes.ErrValidationFailed, "schema is required").WithSuggestion("Fix schema.yaml and try again")
	}
	if err := mutationguard.ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, req.FilePath); err != nil {
		return nil, err
	}
	rt, owned := vaultruntime.FromRequest(req.Runtime, req.VaultPath, req.VaultConfig, req.Schema, req.ParseOptions)
	if owned {
		defer rt.Close()
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
		return nil, svcerr.New(codes.ErrInvalidInput, "file has no frontmatter").WithSuggestion("The file must have YAML frontmatter (---) to set fields")
	}

	objectType := fm.ObjectType
	if objectType == "" {
		objectType = "page"
	}

	refCtx := createRefValidationContext(rt, req.ParseOptions)
	newContent, warningMessages, err := fieldmutation.PrepareValidatedFrontmatterMutationValues(
		string(content),
		fm,
		objectType,
		req.TypedUpdates,
		req.Schema,
		req.AllowedFields,
		refCtx,
	)
	if err != nil {
		return nil, err
	}

	updatedFM, err := parser.ParseFrontmatter(newContent)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "failed to parse updated frontmatter", err)
	}
	if updatedFM == nil {
		return nil, svcerr.New(codes.ErrInvalidInput, "file has no frontmatter after update")
	}

	resolvedUpdates := make(map[string]string, len(req.TypedUpdates))
	for key := range req.TypedUpdates {
		resolvedUpdates[key] = fieldmutation.SerializeFieldValueLiteral(updatedFM.Fields[key])
	}

	if !req.Preview {
		if err := atomicfile.WriteFile(req.FilePath, []byte(newContent), 0o644); err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to write file", err)
		}
	}

	previousFields := make(map[string]fieldvalue.FieldValue, len(fm.Fields))
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
