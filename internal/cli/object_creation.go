package cli

import (
	"fmt"
	"path/filepath"
	"strings"
)

func validateObjectTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("title cannot be empty")
	}
	if strings.ContainsAny(title, "/\\") {
		return fmt.Errorf("title cannot contain path separators")
	}
	return nil
}

func validateObjectTargetPath(targetPath string) error {
	normalized := strings.TrimSpace(targetPath)
	if normalized == "" {
		return fmt.Errorf("path cannot be empty")
	}
	normalized = strings.ReplaceAll(filepath.ToSlash(normalized), "\\", "/")
	if strings.HasSuffix(normalized, "/") {
		return fmt.Errorf("path must include a filename, not just a directory")
	}

	base := strings.TrimSuffix(filepath.Base(normalized), ".md")
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == ".." {
		return fmt.Errorf("path must include a valid filename")
	}

	return nil
}
