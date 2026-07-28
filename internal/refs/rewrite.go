// Package refs provides structural rewriting of Raven references in file
// content. It mirrors the parser's extraction logic so that rewriting and
// extraction agree on edge cases that naive string replacement gets wrong:
//
//   - wikilink display text and section fragments ([[target#frag|Display]])
//   - inline code spans and fenced code blocks (never rewritten)
//   - quoted YAML frontmatter values, inline flow arrays, and block sequences
//
// The YAML frontmatter region is located with parser.FrontmatterBounds rather
// than ad hoc fence detection, and section fragments are split with
// paths.ParseSectionID.
//
// Callers supply a Decider that maps a matched reference base to its
// replacement base. Wikilink fragments and display text are preserved
// automatically by the rewriter, so a
// Decider only needs to answer "given this base target, what is the new base?".
package refs

import (
	"regexp"
	"strings"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/wikilink"
)

// Kind classifies where a reference occurrence was found.
type Kind int

const (
	// KindWikilink is a [[target]] reference in the body or frontmatter.
	KindWikilink Kind = iota
	// KindFrontmatter is a bare/quoted YAML frontmatter scalar or sequence value.
	KindFrontmatter
)

// Occurrence describes a single reference occurrence encountered while
// rewriting. Base is the target with any section fragment removed; Fragment
// holds that fragment (without the leading '#') when HasFragment is true.
type Occurrence struct {
	Kind        Kind
	Base        string
	Fragment    string
	HasFragment bool
}

// Decider returns the replacement base target for a matched occurrence, plus
// whether to apply the change. The rewriter re-attaches the occurrence's
// fragment and preserves display text / link titles, so a Decider must return
// only the new base (without a fragment).
type Decider func(occ Occurrence) (newBase string, ok bool)

// Frontmatter mapping/sequence value matchers. These capture the value region
// only; matching against reference targets happens in Go (not the regex), so
// they are compiled once and reused across all rewrites.
var (
	fmMappingValueRe  = regexp.MustCompile(`^(\s*(?:-\s+)?[^:#\n]+:\s+)(\S.*)$`)
	fmSequenceValueRe = regexp.MustCompile(`^(\s*-\s+)(\S.*)$`)
)

// RewriteContent rewrites references across an entire document: the YAML
// frontmatter region (located via parser.FrontmatterBounds) and the markdown
// body (skipping fenced code blocks and inline code spans). It returns the
// updated content and whether any change was made.
func RewriteContent(content string, decide Decider) (string, bool) {
	if decide == nil {
		return content, false
	}

	lines := strings.Split(content, "\n")
	changed := false

	bodyStart := 0
	if start, end, ok := parser.FrontmatterBounds(lines); ok && end != -1 {
		for i := start + 1; i < end; i++ {
			if updated, c := rewriteFrontmatterLine(lines[i], decide); c {
				lines[i] = updated
				changed = true
			}
		}
		bodyStart = end + 1
	}

	var fence parser.FenceState
	for i := bodyStart; i < len(lines); i++ {
		if fence.UpdateFenceState(lines[i]) {
			continue
		}
		if fence.InFence {
			continue
		}
		if updated, c := rewriteBodyLine(lines[i], decide); c {
			lines[i] = updated
			changed = true
		}
	}

	if !changed {
		return content, false
	}
	return strings.Join(lines, "\n"), true
}

// RewriteContentAtLine prefers rewriting reference occurrences on the given
// 1-indexed line, falling back to RewriteContent when that line contains no
// matching reference. A line <= 0 rewrites the whole document.
//
// The targeted line is treated as a body line (wikilinks + markdown links);
// bare frontmatter values only match through the whole-document fallback, which
// mirrors how the move path historically behaved.
func RewriteContentAtLine(content string, line int, decide Decider) (string, bool) {
	if decide == nil {
		return content, false
	}
	if line <= 0 {
		return RewriteContent(content, decide)
	}

	lines := strings.Split(content, "\n")
	idx := line - 1
	if idx < 0 || idx >= len(lines) {
		return content, false
	}

	if updated, c := rewriteBodyLine(lines[idx], decide); c {
		lines[idx] = updated
		return strings.Join(lines, "\n"), true
	}

	return RewriteContent(content, decide)
}

// span is a pending replacement of content[start:end] with repl.
type span struct {
	start int
	end   int
	repl  string
}

// rewriteBodyLine rewrites wikilink and markdown-link references on a single
// body line, skipping inline code spans.
func rewriteBodyLine(line string, decide Decider) (string, bool) {
	masked := parser.RemoveInlineCode(line)
	return applySpans(line, wikilinkSpans(line, masked, decide))
}

// rewriteWikilinksOnLine rewrites only wikilink references on a line, skipping
// inline code spans. It is used for frontmatter lines, where markdown link
// syntax is not a reference form.
func rewriteWikilinksOnLine(line string, decide Decider) (string, bool) {
	masked := parser.RemoveInlineCode(line)
	return applySpans(line, wikilinkSpans(line, masked, decide))
}

// wikilinkSpans collects replacement spans for wikilinks on a line. It scans the
// masked line (code spans blanked) but reconstructs from the original text.
func wikilinkSpans(line, masked string, decide Decider) []span {
	matches := wikilink.FindAllInLine(masked, false)
	if len(matches) == 0 {
		return nil
	}

	var edits []span
	for _, m := range matches {
		literal := line[m.Start:m.End]
		target, display, ok := wikilink.ParseExact(literal)
		if !ok {
			continue
		}
		base, fragment, isSection := paths.ParseSectionID(target)
		newBase, replace := decide(Occurrence{
			Kind:        KindWikilink,
			Base:        base,
			Fragment:    fragment,
			HasFragment: isSection,
		})
		if !replace {
			continue
		}
		edits = append(edits, span{
			start: m.Start,
			end:   m.End,
			repl:  buildWikilink(newBase, fragment, isSection, display),
		})
	}
	return edits
}

// applySpans applies non-overlapping replacement spans (in any order) to line.
func applySpans(line string, edits []span) (string, bool) {
	if len(edits) == 0 {
		return line, false
	}

	// Insertion sort keeps ordering stable and avoids importing sort for a
	// handful of edits.
	for i := 1; i < len(edits); i++ {
		for j := i; j > 0 && edits[j-1].start > edits[j].start; j-- {
			edits[j-1], edits[j] = edits[j], edits[j-1]
		}
	}

	var b strings.Builder
	last := 0
	changed := false
	for _, e := range edits {
		if e.start < last || e.end > len(line) || e.start > e.end {
			continue
		}
		b.WriteString(line[last:e.start])
		b.WriteString(e.repl)
		last = e.end
		changed = true
	}
	if !changed {
		return line, false
	}
	b.WriteString(line[last:])
	return b.String(), true
}

// buildWikilink reconstructs a wikilink literal from its parts, preserving the
// section fragment and display text.
func buildWikilink(base, fragment string, hasFragment bool, display *string) string {
	var b strings.Builder
	b.WriteString("[[")
	b.WriteString(base)
	if hasFragment {
		b.WriteString("#")
		b.WriteString(fragment)
	}
	if display != nil {
		b.WriteString("|")
		b.WriteString(*display)
	}
	b.WriteString("]]")
	return b.String()
}
