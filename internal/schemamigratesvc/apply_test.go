package schemamigratesvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestWriteStagedFileMapsIncludesSuggestion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}

	_, err := writeStagedFileMaps(
		"Some files may already be converted; review the vault and run 'rvn reindex --full'",
		map[string][]byte{
			filepath.Join(dir, "ok.md"):        []byte("ok\n"),
			filepath.Join(blocker, "child.md"): []byte("nope\n"),
			filepath.Join(dir, "z-later.md"):   []byte("later\n"),
		},
	)
	if err == nil {
		t.Fatal("expected write error")
	}
	var svcErr *svcerr.Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("error = %T %v, want svcerr.Error", err, err)
	}
	if svcErr.Code != codes.ErrFileWrite {
		t.Fatalf("code = %s, want %s", svcErr.Code, codes.ErrFileWrite)
	}
	if !strings.Contains(svcErr.Suggestion, "Some files may already be converted") {
		t.Fatalf("suggestion %q missing convert hint", svcErr.Suggestion)
	}
}

func TestApplyStagedFilesThenInvalidateWritesFilesAndRecordsJournal(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  person:
    fields:
      name: { type: string }
traits: {}
`).
		WithRavenYAML("auto_reindex: false\n").
		WithFile("notes/a.md", "hello\n").
		Build()

	rt := migrationTestRuntime(t, v.Path)
	notePath := filepath.Join(v.Path, "notes/a.md")
	newSchema := []byte(`version: 1
types:
  person:
    fields:
      name: { type: string }
      email: { type: string }
traits: {}
`)

	applied, err := applyStagedFilesThenInvalidate(rt, stagedApply{
		SchemaYAML:    newSchema,
		SchemaApplied: 1,
		FileSets: []map[string][]byte{
			{notePath: []byte("updated\n")},
		},
		WriteSuggestion: "Some files may already be converted; review the vault and run 'rvn reindex --full'",
		ReloadContext:   "convert",
	})
	if err != nil {
		t.Fatalf("applyStagedFilesThenInvalidate: %v", err)
	}
	if applied != 2 {
		t.Fatalf("applied = %d, want 2 (schema + markdown)", applied)
	}
	if got := v.ReadFile("notes/a.md"); got != "updated\n" {
		t.Fatalf("markdown = %q, want updated", got)
	}
	if !strings.Contains(v.ReadFile("schema.yaml"), "email:") {
		t.Fatalf("schema missing email field:\n%s", v.ReadFile("schema.yaml"))
	}

	journal, err := indexjournal.Load(v.Path)
	if err != nil {
		t.Fatalf("load journal: %v", err)
	}
	if !journal.Dirty() {
		t.Fatal("expected journal to stay dirty when auto-reindex is disabled")
	}
}

func TestSortedKeysSharedWithSchemaPlans(t *testing.T) {
	t.Parallel()

	got := schemasvc.SortedKeys(map[string]int{"z": 1, "a": 2, "m": 3})
	want := []string{"a", "m", "z"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("SortedKeys = %v, want %v", got, want)
	}
}
