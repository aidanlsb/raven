package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/paths"
)

func validateObjectTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("title cannot be empty")
	}
	// Titles are display names: path separators are allowed here and are
	// slugified into the filename/path. Use --path for explicit path control.
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

	base := strings.TrimSpace(paths.TrimMDExtension(filepath.Base(normalized)))
	if base == "" || base == "." || base == ".." {
		return fmt.Errorf("path must include a valid filename")
	}

	return nil
}
