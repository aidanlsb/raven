package refs

import (
	"strings"

	"github.com/aidanlsb/raven/internal/paths"
)

// rewriteFrontmatterLine rewrites references on a single YAML frontmatter line.
//
// It first rewrites any wikilinks (which may appear bare or inside quotes, e.g.
// `owner: [[people/tido]]`), then rewrites bare/quoted plain-path references in
// scalar values, inline flow arrays, and block sequence items.
func rewriteFrontmatterLine(line string, decide Decider) (string, bool) {
	changed := false

	// Wikilinks anywhere on the line (code spans are skipped, matching the
	// parser's frontmatter ref extraction which also skips inline code).
	if updated, c := rewriteWikilinksOnLine(line, decide); c {
		line = updated
		changed = true
	}

	if prefix, value, ok := splitFrontmatterMapping(line); ok {
		if updated, c := rewriteFrontmatterValue(value, decide); c {
			return prefix + updated, true
		}
		return line, changed
	}
	if prefix, value, ok := splitFrontmatterSequence(line); ok {
		if updated, c := rewriteFrontmatterValue(value, decide); c {
			return prefix + updated, true
		}
		return line, changed
	}

	return line, changed
}

// splitFrontmatterMapping splits a `key: value` (or `- key: value`) line into
// its prefix (indent, key, colon, spaces) and the value region.
func splitFrontmatterMapping(line string) (prefix, value string, ok bool) {
	m := fmMappingValueRe.FindStringSubmatchIndex(line)
	if m == nil {
		return "", "", false
	}
	return line[m[2]:m[3]], line[m[4]:m[5]], true
}

// splitFrontmatterSequence splits a `- value` block sequence item into its
// prefix (indent, dash, spaces) and the value region.
func splitFrontmatterSequence(line string) (prefix, value string, ok bool) {
	m := fmSequenceValueRe.FindStringSubmatchIndex(line)
	if m == nil {
		return "", "", false
	}
	return line[m[2]:m[3]], line[m[4]:m[5]], true
}

// rewriteFrontmatterValue rewrites a YAML value region (which may carry a
// trailing comment). It handles inline flow arrays and scalar values, leaving
// wikilink values (already handled upstream) untouched.
func rewriteFrontmatterValue(rest string, decide Decider) (string, bool) {
	value, comment := splitTrailingComment(rest)
	core := strings.TrimRight(value, " \t")
	trailing := value[len(core):]

	if strings.HasPrefix(core, "[") && strings.HasSuffix(core, "]") && len(core) >= 2 {
		inner := core[1 : len(core)-1]
		if updated, c := rewriteFlowSequence(inner, decide); c {
			return "[" + updated + "]" + trailing + comment, true
		}
		return rest, false
	}

	if updated, c := rewriteScalarToken(core, decide); c {
		return updated + trailing + comment, true
	}
	return rest, false
}

// rewriteFlowSequence rewrites each element of an inline flow array, preserving
// the surrounding whitespace of each element.
func rewriteFlowSequence(inner string, decide Decider) (string, bool) {
	elems := splitTopLevelCommas(inner)
	changed := false
	for i, raw := range elems {
		lead := len(raw) - len(strings.TrimLeft(raw, " \t"))
		trimmed := strings.TrimRight(raw[lead:], " \t")
		core := trimmed
		if core == "" {
			continue
		}
		trailStart := lead + len(core)
		if updated, c := rewriteScalarToken(core, decide); c {
			elems[i] = raw[:lead] + updated + raw[trailStart:]
			changed = true
		}
	}
	if !changed {
		return inner, false
	}
	return strings.Join(elems, ","), true
}

// rewriteScalarToken rewrites a single YAML scalar reference token, preserving
// the original quote style. Tokens containing a wikilink literal are skipped
// (handled by the wikilink pass).
func rewriteScalarToken(token string, decide Decider) (string, bool) {
	if token == "" || strings.Contains(token, "[[") {
		return token, false
	}

	quote := byte(0)
	inner := token
	if len(token) >= 2 && (token[0] == '"' || token[0] == '\'') && token[len(token)-1] == token[0] {
		quote = token[0]
		inner = token[1 : len(token)-1]
	}
	if inner == "" {
		return token, false
	}

	base, fragment, isSection := paths.ParseSectionID(inner)
	newBase, ok := decide(Occurrence{
		Kind:        KindFrontmatter,
		Base:        base,
		Fragment:    fragment,
		HasFragment: isSection,
	})
	if !ok {
		return token, false
	}

	rebuilt := newBase
	if isSection {
		rebuilt += "#" + fragment
	}
	if quote != 0 {
		rebuilt = string(quote) + rebuilt + string(quote)
	}
	return rebuilt, true
}

// splitTrailingComment separates a YAML value from a trailing `# comment`,
// respecting quotes and flow collection brackets. A '#' only starts a comment
// when it is at the start of the value or preceded by whitespace.
func splitTrailingComment(value string) (val, comment string) {
	var quote byte
	depth := 0
	for i := 0; i < len(value); i++ {
		c := value[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '[', '{':
			depth++
		case ']', '}':
			if depth > 0 {
				depth--
			}
		case '#':
			if i == 0 || value[i-1] == ' ' || value[i-1] == '\t' {
				j := i
				for j > 0 && (value[j-1] == ' ' || value[j-1] == '\t') {
					j--
				}
				return value[:j], value[j:]
			}
		}
	}
	return value, ""
}

// splitTopLevelCommas splits s on commas that are not inside quotes or nested
// flow collections.
func splitTopLevelCommas(s string) []string {
	var out []string
	var quote byte
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '[', '{':
			depth++
		case ']', '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
}
