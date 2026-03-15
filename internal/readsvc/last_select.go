package readsvc

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/aidanlsb/raven/internal/lastquery"
	"github.com/aidanlsb/raven/internal/lastresults"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/query"
)

type NoLastResultsError struct{}

func (e *NoLastResultsError) Error() string {
	return "no results available"
}

type InvalidSelectionError struct {
	Message string
	Total   int
}

func (e *InvalidSelectionError) Error() string {
	if e == nil {
		return "invalid selection"
	}
	if e.Message != "" {
		return e.Message
	}
	return "invalid selection"
}

type SelectLastRequest struct {
	VaultPath  string
	NumberArgs []string
}

type SelectLastResult struct {
	Last     *lastresults.LastResults
	Selected []model.Result
	Numbers  []int
}

func SelectLast(req SelectLastRequest) (*SelectLastResult, error) {
	if req.VaultPath == "" {
		return nil, fmt.Errorf("vault path is required")
	}

	lr, err := lastresults.Read(req.VaultPath)
	if err != nil {
		if errors.Is(err, lastresults.ErrNoLastResults) {
			return nil, &NoLastResultsError{}
		}
		return nil, err
	}

	if len(req.NumberArgs) == 0 {
		selected, err := lr.DecodeAll()
		if err != nil {
			return nil, err
		}
		numbers := make([]int, 0, len(selected))
		for i := range selected {
			numbers = append(numbers, i+1)
		}
		return &SelectLastResult{Last: lr, Selected: selected, Numbers: numbers}, nil
	}

	nums, err := lastquery.ParseNumberArgs(req.NumberArgs)
	if err != nil {
		return nil, &InvalidSelectionError{Message: err.Error(), Total: len(lr.Results)}
	}
	entries, err := lr.GetByNumbers(nums)
	if err != nil {
		return nil, &InvalidSelectionError{Message: err.Error(), Total: len(lr.Results)}
	}

	return &SelectLastResult{Last: lr, Selected: entries, Numbers: nums}, nil
}

func QueryTypeFromLast(lr *lastresults.LastResults, results []model.Result) (string, error) {
	if len(results) > 0 {
		kind := results[0].GetKind()
		if kind == "trait" || kind == "object" {
			return kind, nil
		}
	}
	if lr == nil || lr.Query == "" {
		return "", fmt.Errorf("missing query string for last results")
	}
	q, err := query.Parse(lr.Query)
	if err != nil {
		return "", err
	}
	if q.Type == query.QueryTypeTrait {
		return "trait", nil
	}
	return "object", nil
}

func ResultContentForOutput(result model.Result) string {
	switch r := result.(type) {
	case model.Object:
		return filepath.Base(r.ID)
	case *model.Object:
		return filepath.Base(r.ID)
	default:
		return result.GetContent()
	}
}
