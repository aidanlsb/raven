package commandimpl

import (
	"context"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestHandleSchemaAddType_AutoReindexOffWritesJournalWithoutWarning(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML("auto_reindex: false\n").
		Build()

	result := HandleSchemaAddType(context.Background(), commandexec.Request{
		VaultPath: vault.Path,
		Args:      map[string]any{"name": "meeting"},
	})
	if !result.OK {
		t.Fatalf("HandleSchemaAddType failed: %#v", result.Error)
	}
	for _, warning := range result.Warnings {
		if warning.Code == codes.WarnIndexUpdateFailed {
			t.Fatalf("unexpected INDEX_UPDATE_FAILED with auto-reindex off: %#v", result.Warnings)
		}
	}

	journal, err := indexjournal.Load(vault.Path)
	if err != nil {
		t.Fatalf("load journal: %v", err)
	}
	if !journal.Dirty() {
		t.Fatal("expected journal to stay dirty when auto-reindex is off")
	}
}

func TestHandleSchemaAddType_SuccessfulRefreshHasNoWarning(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()

	result := HandleSchemaAddType(context.Background(), commandexec.Request{
		VaultPath: vault.Path,
		Args:      map[string]any{"name": "meeting"},
	})
	if !result.OK {
		t.Fatalf("HandleSchemaAddType failed: %#v", result.Error)
	}
	for _, warning := range result.Warnings {
		if warning.Code == codes.WarnIndexUpdateFailed {
			t.Fatalf("unexpected INDEX_UPDATE_FAILED after successful refresh: %#v", result.Warnings)
		}
	}
}

func TestCanonicalSchemaWarningsIncludesIndexUpdateRef(t *testing.T) {
	t.Parallel()

	warnings := canonicalSchemaWarnings([]schemasvc.Warning{
		schemasvc.NewIndexUpdateFailedWarning(errForWarning("refresh failed")),
	})
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want 1", warnings)
	}
	if warnings[0].Code != codes.WarnIndexUpdateFailed {
		t.Fatalf("code = %q, want %q", warnings[0].Code, codes.WarnIndexUpdateFailed)
	}
	if warnings[0].Ref != reindexsvc.IndexUpdateFailedWarningRef {
		t.Fatalf("ref = %q, want %q", warnings[0].Ref, reindexsvc.IndexUpdateFailedWarningRef)
	}
	if !strings.Contains(warnings[0].Message, "refresh failed") {
		t.Fatalf("message = %q, want refresh error", warnings[0].Message)
	}
}

type warningErr string

func (e warningErr) Error() string { return string(e) }

func errForWarning(message string) error { return warningErr(message) }
