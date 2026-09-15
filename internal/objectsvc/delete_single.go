package objectsvc

import (
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type DeleteByReferenceRequest struct {
	Reference string
	Behavior  string
	TrashDir  string
}

type DeleteByReferenceResult struct {
	ObjectID  string
	Behavior  string
	TrashPath string
	Backlinks []model.Reference
	ChangeSet mutation.ChangeSet
}

func PreviewDeleteByReference(rt *vaultruntime.Runtime, req DeleteByReferenceRequest) (*DeleteByReferenceResult, error) {
	if err := requireVaultConfig(rt); err != nil {
		return nil, err
	}
	result, _, err := prepareDeleteByReference(rt, req)
	return result, err
}

func prepareDeleteByReference(rt *vaultruntime.Runtime, req DeleteByReferenceRequest) (*DeleteByReferenceResult, *deleteTarget, error) {
	if strings.TrimSpace(req.Reference) == "" {
		return nil, nil, svcerr.New(codes.ErrInvalidInput, "reference or file path is required").WithSuggestion("Usage: rvn delete <reference-or-file-path>")
	}

	filePath, relPath, isFile, err := resolveLiteralNonMarkdownFileForMutation(rt, req.Reference)
	if err != nil {
		return nil, nil, err
	}
	var target *deleteTarget
	if isFile {
		target, err = deleteTargetFromFilePath(rt.VaultPath, rt.VaultCfg, filePath, relPath)
	} else {
		resolved, resolveErr := resolveReferenceForMutation(rt, req.Reference)
		if resolveErr != nil {
			return nil, nil, resolveErr
		}
		if resolved.IsSection {
			return nil, nil, svcerr.New(codes.ErrInvalidInput, "delete only supports file-level objects").
				WithSuggestion("Use 'rvn section delete <file#section>' to preview the section subtree, then add --confirm to delete it")
		}
		target, err = deleteTargetFromFilePath(rt.VaultPath, rt.VaultCfg, resolved.FilePath, resolved.ObjectID)
	}
	if err != nil {
		return nil, nil, err
	}

	if err := rt.OpenDB(); err != nil {
		return nil, nil, svcerr.Wrap(codes.ErrDatabase, "failed to open index database", err).WithSuggestion("Run 'rvn reindex' to rebuild the database")
	}

	var backlinks []model.Reference
	if target.RavenObject {
		backlinks, err = rt.DB.Backlinks(target.ObjectID)
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

func DeleteByReference(rt *vaultruntime.Runtime, req DeleteByReferenceRequest) (*DeleteByReferenceResult, error) {
	if err := requireVaultConfig(rt); err != nil {
		return nil, err
	}

	preview, target, err := prepareDeleteByReference(rt, req)
	if err != nil {
		return nil, err
	}

	delResult, err := DeleteFile(rt, DeleteFileRequest{
		FilePath: target.FilePath,
		Behavior: req.Behavior,
		TrashDir: req.TrashDir,
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
