//go:build !linux && !darwin && !windows

package objectsvc

func moveFileNoReplace(sourcePath, destinationPath string) error {
	return moveFileByCreateNoReplace(sourcePath, destinationPath)
}
