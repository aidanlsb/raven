package refresolve

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestNormalizeServiceErrorPreservesResolverDetails(t *testing.T) {
	t.Parallel()
	err := &AmbiguousRefError{
		Reference: "raven", Matches: []string{"projects/raven", "notes/raven"},
		MatchSources: map[string]string{"projects/raven": "alias"},
	}
	serviceErr, ok := svcerr.AsError(NormalizeServiceError(err, "raven"))
	if !ok || serviceErr.Code != codes.ErrRefAmbiguous {
		t.Fatalf("error = %#v, want REF_AMBIGUOUS", serviceErr)
	}
	if serviceErr.Details["reference"] != "raven" || serviceErr.Details["match_sources"] == nil {
		t.Fatalf("details = %#v, want resolver details", serviceErr.Details)
	}
}

func TestNormalizeServiceErrorPreservesNotFoundDetail(t *testing.T) {
	t.Parallel()
	serviceErr, ok := svcerr.AsError(NormalizeServiceError(
		&RefNotFoundError{Reference: "missing", Detail: "not indexed"},
		"missing",
	))
	if !ok || serviceErr.Code != codes.ErrRefNotFound ||
		serviceErr.Message != "reference 'missing' not found: not indexed" ||
		serviceErr.Suggestion == "" {
		t.Fatalf("error = %#v", serviceErr)
	}
}

func TestNormalizeServiceErrorClassifiesStaleIndex(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("resolver unavailable: %w", &vaultruntime.SetupError{
		Stage: vaultruntime.StageDatabase, Failure: vaultruntime.SetupFailureIndexRebuildRequired,
		Err: errors.New("stale index"),
	})
	serviceErr, ok := svcerr.AsError(NormalizeServiceError(err, "target"))
	if !ok || serviceErr.Code != codes.ErrDatabaseVersion {
		t.Fatalf("error = %#v, want DATABASE_VERSION_MISMATCH", serviceErr)
	}
}
