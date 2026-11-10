// Package vaultsession owns one vault-scoped command invocation: config/schema
// load policy, index open/close, read-time refresh, mutation journal begin
// /abandon, and post-mutation index projection.
package vaultsession

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// Session is invocation-scoped. Callers must Close it.
type Session struct {
	rt              *vaultruntime.Runtime
	refreshWarnings []commandexec.Warning
}

// Open loads a vault session for mode. Errors are *svcerr.Error with the
// stable setup codes handlers already surface through FromServiceError.
func Open(path string, mode Mode) (*Session, error) {
	path = strings.TrimSpace(path)
	rt, err := openRuntime(path, mode)
	if err != nil {
		return nil, mapSetupError(err)
	}
	sess := &Session{rt: rt}
	if mode.kind == modeRead {
		if err := sess.refresh(mode.refresh); err != nil {
			rt.Close()
			return nil, err
		}
		if mode.openDB {
			if err := sess.ensureDB(); err != nil {
				rt.Close()
				return nil, err
			}
		}
	} else if mode.openDB {
		if err := sess.ensureDB(); err != nil {
			rt.Close()
			return nil, err
		}
	}
	return sess, nil
}

func openRuntime(path string, mode Mode) (*vaultruntime.Runtime, error) {
	switch mode.kind {
	case modeRequired:
		return vaultruntime.New(path, vaultruntime.Options{RequireSchema: true})
	case modeConfig:
		return vaultruntime.New(path, vaultruntime.Options{})
	case modeConfigOnly:
		return vaultruntime.New(path, vaultruntime.Options{SkipSchema: true})
	case modeSchemaMutation:
		return openSchemaMutation(path)
	case modeSchemaFirst:
		return vaultruntime.New(path, vaultruntime.Options{RequireSchema: true, SchemaFirst: true})
	case modeLazy:
		return vaultruntime.New(path, vaultruntime.Options{SkipConfig: true, SkipSchema: true})
	case modeRead:
		return vaultruntime.New(path, vaultruntime.Options{})
	default:
		return vaultruntime.New(path, vaultruntime.Options{})
	}
}

func openSchemaMutation(path string) (*vaultruntime.Runtime, error) {
	rt, err := vaultruntime.New(path, vaultruntime.Options{
		OpenDB:        true,
		RequireSchema: true,
	})
	if err == nil {
		return rt, nil
	}
	var setupErr *vaultruntime.SetupError
	if errors.As(err, &setupErr) && setupErr.Stage == vaultruntime.StageConfig {
		rt, err = vaultruntime.New(path, vaultruntime.Options{
			OpenDB:        true,
			SkipConfig:    true,
			RequireSchema: true,
		})
		if err == nil {
			return rt, nil
		}
	}
	return nil, err
}

func (s *Session) refresh(policy RefreshPolicy) error {
	if s == nil || s.rt == nil {
		return svcerrInternal("vault runtime is required")
	}
	if !readsvc.ShouldRefresh(s.rt, policy.NoRefresh, policy.Refresh) || !readsvc.CanRefresh(s.rt) {
		return nil
	}
	report, err := readsvc.EnsureIndexFresh(s.rt)
	if err != nil {
		return mapRefreshError(err)
	}
	s.refreshWarnings = refreshCommandWarnings(report)
	return nil
}

func (s *Session) ensureDB() error {
	if s == nil || s.rt == nil {
		return svcerrInternal("vault runtime is required")
	}
	if s.rt.DB != nil {
		return nil
	}
	if err := s.rt.OpenDB(); err != nil {
		return mapSetupError(err)
	}
	return nil
}

func svcerrInternal(message string) error {
	return svcerr.New(codes.ErrInternal, message)
}

func refreshCommandWarnings(report readsvc.SmartReindexReport) []commandexec.Warning {
	issues := readsvc.RefreshWarnings(report)
	if len(issues) == 0 {
		return nil
	}
	warnings := make([]commandexec.Warning, 0, len(issues))
	for _, issue := range issues {
		warnings = append(warnings, commandexec.Warning{
			Code: issue.Code, Message: issue.Message, Ref: issue.Ref,
		})
	}
	return warnings
}

// Runtime exposes the assembled vault runtime for domain services.
func (s *Session) Runtime() *vaultruntime.Runtime {
	if s == nil {
		return nil
	}
	return s.rt
}

// RefreshWarnings are non-fatal issues from the pre-read index refresh.
func (s *Session) RefreshWarnings() []commandexec.Warning {
	if s == nil {
		return nil
	}
	return s.refreshWarnings
}

// Refresh incrementally reindexes according to policy. HandleRead uses this
// after Config() so a skipped refresh does not force an index open.
func (s *Session) Refresh(policy RefreshPolicy) error {
	return s.refresh(policy)
}

// SchemaLoadWarning is the stable SCHEMA_LOAD_FAILED warning when Open
// tolerated a corrupt schema. Empty when schema loaded or was required.
func (s *Session) SchemaLoadWarning() (commandexec.Warning, bool) {
	if s == nil || s.rt == nil || s.rt.SchemaLoadErr == nil {
		return commandexec.Warning{}, false
	}
	return commandexec.Warning{
		Code:    codes.WarnSchemaLoadFailed,
		Message: fmt.Sprintf("schema failed to load; type-aware output may be incomplete: %v", s.rt.SchemaLoadErr),
	}, true
}

// Close releases the session. Safe on nil.
func (s *Session) Close() {
	if s == nil || s.rt == nil {
		return
	}
	s.rt.Close()
	s.rt = nil
}

// Mutate begins the write-ahead index journal, runs fn, and projects the
// returned ChangeSet. On fn error the journal is abandoned (unknown guard
// preserved). Callers skip Mutate for preview paths that do not write.
func (s *Session) Mutate(fn func(*vaultruntime.Runtime) (mutation.ChangeSet, error)) (MutationResult, error) {
	if s == nil || s.rt == nil {
		return MutationResult{}, svcerrInternal("vault runtime is required")
	}
	return mutate(s.rt, fn)
}

// Apply runs fn under Mutate and returns fn's value alongside the projection.
func Apply[T any](s *Session, fn func(*vaultruntime.Runtime) (T, mutation.ChangeSet, error)) (T, MutationResult, error) {
	var value T
	proj, err := s.Mutate(func(rt *vaultruntime.Runtime) (mutation.ChangeSet, error) {
		var cs mutation.ChangeSet
		var fnErr error
		value, cs, fnErr = fn(rt)
		return cs, fnErr
	})
	return value, proj, err
}
