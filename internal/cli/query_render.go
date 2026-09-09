package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/querysvc"
	"github.com/aidanlsb/raven/internal/ui"
)

// renderCanonicalQueryHuman renders a successful canonical query response as
// human-readable output (tables, IDs, counts, or an interactive picker). The
// canonical handler emits a typed commandpayload result, so this dispatcher
// type-asserts the payload rather than rehydrating a generic map envelope. The
// canonical handler already loaded and validated the schema; the schema.Load
// calls here are best-effort display enrichment.
func renderCanonicalQueryHuman(queryStr string, data interface{}, browse bool) error {
	switch payload := data.(type) {
	case commandpayload.QueryCountResult:
		fmt.Println(payload.Total)
		return nil
	case commandpayload.QueryIDsResult:
		for _, id := range payload.IDs {
			fmt.Println(id)
		}
		return nil
	case commandpayload.QueryObjectResult:
		objects := objectsFromItems(payload.Items)
		label := queryLabelOrParse(payload.Type, queryStr)
		if browse {
			if len(objects) == 0 {
				sch, _ := loadSchemaSafe(getVaultPath())
				printQueryObjectResults(queryStr, label, objects, sch)
				return nil
			}
			sch, _ := loadSchemaSafe(getVaultPath())
			return browseQueryResults(browseItemsForObjectResults(objects, sch), objectBrowseHeaders(objects, sch), objectBrowseLayout(objects, sch))
		}
		if ShouldUsePipeFormat() {
			WritePipeableList(os.Stdout, pipeItemsForObjectResults(objects))
			return nil
		}
		sch, _ := loadSchemaSafe(getVaultPath())
		printQueryObjectResults(queryStr, label, objects, sch)
		return nil
	case commandpayload.QueryTraitResult:
		traits := traitsFromItems(payload.Items)
		label := queryLabelOrParse(payload.Trait, queryStr)
		if browse {
			if len(traits) == 0 {
				printQueryTraitResults(queryStr, label, traits)
				return nil
			}
			return browseQueryResults(browseItemsForTraitResults(traits), traitBrowseHeaders(), ui.TraitLayout())
		}
		if ShouldUsePipeFormat() {
			WritePipeableList(os.Stdout, pipeItemsForTraitResults(traits))
			return nil
		}
		printQueryTraitResults(queryStr, label, traits)
		return nil
	case commandpayload.QuerySectionResult:
		sections := sectionsFromItems(payload.Items)
		if browse {
			if len(sections) == 0 {
				printQuerySectionResults(queryStr, sections)
				return nil
			}
			return browseQueryResults(browseItemsForSectionResults(sections), sectionBrowseHeaders(), ui.SearchLayout())
		}
		if ShouldUsePipeFormat() {
			WritePipeableList(os.Stdout, pipeItemsForSectionResults(sections))
			return nil
		}
		printQuerySectionResults(queryStr, sections)
		return nil
	case commandpayload.QueryLinkResult:
		links := linksFromItems(payload.Items)
		if browse {
			if len(links) == 0 {
				printQueryLinkResults(queryStr, links)
				return nil
			}
			return browseQueryResults(browseItemsForLinkResults(links), linkBrowseHeaders(), ui.SearchLayout())
		}
		if ShouldUsePipeFormat() {
			WritePipeableList(os.Stdout, pipeItemsForLinkResults(links))
			return nil
		}
		printQueryLinkResults(queryStr, links)
		return nil
	default:
		return handleErrorMsg(ErrInternal, "unexpected query result shape", "")
	}
}

// queryLabelOrParse returns the explicit type/trait label from the typed
// payload when present, falling back to the parsed query root otherwise. Saved
// queries carry no type/trait discriminator, so the label is recovered from the
// resolved query string.
func queryLabelOrParse(label, queryStr string) string {
	if label != "" {
		return label
	}
	parsed, err := query.Parse(queryStr)
	if err != nil {
		return ""
	}
	return parsed.TypeName
}

