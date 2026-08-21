//go:build darwin

package objectsvc

import "golang.org/x/sys/unix"

func moveFileNoReplace(sourcePath, destinationPath string) error {
	return unix.RenamexNp(sourcePath, destinationPath, unix.RENAME_EXCL)
}
