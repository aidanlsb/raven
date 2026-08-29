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
	// slugified into the filename/path. Use --object-path for explicit path control.
	return nil
}

func validateObjectPath(objectPath string) error {
	normalized := strings.TrimSpace(objectPath)
	if normalized == "" {
		return fmt.Errorf("object path cannot be empty")
	}
	normalized = strings.ReplaceAll(filepath.ToSlash(normalized), "\\", "/")
	if strings.HasSuffix(normalized, "/") {
		return fmt.Errorf("object path must include a filename, not just a directory")
	}

	base := strings.TrimSpace(paths.TrimMDExtension(filepath.Base(normalized)))
	if base == "" || base == "." || base == ".." {
		return fmt.Errorf("object path must include a valid filename")
	}

	// Reject vault file paths (type/doc/foo.md) instead of object paths (doc/foo)
	if strings.HasPrefix(normalized, "type/") {
		return fmt.Errorf("object path must not start with 'type/' - use the object ID from data.id, not data.path")
	}
	if strings.HasSuffix(normalized, ".md") {
		return fmt.Errorf("object path must not end with '.md' - use the object ID from data.id, not data.path")
	}

	return nil
}
