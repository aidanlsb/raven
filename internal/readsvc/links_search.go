package readsvc

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/model"
)

func Search(rt *Runtime, queryStr, objectType string, limit int) ([]model.SearchMatch, error) {
	if rt == nil || rt.DB == nil {
		return nil, fmt.Errorf("runtime with database is required")
	}
	if objectType != "" {
		return rt.DB.SearchWithType(queryStr, objectType, limit)
	}
	return rt.DB.Search(queryStr, limit)
}

func Backlinks(rt *Runtime, target string) ([]model.Reference, error) {
	if rt == nil || rt.DB == nil {
		return nil, fmt.Errorf("runtime with database is required")
	}
	return rt.DB.Backlinks(target)
}

func Outlinks(rt *Runtime, source string) ([]model.Reference, error) {
	if rt == nil || rt.DB == nil {
		return nil, fmt.Errorf("runtime with database is required")
	}
	return rt.DB.Outlinks(source)
}
