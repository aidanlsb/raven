//go:build linux

package objectsvc

import (
	"errors"

	"golang.org/x/sys/unix"
)

func moveFileNoReplace(sourcePath, destinationPath string) error {
	err := unix.Renameat2(
		unix.AT_FDCWD,
		sourcePath,
		unix.AT_FDCWD,
		destinationPath,
		unix.RENAME_NOREPLACE,
	)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
		return moveRegularFileByLinkNoReplace(sourcePath, destinationPath)
	}
	return err
}
