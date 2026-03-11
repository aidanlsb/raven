package readsvc

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/query"
)

type ExecuteQueryRequest struct {
	QueryString string
	IDsOnly     bool
	Limit       int
	Offset      int
	CountOnly   bool
}

type ExecuteQueryResult struct {
	QueryType string
	TypeName  string
	Total     int
	Returned  int
	Offset    int
	Limit     int
	IDs       []string
	Objects   []model.Object
	Traits    []model.Trait
}

func ExecuteQuery(rt *Runtime, req ExecuteQueryRequest) (*ExecuteQueryResult, error) {
	if rt == nil || rt.DB == nil {
		return nil, fmt.Errorf("runtime with database is required")
	}
	if req.Limit < 0 {
		return nil, fmt.Errorf("limit must be >= 0")
	}
	if req.Offset < 0 {
		return nil, fmt.Errorf("offset must be >= 0")
	}

	q, err := query.Parse(req.QueryString)
	if err != nil {
		return nil, err
	}

	if rt.Schema != nil {
		validator := query.NewValidator(rt.Schema)
		if err := validator.Validate(q); err != nil {
			return nil, err
		}
	}

	executor := query.NewExecutor(rt.DB.DB())
	if res, err := rt.DB.Resolver(index.ResolverOptions{
		DailyDirectory: rt.VaultCfg.GetDailyDirectory(),
		Schema:         rt.Schema,
	}); err == nil {
		executor.SetResolver(res)
	}
	executor.SetSchema(rt.Schema)

	queryType := "trait"
	if q.Type == query.QueryTypeObject {
		queryType = "object"
	}
	result := &ExecuteQueryResult{
		QueryType: queryType,
		TypeName:  q.TypeName,
		Offset:    req.Offset,
		Limit:     req.Limit,
	}

	if q.Type == query.QueryTypeObject {
		rows, err := executor.ExecuteObjectQuery(q)
		if err != nil {
			return nil, err
		}
		windowed := applyQueryWindow(rows, req.Offset, req.Limit)

		result.Total = len(rows)
		if req.CountOnly {
			return result, nil
		}

		if req.IDsOnly {
			ids := make([]string, 0, len(windowed))
			for _, row := range windowed {
				ids = append(ids, row.ID)
			}
			result.IDs = ids
			result.Returned = len(ids)
			return result, nil
		}

		result.Objects = windowed
		result.Returned = len(windowed)
		return result, nil
	}

	rows, err := executor.ExecuteTraitQuery(q)
	if err != nil {
		return nil, err
	}
	windowed := applyQueryWindow(rows, req.Offset, req.Limit)

	result.Total = len(rows)
	if req.CountOnly {
		return result, nil
	}

	if req.IDsOnly {
		ids := make([]string, 0, len(windowed))
		for _, row := range windowed {
			ids = append(ids, row.ID)
		}
		result.IDs = ids
		result.Returned = len(ids)
		return result, nil
	}

	result.Traits = windowed
	result.Returned = len(windowed)
	return result, nil
}

func applyQueryWindow[T any](items []T, offset, limit int) []T {
	if offset >= len(items) {
		return []T{}
	}

	window := items[offset:]
	if limit > 0 && limit < len(window) {
		window = window[:limit]
	}
	return window
}
