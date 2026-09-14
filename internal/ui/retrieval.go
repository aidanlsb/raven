package ui

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/linktarget"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/schema"
)

// RetrievalLinks formats path:line cells in retrieval tables. The CLI supplies
// editor hyperlinks; tests and other callers may leave the fields nil.
type RetrievalLinks struct {
	Location  func(relPath string, line int) string
	VaultFile func(label, key string, line int) string
}

func (l RetrievalLinks) location(relPath string, line int) string {
	if l.Location != nil {
		return l.Location(relPath, line)
	}
	if line > 0 {
		return fmt.Sprintf("%s:%d", relPath, line)
	}
	return relPath
}

func (l RetrievalLinks) vaultFile(label, key string, line int) string {
	if l.VaultFile != nil {
		return l.VaultFile(label, key, line)
	}
	return label
}

func TraitTableHeaders() []string {
	return []string{"#", "content", "trait", "location"}
}

func SectionTableHeaders() []string {
	return []string{"#", "title", "heading", "location"}
}

func LinkTableHeaders() []string {
	return []string{"#", "target", "kind", "location"}
}

func PrintQueryObjectResults(queryStr, typeName string, results []model.Object, sch *schema.Schema, links RetrievalLinks) {
	if len(results) == 0 {
		fmt.Println(Starf("No objects found for: %s", queryStr))
		return
	}

	fmt.Printf("%s %s\n\n", SectionHeader(typeName), Badge(fmt.Sprintf("%d", len(results))))
	PrintObjectTable(results, sch, links)
}

func PrintQueryTraitResults(queryStr, traitName string, results []model.Trait, links RetrievalLinks) {
	if len(results) == 0 {
		fmt.Println(Starf("No traits found for: %s", queryStr))
		return
	}

	fmt.Printf("%s %s\n\n", SectionHeader("@"+traitName), Badge(fmt.Sprintf("%d", len(results))))

	display := NewDisplayContext()
	table := NewResultsTable(display, TraitLayout())
	table.SetHeaders(TraitTableHeaders())

	contentWidth := table.ContentWidth("content")
	maxContentLen := contentWidth * 2

	for i, r := range results {
		value := ""
		if idx := r.IndexValueString(); idx != nil && *idx != r.TraitType {
			value = *idx
		}
		traitStr := Trait(r.TraitType, value)

		content := r.Content
		if content == "" {
			content = "(no content)"
		}

		truncated := false
		if len(content) > maxContentLen {
			if snippetHasCodeBlock(content) {
				maxCodeLen := maxContentLen * 3
				if len(content) > maxCodeLen {
					content = TruncateWithEllipsis(content, maxCodeLen)
					truncated = true
				}
			} else {
				content = TruncateWithEllipsis(content, maxContentLen)
				truncated = true
			}
		}

		content = normalizeInlineCodeSnippet(content, truncated)
		content = HighlightTraits(content)
		location := links.location(r.FilePath, r.Line)

		table.AddRow(ResultRow{
			Num:      i + 1,
			Cells:    []string{FormatRowNum(i+1, len(results)), content, traitStr, location},
			Location: fmt.Sprintf("%s:%d", r.FilePath, r.Line),
		})
	}

	fmt.Println(table.Render())
}

func PrintQuerySectionResults(queryStr string, results []model.Section, links RetrievalLinks) {
	if len(results) == 0 {
		fmt.Println(Starf("No sections found for: %s", queryStr))
		return
	}

	fmt.Printf("%s %s\n\n", SectionHeader("section"), Badge(fmt.Sprintf("%d", len(results))))

	display := NewDisplayContext()
	table := NewResultsTable(display, SearchLayout())
	table.SetHeaders(SectionTableHeaders())

	for i, r := range results {
		meta := fmt.Sprintf("h%d #%s", r.Level, r.Slug)
		location := links.location(r.FilePath, r.LineStart)
		table.AddRow(ResultRow{
			Num:      i + 1,
			Cells:    []string{FormatRowNum(i+1, len(results)), TruncateWithEllipsis(r.Title, table.GetColumnWidth(1)), meta, location},
			Location: fmt.Sprintf("%s:%d", r.FilePath, r.LineStart),
		})
	}

	fmt.Println(table.Render())
}

