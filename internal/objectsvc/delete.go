package objectsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(ErrorInvalidInput, "vault path is required", "", nil, nil)
	}
	if strings.TrimSpace(req.FilePath) == "" {
		return nil, newError(ErrorInvalidInput, "file path is required", "", nil, nil)
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
			return nil, newError(ErrorFileWrite, "failed to create trash directory", "", nil, err)
		}

		relPath, err := filepath.Rel(req.VaultPath, req.FilePath)
		if err != nil {
			return nil, newError(ErrorInvalidInput, "failed to compute relative path", "", nil, err)
		}
		destPath := filepath.Join(trashRoot, relPath)

		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return nil, newError(ErrorFileWrite, "failed to create trash parent directory", "", nil, err)
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
			return nil, newError(ErrorFileWrite, "failed to move file to trash", "", nil, err)
		}

		return &DeleteFileResult{
			Behavior:  behavior,
			TrashPath: destPath,
		}, nil

	case "permanent":
		if err := os.Remove(req.FilePath); err != nil {
			return nil, newError(ErrorFileWrite, "failed to delete file", "", nil, err)
		}
		return &DeleteFileResult{Behavior: behavior}, nil

	default:
		return nil, newError(ErrorInvalidInput, fmt.Sprintf("invalid deletion behavior: %s", behavior), "Use 'trash' or 'permanent'", nil, nil)
	}
}
