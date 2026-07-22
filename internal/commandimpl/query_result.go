package commandimpl

import (
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/readsvc"
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

func shapeQueryResult(result *readsvc.ExecuteQueryResult, options queryResultShapeOptions) commandexec.Result {
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
		case "asset", "section":
			// Asset and section counts carry no type/trait discriminator.
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
	case "asset":
		meta.Count = result.Returned
		payload := commandpayload.QueryAssetResult{
			QueryKind:  "asset",
			Items:      assetQueryItems(result),
			Pagination: queryPagination(result),
		}
		if options.IsSavedQuery && options.SavedQueryName != "" {
			payload.SavedQuery = options.SavedQueryName
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
func queryPagination(result *readsvc.ExecuteQueryResult) commandpayload.Pagination {
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

func objectQueryItems(result *readsvc.ExecuteQueryResult) []commandpayload.ObjectItem {
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

func traitQueryItems(result *readsvc.ExecuteQueryResult) []commandpayload.TraitItem {
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

func assetQueryItems(result *readsvc.ExecuteQueryResult) []commandpayload.AssetItem {
	items := make([]commandpayload.AssetItem, len(result.Assets))
	for i, row := range result.Assets {
		items[i] = commandpayload.AssetItem{
			Num:       result.Offset + i + 1,
			ID:        row.ID,
			FilePath:  row.FilePath,
			Filename:  row.Filename,
			Extension: row.Extension,
			MediaType: row.MediaType,
			SizeBytes: row.SizeBytes,
		}
	}
	return items
}

func sectionQueryItems(result *readsvc.ExecuteQueryResult) []commandpayload.SectionItem {
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
