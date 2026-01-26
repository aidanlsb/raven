package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
)

func printQueryObjectResults(queryStr, typeName string, results []query.PipelineObjectResult, sch *schema.Schema) {
	if len(results) == 0 {
		fmt.Println(ui.Starf("No objects found for: %s", queryStr))
		return
	}

	fmt.Printf("%s %s\n\n", ui.Header(typeName), ui.Hint(fmt.Sprintf("(%d)", len(results))))
	printObjectTable(results, sch)
}

func printQueryTraitResults(queryStr, traitName string, results []query.PipelineTraitResult) {
	if len(results) == 0 {
		fmt.Println(ui.Starf("No traits found for: %s", queryStr))
		return
	}

	fmt.Printf("%s %s\n\n", ui.Header("@"+traitName), ui.Hint(fmt.Sprintf("(%d)", len(results))))

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

		// Truncate content to fit two lines if needed
		if len(content) > maxContentLen {
			content = ui.TruncateWithEllipsis(content, maxContentLen)
		}

		// Highlight traits in content
		content = ui.HighlightTraits(content)

		location := formatLocationLinkSimple(r.FilePath, r.Line)

		table.AddRow(ui.ResultRow{
			Num:      i + 1,
			Cells:    []string{ui.FormatRowNum(i+1, len(results)), content, traitStr, location},
			Location: fmt.Sprintf("%s:%d", r.FilePath, r.Line),
		})
	}

	fmt.Println(table.Render())
}

func pipeItemsForObjectResults(results []query.PipelineObjectResult) []PipeableItem {
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

func pipeItemsForTraitResults(results []query.PipelineTraitResult) []PipeableItem {
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

func pipelineObjectResultsFromObjects(objects []model.Object) []query.PipelineObjectResult {
	results := make([]query.PipelineObjectResult, len(objects))
	for i, obj := range objects {
		results[i] = query.PipelineObjectResult{
			Object:   obj,
			Computed: make(map[string]interface{}),
		}
	}
	return results
}

func pipelineTraitResultsFromTraits(traits []model.Trait) []query.PipelineTraitResult {
	results := make([]query.PipelineTraitResult, len(traits))
	for i, trait := range traits {
		results[i] = query.PipelineTraitResult{
			Trait:    trait,
			Computed: make(map[string]interface{}),
		}
	}
	return results
}

func printSearchResults(queryStr string, results []model.SearchMatch) {
	if len(results) == 0 {
		fmt.Println(ui.Starf("No results found for: %s", queryStr))
		return
	}

	fmt.Printf("%s %s\n\n", ui.Header(queryStr), ui.Hint(fmt.Sprintf("(%d results)", len(results))))

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

	s := cleanRawSnippet(snippet)
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

	return extractSnippetWindow(s, matchStart, matchEnd, contextBefore, contextAfter)
}

// cleanRawSnippet removes frontmatter, collapses whitespace, and returns clean text.
func cleanRawSnippet(snippet string) string {
	s := snippet

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

func printBacklinksResults(target string, links []model.Reference) {
	if len(links) == 0 {
		fmt.Println(ui.Starf("No backlinks found for '%s'", target))
		return
	}

	fmt.Printf("%s %s\n\n", ui.Header("Backlinks to "+target), ui.Hint(fmt.Sprintf("(%d)", len(links))))

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
