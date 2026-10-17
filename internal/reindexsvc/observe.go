package reindexsvc

import (
	"context"
	"path"
	"strings"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// DefaultObserveBurst is the number of distinct paths that promote a
// watch batch from per-file projection to incremental reindex.
const DefaultObserveBurst = 32

// ObserveRequest is a filesystem-observed batch of vault-relative paths.
type ObserveRequest struct {
	Paths       []string
	Overflow    bool
	BurstLimit  int
	Context     context.Context
	Incremental bool
}

// ObserveFiles projects watcher-observed vault changes through the journal
// and index. Small batches use ProjectObservedChanges. Overflow, schema or
// config edits, and large bursts run an incremental reindex.
func ObserveFiles(rt *vaultruntime.Runtime, req ObserveRequest) error {
	if rt == nil {
		return nil
	}
	if req.Context != nil {
		select {
		case <-req.Context.Done():
			return req.Context.Err()
		default:
		}
	}

	burstLimit := req.BurstLimit
	if burstLimit <= 0 {
		burstLimit = DefaultObserveBurst
	}

	relPaths := uniqueProjectionPaths(req.Paths)
	schemaChanged, configChanged := false, false
	var filePaths []string
	for _, relPath := range relPaths {
		switch {
		case isSchemaPath(relPath):
			schemaChanged = true
		case isVaultConfigPath(relPath):
			configChanged = true
		default:
			filePaths = append(filePaths, relPath)
		}
	}

	if schemaChanged {
		if err := rt.ReloadSchema(true); err != nil {
			return err
		}
	}
	if configChanged {
		if err := rt.ReloadConfig(); err != nil {
			return err
		}
	}

	incremental := req.Incremental || req.Overflow || schemaChanged || configChanged || len(filePaths) >= burstLimit
	if incremental {
		_, err := Run(rt, RunRequest{Context: req.Context})
		return err
	}

	if err := rt.OpenDB(); err != nil {
		return err
	}

	changes := mutation.NewChangeSet()
	for _, relPath := range filePaths {
		if !paths.HasMDExtension(relPath) {
			continue
		}
		action, err := rt.DB.FileProjectionAction(rt.VaultPath, relPath)
		if err != nil {
			return err
		}
		switch action {
		case index.FileProjectionSkip:
			continue
		case index.FileProjectionRemove:
			changes.AddDeleted(relPath)
		default:
			changes.AddChanged(relPath)
		}
	}
	if changes.Empty() {
		return nil
	}
	_ = ProjectObservedChanges(rt, changes, "")
	return nil
}

func isSchemaPath(relPath string) bool {
	return path.Base(relPath) == paths.SchemaFilename && !strings.Contains(relPath, "/")
}

const vaultConfigFilename = "raven.yaml"

func isVaultConfigPath(relPath string) bool {
	return path.Base(relPath) == vaultConfigFilename && !strings.Contains(relPath, "/")
}