func listSavedQueries(queries []querysvc.SavedQueryInfo) error {
	fmt.Println(ui.SectionHeader("Saved queries"))
	if len(queries) == 0 {
		fmt.Println(ui.Bullet(ui.Hint("(none defined)")))
		fmt.Printf("\n%s\n", ui.Hint("Define queries in raven.yaml under 'queries:'"))
		return nil
	}
	for _, q := range queries {
		desc := q.Description
		if desc == "" {
			desc = q.Query
		}
		if len(q.Args) > 0 {
			fmt.Println(ui.Bullet(fmt.Sprintf("%s %s %s", ui.Bold.Render(q.Name), desc, ui.Hint("(args: "+strings.Join(q.Args, ", ")+")"))))
			continue
		}
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render(q.Name), desc)))
	}
	return nil
}

func savedQueriesFromResult(raw interface{}) []querysvc.SavedQueryInfo {
	if rows, ok := raw.([]map[string]interface{}); ok {
		queries := make([]querysvc.SavedQueryInfo, 0, len(rows))
		for _, entry := range rows {
			queries = append(queries, querysvc.SavedQueryInfo{
				Name:        stringValue(entry["name"]),
				Query:       stringValue(entry["query"]),
				Args:        stringSliceFromAny(entry["args"]),
				Description: stringValue(entry["description"]),
			})
		}
		return queries
	}

	rows, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	queries := make([]querysvc.SavedQueryInfo, 0, len(rows))
	for _, row := range rows {
		entry, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		queries = append(queries, querysvc.SavedQueryInfo{
			Name:        stringValue(entry["name"]),
			Query:       stringValue(entry["query"]),
			Args:        stringSliceFromAny(entry["args"]),
			Description: stringValue(entry["description"]),
		})
	}
	return queries
}

// objectsFromItems adapts the typed query payload items into the model rows the
// shared retrieval renderers consume. It is a thin field copy with no map
// decoding: the canonical handler emits typed commandpayload items in-process.
func objectsFromItems(items []commandpayload.ObjectItem) []model.Object {
	results := make([]model.Object, 0, len(items))
	for _, item := range items {
		results = append(results, model.Object{
			ID:        item.ID,
			Type:      item.Type,
			Fields:    item.Fields,
			FilePath:  item.FilePath,
			LineStart: item.Line,
		})
	}
	return results
}

func traitsFromItems(items []commandpayload.TraitItem) []model.Trait {
	results := make([]model.Trait, 0, len(items))
	for _, item := range items {
		trait := model.Trait{
			ID:            item.ID,
			TraitType:     item.TraitType,
			Content:       item.Content,
			FilePath:      item.FilePath,
			Line:          item.Line,
			ParentScopeID: item.ScopeID,
		}
		trait.SetIndexValueString(item.Value)
		results = append(results, trait)
	}
	return results
}

func sectionsFromItems(items []commandpayload.SectionItem) []model.Section {
	results := make([]model.Section, 0, len(items))
	for _, item := range items {
		results = append(results, model.Section{
			ID:              item.ID,
			FileObjectID:    item.FileObjectID,
			FilePath:        item.FilePath,
			Slug:            item.Slug,
			Title:           item.Title,
			Level:           item.Level,
			LineStart:       item.LineStart,
			LineEnd:         item.LineEnd,
			SubtreeLineEnd:  item.SubtreeLineEnd,
			ParentSectionID: item.ParentSectionID,
		})
	}
	return results
}

func linksFromItems(items []commandpayload.LinkItem) []model.Link {
	results := make([]model.Link, 0, len(items))
	for _, item := range items {
		results = append(results, model.Link{
			SourceID:      item.SourceID,
			SourceType:    item.SourceType,
			FilePath:      item.FilePath,
			Line:          item.Line,
			PositionStart: item.PositionStart,
			PositionEnd:   item.PositionEnd,
			RawTarget:     item.RawTarget,
			Display:       item.Display,
			IsImage:       item.IsImage,
			Scheme:        item.Scheme,
			Ext:           item.Ext,
			NormalizedKey: item.NormalizedKey,
		})
	}
	return results
}
