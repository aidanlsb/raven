package commandimpl

import (
	"testing"

	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/querysvc"
)

func TestShapeQueryResultPreservesSavedQueryPagination(t *testing.T) {
	t.Parallel()

	result := shapeQueryResult(&querysvc.ExecuteResult{
		QueryKind: "type",
		TypeName:  "project",
		Total:     3,
		Returned:  1,
		Offset:    1,
		Limit:     1,
		Objects: []model.Object{{
			ID:        "projects/raven",
			Type:      "project",
			FilePath:  "projects/raven.md",
			LineStart: 1,
		}},
	}, queryResultShapeOptions{
		IsSavedQuery:   true,
		SavedQueryName: "active-projects",
		QueryTimeMs:    42,
	})

	if !result.OK {
		t.Fatalf("shapeQueryResult() failed: %#v", result.Error)
	}
	payload, ok := result.Data.(commandpayload.QueryObjectResult)
	if !ok {
		t.Fatalf("Data type = %T, want commandpayload.QueryObjectResult", result.Data)
	}
	if payload.SavedQuery != "active-projects" || payload.Type != "" {
		t.Fatalf("saved_query/type = %q/%q, want active-projects/empty", payload.SavedQuery, payload.Type)
	}
	if !payload.HasMore || payload.NextOffset == nil || *payload.NextOffset != 2 {
		t.Fatalf("pagination = %#v, want has_more with next_offset 2", payload.Pagination)
	}
	if result.Meta == nil || result.Meta.Count != 1 || result.Meta.QueryTimeMs != 42 {
		t.Fatalf("meta = %#v, want count 1 and query_time_ms 42", result.Meta)
	}
}
