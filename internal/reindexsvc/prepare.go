package reindexsvc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// preparedRun holds the validated runtime, open index, and locks needed by
// planFiles and apply. Close releases the projection lock then the database.
type preparedRun struct {
	vaultPath      string
	ctx            context.Context
	schema         *schema.Schema
	parseOptions   *parser.ParseOptions
	db             *index.Database
	rebuildSession *index.RebuildSession
	projectionLock *indexjournal.ProjectionLock
	excludeMatcher *ravenignore.Matcher
	incremental    bool
	dryRun         bool
	wasRebuilt     bool
	dailyDir       string
}

func (p *preparedRun) Close() {
	if p == nil {
		return
	}
	if p.projectionLock != nil {
		_ = p.projectionLock.Close()
		p.projectionLock = nil
	}
	if p.rebuildSession != nil {
		_ = p.rebuildSession.Close()
		p.rebuildSession = nil
		p.db = nil
		return
	}
	if p.db != nil {
		_ = p.db.Close()
		p.db = nil
	}
}

func prepare(rt *vaultruntime.Runtime, req RunRequest) (*preparedRun, error) {
	if err := vaultruntime.Require(rt); err != nil {
		message := "vault path is required"
		if rt == nil {
			message = "vault runtime is required"
		}
		return nil, svcerr.Wrap(codes.ErrInvalidInput, message, err)
	}
	vaultPath := strings.TrimSpace(rt.VaultPath)

	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}

	if rt.SchemaLoadErr != nil {
		return nil, svcerr.Wrap(codes.ErrSchemaInvalid, fmt.Sprintf("failed to load schema: %v", rt.SchemaLoadErr), rt.SchemaLoadErr).WithSuggestion("Run 'rvn init' to create a schema")
	}
	if rt.Schema == nil {
		return nil, svcerr.New(codes.ErrSchemaInvalid, "failed to load schema: schema runtime is required").WithSuggestion("Run 'rvn init' to create a schema")
	}
	vaultCfg := rt.VaultCfg
	if vaultCfg == nil {
		return nil, svcerr.New(codes.ErrConfigInvalid, "failed to load raven.yaml: vault config runtime is required").WithSuggestion("Fix raven.yaml and try again")
	}

	db, rebuildSession, wasRebuilt, err := openRunDatabase(vaultPath, req.Full, req.DryRun)
	if err != nil {
		suggestion := "Run 'rvn reindex' to rebuild the database"
		if errors.Is(err, index.ErrIndexLocked) {
			suggestion = "Another process (such as rvn lsp) is holding the index; stop the LSP or wait for the other process to finish, then try again"
		}
		return nil, wrapDB(err, "failed to open database").WithSuggestion(suggestion)
	}

	prep := &preparedRun{
		vaultPath:      vaultPath,
		ctx:            ctx,
		schema:         rt.Schema,
		parseOptions:   rt.ParseOptions,
		db:             db,
		rebuildSession: rebuildSession,
		dryRun:         req.DryRun,
		wasRebuilt:     wasRebuilt,
	}

	incremental := !req.Full
	if wasRebuilt {
		incremental = false
	}
	prep.incremental = incremental

	if !incremental && !req.DryRun {
		if rebuildSession == nil {
			prep.Close()
			err := errors.New("full reindex requires an exclusive rebuild session")
			return nil, svcerr.Wrap(codes.ErrInternal, err.Error(), err)
		}
		if err := rebuildSession.BeginFullRebuild(); err != nil {
			prep.Close()
			return nil, wrapDB(err, "failed to mark index for full reindex")
		}
		if err := db.ClearAllData(); err != nil {
			prep.Close()
			return nil, wrapDB(err, "failed to clear database for full reindex")
		}
	}

	dailyDir := vaultCfg.GetDailyDirectory()
	if dailyDir == "" {
		dailyDir = "daily"
	}
	prep.dailyDir = dailyDir
	db.SetDailyDirectory(dailyDir)
	if !req.DryRun {
		// Bulk reindex always does a full resolver pass after indexing the walk set.
		// Avoid rebuilding whole-vault resolver state once per file on the hot path.
		db.SetAutoResolveRefs(false)
	}

	excludeMatcher, err := ravenignore.NewMatcher(vaultCfg.GetExcludePatterns())
	if err != nil {
		prep.Close()
		return nil, svcerr.Wrap(codes.ErrConfigInvalid, fmt.Sprintf("invalid exclude config: %v", err), err).WithSuggestion("Fix raven.yaml exclude patterns and try again")
	}
	prep.excludeMatcher = excludeMatcher

	if !req.DryRun {
		projectionLock, err := indexjournal.LockProjection(vaultPath)
		if err != nil {
			prep.Close()
			return nil, wrapDB(err, "failed to lock index projection").WithSuggestion("Wait for the active write or refresh to finish, then retry")
		}
		prep.projectionLock = projectionLock
	}

	return prep, nil
}

func openRunDatabase(vaultPath string, full, dryRun bool) (*index.Database, *index.RebuildSession, bool, error) {
	if !full {
		db, err := index.Open(vaultPath)
		if err == nil {
			return db, nil, false, nil
		}
		if !errors.Is(err, index.ErrIndexRebuildRequired) {
			return nil, nil, false, err
		}
	}

	session, err := index.OpenWithRebuild(vaultPath, index.RebuildOptions{DryRun: dryRun})
	if err != nil {
		return nil, nil, false, err
	}
	return session.Database(), session, session.SchemaRebuilt(), nil
}
