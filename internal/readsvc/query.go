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
	Sections  []model.Section
	Links     []model.Link
}

// HasMore reports whether more results exist beyond the returned window.
// It is derived from the authoritative Total together with the current
// Offset and the number of rows Returned. For unlimited queries (no limit
// and no offset) Total equals Returned, so HasMore is false.
func (r *ExecuteQueryResult) HasMore() bool {
	if r == nil {
		return false
	}
	return r.Offset+r.Returned < r.Total
}

// NextOffset returns the offset an agent should use to fetch the next page.
// It is only meaningful when HasMore is true.
func (r *ExecuteQueryResult) NextOffset() int {
	if r == nil {
		return 0
	}
	return r.Offset + r.Returned
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

	// Structural validation is mandatory regardless of schema presence: the
	// root/predicate legality matrix, trait .value rules, and built-in field
	// checks always run. Schema-dependent checks (unknown types/traits/fields)
	// only run when a schema is available. NewValidator tolerates a nil schema.
	if err := query.NewValidator(rt.Schema).Validate(q); err != nil {
		return nil, err
	}

	executor := query.NewExecutor(rt.DB.DB())
	executor.SetDailyDirectory(rt.VaultCfg.GetDailyDirectory())
	executor.SetSchema(rt.Schema)

	runResult, err := executor.Run(q, query.RunRequest{
		IDsOnly:   req.IDsOnly,
		CountOnly: req.CountOnly,
		Limit:     req.Limit,
		Offset:    req.Offset,
	})
	if err != nil {
		return nil, err
	}

	return &ExecuteQueryResult{
		QueryKind: queryKindString(q.Type),
		TypeName:  q.TypeName,
		Total:     runResult.Total,
		Returned:  runResult.Returned,
		Offset:    req.Offset,
		Limit:     req.Limit,
		IDs:       runResult.IDs,
		Objects:   runResult.Objects,
		Traits:    runResult.Traits,
		Sections:  runResult.Sections,
		Links:     runResult.Links,
	}, nil
}

func queryKindString(t query.QueryType) string {
	switch t {
	case query.QueryTypeObject:
		return "type"
	case query.QueryTypeSection:
		return "section"
	case query.QueryTypeLink:
		return "link"
	default:
		return "trait"
	}
}
