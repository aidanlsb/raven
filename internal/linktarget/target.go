// Package linktarget classifies and normalizes Markdown link destinations.
package linktarget

import (
	"path"
	"path/filepath"
	"strings"
)

// Scheme is the broad target category stored in the link index.
type Scheme string

const (
	SchemeFile  Scheme = "file"
	SchemeURL   Scheme = "url"
	SchemeOther Scheme = "other"
)

// Info is the derived, stable metadata for a link target.
type Info struct {
	Scheme        Scheme
	Ext           string
	NormalizedKey string
}

// Analyze classifies raw and derives its extension and canonical identity.
// sourceFile is vault-relative; vaultPath is the absolute or working-directory
// relative vault root.
func Analyze(raw, sourceFile, vaultPath string) Info {
	target := strings.TrimSpace(raw)
	scheme := Classify(target)
	info := Info{Scheme: scheme}

	switch scheme {
	case SchemeFile:
		filePath := fileDestinationPath(target)
		info.Ext = fileExtension(filePath)
		info.NormalizedKey = normalizeFile(filePath, sourceFile, vaultPath)
	case SchemeURL:
		info.NormalizedKey = normalizeURL(target)
	default:
		info.NormalizedKey = target
	}

	return info
}

// Classify maps a target to the broad category used by the link index.
func Classify(raw string) Scheme {
	target := strings.TrimSpace(raw)
	if target == "" {
		return SchemeFile
	}
	if isWindowsAbsolute(target) {
		return SchemeFile
	}
	if strings.HasPrefix(target, "//") {
		return SchemeURL
	}

	scheme, ok := uriScheme(target)
	if !ok {
		return SchemeFile
	}
	switch strings.ToLower(scheme) {
	case "file":
		return SchemeFile
	case "http", "https":
		return SchemeURL
	default:
		if strings.HasPrefix(target[len(scheme)+1:], "//") {
			return SchemeURL
		}
		return SchemeOther
	}
}

// IsRavenTarget reports whether a Markdown destination names a Raven Markdown
// object or section inside the vault. Markdown files outside the vault remain
// ordinary file link targets. Wikilinks are parsed separately.
func IsRavenTarget(raw, sourceFile, vaultPath string) bool {
	target := strings.TrimSpace(raw)
	if target == "" || strings.HasPrefix(target, "#") {
		return true
	}
	if Classify(target) != SchemeFile {
		return false
	}
	info := Analyze(target, sourceFile, vaultPath)
	if !strings.EqualFold(info.Ext, "md") {
		return false
	}
	return !filepath.IsAbs(filepath.FromSlash(info.NormalizedKey)) && !isWindowsAbsolute(info.NormalizedKey)
}

// ResolveFileKey turns a normalized file-link key into an OS path. Keys inside
// the vault are vault-relative; external absolute targets remain absolute.
func ResolveFileKey(normalizedKey, vaultPath string) string {
	target := filepath.FromSlash(normalizedKey)
	if filepath.IsAbs(target) || isWindowsAbsolute(normalizedKey) {
		return filepath.Clean(target)
	}
	return filepath.Clean(filepath.Join(vaultPath, target))
}

