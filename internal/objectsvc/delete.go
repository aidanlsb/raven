package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type DeleteFileRequest struct {
	FilePath string
	Behavior string
	TrashDir string
	Now      func() time.Time
}

type DeleteFileResult struct {
	Behavior  string
	TrashPath string
}

func DeleteFile(rt *vaultruntime.Runtime, req DeleteFileRequest) (*DeleteFileResult, error) {
	if err := requireRuntime(rt); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.FilePath) == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "file path is required")
	}

	behavior, trashDir := normalizeDeletionBehavior(req.Behavior, req.TrashDir)

	switch behavior {
	case "trash":
		_, trashRoot, err := resolveTrashRootFromDir(rt.VaultPath, trashDir)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(trashRoot, 0o755); err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create trash directory", err)
		}
		if err := validateTrashRoot(rt.VaultPath, trashRoot); err != nil {
			return nil, err
		}

		relPath, err := filepath.Rel(rt.VaultPath, req.FilePath)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrInvalidInput, "failed to compute relative path", err)
		}
		relPath = filepath.Clean(relPath)
		if !paths.IsCleanRelSubpath(filepath.ToSlash(relPath)) {
			return nil, svcerr.New(codes.ErrFileOutsideVault, "file path is outside the vault")
		}
		if err := paths.ValidateWithinVault(rt.VaultPath, req.FilePath); err != nil {
			return nil, svcerr.Wrap(codes.ErrFileOutsideVault, "file path is outside the vault", err)
		}
		if err := ensureTrashParent(trashRoot, relPath); err != nil {
			return nil, err
		}
		destPath := filepath.Join(trashRoot, relPath)
		if err := paths.ValidateWithinVault(trashRoot, destPath); err != nil {
			return nil, svcerr.Wrap(codes.ErrFileOutsideVault, "trash destination is outside the vault", err)
		}

		if err := moveFileNoReplace(req.FilePath, destPath); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to move file to trash", err)
			}
			nowFn := req.Now
			if nowFn == nil {
				nowFn = time.Now
			}
			timestamp := nowFn().Format("2006-01-02-150405")
			ext := filepath.Ext(destPath)
			base := strings.TrimSuffix(filepath.Base(destPath), ext)
			tag := trashCollisionTag(filepath.ToSlash(relPath))
			for version := 1; ; version++ {
				candidate := filepath.Join(filepath.Dir(destPath), fmt.Sprintf("%s.raven-trash-%s-%s-%d%s", base, tag, timestamp, version, ext))
				candidateRel, relErr := filepath.Rel(trashRoot, candidate)
				if relErr != nil {
					return nil, svcerr.Wrap(codes.ErrInternal, "failed to resolve trash collision path", relErr)
				}
				metadataPath, metadataErr := createTrashMetadataNoReplace(trashRoot, filepath.ToSlash(candidateRel), filepath.ToSlash(relPath))
				if metadataErr != nil {
					if errors.Is(metadataErr, os.ErrExist) {
						continue
					}
					return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to record trash restore path", metadataErr)
				}
				if err := moveFileNoReplace(req.FilePath, candidate); err == nil {
					destPath = candidate
					break
				} else {
					_ = os.Remove(metadataPath)
					if !errors.Is(err, os.ErrExist) {
						return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to move file to trash", err)
					}
				}
			}
		}

		return &DeleteFileResult{
			Behavior:  behavior,
			TrashPath: destPath,
		}, nil

	case "permanent":
		if err := os.Remove(req.FilePath); err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to delete file", err)
		}
		return &DeleteFileResult{Behavior: behavior}, nil

	default:
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid deletion behavior: %s", behavior)).WithSuggestion("Use 'trash' or 'permanent'")
	}
}

func normalizeDeletionBehavior(behavior, trashDir string) (string, string) {
	behavior = strings.TrimSpace(behavior)
	if behavior == "" {
		behavior = "trash"
	}
	trashDir = strings.TrimSpace(trashDir)
	if trashDir == "" {
		trashDir = ".trash"
	}
	return behavior, trashDir
}
