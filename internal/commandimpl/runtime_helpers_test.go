package commandimpl

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/testutil"
)

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

			vaultCfg, err := config.LoadVaultConfig(v.Path)
			if err != nil {
				t.Fatalf("load vault config: %v", err)
			}

			filePath := filepath.Join(v.Path, "people", "alice.md")
			tc.mutate(t, v.Path, filePath)

			warnings := autoReindexWarnings(v.Path, vaultCfg, filePath)
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
