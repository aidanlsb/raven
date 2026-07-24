package readsvc

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/model"
)

// BacklinkOptions controls path variants considered by a backlink read.
type BacklinkOptions struct {
	ObjectsRoot string
	PagesRoot   string
}

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
	return BacklinksWithOptions(rt, target, BacklinkOptions{})
}

// BacklinksWithOptions reads inbound references without exposing the index
// handle. The runtime database is opened on demand.
func BacklinksWithOptions(rt *Runtime, target string, opts BacklinkOptions) ([]model.Reference, error) {
	if rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	if err := rt.OpenDB(); err != nil {
		return nil, err
	}
	return rt.DB.BacklinksWithRoots(target, opts.ObjectsRoot, opts.PagesRoot)
}

func Outlinks(rt *Runtime, source string) ([]model.Reference, error) {
	if rt == nil || rt.DB == nil {
		return nil, fmt.Errorf("runtime with database is required")
	}
	return rt.DB.Outlinks(source)
}