// RetargetFile preserves the authored style of a file-link destination while
// changing its semantic target. Angle brackets, query/fragment suffixes,
// relative-vs-absolute form, explicit "./", file: URI form, and escaped
// parentheses are retained.
func RetargetFile(rawTarget, sourceFile, vaultPath, newNormalizedKey string) string {
	target := strings.TrimSpace(rawTarget)
	if target == "" {
		return rawTarget
	}

	leading := rawTarget[:len(rawTarget)-len(strings.TrimLeft(rawTarget, " \t\r\n"))]
	trailing := rawTarget[len(strings.TrimRight(rawTarget, " \t\r\n")):]

	angle := len(target) >= 2 && target[0] == '<' && target[len(target)-1] == '>'
	if angle {
		target = target[1 : len(target)-1]
	}
	authoredPath, suffix := splitFileSuffix(target)
	semanticPath := unescapePathStyle(authoredPath)

	newAbs := ResolveFileKey(newNormalizedKey, vaultPath)
	replacement := ""
	if scheme, ok := uriScheme(semanticPath); ok && strings.EqualFold(scheme, "file") {
		prefixEnd := len(scheme) + 1
		prefix := authoredPath[:prefixEnd]
		rest := semanticPath[prefixEnd:]
		if strings.HasPrefix(rest, "//") {
			prefix += "//"
		}
		replacement = prefix + filepath.ToSlash(newAbs)
	} else if filepath.IsAbs(filepath.FromSlash(semanticPath)) || isWindowsAbsolute(semanticPath) {
		replacement = filepath.ToSlash(newAbs)
		if strings.Contains(semanticPath, `\`) && isWindowsAbsolute(semanticPath) {
			replacement = strings.ReplaceAll(replacement, "/", `\`)
		}
	} else {
		sourceDir := filepath.Dir(filepath.FromSlash(sourceFile))
		newPath := filepath.FromSlash(newNormalizedKey)
		if filepath.IsAbs(newPath) || isWindowsAbsolute(newNormalizedKey) {
			newPath = newAbs
			sourceDir = filepath.Join(vaultPath, sourceDir)
		}
		rel, err := filepath.Rel(sourceDir, newPath)
		if err != nil {
			replacement = filepath.ToSlash(newAbs)
		} else {
			replacement = filepath.ToSlash(rel)
		}
		if strings.HasPrefix(semanticPath, "./") && !strings.HasPrefix(replacement, ".") {
			replacement = "./" + replacement
		}
	}

	replacement = preservePathEscapes(authoredPath, replacement)
	replacement += suffix
	if angle {
		replacement = "<" + replacement + ">"
	}
	return leading + replacement + trailing
}

func splitFileSuffix(target string) (string, string) {
	for i := 0; i < len(target); i++ {
		if target[i] == '\\' {
			i++
			continue
		}
		if target[i] == '?' || target[i] == '#' {
			return target[:i], target[i:]
		}
	}
	return target, ""
}

func unescapePathStyle(target string) string {
	var b strings.Builder
	b.Grow(len(target))
	for i := 0; i < len(target); i++ {
		if target[i] == '\\' && i+1 < len(target) && isMarkdownEscapable(target[i+1]) {
			i++
		}
		b.WriteByte(target[i])
	}
	return b.String()
}

func preservePathEscapes(authored, replacement string) string {
	escapeParens := strings.Contains(authored, `\(`) || strings.Contains(authored, `\)`)
	if !escapeParens {
		return replacement
	}
	replacement = strings.ReplaceAll(replacement, `\`, `\\`)
	replacement = strings.ReplaceAll(replacement, "(", `\(`)
	return strings.ReplaceAll(replacement, ")", `\)`)
}

func isMarkdownEscapable(c byte) bool {
	return (c >= '!' && c <= '/') ||
		(c >= ':' && c <= '@') ||
		(c >= '[' && c <= '`') ||
		(c >= '{' && c <= '~')
}

func fileDestinationPath(target string) string {
	if scheme, ok := uriScheme(target); ok && strings.EqualFold(scheme, "file") {
		target = target[len(scheme)+1:]
		target = strings.TrimPrefix(target, "//")
		if !strings.HasPrefix(target, "/") {
			target = "/" + target
		}
	}
	if idx := strings.IndexAny(target, "?#"); idx >= 0 {
		target = target[:idx]
	}
	return strings.ReplaceAll(target, "\\", "/")
}

func fileExtension(target string) string {
	base := path.Base(target)
	if base == "." || base == "/" || (strings.HasPrefix(base, ".") && !strings.Contains(base[1:], ".")) {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(path.Ext(base), "."))
}

func normalizeFile(target, sourceFile, vaultPath string) string {
	target = strings.ReplaceAll(target, "\\", "/")
	// filepath understands drive-qualified paths on Windows. On other hosts,
	// retain a clean portable key rather than treating the drive as relative.
	if isWindowsAbsolute(target) && !filepath.IsAbs(filepath.FromSlash(target)) {
		return path.Clean(target)
	}

	absVault, err := filepath.Abs(vaultPath)
	if err != nil {
		absVault = filepath.Clean(vaultPath)
	}
	absVault = filepath.Clean(absVault)

	var absTarget string
	if filepath.IsAbs(filepath.FromSlash(target)) {
		absTarget = filepath.Clean(filepath.FromSlash(target))
	} else {
		sourceDir := filepath.Dir(filepath.FromSlash(sourceFile))
		absTarget = filepath.Clean(filepath.Join(absVault, sourceDir, filepath.FromSlash(target)))
	}

	if rel, relErr := filepath.Rel(absVault, absTarget); relErr == nil && isInsideRelativePath(rel) {
		return filepath.ToSlash(filepath.Clean(rel))
	}
	return filepath.ToSlash(absTarget)
}

func isInsideRelativePath(rel string) bool {
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func normalizeURL(raw string) string {
	authorityStart, scheme, ok := urlAuthority(raw)
	if !ok {
		return raw
	}
	authorityEnd := len(raw)
	if i := strings.IndexAny(raw[authorityStart:], "/?#"); i >= 0 {
		authorityEnd = authorityStart + i
	}

	authority := raw[authorityStart:authorityEnd]
	userinfo := ""
	hostport := authority
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		userinfo = authority[:at+1]
		hostport = authority[at+1:]
	}

	host, port, hasPort := splitHostPort(hostport)
	host = strings.ToLower(host)
	if isDefaultPort(scheme, port) {
		hasPort = false
	}

	normalizedHost := host
	if hasPort {
		normalizedHost += ":" + port
	}
	return raw[:authorityStart] + userinfo + normalizedHost + raw[authorityEnd:]
}

func urlAuthority(raw string) (start int, scheme string, ok bool) {
	if strings.HasPrefix(raw, "//") {
		return 2, "", true
	}
	scheme, hasScheme := uriScheme(raw)
	if !hasScheme {
		return 0, "", false
	}
	separator := len(scheme) + 1
	if !strings.HasPrefix(raw[separator:], "//") {
		return 0, scheme, false
	}
	return separator + 2, scheme, true
}

func splitHostPort(hostport string) (host, port string, hasPort bool) {
	if strings.HasPrefix(hostport, "[") {
		if closeBracket := strings.IndexByte(hostport, ']'); closeBracket >= 0 {
			host = hostport[:closeBracket+1]
			if len(hostport) > closeBracket+1 && hostport[closeBracket+1] == ':' {
				port = hostport[closeBracket+2:]
				hasPort = true
			}
			return host, port, hasPort
		}
	}
	if strings.Count(hostport, ":") == 1 {
		if colon := strings.LastIndexByte(hostport, ':'); colon >= 0 {
			return hostport[:colon], hostport[colon+1:], true
		}
	}
	return hostport, "", false
}

func isDefaultPort(scheme, port string) bool {
	switch strings.ToLower(scheme) {
	case "http":
		return port == "80"
	case "https":
		return port == "443"
	default:
		return false
	}
}

func uriScheme(target string) (string, bool) {
	colon := strings.IndexByte(target, ':')
	if colon <= 0 {
		return "", false
	}
	for i := 0; i < colon; i++ {
		c := target[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9') || (i > 0 && (c == '+' || c == '-' || c == '.')) {
			continue
		}
		return "", false
	}
	return target[:colon], true
}

func isWindowsAbsolute(target string) bool {
	if len(target) < 3 {
		return false
	}
	drive := target[0]
	return ((drive >= 'a' && drive <= 'z') || (drive >= 'A' && drive <= 'Z')) &&
		target[1] == ':' && (target[2] == '/' || target[2] == '\\')
}
