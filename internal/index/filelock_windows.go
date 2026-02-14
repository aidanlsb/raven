//go:build windows

package index

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

const indexLockRegionSize uint32 = 1

func lockFileExclusiveNonBlocking(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		indexLockRegionSize,
		0,
		&overlapped,
	)
}

func unlockFile(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(
		windows.Handle(file.Fd()),
		0,
		indexLockRegionSize,
		0,
		&overlapped,
	)
}

func isWouldBlockError(err error) bool {
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
