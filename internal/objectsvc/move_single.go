package objectsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type MoveByReferenceRequest struct {
	VaultPath     string
	VaultConfig   *config.VaultConfig
	Schema        *schema.Schema
	Reference     string
	Destination   string
	UpdateRefs    bool
	SkipTypeCheck bool
	Preview       bool
	ParseOptions  *parser.ParseOptions
	Runtime       *vaultruntime.Runtime
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
	UpdatedRefFields  []RefFieldUpdate
	WarningMessages   []string
	NeedsConfirm      bool
	Reason            string
	TypeMismatch      *MoveTypeMismatch
	ResolvedDestInput string
	ChangeSet         mutation.ChangeSet
}

func MoveByReference(req MoveByReferenceRequest) (*MoveByReferenceResult, error) {
	if err := vaultruntime.RequirePath(req.VaultPath); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if req.VaultConfig == nil {
		return nil, svcerr.New(codes.ErrValidationFailed, "vault config is required").WithSuggestion("Fix raven.yaml and try again")
	}
	rt, owned := vaultruntime.FromRequest(req.Runtime, req.VaultPath, req.VaultConfig, req.Schema, req.ParseOptions)
	if owned {
		defer rt.Close()
	}
	req.Runtime = rt
	if strings.TrimSpace(req.Reference) == "" || strings.TrimSpace(req.Destination) == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "source and destination are required").WithSuggestion("Usage: rvn move <source> <destination>")
	}
	if _, _, isSection := paths.ParseSectionID(strings.TrimSpace(req.Reference)); isSection {
		return nil, svcerr.New(codes.ErrInvalidInput, "rvn move does not accept section sources").WithSuggestion(`Use 'rvn section move <file#section>' to reorder/reparent, or 'rvn section rename <file#section> "<new heading text>"' to change heading identity`).WithDetails(map[string]interface{}{"source": strings.TrimSpace(req.Reference)})
	}

	sourceFile, sourceRelPath, sourceIsFile, err := resolveLiteralNonMarkdownFileForMutation(rt, req.Reference)
	if err != nil {
		return nil, err
	}
	if !sourceIsFile {
		resolved, resolveErr := resolveReferenceForMutation(rt, req.Reference)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if resolved.IsSection {
			return nil, svcerr.New(codes.ErrInvalidInput, "rvn move does not accept section sources").WithSuggestion(`Use 'rvn section move <file#section>' to reorder/reparent, or 'rvn section rename <file#section> "<new heading text>"' to change heading identity`).WithDetails(map[string]interface{}{"source": resolved.ObjectID})
		}
		sourceFile = resolved.FilePath
	}

	if err := paths.ValidateWithinVault(req.VaultPath, sourceFile); err != nil {
		return nil, svcerr.Wrap(codes.ErrValidationFailed, "source path is outside vault", err).WithSuggestion("Files can only be moved within the vault")
	}

	if sourceRelPath == "" {
		sourceRelPath, err = filepath.Rel(req.VaultPath, sourceFile)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrInternal, "failed to resolve source path", err)
		}
		sourceRelPath = paths.NormalizeVaultRelPath(sourceRelPath)
	}
	if err := mutationguard.ValidateContentMutationRelPath(req.VaultConfig, sourceRelPath); err != nil {
		return nil, err
	}
	sourceID := req.VaultConfig.FilePathToObjectID(sourceRelPath)
	if sourceIsFile {
		sourceID = sourceRelPath
	}

	destination := req.Destination
	destinationIsDirectory := strings.HasSuffix(destination, "/") || strings.HasSuffix(destination, "\\")
	if destinationIsDirectory {
		sourceBase := filepath.Base(sourceRelPath)
		if !sourceIsFile {
			sourceBase = strings.TrimSuffix(sourceBase, ".md")
		}
		if strings.TrimSpace(sourceBase) == "" {
			return nil, svcerr.New(codes.ErrInvalidInput, "source has an invalid filename").WithSuggestion("Use an explicit destination file path")
		}
		destination = filepath.ToSlash(filepath.Join(destination, sourceBase))
	}

	if !sourceIsFile {
		destination = paths.EnsureMDExtension(destination)
	}
	destinationBase := filepath.Base(destination)
	if !sourceIsFile {
		destinationBase = strings.TrimSuffix(destinationBase, ".md")
	}
	if strings.TrimSpace(destinationBase) == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "destination has an empty filename").WithSuggestion("Use a non-empty destination filename or a directory ending with /")
	}
	if sourceIsFile && filepath.Ext(destinationBase) == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "file destination must include a file extension").WithSuggestion("Use a destination filename with an extension or a directory ending with /")
	}

	destPath := destination
	if !sourceIsFile && req.VaultConfig.HasDirectoriesConfig() {
		destPath = req.VaultConfig.ResolveReferenceToFilePath(strings.TrimSuffix(destination, ".md"))
	}
	destPath = paths.NormalizeVaultRelPath(destPath)
	destFile := filepath.Join(req.VaultPath, destPath)

	if err := paths.ValidateWithinVault(req.VaultPath, destFile); err != nil {
		return nil, svcerr.Wrap(codes.ErrValidationFailed, "destination path is outside vault", err).WithSuggestion("Files can only be moved within the vault")
	}
	relDest, _ := filepath.Rel(req.VaultPath, destFile)
	if err := mutationguard.ValidateContentMutationRelPath(req.VaultConfig, relDest); err != nil {
		return nil, err
	}
	if _, err := os.Stat(destFile); err == nil {
		return nil, svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("Destination '%s' already exists", destination)).WithSuggestion("Choose a different destination or delete the existing file first")
	}

	if sourceIsFile {
		serviceResult, err := MoveFile(MoveFileRequest{
			VaultPath:         req.VaultPath,
			SourceFile:        sourceFile,
			DestinationFile:   destFile,
			SourceObjectID:    sourceID,
			DestinationObject: destPath,
			UpdateRefs:        req.UpdateRefs,
			Preview:           req.Preview,
			VaultConfig:       req.VaultConfig,
			Schema:            req.Schema,
			ParseOptions:      req.ParseOptions,
			Runtime:           rt,
		})
		if err != nil {
			return nil, err
		}
		return &MoveByReferenceResult{
			SourceID:         sourceID,
			SourceRelative:   sourceRelPath,
			DestinationID:    destPath,
			DestinationRel:   destPath,
			UpdatedRefs:      serviceResult.UpdatedRefs,
			UpdatedRefFields: serviceResult.UpdatedRefFields,
			WarningMessages:  serviceResult.WarningMessages,
			ChangeSet:        serviceResult.ChangeSet,
		}, nil
	}

	sch := req.Schema
	if sch == nil {
		sch = schema.New()
	}

	content, err := os.ReadFile(sourceFile)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to read source file", err)
	}
	doc, err := parser.ParseDocumentWithOptions(string(content), sourceFile, req.VaultPath, req.ParseOptions)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrValidationFailed, "failed to parse source file", err).WithSuggestion("Failed to parse source file")
	}

	fileType := ""
	if len(doc.Objects) > 0 {
		fileType = doc.Objects[0].Type
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
		Preview:           req.Preview,
		VaultConfig:       req.VaultConfig,
		Schema:            sch,
		ParseOptions:      req.ParseOptions,
		Runtime:           rt,
	})
	if err != nil {
		return nil, err
	}

	return &MoveByReferenceResult{
		SourceID:         sourceID,
		SourceRelative:   sourceRelPath,
		DestinationID:    req.VaultConfig.FilePathToObjectID(destPath),
		DestinationRel:   destPath,
		UpdatedRefs:      serviceResult.UpdatedRefs,
		UpdatedRefFields: serviceResult.UpdatedRefFields,
		WarningMessages:  serviceResult.WarningMessages,
		ChangeSet:        serviceResult.ChangeSet,
	}, nil
}
