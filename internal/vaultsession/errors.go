package vaultsession

import (
	"errors"
	"fmt"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func mapSetupError(err error) error {
	if err == nil {
		return svcerr.New(codes.ErrInternal, "failed to initialize runtime")
	}
	if mapped, ok := mapIndexRebuildRequired(err); ok {
		return mapped
	}
	if errors.Is(err, vaultruntime.ErrVaultPathRequired) {
		return svcerr.New(codes.ErrVaultNotSpecified, "no vault path resolved").
			WithSuggestion("Use --vault-path, --vault, active_vault, or default_vault")
	}

	var setupErr *vaultruntime.SetupError
	if errors.As(err, &setupErr) {
		switch setupErr.Stage {
		case vaultruntime.StageConfig:
			return svcerr.Wrap(codes.ErrConfigInvalid, "failed to load raven.yaml", setupErr.Err).
				WithSuggestion("Fix raven.yaml and try again")
		case vaultruntime.StageSchema:
			return svcerr.Wrap(codes.ErrSchemaInvalid, "failed to load schema", setupErr.Err).
				WithSuggestion("Fix schema.yaml and try again")
		case vaultruntime.StageDatabase:
			if errors.Is(setupErr.Err, index.ErrIndexLocked) {
				return svcerr.Wrap(codes.ErrIndexLocked, "index is locked for rebuild", setupErr.Err).
					WithSuggestion("Wait for the other process to finish and retry")
			}
			return svcerr.Wrap(codes.ErrDatabase, "failed to open database", setupErr.Err).
				WithSuggestion("Run 'rvn reindex' to rebuild the database")
		}
	}

	return svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err).
		WithSuggestion("Resolve a vault before invoking the command")
}

func mapIndexRebuildRequired(err error) (error, bool) {
	var setupErr *vaultruntime.SetupError
	if errors.As(err, &setupErr) && setupErr.Failure == vaultruntime.SetupFailureIndexRebuildRequired {
		return svcerr.Wrap(codes.ErrDatabaseVersion, "index schema is stale or a rebuild was interrupted", err).
			WithSuggestion("Run 'rvn reindex --full' to rebuild the index"), true
	}
	return nil, false
}

func mapRefreshError(err error) error {
	if err == nil {
		return svcerr.New(codes.ErrDatabase, "failed to refresh index").
			WithSuggestion("Run 'rvn reindex' to rebuild the database")
	}
	if _, ok := svcerr.AsError(err); ok {
		return err
	}
	if mapped, ok := mapIndexRebuildRequired(err); ok {
		return mapped
	}
	return svcerr.Wrap(codes.ErrDatabase, fmt.Sprintf("failed to refresh index: %v", err), err).
		WithSuggestion("Run 'rvn reindex' to rebuild the database")
}

func mapJournalBeginError(err error) error {
	return svcerr.Wrap(codes.ErrDatabase, "failed to establish index recovery guard", err).
		WithDetails(map[string]any{"cause": err.Error()}).
		WithSuggestion("Restore write access to .raven and retry; no vault content was changed")
}
