//go:build windows

package objectsvc

import (
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

func moveFileNoReplace(sourcePath, destinationPath string) error {
	source, err := windows.UTF16PtrFromString(sourcePath)
	if err != nil {
		return err
	}
	destination, err := windows.UTF16PtrFromString(destinationPath)
	if err != nil {
		return err
	}
	if err := windows.MoveFile(source, destination); err != nil {
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
			return &os.PathError{Op: "rename", Path: destinationPath, Err: fs.ErrExist}
		}
		return err
	}
	return nil
}
