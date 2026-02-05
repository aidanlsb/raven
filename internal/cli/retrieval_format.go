package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
)

func printQueryObjectResults(queryStr, typeName string, results []model.Object, sch *schema.Schema) {
	if len(results) == 0 {
		fmt.Println(ui.Starf("No objects found for: %s", queryStr))
		return
	}

	fmt.Printf("%s %s\n\n", ui.SectionHeader(typeName), ui.Badge(fmt.Sprintf("%d", len(results))))
	printObjectTable(results, sch)
}

func printQueryTraitResults(queryStr, traitName string, results []model.Trait) {
	if len(results) == 0 {
		fmt.Println(ui.Starf("No traits found for: %s", queryStr))
		return
	}

	fmt.Printf("%s %s\n\n", ui.SectionHeader("@"+traitName), ui.Badge(fmt.Sprintf("%d", len(results))))

	display := ui.NewDisplayContext()
	table := ui.NewResultsTable(display, ui.TraitLayout)

	// Get the calculated content width for dynamic content sizing
	// Use 2x width to allow for two-line content
	contentWidth := table.ContentWidth("content")
	maxContentLen := contentWidth * 2

	for i, r := range results {
		value := ""
		if r.Value != nil && *r.Value != r.TraitType {
			value = *r.Value
		}
		traitStr := ui.Trait(r.TraitType, value)

		content := r.Content
		if content == "" {
			content = "(no content)"
		}

		truncated := false
		// Truncate content to fit two lines if needed
		if len(content) > maxContentLen {
			if snippetHasCodeBlock(content) {
				maxCodeLen := maxContentLen * 3
				if len(content) > maxCodeLen {
					content = ui.TruncateWithEllipsis(content, maxCodeLen)
					truncated = true
				}
			} else {
				content = ui.TruncateWithEllipsis(content, maxContentLen)
				truncated = true
			}
		}

		content = normalizeInlineCodeSnippet(content, truncated)

		// Highlight traits in content
		content = ui.HighlightTraits(content)

		location := formatLocationLinkSimpleStyled(r.FilePath, r.Line, ui.Muted.Render)

		table.AddRow(ui.ResultRow{
			Num:      i + 1,
			Cells:    []string{ui.FormatRowNum(i+1, len(results)), content, traitStr, location},
			Location: fmt.Sprintf("%s:%d", r.FilePath, r.Line),
		})
	}

	fmt.Println(table.Render())
}

func pipeItemsForObjectResults(results []model.Object) []PipeableItem {
	pipeItems := make([]PipeableItem, len(results))
	for i, r := range results {
		pipeItems[i] = PipeableItem{
			Num:      i + 1,
			ID:       r.ID,
			Content:  filepath.Base(r.ID),
			Location: fmt.Sprintf("%s:%d", r.FilePath, r.LineStart),
		}
	}
	return pipeItems
}

func pipeItemsForTraitResults(results []model.Trait) []PipeableItem {
	pipeItems := make([]PipeableItem, len(results))
	for i, r := range results {
		pipeItems[i] = PipeableItem{
			Num:      i + 1,
			ID:       r.ID,
			Content:  TruncateContent(r.Content, 60),
			Location: fmt.Sprintf("%s:%d", r.FilePath, r.Line),
		}
	}
	return pipeItems
}

func printSearchResults(queryStr string, results []model.SearchMatch) {
	if len(results) == 0 {
		fmt.Println(ui.Starf("No results found for: %s", queryStr))
		return
	}

	fmt.Printf("%s %s\n\n", ui.SectionHeader(queryStr), ui.Badge(fmt.Sprintf("%d results", len(results))))

	display := ui.NewDisplayContext()
	table := ui.NewResultsTable(display, ui.SearchLayout)

	// Get the calculated content width for dynamic snippet sizing
	// Use 2x width to allow for two-line content
	contentWidth := table.ContentWidth("content")
	maxSnippetLen := contentWidth * 2 // two lines of content

	for i, result := range results {
		// Content/snippet - clean and prepare with dynamic sizing
		snippet := cleanSearchSnippetDynamic(result.Snippet, maxSnippetLen)
		if snippet == "" {
			snippet = "(no match preview)"
		}
		// Remove the match markers
		snippet = strings.ReplaceAll(snippet, "»", "")
		snippet = strings.ReplaceAll(snippet, "«", "")
		snippet = normalizeInlineCodeSnippet(snippet, snippetHasEllipsis(snippet))

		// Parent/title - use meta column width
		metaWidth := table.ContentWidth("meta")
		title := result.Title
		if len(title) > metaWidth-3 {
			title = title[:metaWidth-6] + "..."
		}

		// File path (just filename) - use file column width
		fileWidth := table.ContentWidth("file")
		filePath := filepath.Base(result.FilePath)
		if len(filePath) > fileWidth-3 {
			filePath = filePath[:fileWidth-6] + "..."
		}

		table.AddRow(ui.ResultRow{
			Num:      i + 1,
			Cells:    []string{ui.FormatRowNum(i+1, len(results)), snippet, title, filePath},
			Location: fmt.Sprintf("%s:%d", result.FilePath, 1),
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

func printBacklinksResults(target string, links []model.Reference) {
	if len(links) == 0 {
		fmt.Println(ui.Starf("No backlinks found for '%s'", target))
		return
	}

	fmt.Printf("%s %s\n\n", ui.SectionHeader("Backlinks to "+target), ui.Badge(fmt.Sprintf("%d", len(links))))

	display := ui.NewDisplayContext()
	table := ui.NewResultsTable(display, ui.BacklinksLayout)

	for i, link := range links {
		displayText := link.SourceID
		if link.DisplayText != nil {
			displayText = *link.DisplayText
		}

		line := 0
		if link.Line != nil {
			line = *link.Line
		}

		location := formatLocationLinkSimple(link.FilePath, line)

		table.AddRow(ui.ResultRow{
			Num:      i + 1,
			Cells:    []string{ui.FormatRowNum(i+1, len(links)), displayText, location},
			Location: fmt.Sprintf("%s:%d", link.FilePath, line),
		})
	}

	fmt.Println(table.Render())
}

func printOutlinksResults(source string, links []model.Reference) {
	if len(links) == 0 {
		fmt.Println(ui.Starf("No outlinks found for '%s'", source))
		return
	}

	fmt.Printf("%s %s\n\n", ui.SectionHeader("Outlinks from "+source), ui.Badge(fmt.Sprintf("%d", len(links))))

	display := ui.NewDisplayContext()
	table := ui.NewResultsTable(display, ui.BacklinksLayout)

	for i, link := range links {
		target := link.TargetRaw
		if link.DisplayText != nil && *link.DisplayText != "" && *link.DisplayText != link.TargetRaw {
			target = fmt.Sprintf("%s (%s)", *link.DisplayText, link.TargetRaw)
		}

		line := 0
		if link.Line != nil {
			line = *link.Line
		}

		location := formatLocationLinkSimple(link.FilePath, line)

		table.AddRow(ui.ResultRow{
			Num:      i + 1,
			Cells:    []string{ui.FormatRowNum(i+1, len(links)), target, location},
			Location: fmt.Sprintf("%s:%d", link.FilePath, line),
		})
	}

	fmt.Println(table.Render())
}
