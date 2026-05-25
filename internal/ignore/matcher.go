package ignore

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Matcher checks vault-relative paths against Raven exclude patterns.
type Matcher struct {
	patterns []pattern
}

type pattern struct {
	raw      string
	glob     string
	negated  bool
	dirOnly  bool
	anchored bool
	hasSlash bool
}

// NewMatcher builds a matcher from raven.yaml exclude patterns.
func NewMatcher(patterns []string) (*Matcher, error) {
	normalized := NormalizePatterns(patterns)
	if len(normalized) == 0 {
		return &Matcher{}, nil
	}

	compiled := make([]pattern, 0, len(normalized))
	for _, raw := range normalized {
		next, err := parsePattern(raw)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, next)
	}
	return &Matcher{patterns: compiled}, nil
}

// NormalizePatterns trims empty patterns while preserving gitignore syntax.
func NormalizePatterns(patterns []string) []string {
	if len(patterns) == 0 {
		return nil
	}
	out := make([]string, 0, len(patterns))
	for _, raw := range patterns {
		pattern := strings.TrimSpace(filepath.ToSlash(raw))
		if pattern == "" {
			continue
		}
		if strings.ContainsAny(pattern, "\r\n") {
			continue
		}
		out = append(out, pattern)
	}
	return out
}

// Match reports whether a vault-relative path is excluded.
func (m *Matcher) Match(relPath string, isDir bool) bool {
	if m == nil || len(m.patterns) == 0 {
		return false
	}
	relPath = NormalizePath(relPath)
	if relPath == "" || relPath == "." {
		return false
	}
	matched := false
	for _, pattern := range m.patterns {
		if pattern.matches(relPath, isDir) {
			matched = !pattern.negated
		}
	}
	return matched
}

// NormalizePath normalizes a path before matching.
func NormalizePath(path string) string {
	path = strings.TrimSpace(filepath.ToSlash(path))
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimPrefix(path, "/")
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	return path
}

func parsePattern(raw string) (pattern, error) {
	p := pattern{raw: raw}
	glob := raw
	if strings.HasPrefix(glob, `\!`) || strings.HasPrefix(glob, `\#`) {
		glob = glob[1:]
	} else if strings.HasPrefix(glob, "!") {
		p.negated = true
		glob = strings.TrimPrefix(glob, "!")
	}
	if strings.HasPrefix(glob, "/") {
		p.anchored = true
		glob = strings.TrimPrefix(glob, "/")
	}
	if strings.HasSuffix(glob, "/") {
		p.dirOnly = true
		glob = strings.TrimSuffix(glob, "/")
	}
	if glob == "" {
		return pattern{}, fmt.Errorf("invalid exclude pattern %q: empty pattern", raw)
	}
	p.glob = glob
	p.hasSlash = strings.Contains(glob, "/")
	if err := validateGlob(glob); err != nil {
		return pattern{}, fmt.Errorf("invalid exclude pattern %q: %w", raw, err)
	}
	return p, nil
}

func (p pattern) matches(relPath string, isDir bool) bool {
	if p.dirOnly {
		return p.matchesDirectory(relPath, isDir)
	}
	if p.hasSlash {
		return globMatch(p.glob, relPath)
	}
	if p.anchored {
		return globMatch(p.glob, relPath)
	}
	return globMatch(p.glob, path.Base(relPath))
}

func (p pattern) matchesDirectory(relPath string, isDir bool) bool {
	dirs := parentDirs(relPath, isDir)
	for _, dir := range dirs {
		if p.hasSlash || p.anchored {
			if globMatch(p.glob, dir) {
				return true
			}
			continue
		}
		for _, segment := range strings.Split(dir, "/") {
			if globMatch(p.glob, segment) {
				return true
			}
		}
	}
	return false
}

func parentDirs(relPath string, isDir bool) []string {
	if isDir {
		return dirPrefixes(relPath)
	}
	dir := path.Dir(relPath)
	if dir == "." || dir == "/" {
		return nil
	}
	return dirPrefixes(dir)
}

func dirPrefixes(dir string) []string {
	parts := strings.Split(dir, "/")
	out := make([]string, 0, len(parts))
	for i := range parts {
		out = append(out, strings.Join(parts[:i+1], "/"))
	}
	return out
}

func validateGlob(glob string) error {
	if strings.Contains(glob, "**") {
		_, err := regexp.Compile(globRegexp(glob))
		return err
	}
	_, err := path.Match(glob, "")
	return err
}

func globMatch(glob, value string) bool {
	if strings.Contains(glob, "**") {
		matched, err := regexp.MatchString(globRegexp(glob), value)
		return err == nil && matched
	}
	matched, err := path.Match(glob, value)
	return err == nil && matched
}

func globRegexp(glob string) string {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch glob[i] {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				for i+1 < len(glob) && glob[i+1] == '*' {
					i++
				}
				if i+1 < len(glob) && glob[i+1] == '/' {
					b.WriteString(`(?:.*/)?`)
					i++
					continue
				}
				b.WriteString(".*")
			} else {
				b.WriteString(`[^/]*`)
			}
		case '?':
			b.WriteString(`[^/]`)
		default:
			b.WriteString(regexp.QuoteMeta(string(glob[i])))
		}
	}
	b.WriteString("$")
	return b.String()
}
