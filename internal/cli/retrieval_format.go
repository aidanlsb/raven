package cli

import (
	"fmt"
	"path/filepath"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
)

func retrievalLinks() ui.RetrievalLinks {
	return ui.RetrievalLinks{
		Location: func(relPath string, line int) string {
			return formatLocationLinkSimpleStyled(relPath, line, ui.Muted.Render)
		},
		VaultFile: func(label, key string, line int) string {
			return formatVaultFileLinkSimpleStyled(label, key, line, nil)
		},
	}
}

func printQueryObjectResults(queryStr, typeName string, results []model.Object, sch *schema.Schema) {
	ui.PrintQueryObjectResults(queryStr, typeName, results, sch, retrievalLinks())
}

func printQueryTraitResults(queryStr, traitName string, results []model.Trait) {
	ui.PrintQueryTraitResults(queryStr, traitName, results, retrievalLinks())
}

func printQuerySectionResults(queryStr string, results []model.Section) {
	ui.PrintQuerySectionResults(queryStr, results, retrievalLinks())
}

func printQueryLinkResults(queryStr string, results []model.Link) {
	ui.PrintQueryLinkResults(queryStr, results, retrievalLinks())
}

func printSearchResults(queryStr string, results []model.SearchMatch) {
	ui.PrintSearchResults(queryStr, results, retrievalLinks())
}

func printObjectTable(results []model.Object, sch *schema.Schema) {
	ui.PrintObjectTable(results, sch, retrievalLinks())
}

func printBacklinksResults(target string, links []model.Reference) {
	ui.PrintBacklinksResults(target, links, retrievalLinks())
}

func printBacklinksGroups(groups []model.BacklinksGroup, errors []model.ReferenceInputError) {
	ui.PrintBacklinksGroups(groups, errors, retrievalLinks())
}

func printOutlinksResults(source string, links []model.Reference) {
	ui.PrintOutlinksResults(source, links, retrievalLinks())
}

func printOutlinksGroups(groups []model.OutlinksGroup, errors []model.ReferenceInputError) {
	ui.PrintOutlinksGroups(groups, errors, retrievalLinks())
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

func pipeItemsForSectionResults(results []model.Section) []PipeableItem {
	pipeItems := make([]PipeableItem, len(results))
	for i, r := range results {
		pipeItems[i] = PipeableItem{
			Num:      i + 1,
			ID:       r.ID,
			Content:  r.Title,
			Location: fmt.Sprintf("%s:%d", r.FilePath, r.LineStart),
		}
	}
	return pipeItems
}

func pipeItemsForLinkResults(results []model.Link) []PipeableItem {
	pipeItems := make([]PipeableItem, len(results))
	for i, r := range results {
		content := r.Display
		if content == "" {
			content = r.RawTarget
		} else if content != r.RawTarget {
			content += " (" + r.RawTarget + ")"
		}
		pipeItems[i] = PipeableItem{
			Num:      i + 1,
			ID:       r.SourceID,
			Content:  content,
			Location: fmt.Sprintf("%s:%d", r.FilePath, r.Line),
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

func referenceInputErrorsFromAny(raw interface{}) []model.ReferenceInputError {
	switch values := raw.(type) {
	case []model.ReferenceInputError:
		return values
	case []interface{}:
		out := make([]model.ReferenceInputError, 0, len(values))
		for _, value := range values {
			entry, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			out = append(out, model.ReferenceInputError{
				Input:      stringValue(entry["input"]),
				Code:       stringValue(entry["code"]),
				Message:    stringValue(entry["message"]),
				Suggestion: stringValue(entry["suggestion"]),
				Details:    entry["details"],
			})
		}
		return out
	default:
		return nil
	}
}
