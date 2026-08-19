package objectsvc

import (
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type DeleteByReferenceRequest struct {
	VaultPath   string
	VaultConfig *config.VaultConfig
	Schema      *schema.Schema
	Reference   string
	Behavior    string
	TrashDir    string
	Runtime     *vaultruntime.Runtime
}

type DeleteByReferenceResult struct {
	ObjectID  string
	Behavior  string
	TrashPath string
	Backlinks []model.Reference
	ChangeSet mutation.ChangeSet
}

func PreviewDeleteByReference(req DeleteByReferenceRequest) (*DeleteByReferenceResult, error) {
	rt, owned := vaultruntime.FromRequest(req.Runtime, req.VaultPath, req.VaultConfig, req.Schema, nil)
	if owned {
		defer rt.Close()
	}
	req.Runtime = rt
	result, _, err := prepareDeleteByReference(req)
	return result, err
}

func prepareDeleteByReference(req DeleteByReferenceRequest) (*DeleteByReferenceResult, *deleteTarget, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, nil, newError(ErrorInvalidInput, "vault path is required", "", nil, nil)
	}
	if req.VaultConfig == nil {
		return nil, nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}
	if strings.TrimSpace(req.Reference) == "" {
		return nil, nil, newError(ErrorInvalidInput, "reference or file path is required", "Usage: rvn delete <reference-or-file-path>", nil, nil)
	}

	filePath, relPath, isFile, err := resolveLiteralNonMarkdownFileForMutation(req.Runtime, req.Reference)
	if err != nil {
		return nil, nil, err
	}
	var target *deleteTarget
	if isFile {
		target, err = deleteTargetFromFilePath(req.VaultPath, req.VaultConfig, filePath, relPath)
	} else {
		resolved, resolveErr := resolveReferenceForMutation(req.Runtime, req.Reference)
		if resolveErr != nil {
			return nil, nil, resolveErr
		}
		if resolved.IsSection {
			return nil, nil, newError(ErrorInvalidInput, "delete only supports file-level objects", "Use a file-level object ID without a section fragment", nil, nil)
		}
		target, err = deleteTargetFromFilePath(req.VaultPath, req.VaultConfig, resolved.FilePath, resolved.ObjectID)
	}
	if err != nil {
		return nil, nil, err
	}

	if err := req.Runtime.OpenDB(); err != nil {
		return nil, nil, newError(ErrorDatabase, "failed to open index database", "Run 'rvn reindex' to rebuild the database", nil, err)
	}

	var backlinks []model.Reference
	if target.RavenObject {
		backlinks, err = req.Runtime.DB.Backlinks(target.ObjectID)
		if err != nil {
			return nil, nil, newError(ErrorDatabase, "failed to read backlinks", "Run 'rvn reindex' to rebuild the database", nil, err)
		}
	}

	return &DeleteByReferenceResult{
		ObjectID:  target.ObjectID,
		Behavior:  req.Behavior,
		Backlinks: backlinks,
	}, target, nil
}

func DeleteByReference(req DeleteByReferenceRequest) (*DeleteByReferenceResult, error) {
	rt, owned := vaultruntime.FromRequest(req.Runtime, req.VaultPath, req.VaultConfig, req.Schema, nil)
	if owned {
		defer rt.Close()
	}
	req.Runtime = rt

	preview, target, err := prepareDeleteByReference(req)
	if err != nil {
		return nil, err
	}

	delResult, err := DeleteFile(DeleteFileRequest{
		VaultPath: req.VaultPath,
		FilePath:  target.FilePath,
		Behavior:  req.Behavior,
		TrashDir:  req.TrashDir,
	})
	if err != nil {
		return nil, err
	}

	changes := mutation.NewChangeSet()
	changes.AddDeleted(target.RelativePath)

	return &DeleteByReferenceResult{
		ObjectID:  preview.ObjectID,
		Behavior:  delResult.Behavior,
		TrashPath: delResult.TrashPath,
		Backlinks: preview.Backlinks,
		ChangeSet: changes,
	}, nil
}
