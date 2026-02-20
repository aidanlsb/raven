package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveInstallRoot resolves the skill installation root for a target/scope pair.
// If destOverride is provided, it takes precedence.
func ResolveInstallRoot(target Target, scope Scope, destOverride, cwd string) (string, error) {
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

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	var root string
	switch target {
	case TargetCodex:
		switch scope {
		case ScopeUser:
			base := strings.TrimSpace(os.Getenv("CODEX_HOME"))
			if base == "" {
				base = filepath.Join(home, ".codex")
			}
			root = filepath.Join(base, "skills")
		case ScopeProject:
			root = filepath.Join(cwd, ".codex", "skills")
		default:
			return "", fmt.Errorf("unsupported scope %q for target %q", scope, target)
		}
	case TargetClaude:
		switch scope {
		case ScopeUser:
			root = filepath.Join(home, ".claude", "skills")
		case ScopeProject:
			root = filepath.Join(cwd, ".claude", "skills")
		default:
			return "", fmt.Errorf("unsupported scope %q for target %q", scope, target)
		}
	case TargetCursor:
		switch scope {
		case ScopeUser:
			root = filepath.Join(home, ".cursor", "skills")
		case ScopeProject:
			root = filepath.Join(cwd, ".cursor", "skills")
		default:
			return "", fmt.Errorf("unsupported scope %q for target %q", scope, target)
		}
	default:
		return "", fmt.Errorf("unsupported target %q", target)
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
