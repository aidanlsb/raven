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

	rows := make([]traitTableRow, len(results))
	for i, r := range results {
		value := ""
		if r.Value != nil && *r.Value != r.TraitType {
			value = *r.Value
		}
		traitStr := ui.Trait(r.TraitType, value)

		rows[i] = traitTableRow{
			num:      i + 1, // 1-indexed for user reference
			content:  r.Content,
			traits:   traitStr,
			location: formatLocationLinkSimple(r.FilePath, r.Line),
		}
	}
	printTraitRows(rows)
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

func printSearchResults(query string, results []model.SearchMatch) {
	if len(results) == 0 {
		fmt.Println(ui.Starf("No results found for: %s", query))
		return
	}

	fmt.Printf("%s %s\n\n", ui.Header(query), ui.Hint(fmt.Sprintf("(%d results)", len(results))))
	for i, result := range results {
		fmt.Printf("%s %s\n", ui.Bold.Render(fmt.Sprintf("%d.", i+1)), result.Title)
		fmt.Printf("   %s\n", formatLocationLinkSimple(result.FilePath, 1))
		if result.Snippet != "" {
			snippet := strings.ReplaceAll(result.Snippet, "\n", " ")
			snippet = strings.TrimSpace(snippet)
			if len(snippet) > 120 {
				snippet = snippet[:120] + "..."
			}
			fmt.Printf("   %s\n", snippet)
		}
		fmt.Println()
	}
}

func printBacklinksResults(target string, links []model.Reference) {
	if len(links) == 0 {
		fmt.Println(ui.Starf("No backlinks found for '%s'", target))
		return
	}

	fmt.Printf("%s %s\n\n", ui.Header("Backlinks to "+target), ui.Hint(fmt.Sprintf("(%d)", len(links))))
	for _, link := range links {
		display := link.SourceID
		if link.DisplayText != nil {
			display = *link.DisplayText
		}

		line := 0
		if link.Line != nil {
			line = *link.Line
		}

		fmt.Printf("  %s %s %s\n", ui.SymbolAttention, display, formatLocationLinkSimple(link.FilePath, line))
	}
}
