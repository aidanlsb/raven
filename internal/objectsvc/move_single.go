package objectsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
)

type MoveByReferenceRequest struct {
	VaultPath      string
	VaultConfig    *config.VaultConfig
	Schema         *schema.Schema
	Reference      string
	Destination    string
	UpdateRefs     bool
	SkipTypeCheck  bool
	ParseOptions   *parser.ParseOptions
	FailOnIndexErr bool
}

type MoveTypeMismatch struct {
	DestinationDir string
	ExpectedType   string
	ActualType     string
}

type MoveByReferenceResult struct {
	SourceID          string
	SourceRelative    string
	DestinationID     string
	DestinationRel    string
	UpdatedRefs       []string
	WarningMessages   []string
	NeedsConfirm      bool
	Reason            string
	TypeMismatch      *MoveTypeMismatch
	ResolvedDestInput string
}

func MoveByReference(req MoveByReferenceRequest) (*MoveByReferenceResult, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(ErrorInvalidInput, "vault path is required", "", nil, nil)
	}
	if req.VaultConfig == nil {
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}
	if strings.TrimSpace(req.Reference) == "" || strings.TrimSpace(req.Destination) == "" {
		return nil, newError(ErrorInvalidInput, "source and destination are required", "Usage: rvn move <source> <destination>", nil, nil)
	}

	resolved, err := resolveReferenceForMutation(req.VaultPath, req.VaultConfig, req.Schema, req.Reference)
	if err != nil {
		return nil, err
	}
	sourceFile := resolved.FilePath

	if err := paths.ValidateWithinVault(req.VaultPath, sourceFile); err != nil {
		return nil, newError(ErrorValidationFailed, "source path is outside vault", "Files can only be moved within the vault", nil, err)
	}

	sourceRelPath, err := filepath.Rel(req.VaultPath, sourceFile)
	if err != nil {
		return nil, newError(ErrorUnexpected, "failed to resolve source path", "", nil, err)
	}
	sourceID := req.VaultConfig.FilePathToObjectID(sourceRelPath)

	destination := req.Destination
	destinationIsDirectory := strings.HasSuffix(destination, "/") || strings.HasSuffix(destination, "\\")
	if destinationIsDirectory {
		sourceBase := strings.TrimSuffix(filepath.Base(sourceRelPath), ".md")
		if strings.TrimSpace(sourceBase) == "" {
			return nil, newError(ErrorInvalidInput, "source has an invalid filename", "Use an explicit destination file path", nil, nil)
		}
		destination = filepath.ToSlash(filepath.Join(destination, sourceBase))
	}

	destination = paths.EnsureMDExtension(destination)
	destinationBase := strings.TrimSuffix(filepath.Base(destination), ".md")
	if strings.TrimSpace(destinationBase) == "" {
		return nil, newError(ErrorInvalidInput, "destination has an empty filename", "Use a non-empty destination filename or a directory ending with /", nil, nil)
	}

	destPath := destination
	if req.VaultConfig.HasDirectoriesConfig() {
		destPath = req.VaultConfig.ResolveReferenceToFilePath(strings.TrimSuffix(destination, ".md"))
	}
	destFile := filepath.Join(req.VaultPath, destPath)

	if err := paths.ValidateWithinVault(req.VaultPath, destFile); err != nil {
		return nil, newError(ErrorValidationFailed, "destination path is outside vault", "Files can only be moved within the vault", nil, err)
	}
	relDest, _ := filepath.Rel(req.VaultPath, destFile)
	if strings.HasPrefix(relDest, ".raven") || strings.HasPrefix(relDest, ".trash") {
		return nil, newError(ErrorValidationFailed, "cannot move to system directory", "Destination cannot be in .raven or .trash directories", nil, nil)
	}
	if _, err := os.Stat(destFile); err == nil {
		return nil, newError(ErrorValidationFailed, fmt.Sprintf("Destination '%s' already exists", destination), "Choose a different destination or delete the existing file first", nil, nil)
	}

	sch := req.Schema
	if sch == nil {
		sch = schema.NewSchema()
	}

	content, err := os.ReadFile(sourceFile)
	if err != nil {
		return nil, newError(ErrorFileRead, "failed to read source file", "", nil, err)
	}
	doc, err := parser.ParseDocumentWithOptions(string(content), sourceFile, req.VaultPath, req.ParseOptions)
	if err != nil {
		return nil, newError(ErrorValidationFailed, "failed to parse source file", "Failed to parse source file", nil, err)
	}

	fileType := ""
	if len(doc.Objects) > 0 {
		fileType = doc.Objects[0].ObjectType
	}

	destDir := filepath.Dir(relDest)
	for typeName, typeDef := range sch.Types {
		if typeDef.DefaultPath == "" {
			continue
		}
		defaultPath := strings.TrimSuffix(typeDef.DefaultPath, "/")
		if destDir == defaultPath && typeName != fileType && !req.SkipTypeCheck {
			return &MoveByReferenceResult{
				SourceID:       sourceID,
				SourceRelative: sourceRelPath,
				DestinationID:  req.VaultConfig.FilePathToObjectID(destPath),
				DestinationRel: destPath,
				NeedsConfirm:   true,
				Reason:         fmt.Sprintf("Type mismatch: file is '%s' but destination is default path for '%s'", fileType, typeName),
				TypeMismatch: &MoveTypeMismatch{
					DestinationDir: destDir,
					ExpectedType:   typeName,
					ActualType:     fileType,
				},
			}, nil
		}
	}

	serviceResult, err := MoveFile(MoveFileRequest{
		VaultPath:         req.VaultPath,
		SourceFile:        sourceFile,
		DestinationFile:   destFile,
		SourceObjectID:    sourceID,
		DestinationObject: req.VaultConfig.FilePathToObjectID(destPath),
		UpdateRefs:        req.UpdateRefs,
		FailOnIndexError:  req.FailOnIndexErr,
		VaultConfig:       req.VaultConfig,
		Schema:            sch,
		ParseOptions:      req.ParseOptions,
	})
	if err != nil {
		return nil, err
	}

	return &MoveByReferenceResult{
		SourceID:        sourceID,
		SourceRelative:  sourceRelPath,
		DestinationID:   req.VaultConfig.FilePathToObjectID(destPath),
		DestinationRel:  destPath,
		UpdatedRefs:     serviceResult.UpdatedRefs,
		WarningMessages: serviceResult.WarningMessages,
	}, nil
}
