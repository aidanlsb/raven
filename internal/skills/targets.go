package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveInstallRoot resolves the Agent Skills installation root for a scope.
// If destOverride is provided, it takes precedence.
func ResolveInstallRoot(scope Scope, destOverride, cwd string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
	}

	if strings.TrimSpace(destOverride) != "" {
		return normalizeInstallRoot(destOverride, cwd)
	}

	var root string
	switch scope {
	case ScopeUser:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		root = filepath.Join(home, ".agents", "skills")
	case ScopeProject:
		root = filepath.Join(cwd, ".agents", "skills")
	default:
		return "", fmt.Errorf("unsupported scope %q", scope)
	}

	return normalizeInstallRoot(root, cwd)
}

func normalizeInstallRoot(raw, cwd string) (string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(raw))
	if cleaned == "" || cleaned == "." {
		return "", fmt.Errorf("install root is empty")
	}
	if !filepath.IsAbs(cleaned) {
		cleaned = filepath.Join(cwd, cleaned)
	}
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("normalize install root: %w", err)
	}
	return abs, nil
}
