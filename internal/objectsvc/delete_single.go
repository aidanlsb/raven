package objectsvc

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/schema"
)

type DeleteByReferenceRequest struct {
	VaultPath   string
	VaultConfig *config.VaultConfig
	Schema      *schema.Schema
	Reference   string
	Behavior    string
	TrashDir    string
}

type DeleteByReferenceResult struct {
	ObjectID        string
	Behavior        string
	TrashPath       string
	Backlinks       []model.Reference
	WarningMessages []string
}

func PreviewDeleteByReference(req DeleteByReferenceRequest) (*DeleteByReferenceResult, error) {
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
		return nil, nil, newError(ErrorInvalidInput, "reference is required", "Usage: rvn delete <object-or-asset-id>", nil, nil)
	}

	resolved, err := resolveReferenceForMutation(req.VaultPath, req.VaultConfig, req.Schema, req.Reference)
	if err != nil {
		return nil, nil, err
	}
	if resolved.IsSection {
		return nil, nil, newError(ErrorInvalidInput, "delete only supports file-level objects and assets", "Use a file-level object or asset ID without a section fragment", nil, nil)
	}

	target, err := deleteTargetFromFilePath(req.VaultPath, req.VaultConfig, resolved.FilePath, resolved.ObjectID)
	if err != nil {
		return nil, nil, err
	}

	db, err := index.Open(req.VaultPath)
	if err != nil {
		return nil, nil, newError(ErrorDatabase, "failed to open index database", "Run 'rvn reindex' to rebuild the database", nil, err)
	}
	defer db.Close()
	db.SetDailyDirectory(req.VaultConfig.GetDailyDirectory())

	backlinks, err := db.Backlinks(target.ObjectID)
	if err != nil {
		return nil, nil, newError(ErrorDatabase, "failed to read backlinks", "Run 'rvn reindex' to rebuild the database", nil, err)
	}

	return &DeleteByReferenceResult{
		ObjectID:  target.ObjectID,
		Behavior:  req.Behavior,
		Backlinks: backlinks,
	}, target, nil
}

func DeleteByReference(req DeleteByReferenceRequest) (*DeleteByReferenceResult, error) {
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

	warnings := make([]string, 0)
	db, err := index.Open(req.VaultPath)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Failed to open index database while removing deleted object: %v", err))
	} else {
		defer db.Close()
		db.SetDailyDirectory(req.VaultConfig.GetDailyDirectory())
		removeErr := removeDeleteTargetFromIndex(db, target)
		if removeErr != nil {
			if errors.Is(removeErr, index.ErrObjectNotFound) {
				warnings = append(warnings, "Object not found in index; consider running 'rvn reindex'")
			} else {
				warnings = append(warnings, fmt.Sprintf("Failed to remove deleted file from index: %v", removeErr))
			}
		}
	}

	return &DeleteByReferenceResult{
		ObjectID:        preview.ObjectID,
		Behavior:        delResult.Behavior,
		TrashPath:       delResult.TrashPath,
		Backlinks:       preview.Backlinks,
		WarningMessages: warnings,
	}, nil
}
