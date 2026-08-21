//go:build !linux && !darwin

package objectsvc

import (
	"errors"
	"io/fs"
	"os"
)

func moveFileNoReplace(sourcePath, destinationPath string) error {
	if _, err := os.Lstat(destinationPath); err == nil {
		return &os.PathError{Op: "rename", Path: destinationPath, Err: fs.ErrExist}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(sourcePath, destinationPath)
}
