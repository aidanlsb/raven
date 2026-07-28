package query

import "github.com/aidanlsb/raven/internal/model"

// RunRequest selects the execution mode for Run. It mirrors the read-service
// request so callers can express count-only, ids-only, and paginated reads
// without repeating the per-root dispatch.
type RunRequest struct {
	IDsOnly   bool
	CountOnly bool
	Limit     int
	Offset    int
}

// RunResult is the unified result of Run. Only the slice matching the query
// root is populated (plus IDs for ids-only mode); the others stay nil.
type RunResult struct {
	Total    int
	Returned int
	IDs      []string
	Objects  []model.Object
	Traits   []model.Trait
	Assets   []model.Asset
	Sections []model.Section
	Links    []model.Link
}

// Run executes a parsed query in the requested mode against any root
// (object/trait/asset/section/link). It is the single execution entry point for the
// read service: the count / ids-only / paginated / full branching lives here
// once instead of being copy-pasted per root.
//
// Pagination semantics match the previous per-root code exactly:
//   - count-only returns Total only.
//   - ids-only returns IDs and Returned; Total is a COUNT(*) when paginated,
//     otherwise len(IDs).
//   - row modes return the rows and Returned; Total is a COUNT(*) when
//     paginated, otherwise len(rows).
func (e *Executor) Run(q *Query, req RunRequest) (*RunResult, error) {
	scoped := e.withExecutionNow()
	result := &RunResult{}
	paginated := req.Limit > 0 || req.Offset > 0

	if req.CountOnly {
		total, err := scoped.entityCount(q)
		if err != nil {
			return nil, err
		}
		result.Total = total
		return result, nil
	}

	if req.IDsOnly {
		ids, err := scoped.entityIDs(q, req.Limit, req.Offset)
		if err != nil {
			return nil, err
		}
		result.IDs = ids
		result.Returned = len(ids)
		if paginated {
			total, err := scoped.entityCount(q)
			if err != nil {
				return nil, err
			}
			result.Total = total
		} else {
			result.Total = len(ids)
		}
		return result, nil
	}

	returned, err := scoped.entityRows(q, req.Limit, req.Offset, result)
	if err != nil {
		return nil, err
	}
	result.Returned = returned
	if paginated {
		total, err := scoped.entityCount(q)
		if err != nil {
			return nil, err
		}
		result.Total = total
	} else {
		result.Total = returned
	}
	return result, nil
}

func (e *Executor) entityCount(q *Query) (int, error) {
	spec, err := specForQueryType(q.Type)
	if err != nil {
		return 0, err
	}
	return runEntityCount(e, q, spec)
}

func (e *Executor) entityIDs(q *Query, limit, offset int) ([]string, error) {
	spec, err := specForQueryType(q.Type)
	if err != nil {
		return nil, err
	}
	return runEntityIDs(e, q, spec, limit, offset)
}

// entityRows runs the row-returning query for q and stores the typed rows in
// the matching RunResult field, returning the number of rows returned.
func (e *Executor) entityRows(q *Query, limit, offset int, result *RunResult) (int, error) {
	switch q.Type {
	case QueryTypeObject:
		rows, err := runEntityPageRows(e, q, objectSpec, limit, offset, scanObjectRows)
		if err != nil {
			return 0, err
		}
		result.Objects = rows
		return len(rows), nil
	case QueryTypeTrait:
		rows, err := runEntityPageRows(e, q, traitSpec, limit, offset, scanTraitRows)
		if err != nil {
			return 0, err
		}
		result.Traits = rows
		return len(rows), nil
	case QueryTypeAsset:
		rows, err := runEntityPageRows(e, q, assetSpec, limit, offset, scanAssetRows)
		if err != nil {
			return 0, err
		}
		result.Assets = rows
		return len(rows), nil
	case QueryTypeSection:
		rows, err := runEntityPageRows(e, q, sectionSpec, limit, offset, scanSectionRows)
		if err != nil {
			return 0, err
		}
		result.Sections = rows
		return len(rows), nil
	case QueryTypeLink:
		rows, err := runEntityPageRows(e, q, linkSpec, limit, offset, scanLinkRows)
		if err != nil {
			return 0, err
		}
		result.Links = rows
		return len(rows), nil
	default:
		return 0, errUnexpectedQueryType(QueryTypeObject, q.Type)
	}
}
