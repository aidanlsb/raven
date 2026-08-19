package objectsvc

import (
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
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
	if err := vaultruntime.RequirePath(req.VaultPath); err != nil {
		return nil, nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if req.VaultConfig == nil {
		return nil, nil, svcerr.New(codes.ErrValidationFailed, "vault config is required").WithSuggestion("Fix raven.yaml and try again")
	}
	if strings.TrimSpace(req.Reference) == "" {
		return nil, nil, svcerr.New(codes.ErrInvalidInput, "reference or file path is required").WithSuggestion("Usage: rvn delete <reference-or-file-path>")
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
			return nil, nil, svcerr.New(codes.ErrInvalidInput, "delete only supports file-level objects").WithSuggestion("Use a file-level object ID without a section fragment")
		}
		target, err = deleteTargetFromFilePath(req.VaultPath, req.VaultConfig, resolved.FilePath, resolved.ObjectID)
	}
	if err != nil {
		return nil, nil, err
	}

	if err := req.Runtime.OpenDB(); err != nil {
		return nil, nil, svcerr.Wrap(codes.ErrDatabase, "failed to open index database", err).WithSuggestion("Run 'rvn reindex' to rebuild the database")
	}

	var backlinks []model.Reference
	if target.RavenObject {
		backlinks, err = req.Runtime.DB.Backlinks(target.ObjectID)
		if err != nil {
			return nil, nil, svcerr.Wrap(codes.ErrDatabase, "failed to read backlinks", err).WithSuggestion("Run 'rvn reindex' to rebuild the database")
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
