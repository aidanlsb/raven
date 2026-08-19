package objectsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type DeleteFileRequest struct {
	VaultPath string
	FilePath  string
	Behavior  string
	TrashDir  string
	Now       func() time.Time
}

type DeleteFileResult struct {
	Behavior  string
	TrashPath string
}

func DeleteFile(req DeleteFileRequest) (*DeleteFileResult, error) {
	if err := vaultruntime.RequirePath(req.VaultPath); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if strings.TrimSpace(req.FilePath) == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "file path is required")
	}

	behavior := strings.TrimSpace(req.Behavior)
	if behavior == "" {
		behavior = "trash"
	}

	switch behavior {
	case "trash":
		trashDir := strings.TrimSpace(req.TrashDir)
		if trashDir == "" {
			trashDir = ".trash"
		}

		trashRoot := filepath.Join(req.VaultPath, trashDir)
		if err := os.MkdirAll(trashRoot, 0o755); err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create trash directory", err)
		}

		relPath, err := filepath.Rel(req.VaultPath, req.FilePath)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrInvalidInput, "failed to compute relative path", err)
		}
		destPath := filepath.Join(trashRoot, relPath)

		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create trash parent directory", err)
		}

		if _, err := os.Stat(destPath); err == nil {
			nowFn := req.Now
			if nowFn == nil {
				nowFn = time.Now
			}
			timestamp := nowFn().Format("2006-01-02-150405")
			ext := filepath.Ext(destPath)
			base := strings.TrimSuffix(filepath.Base(destPath), ext)
			destPath = filepath.Join(filepath.Dir(destPath), fmt.Sprintf("%s-%s%s", base, timestamp, ext))
		}

		if err := os.Rename(req.FilePath, destPath); err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to move file to trash", err)
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