func PrintQueryLinkResults(queryStr string, results []model.Link, links RetrievalLinks) {
	if len(results) == 0 {
		fmt.Println(Starf("No links found for: %s", queryStr))
		return
	}

	fmt.Printf("%s %s\n\n", SectionHeader("link"), Badge(fmt.Sprintf("%d", len(results))))

	display := NewDisplayContext()
	table := NewResultsTable(display, SearchLayout())
	table.SetHeaders(LinkTableHeaders())

	for i, r := range results {
		target := r.RawTarget
		if r.Display != "" && r.Display != r.RawTarget {
			target = fmt.Sprintf("%s (%s)", r.Display, r.RawTarget)
		}
		kind := r.Scheme
		if r.Ext != "" {
			kind += " ." + r.Ext
		}
		if r.IsImage {
			kind += " image"
		}
		target = TruncateWithEllipsis(target, table.GetColumnWidth(1))
		if r.Scheme == string(linktarget.SchemeFile) && linktarget.IsVaultRelativeFileKey(r.NormalizedKey) {
			target = links.vaultFile(target, r.NormalizedKey, 1)
		}
		location := links.location(r.FilePath, r.Line)
		table.AddRow(ResultRow{
			Num:      i + 1,
			Cells:    []string{FormatRowNum(i+1, len(results)), target, kind, location},
			Location: fmt.Sprintf("%s:%d", r.FilePath, r.Line),
		})
	}

	fmt.Println(table.Render())
}

func PrintSearchResults(queryStr string, results []model.SearchMatch, links RetrievalLinks) {
	if len(results) == 0 {
		fmt.Println(Starf("No results found for: %s", queryStr))
		return
	}

	fmt.Printf("%s %s\n\n", SectionHeader(queryStr), Badge(fmt.Sprintf("%d results", len(results))))

	display := NewDisplayContext()
	table := NewResultsTable(display, SearchLayout())

	contentWidth := table.ContentWidth("content")
	maxSnippetLen := contentWidth * 2

	for i, result := range results {
		snippet := cleanSearchSnippetDynamic(result.Snippet, maxSnippetLen)
		if snippet == "" {
			snippet = "(no match preview)"
		}
		snippet = strings.ReplaceAll(snippet, "»", "")
		snippet = strings.ReplaceAll(snippet, "«", "")
		snippet = normalizeInlineCodeSnippet(snippet, snippetHasEllipsis(snippet))

		metaWidth := table.ContentWidth("meta")
		title := result.Title
		if len(title) > metaWidth-3 {
			title = title[:metaWidth-6] + "..."
		}

		line := result.LineStart
		if line == 0 {
			line = 1
		}
		filePath := links.location(result.FilePath, line)

		table.AddRow(ResultRow{
			Num:      i + 1,
			Cells:    []string{FormatRowNum(i+1, len(results)), snippet, title, filePath},
			Location: fmt.Sprintf("%s:%d", result.FilePath, line),
		})
	}

	fmt.Println(table.Render())
}

// cleanSearchSnippetDynamic removes frontmatter, cleans up, and centers a search snippet around the match.
// Uses maxLen to determine how much context to show (typically 2x content column width for two-line display).
func cleanSearchSnippetDynamic(snippet string, maxLen int) string {
	if snippet == "" {
		return ""
	}

	preserveWhitespace := snippetHasCodeBlock(snippet)
	if preserveWhitespace {
		// Allow more context so code blocks have a chance to display.
		maxLen *= 3
	}

	s := cleanRawSnippet(snippet, preserveWhitespace)
	if s == "" {
		return ""
	}

	// Find match markers
	matchStart := strings.Index(s, "»")
	if matchStart == -1 {
		return ""
	}
	matchEnd := strings.Index(s, "«")
	if matchEnd == -1 {
		matchEnd = matchStart + 10
	}
	matchLen := matchEnd - matchStart

	// Calculate context before/after based on maxLen
	// We want to center the match and use available space efficiently
	availableContext := maxLen - matchLen
	if availableContext < 0 {
		availableContext = 0
	}
	// Distribute context: slightly more after than before
	contextBefore := availableContext * 2 / 5
	contextAfter := availableContext - contextBefore

	result := extractSnippetWindow(s, matchStart, matchEnd, contextBefore, contextAfter)
	if preserveWhitespace {
		result = limitSnippetLines(result, 8)
	}
	return result
}

