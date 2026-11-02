package readsvc

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// BacklinkOptions controls path variants considered by a backlink read.
type BacklinkOptions struct {
	ObjectsRoot string
	PagesRoot   string
}

func Search(rt *vaultruntime.Runtime, queryStr, objectType string, limit, offset int) ([]model.SearchMatch, int, error) {
	if rt == nil || rt.DB == nil {
		return nil, 0, fmt.Errorf("runtime with database is required")
	}
	return rt.DB.SearchPage(queryStr, objectType, limit, offset)
}

func Backlinks(rt *vaultruntime.Runtime, target string) ([]model.Reference, error) {
	return BacklinksWithOptions(rt, target, BacklinkOptions{})
}

// BacklinksWithOptions reads inbound references without exposing the index
// handle. The runtime database is opened on demand.
func BacklinksWithOptions(rt *vaultruntime.Runtime, target string, opts BacklinkOptions) ([]model.Reference, error) {
	if rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	if err := rt.OpenDB(); err != nil {
		return nil, err
	}
	return rt.DB.BacklinksWithRoots(target, opts.ObjectsRoot, opts.PagesRoot)
}

func Outlinks(rt *vaultruntime.Runtime, source string) ([]model.Reference, error) {
	if rt == nil || rt.DB == nil {
		return nil, fmt.Errorf("runtime with database is required")
	}
	return rt.DB.Outlinks(source)
}
