package commandimpl

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestMissingRefWarning_InferredType(t *testing.T) {
	t.Parallel()

	w := missingRefWarning(&check.MissingRef{TargetPath: "people/ghost", InferredType: "person"})

	if w.Code != codes.WarnRefTargetMissing {
		t.Fatalf("code = %q, want %q", w.Code, codes.WarnRefTargetMissing)
	}
	if w.SuggestedType != "person" {
		t.Fatalf("suggested_type = %q, want person", w.SuggestedType)
	}
	if w.CreateCommand == "" {
		t.Fatal("expected create_command hint, got empty")
	}
	if w.CreateInvoke == nil {
		t.Fatal("expected structured create_invoke, got nil")
	}
	if w.CreateInvoke.Command != "new" {
		t.Fatalf("create_invoke.command = %q, want new", w.CreateInvoke.Command)
	}
	if got := w.CreateInvoke.Args["type"]; got != "person" {
		t.Fatalf("create_invoke.args.type = %#v, want person", got)
	}
	if got := w.CreateInvoke.Args["path"]; got != "people/ghost" {
		t.Fatalf("create_invoke.args.path = %#v, want people/ghost", got)
	}
	if got := w.CreateInvoke.Args["title"]; got != "ghost" {
		t.Fatalf("create_invoke.args.title = %#v, want ghost", got)
	}
}

func TestIndexProjectionFailureWarningsRecordRecoveryScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		projectionErr error
		wantCode      codes.WarningCode
		wantFullScan  bool
		wantPaths     []string
	}{
		{
			name:          "pre-commit failure keeps concrete path",
			projectionErr: errors.New("index write failed"),
			wantCode:      codes.WarnIndexUpdateFailed,
			wantPaths:     []string{"notes/changed.md"},
		},
		{
			name: "post-commit vault resolution failure widens scope",
			projectionErr: &index.PostCommitReferenceResolutionError{
				FilePath:  "notes/changed.md",
				VaultWide: true,
				Err:       errors.New("reference resolution failed"),
			},
			wantCode:     codes.WarnRefResolutionIncomplete,
			wantFullScan: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			vaultPath := t.TempDir()
			rt := &vaultruntime.Runtime{VaultPath: vaultPath}
			filePath := filepath.Join(vaultPath, "notes", "changed.md")
			warnings := indexProjectionFailureWarnings(rt, filePath, "update index", tc.projectionErr)
			if len(warnings) != 1 || warnings[0].Code != tc.wantCode {
				t.Fatalf("warnings = %#v, want code %q", warnings, tc.wantCode)
			}

			pending, err := indexjournal.Load(vaultPath)
			if err != nil {
				t.Fatalf("load index journal: %v", err)
			}
			if !pending.Dirty() || pending.RequiresFullScan() != tc.wantFullScan {
				t.Fatalf("pending = %#v, want full scan %v", pending, tc.wantFullScan)
			}
			if got := pending.Paths(); !equalStrings(got, tc.wantPaths) {
				t.Fatalf("pending paths = %#v, want %#v", got, tc.wantPaths)
			}
		})
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestMissingRefWarning_UnknownTypeFallsBackToCheck(t *testing.T) {
	t.Parallel()

	w := missingRefWarning(&check.MissingRef{TargetPath: "misc/thing"})

	if w.Code != codes.WarnRefTargetMissing {
		t.Fatalf("code = %q, want %q", w.Code, codes.WarnRefTargetMissing)
	}
	if w.SuggestedType != "" {
		t.Fatalf("suggested_type = %q, want empty", w.SuggestedType)
	}
	if w.CreateInvoke == nil {
		t.Fatal("expected structured create_invoke, got nil")
	}
	if w.CreateInvoke.Command != "check create-missing" {
		t.Fatalf("create_invoke.command = %q, want check create-missing", w.CreateInvoke.Command)
	}
	if got := w.CreateInvoke.Args["confirm"]; got != true {
		t.Fatalf("create_invoke.args.confirm = %#v, want true", got)
	}
}

func TestAutoReindexWarnings_ClassifiesFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mutate      func(t *testing.T, vaultPath, filePath string)
		wantMessage string
	}{
		{
			name: "schema load failure",
			mutate: func(t *testing.T, vaultPath, _ string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("version: [\n"), 0o644); err != nil {
					t.Fatalf("write invalid schema: %v", err)
				}
			},
			wantMessage: "failed to load schema",
		},
		{
			name: "parse failure",
			mutate: func(t *testing.T, _, filePath string) {
				t.Helper()
				if err := os.WriteFile(filePath, []byte("---\ntype: person\nname: [\n---\n"), 0o644); err != nil {
					t.Fatalf("write invalid content: %v", err)
				}
			},
			wantMessage: "failed to parse file",
		},
		{
			name: "database open failure",
			mutate: func(t *testing.T, vaultPath, _ string) {
				t.Helper()
				ravenDir := filepath.Join(vaultPath, ".raven")
				if err := os.RemoveAll(ravenDir); err != nil {
					t.Fatalf("remove .raven: %v", err)
				}
				if err := os.WriteFile(ravenDir, []byte("not a directory"), 0o644); err != nil {
					t.Fatalf("write .raven file: %v", err)
				}
			},
			wantMessage: "failed to open index database",
		},
		{
			name: "index update failure",
			mutate: func(t *testing.T, vaultPath, _ string) {
				t.Helper()
				db, err := index.Open(vaultPath)
				if err != nil {
					t.Fatalf("open index: %v", err)
				}
				db.Close()

				sqlDB, err := sql.Open("sqlite", filepath.Join(vaultPath, ".raven", "index.db"))
				if err != nil {
					t.Fatalf("open sqlite db: %v", err)
				}
				defer sqlDB.Close()
				if _, err := sqlDB.Exec(`
					CREATE TRIGGER fail_objects_insert
					BEFORE INSERT ON objects
					BEGIN
						SELECT RAISE(ABORT, 'index write failed');
					END;
				`); err != nil {
					t.Fatalf("create trigger: %v", err)
				}
			},
			wantMessage: "failed to update index",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := testutil.NewTestVault(t).
				WithSchema(testutil.PersonProjectSchema()).
				WithFile("people/alice.md", `---
type: person
name: Alice
---

Body
`).
				Build()

			filePath := filepath.Join(v.Path, "people", "alice.md")
			tc.mutate(t, v.Path, filePath)

			rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
			warnings := autoReindexWarnings(rt, filePath)
			if len(warnings) != 1 {
				t.Fatalf("warnings = %#v, want 1 warning", warnings)
			}
			if warnings[0].Code != indexUpdateFailedWarningCode {
				t.Fatalf("warning code = %q, want %q", warnings[0].Code, indexUpdateFailedWarningCode)
			}
			if warnings[0].Ref != indexUpdateFailedWarningRef {
				t.Fatalf("warning ref = %q, want %q", warnings[0].Ref, indexUpdateFailedWarningRef)
			}
			if !strings.Contains(warnings[0].Message, tc.wantMessage) {
				t.Fatalf("warning message = %q, want substring %q", warnings[0].Message, tc.wantMessage)
			}
		})
	}
}
