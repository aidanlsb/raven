package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/refresolve"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type AddRequest struct {
	Text        string
	ToReference string
}

type AddResult struct {
	File      string
	Line      int
	ChangeSet mutation.ChangeSet
}

func Add(rt *vaultruntime.Runtime, req AddRequest) (*AddResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault runtime is required", err)
	}
	vaultPath := rt.VaultPath
	vaultCfg := rt.VaultCfg
	captureCfg := vaultCfg.GetCaptureConfig()
	var destPath string
	var isDailyNote bool
	var targetObjectID string

	if strings.TrimSpace(req.ToReference) != "" {
		resolved, err := refresolve.ResolveDynamic(req.ToReference, rt, true)
		if err != nil {
			return nil, refresolve.NormalizeServiceError(err, req.ToReference)
		}
		destPath = resolved.FilePath
		targetObjectID = resolved.ObjectID
		isDailyNote = isDailyNoteObjectID(resolved.FileObjectID, vaultCfg)
	} else if captureCfg.Destination == "daily" {
		today := time.Now()
		dateStr := fmt.Sprintf("%04d-%02d-%02d", today.Year(), today.Month(), today.Day())
		destPath = vaultCfg.DailyNotePath(vaultPath, dateStr)
		isDailyNote = true
	} else {
		destPath = filepath.Join(vaultPath, captureCfg.Destination)
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			return nil, svcerr.New(codes.ErrFileNotFound, fmt.Sprintf("Configured capture destination '%s' does not exist", captureCfg.Destination)).
				WithSuggestion("Create the file first or change capture.destination in raven.yaml")
		}
	}

	if err := paths.ValidateWithinVault(vaultPath, destPath); err != nil {
		if errors.Is(err, paths.ErrPathOutsideVault) {
			return nil, svcerr.Wrap(codes.ErrFileOutsideVault, fmt.Sprintf("cannot capture outside vault: %s", destPath), err)
		}
		return nil, svcerr.Wrap(codes.ErrInternal, err.Error(), err)
	}
	if err := mutationguard.ValidateContentMutationFilePath(vaultPath, vaultCfg, destPath); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, err.Error(), err)
	}
	appendResult, err := Append(rt, destPath, req.Text, captureCfg, isDailyNote, targetObjectID)
	if err != nil {
		return nil, err
	}
	relPath, _ := filepath.Rel(vaultPath, destPath)
	return &AddResult{
		File: filepath.ToSlash(relPath), Line: appendResult.Line, ChangeSet: appendResult.ChangeSet,
	}, nil
}

func isDailyNoteObjectID(objectID string, vaultCfg *config.VaultConfig) bool {
	if objectID == "" {
		return false
	}
	baseID := objectID
	if parts := strings.SplitN(objectID, "#", 2); len(parts) == 2 {
		baseID = parts[0]
	}
	if dates.IsValidDate(baseID) {
		return true
	}
	dailyDir := "daily"
	if vaultCfg != nil && vaultCfg.GetDailyDirectory() != "" {
		dailyDir = vaultCfg.GetDailyDirectory()
	}
	if !strings.HasPrefix(baseID, dailyDir+"/") {
		return false
	}
	return dates.IsValidDate(strings.TrimPrefix(baseID, dailyDir+"/"))
}