// cleanRawSnippet removes frontmatter and returns clean text.
// When preserveWhitespace is true, newlines and indentation are retained.
func cleanRawSnippet(snippet string, preserveWhitespace bool) string {
	s := strings.ReplaceAll(snippet, "\r\n", "\n")

	// Remove YAML frontmatter (---\n...\n---)
	for {
		startIdx := strings.Index(s, "---")
		if startIdx == -1 {
			break
		}
		rest := s[startIdx+3:]
		endIdx := strings.Index(rest, "---")
		if endIdx == -1 {
			s = strings.TrimSpace(rest)
			break
		}
		s = s[:startIdx] + rest[endIdx+3:]
	}

	if preserveWhitespace {
		return strings.Trim(s, "\n")
	}

	// Collapse multiple spaces/newlines
	s = strings.ReplaceAll(s, "\n", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.TrimSpace(s)

	// Remove leading # from markdown headers
	for strings.HasPrefix(s, "# ") {
		s = strings.TrimPrefix(s, "# ")
	}

	return s
}

// extractSnippetWindow extracts a window around the match with context before/after.
func extractSnippetWindow(s string, matchStart, matchEnd, contextBefore, contextAfter int) string {
	windowStart := matchStart - contextBefore
	windowEnd := matchEnd + contextAfter

	// Clamp to string bounds
	if windowStart < 0 {
		windowStart = 0
	}
	if windowEnd > len(s) {
		windowEnd = len(s)
	}

	// Adjust to word boundaries (don't cut words in half)
	if windowStart > 0 {
		spaceIdx := strings.Index(s[windowStart:], " ")
		if spaceIdx != -1 && spaceIdx < 10 {
			windowStart += spaceIdx + 1
		}
	}
	if windowEnd < len(s) {
		lastSpace := strings.LastIndex(s[:windowEnd], " ")
		if lastSpace > matchEnd+5 {
			windowEnd = lastSpace
		}
	}

	result := s[windowStart:windowEnd]

	// Add ellipsis if truncated
	if windowStart > 0 {
		result = "..." + result
	}
	if windowEnd < len(s) {
		result = result + "..."
	}

	return strings.TrimSpace(result)
}

func snippetHasCodeBlock(snippet string) bool {
	if strings.Contains(snippet, "```") {
		return true
	}
	lines := strings.Split(snippet, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ") {
			return true
		}
	}
	return false
}

func limitSnippetLines(snippet string, maxLines int) string {
	if maxLines <= 0 {
		return snippet
	}
	lines := strings.Split(snippet, "\n")
	if len(lines) <= maxLines {
		return snippet
	}
	trimmed := strings.Join(lines[:maxLines], "\n")
	return trimmed + "\n..."
}

func normalizeInlineCodeSnippet(snippet string, truncated bool) string {
	if !truncated || !strings.Contains(snippet, "`") {
		return snippet
	}
	if countUnescapedBackticks(snippet)%2 == 0 {
		return snippet
	}

	out := snippet
	if strings.HasPrefix(strings.TrimLeft(out, " \t\n"), "...") {
		out = removeFirstUnescapedBacktick(out)
	}
	if countUnescapedBackticks(out)%2 == 0 {
		return out
	}
	if snippetHasEllipsis(out) {
		return insertBacktickBeforeSuffixEllipsis(out)
	}
	return removeLastUnescapedBacktick(out)
}

func snippetHasEllipsis(snippet string) bool {
	trimmed := strings.TrimRight(snippet, " \t")
	return strings.HasSuffix(trimmed, "...") || strings.HasSuffix(trimmed, "\n...")
}

func countUnescapedBackticks(s string) int {
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '`' && !isEscapedBacktick(s, i) {
			count++
		}
	}
	return count
}

