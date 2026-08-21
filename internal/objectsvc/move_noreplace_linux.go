//go:build linux

package objectsvc

import "golang.org/x/sys/unix"

func moveFileNoReplace(sourcePath, destinationPath string) error {
	return unix.Renameat2(
		unix.AT_FDCWD,
		sourcePath,
		unix.AT_FDCWD,
		destinationPath,
		unix.RENAME_NOREPLACE,
	)
}
