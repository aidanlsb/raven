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
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(ErrorInvalidInput, "vault path is required", "", nil, nil)
	}
	if req.VaultConfig == nil {
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}
	if strings.TrimSpace(req.Reference) == "" {
		return nil, newError(ErrorInvalidInput, "reference is required", "Usage: rvn delete <object-id>", nil, nil)
	}

	resolved, err := resolveReferenceForMutation(req.VaultPath, req.VaultConfig, req.Schema, req.Reference)
	if err != nil {
		return nil, err
	}

	db, err := index.Open(req.VaultPath)
	if err != nil {
		return nil, newError(ErrorDatabase, "failed to open index database", "Run 'rvn reindex' to rebuild the database", nil, err)
	}
	defer db.Close()
	db.SetDailyDirectory(req.VaultConfig.GetDailyDirectory())

	backlinks, err := db.Backlinks(resolved.ObjectID)
	if err != nil {
		return nil, newError(ErrorDatabase, "failed to read backlinks", "Run 'rvn reindex' to rebuild the database", nil, err)
	}

	return &DeleteByReferenceResult{
		ObjectID:  resolved.ObjectID,
		Behavior:  req.Behavior,
		Backlinks: backlinks,
	}, nil
}

func DeleteByReference(req DeleteByReferenceRequest) (*DeleteByReferenceResult, error) {
	preview, err := PreviewDeleteByReference(req)
	if err != nil {
		return nil, err
	}

	resolved, err := resolveReferenceForMutation(req.VaultPath, req.VaultConfig, req.Schema, req.Reference)
	if err != nil {
		return nil, err
	}

	delResult, err := DeleteFile(DeleteFileRequest{
		VaultPath: req.VaultPath,
		FilePath:  resolved.FilePath,
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
		if err := db.RemoveDocument(preview.ObjectID); err != nil {
			if errors.Is(err, index.ErrObjectNotFound) {
				warnings = append(warnings, "Object not found in index; consider running 'rvn reindex'")
			} else {
				warnings = append(warnings, fmt.Sprintf("Failed to remove deleted object from index: %v", err))
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
