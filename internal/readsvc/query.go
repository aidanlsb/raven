package readsvc

import (
	"fmt"

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
	QueryKind string
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
	executor.SetDailyDirectory(rt.VaultCfg.GetDailyDirectory())
	executor.SetSchema(rt.Schema)

	queryKind := "trait"
	if q.Type == query.QueryTypeObject {
		queryKind = "type"
	}
	result := &ExecuteQueryResult{
		QueryKind: queryKind,
		TypeName:  q.TypeName,
		Offset:    req.Offset,
		Limit:     req.Limit,
	}
	paginated := req.Limit > 0 || req.Offset > 0

	if q.Type == query.QueryTypeObject {
		if req.CountOnly {
			total, err := executor.ExecuteObjectCountQuery(q)
			if err != nil {
				return nil, err
			}
			result.Total = total
			return result, nil
		}

		if req.IDsOnly {
			ids, err := executor.ExecuteObjectIDQuery(q, req.Limit, req.Offset)
			if err != nil {
				return nil, err
			}
			if paginated {
				total, err := executor.ExecuteObjectCountQuery(q)
				if err != nil {
					return nil, err
				}
				result.Total = total
			} else {
				result.Total = len(ids)
			}
			result.IDs = ids
			result.Returned = len(ids)
			return result, nil
		}

		if paginated {
			total, err := executor.ExecuteObjectCountQuery(q)
			if err != nil {
				return nil, err
			}
			rows, err := executor.ExecuteObjectPageQuery(q, req.Limit, req.Offset)
			if err != nil {
				return nil, err
			}
			result.Total = total
			result.Objects = rows
			result.Returned = len(rows)
			return result, nil
		}

		rows, err := executor.ExecuteObjectQuery(q)
		if err != nil {
			return nil, err
		}
		result.Total = len(rows)
		result.Objects = rows
		result.Returned = len(rows)
		return result, nil
	}

	if req.CountOnly {
		total, err := executor.ExecuteTraitCountQuery(q)
		if err != nil {
			return nil, err
		}
		result.Total = total
		return result, nil
	}

	if req.IDsOnly {
		ids, err := executor.ExecuteTraitIDQuery(q, req.Limit, req.Offset)
		if err != nil {
			return nil, err
		}
		if paginated {
			total, err := executor.ExecuteTraitCountQuery(q)
			if err != nil {
				return nil, err
			}
			result.Total = total
		} else {
			result.Total = len(ids)
		}
		result.IDs = ids
		result.Returned = len(ids)
		return result, nil
	}

	if paginated {
		total, err := executor.ExecuteTraitCountQuery(q)
		if err != nil {
			return nil, err
		}
		rows, err := executor.ExecuteTraitPageQuery(q, req.Limit, req.Offset)
		if err != nil {
			return nil, err
		}
		result.Total = total
		result.Traits = rows
		result.Returned = len(rows)
		return result, nil
	}

	rows, err := executor.ExecuteTraitQuery(q)
	if err != nil {
		return nil, err
	}
	result.Total = len(rows)
	result.Traits = rows
	result.Returned = len(rows)
	return result, nil
}