func removeFirstUnescapedBacktick(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '`' && !isEscapedBacktick(s, i) {
			return s[:i] + s[i+1:]
		}
	}
	return s
}

func removeLastUnescapedBacktick(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '`' && !isEscapedBacktick(s, i) {
			return s[:i] + s[i+1:]
		}
	}
	return s
}

func insertBacktickBeforeSuffixEllipsis(s string) string {
	trimmed := strings.TrimRight(s, " \t")
	suffixIdx := -1
	if strings.HasSuffix(trimmed, "\n...") {
		suffixIdx = len(trimmed) - len("...")
	} else if strings.HasSuffix(trimmed, "...") {
		suffixIdx = len(trimmed) - len("...")
	}
	if suffixIdx == -1 {
		return s + "`"
	}
	tail := s[len(trimmed):]
	return trimmed[:suffixIdx] + "`" + trimmed[suffixIdx:] + tail
}

func isEscapedBacktick(s string, idx int) bool {
	if idx <= 0 {
		return false
	}
	return s[idx-1] == '\\'
}

func PrintBacklinksResults(target string, refs []model.Reference, links RetrievalLinks) {
	printReferenceResults(
		"Backlinks to "+target,
		fmt.Sprintf("No backlinks found for '%s'", target),
		refs,
		links,
		func(link model.Reference) string {
			displayText := link.SourceID
			if link.DisplayText != nil {
				displayText = *link.DisplayText
			}
			return displayText
		},
	)
}

func PrintBacklinksGroups(groups []model.BacklinksGroup, errors []model.ReferenceInputError, links RetrievalLinks) {
	for i, group := range groups {
		if i > 0 {
			fmt.Println()
		}
		PrintBacklinksResults(group.Target, group.Items, links)
	}
	printReferenceInputErrors(errors)
}

func PrintOutlinksResults(source string, refs []model.Reference, links RetrievalLinks) {
	printReferenceResults(
		"Outlinks from "+source,
		fmt.Sprintf("No outlinks found for '%s'", source),
		refs,
		links,
		func(link model.Reference) string {
			target := link.TargetRaw
			if link.DisplayText != nil && *link.DisplayText != "" && *link.DisplayText != link.TargetRaw {
				target = fmt.Sprintf("%s (%s)", *link.DisplayText, link.TargetRaw)
			}
			return target
		},
	)
}

func PrintOutlinksGroups(groups []model.OutlinksGroup, errors []model.ReferenceInputError, links RetrievalLinks) {
	for i, group := range groups {
		if i > 0 {
			fmt.Println()
		}
		PrintOutlinksResults(group.Source, group.Items, links)
	}
	printReferenceInputErrors(errors)
}

func printReferenceResults(title, emptyMessage string, refs []model.Reference, links RetrievalLinks, displayText func(model.Reference) string) {
	if len(refs) == 0 {
		fmt.Println(Star(emptyMessage))
		return
	}

	fmt.Printf("%s %s\n\n", SectionHeader(title), Badge(fmt.Sprintf("%d", len(refs))))

	display := NewDisplayContext()
	table := NewResultsTable(display, BacklinksLayout())

	for i, link := range refs {
		line := referenceLine(link)
		location := links.location(link.FilePath, line)

		table.AddRow(ResultRow{
			Num:      i + 1,
			Cells:    []string{FormatRowNum(i+1, len(refs)), displayText(link), location},
			Location: fmt.Sprintf("%s:%d", link.FilePath, line),
		})
	}

	fmt.Println(table.Render())
}

func printReferenceInputErrors(errors []model.ReferenceInputError) {
	if len(errors) == 0 {
		return
	}
	fmt.Println()
	fmt.Printf("%s %s\n", SectionHeader("Errors"), Badge(fmt.Sprintf("%d", len(errors))))
	for _, err := range errors {
		message := strings.TrimSpace(err.Message)
		if message == "" {
			message = "failed"
		}
		fmt.Println(Warningf("%s: %s", err.Input, message))
	}
}

func referenceLine(link model.Reference) int {
	if link.Line == nil {
		return 0
	}
	return *link.Line
}
