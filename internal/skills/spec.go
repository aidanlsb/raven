package skills

import (
	"fmt"
	"path"
	"strings"
)

// Scope controls where skills are installed.
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

// Target identifies an agent runtime skill layout.
type Target string

const (
	TargetCodex  Target = "codex"
	TargetClaude Target = "claude"
	TargetCursor Target = "cursor"
)

var allTargets = []Target{TargetCodex, TargetClaude, TargetCursor}

// Spec is the canonical Raven skill definition.
type Spec struct {
	ID         string   `yaml:"id"`
	Title      string   `yaml:"title"`
	Version    int      `yaml:"version"`
	Summary    string   `yaml:"summary"`
	Entry      string   `yaml:"entry"`
	References []string `yaml:"references"`
	Coverage   struct {
		Commands []string `yaml:"commands"`
	} `yaml:"coverage"`
	Execution struct {
		PreferredTransport string   `yaml:"preferred_transport"`
		CLIBinary          string   `yaml:"cli_binary"`
		CLIArgsSuffix      []string `yaml:"cli_args_suffix"`
		MCPFallback        bool     `yaml:"mcp_fallback"`
	} `yaml:"execution"`
}

// Skill is a loaded skill bundle from the embedded library.
type Skill struct {
	Spec           Spec
	EntryMarkdown  string
	References     map[string]string
	OpenAIMetadata string
}

// Summary is a lightweight list view.
type Summary struct {
	Name      string `json:"name"`
	Title     string `json:"title"`
	Version   int    `json:"version"`
	Summary   string `json:"summary"`
	Installed bool   `json:"installed,omitempty"`
	Path      string `json:"path,omitempty"`
}

func (s *Spec) validate() error {
	if s == nil {
		return fmt.Errorf("skill spec is nil")
	}
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("missing id")
	}
	if strings.TrimSpace(s.Summary) == "" {
		return fmt.Errorf("missing summary")
	}
	if s.Version <= 0 {
		return fmt.Errorf("version must be >= 1")
	}
	if strings.TrimSpace(s.Entry) == "" {
		s.Entry = "body.md"
	}
	entry, err := normalizeRelativePath(s.Entry)
	if err != nil {
		return fmt.Errorf("invalid entry path: %w", err)
	}
	s.Entry = entry

	normalizedRefs := make([]string, 0, len(s.References))
	for _, ref := range s.References {
		if strings.TrimSpace(ref) == "" {
			continue
		}
		n, err := normalizeRelativePath(ref)
		if err != nil {
			return fmt.Errorf("invalid reference path %q: %w", ref, err)
		}
		normalizedRefs = append(normalizedRefs, n)
	}
	s.References = normalizedRefs
	return nil
}

func normalizeRelativePath(p string) (string, error) {
	cleaned := path.Clean(strings.TrimSpace(p))
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("path is empty")
	}
	if strings.HasPrefix(cleaned, "/") || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path escapes skill root")
	}
	return cleaned, nil
}

// ParseTarget parses a user-provided target value.
func ParseTarget(raw string) (Target, error) {
	t := Target(strings.ToLower(strings.TrimSpace(raw)))
	switch t {
	case TargetCodex, TargetClaude, TargetCursor:
		return t, nil
	default:
		return "", fmt.Errorf("unsupported target %q (expected: codex, claude, cursor)", raw)
	}
}

// ParseScope parses a user-provided install scope.
func ParseScope(raw string) (Scope, error) {
	s := Scope(strings.ToLower(strings.TrimSpace(raw)))
	switch s {
	case "", ScopeUser:
		return ScopeUser, nil
	case ScopeProject:
		return ScopeProject, nil
	default:
		return "", fmt.Errorf("unsupported scope %q (expected: user or project)", raw)
	}
}

// AllTargets returns all supported skill targets.
func AllTargets() []Target {
	out := make([]Target, len(allTargets))
	copy(out, allTargets)
	return out
}
