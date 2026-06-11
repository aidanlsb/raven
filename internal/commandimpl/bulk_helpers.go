package commandimpl

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/paths"
)

const warnSectionSkipped = codes.WarnSectionSkipped

type canonicalBulkResult struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Details string `json:"details,omitempty"`
}

type canonicalBulkPreviewItem struct {
	ID      string            `json:"id"`
	Changes map[string]string `json:"changes,omitempty"`
	Action  string            `json:"action"`
	Details string            `json:"details,omitempty"`
}

func commandIDsArg(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, raw := range stringSliceArg(args[key]) {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if strings.Contains(id, "\t") {
			parts := strings.SplitN(id, "\t", 3)
			if len(parts) >= 2 {
				id = strings.TrimSpace(parts[1])
			}
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func stringSliceArg(raw any) []string {
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		parts := strings.Split(s, "\n")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		return out
	default:
		return nil
	}
}

func splitSectionIDs(ids []string) ([]string, []string) {
	fileIDs := make([]string, 0, len(ids))
	sectionIDs := make([]string, 0)
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, _, ok := paths.ParseSectionID(id); ok {
			sectionIDs = append(sectionIDs, id)
			continue
		}
		fileIDs = append(fileIDs, id)
	}
	return fileIDs, sectionIDs
}

func sectionSkipWarnings(sectionIDs []string) []commandexec.Warning {
	if len(sectionIDs) == 0 {
		return nil
	}
	return []commandexec.Warning{{
		Code:    warnSectionSkipped,
		Message: fmt.Sprintf("Skipped %d section ID(s) - bulk operations only support file-level objects", len(sectionIDs)),
		Ref:     strings.Join(sectionIDs, ", "),
	}}
}

func canonicalAddPreviewItems(items []objectsvc.AddBulkPreviewItem) []canonicalBulkPreviewItem {
	out := make([]canonicalBulkPreviewItem, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkPreviewItem{
			ID:      item.ID,
			Action:  item.Action,
			Details: item.Details,
		})
	}
	return out
}

func canonicalAddResults(items []objectsvc.AddBulkResult) []canonicalBulkResult {
	out := make([]canonicalBulkResult, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkResult{
			ID:     item.ID,
			Status: item.Status,
			Reason: item.Reason,
		})
	}
	return out
}

func canonicalSetPreviewItems(items []objectsvc.SetBulkPreviewItem) []canonicalBulkPreviewItem {
	out := make([]canonicalBulkPreviewItem, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkPreviewItem{
			ID:      item.ID,
			Action:  item.Action,
			Changes: item.Changes,
		})
	}
	return out
}

func canonicalSetResults(items []objectsvc.SetBulkResult) []canonicalBulkResult {
	out := make([]canonicalBulkResult, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkResult{
			ID:     item.ID,
			Status: item.Status,
			Reason: item.Reason,
		})
	}
	return out
}

func canonicalDeletePreviewItems(items []objectsvc.DeleteBulkPreviewItem) []canonicalBulkPreviewItem {
	out := make([]canonicalBulkPreviewItem, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkPreviewItem{
			ID:      item.ID,
			Action:  item.Action,
			Details: item.Details,
			Changes: item.Changes,
		})
	}
	return out
}

func canonicalDeleteResults(items []objectsvc.DeleteBulkResult) []canonicalBulkResult {
	out := make([]canonicalBulkResult, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkResult{
			ID:     item.ID,
			Status: item.Status,
			Reason: item.Reason,
		})
	}
	return out
}

func canonicalMovePreviewItems(items []objectsvc.MoveBulkPreviewItem) []canonicalBulkPreviewItem {
	out := make([]canonicalBulkPreviewItem, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkPreviewItem{
			ID:      item.ID,
			Action:  item.Action,
			Details: item.Details,
		})
	}
	return out
}

func canonicalMoveResults(items []objectsvc.MoveBulkResult) []canonicalBulkResult {
	out := make([]canonicalBulkResult, 0, len(items))
	for _, item := range items {
		out = append(out, canonicalBulkResult{
			ID:      item.ID,
			Status:  item.Status,
			Reason:  item.Reason,
			Details: item.Details,
		})
	}
	return out
}
