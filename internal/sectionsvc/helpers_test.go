package sectionsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func testRuntime(t *testing.T, vaultPath string) *vaultruntime.Runtime {
	t.Helper()
	return testutil.NewVaultRuntime(t, vaultPath, vaultruntime.Options{RequireSchema: true})
}

func indexVaultFiles(t *testing.T, vaultPath string, sch *schema.Schema, relPaths ...string) {
	t.Helper()

	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	defer db.Close()

	for _, relPath := range relPaths {
		fullPath := filepath.Join(vaultPath, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			t.Fatalf("read %s: %v", relPath, err)
		}
		doc, err := parser.ParseDocument(string(content), fullPath, vaultPath)
		if err != nil {
			t.Fatalf("parse %s: %v", relPath, err)
		}
		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("index %s: %v", relPath, err)
		}
	}
}

func assertServiceCode(t *testing.T, err error, want codes.ErrorCode) {
	t.Helper()
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("error = %v (%T), want service error %s", err, err, want)
	}
	if svcErr.Code != want {
		t.Fatalf("error code = %s, want %s (%v)", svcErr.Code, want, err)
	}
}

func assertChangeSetChanged(t *testing.T, changes mutation.ChangeSet, want ...string) {
	t.Helper()
	got := make(map[string]int, len(changes.Changed))
	for _, path := range changes.Changed {
		got[path]++
	}
	for _, path := range want {
		if got[path] == 0 {
			t.Fatalf("ChangeSet.Changed = %#v, missing %q", changes.Changed, path)
		}
		got[path]--
	}
	for path, extra := range got {
		if extra != 0 {
			t.Fatalf("ChangeSet.Changed = %#v, unexpected extra %q", changes.Changed, path)
		}
	}
	if len(changes.Deleted) != 0 || len(changes.Moved) != 0 {
		t.Fatalf("ChangeSet extra ops: deleted=%#v moved=%#v", changes.Deleted, changes.Moved)
	}
}

func assertChangeSetEmpty(t *testing.T, changes mutation.ChangeSet) {
	t.Helper()
	if !changes.Empty() {
		t.Fatalf("ChangeSet = %#v, want empty", changes)
	}
}
