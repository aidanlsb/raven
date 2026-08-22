//go:build !darwin && !windows

package objectsvc

import (
	"errors"
	"fmt"
	"os"
)

func moveFileByCreateNoReplace(sourcePath, destinationPath string) error {
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return err
	}
	switch {
	case info.Mode().IsRegular():
		err = os.Link(sourcePath, destinationPath)
	case info.Mode()&os.ModeSymlink != 0:
		var target string
		target, err = os.Readlink(sourcePath)
		if err == nil {
			err = os.Symlink(target, destinationPath)
		}
	default:
		return fmt.Errorf("no-replace move is unsupported for file mode %s", info.Mode())
	}
	if err != nil {
		return err
	}
	if err := os.Remove(sourcePath); err != nil {
		removeErr := os.Remove(destinationPath)
		if removeErr != nil {
			return errors.Join(
				fmt.Errorf("remove source after creating destination: %w", err),
				fmt.Errorf("roll back destination: %w", removeErr),
			)
		}
		return err
	}
	return nil
}
