package commandimpl

import (
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/querysvc"
)

// queryResultShapeOptions contains only transport-neutral response choices.
// Both CLI and MCP receive the commandpayload values built by shapeQueryResult.
type queryResultShapeOptions struct {
	IDsOnly        bool
	CountOnly      bool
	IsSavedQuery   bool
	SavedQueryName string
	QueryTimeMs    int64
}

func shapeQueryResult(result *querysvc.ExecuteResult, options queryResultShapeOptions) commandexec.Result {
	meta := &commandexec.Meta{QueryTimeMs: options.QueryTimeMs}
	if options.CountOnly {
		meta.Count = result.Total
		payload := commandpayload.QueryCountResult{
			QueryKind: result.QueryKind,
			Total:     result.Total,
		}
		switch result.QueryKind {
		case "trait":
			payload.Trait = result.TypeName
		case "section", "link":
			// Bare-root counts carry no type/trait discriminator.
		default:
			payload.Type = result.TypeName
		}
		return commandexec.Success(payload, meta)
	}

	if options.IDsOnly {
		meta.Count = result.Returned
		return commandexec.Success(commandpayload.QueryIDsResult{
			IDs:        result.IDs,
			Pagination: queryPagination(result),
		}, meta)
	}

	switch result.QueryKind {
	case "type":
		meta.Count = result.Returned
		payload := commandpayload.QueryObjectResult{
			QueryKind:  "type",
			Items:      objectQueryItems(result),
			Pagination: queryPagination(result),
		}
		if options.IsSavedQuery && options.SavedQueryName != "" {
			payload.SavedQuery = options.SavedQueryName
		} else {
			payload.Type = result.TypeName
		}
		return commandexec.Success(payload, meta)
	case "section":
		meta.Count = result.Returned
		payload := commandpayload.QuerySectionResult{
			QueryKind:  "section",
			Items:      sectionQueryItems(result),
			Pagination: queryPagination(result),
		}
		if options.IsSavedQuery && options.SavedQueryName != "" {
			payload.SavedQuery = options.SavedQueryName
		}
		return commandexec.Success(payload, meta)
	case "link":
		meta.Count = result.Returned
		payload := commandpayload.QueryLinkResult{
			QueryKind:  "link",
			Items:      linkQueryItems(result),
			Pagination: queryPagination(result),
		}
		if options.IsSavedQuery && options.SavedQueryName != "" {
			payload.SavedQuery = options.SavedQueryName
		}
		return commandexec.Success(payload, meta)
	default:
		meta.Count = result.Returned
		payload := commandpayload.QueryTraitResult{
			QueryKind:  "trait",
			Items:      traitQueryItems(result),
			Pagination: queryPagination(result),
		}
		if options.IsSavedQuery && options.SavedQueryName != "" {
			payload.SavedQuery = options.SavedQueryName
		} else {
			payload.Trait = result.TypeName
		}
		return commandexec.Success(payload, meta)
	}
}

// queryPagination builds the shared paging affordances for a query response.
// `has_more` is always present alongside total/returned/offset/limit so agents
// can loop without guessing. `next_offset` is a forward cursor included only
// when more results remain. For unlimited queries (the default) total equals
// returned, so has_more is false and no next_offset is emitted.
func queryPagination(result *querysvc.ExecuteResult) commandpayload.Pagination {
	paging := commandpayload.Pagination{
		Total:    result.Total,
		Returned: result.Returned,
		Offset:   result.Offset,
		Limit:    result.Limit,
		HasMore:  result.HasMore(),
	}
	if paging.HasMore {
		next := result.NextOffset()
		paging.NextOffset = &next
	}
	return paging
}

func objectQueryItems(result *querysvc.ExecuteResult) []commandpayload.ObjectItem {
	items := make([]commandpayload.ObjectItem, len(result.Objects))
	for i, row := range result.Objects {
		items[i] = commandpayload.ObjectItem{
			Num:      result.Offset + i + 1,
			ID:       row.ID,
			Type:     row.Type,
			Fields:   row.Fields,
			FilePath: row.FilePath,
			Line:     row.LineStart,
		}
	}
	return items
}

func traitQueryItems(result *querysvc.ExecuteResult) []commandpayload.TraitItem {
	items := make([]commandpayload.TraitItem, len(result.Traits))
	for i, row := range result.Traits {
		items[i] = commandpayload.TraitItem{
			Num:       result.Offset + i + 1,
			ID:        row.ID,
			TraitType: row.TraitType,
			Value:     row.IndexValueString(),
			Content:   row.Content,
			FilePath:  row.FilePath,
			Line:      row.Line,
			ScopeID:   row.ParentScopeID,
		}
	}
	return items
}

func sectionQueryItems(result *querysvc.ExecuteResult) []commandpayload.SectionItem {
	items := make([]commandpayload.SectionItem, len(result.Sections))
	for i, row := range result.Sections {
		items[i] = commandpayload.SectionItem{
			Num:             result.Offset + i + 1,
			ID:              row.ID,
			FileObjectID:    row.FileObjectID,
			FilePath:        row.FilePath,
			Slug:            row.Slug,
			Title:           row.Title,
			Level:           row.Level,
			LineStart:       row.LineStart,
			LineEnd:         row.LineEnd,
			DirectLineEnd:   row.LineEnd,
			SubtreeLineEnd:  row.SubtreeLineEnd,
			ParentSectionID: row.ParentSectionID,
		}
	}
	return items
}

func linkQueryItems(result *querysvc.ExecuteResult) []commandpayload.LinkItem {
	items := make([]commandpayload.LinkItem, len(result.Links))
	for i, row := range result.Links {
		items[i] = commandpayload.LinkItem{
			Num:           result.Offset + i + 1,
			SourceID:      row.SourceID,
			SourceType:    row.SourceType,
			FilePath:      row.FilePath,
			Line:          row.Line,
			PositionStart: row.PositionStart,
			PositionEnd:   row.PositionEnd,
			RawTarget:     row.RawTarget,
			Display:       row.Display,
			IsImage:       row.IsImage,
			Scheme:        row.Scheme,
			Ext:           row.Ext,
			NormalizedKey: row.NormalizedKey,
		}
	}
	return items
}
