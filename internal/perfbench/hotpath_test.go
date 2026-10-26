package perfbench

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/querysvc"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func benchVaultOptions() testutil.SynthVaultOptions {
	return testutil.DefaultSynthVaultOptions()
}

func newRuntime(tb testing.TB, vaultPath string) *vaultruntime.Runtime {
	tb.Helper()
	return testutil.NewRuntimeForTest(tb, vaultPath)
}

func fullReindex(tb testing.TB, vaultPath string) {
	tb.Helper()
	rt := newRuntime(tb, vaultPath)
	defer rt.Close()
	result, err := reindexsvc.Run(rt, reindexsvc.RunRequest{Full: true})
	if err != nil {
		tb.Fatalf("full reindex: %v", err)
	}
	if result.FilesIndexed == 0 {
		tb.Fatal("full reindex indexed 0 files")
	}
}

func BenchmarkHotPathReindexFull(b *testing.B) {
	opts := benchVaultOptions()
	vaultPath := testutil.BuildSynthVault(b, opts)
	b.ReportMetric(float64(opts.FileCount()), "files")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		testutil.WipeIndex(b, vaultPath)
		b.StartTimer()
		fullReindex(b, vaultPath)
	}
}

func BenchmarkHotPathReindexIncrementalUnchanged(b *testing.B) {
	vaultPath := testutil.BuildSynthVault(b, benchVaultOptions())
	fullReindex(b, vaultPath)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt := newRuntime(b, vaultPath)
		result, err := reindexsvc.Run(rt, reindexsvc.RunRequest{})
		rt.Close()
		if err != nil {
			b.Fatalf("incremental reindex: %v", err)
		}
		if result.FilesIndexed != 0 {
			b.Fatalf("unchanged incremental indexed %d files, want 0", result.FilesIndexed)
		}
	}
}

func BenchmarkHotPathCheck(b *testing.B) {
	vaultPath := testutil.BuildSynthVault(b, benchVaultOptions())
	fullReindex(b, vaultPath)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt := newRuntime(b, vaultPath)
		_, err := checksvc.Run(rt, checksvc.Options{})
		rt.Close()
		if err != nil {
			b.Fatalf("check: %v", err)
		}
	}
}

func BenchmarkHotPathReadCanonicalID(b *testing.B) {
	vaultPath := testutil.BuildSynthVault(b, benchVaultOptions())
	fullReindex(b, vaultPath)
	target := testutil.SynthPersonID(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt := newRuntime(b, vaultPath)
		if err := rt.OpenDB(); err != nil {
			b.Fatalf("open db: %v", err)
		}
		result, err := readsvc.Read(rt, readsvc.ReadRequest{Reference: target})
		rt.Close()
		if err != nil {
			b.Fatalf("read: %v", err)
		}
		if result.ObjectID != target {
			b.Fatalf("read id = %q, want %q", result.ObjectID, target)
		}
	}
}

func BenchmarkHotPathQueryTypePerson(b *testing.B) {
	vaultPath := testutil.BuildSynthVault(b, benchVaultOptions())
	fullReindex(b, vaultPath)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt := newRuntime(b, vaultPath)
		if err := rt.OpenDB(); err != nil {
			b.Fatalf("open db: %v", err)
		}
		result, err := querysvc.ExecuteQuery(rt, querysvc.ExecuteRequest{
			QueryString: "type:person",
			Limit:       50,
		})
		rt.Close()
		if err != nil {
			b.Fatalf("query: %v", err)
		}
		if result.Returned == 0 {
			b.Fatal("query returned 0 people")
		}
	}
}

func BenchmarkHotPathSearchContent(b *testing.B) {
	vaultPath := testutil.BuildSynthVault(b, benchVaultOptions())
	fullReindex(b, vaultPath)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt := newRuntime(b, vaultPath)
		if err := rt.OpenDB(); err != nil {
			b.Fatalf("open db: %v", err)
		}
		result, err := querysvc.ExecuteQuery(rt, querysvc.ExecuteRequest{
			QueryString: `type:person content("weekly")`,
			Limit:       20,
		})
		rt.Close()
		if err != nil {
			b.Fatalf("search: %v", err)
		}
		if result.Returned == 0 {
			b.Fatal("content search returned 0 rows")
		}
	}
}

func BenchmarkHotPathBacklinks(b *testing.B) {
	vaultPath := testutil.BuildSynthVault(b, benchVaultOptions())
	fullReindex(b, vaultPath)
	target := testutil.SynthPersonID(0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt := newRuntime(b, vaultPath)
		if err := rt.OpenDB(); err != nil {
			b.Fatalf("open db: %v", err)
		}
		_, err := readsvc.Backlinks(rt, target)
		rt.Close()
		if err != nil {
			b.Fatalf("backlinks: %v", err)
		}
	}
}

func BenchmarkHotPathOpenIndex(b *testing.B) {
	vaultPath := testutil.BuildSynthVault(b, benchVaultOptions())
	fullReindex(b, vaultPath)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db, err := index.Open(vaultPath)
		if err != nil {
			b.Fatalf("open: %v", err)
		}
		if err := db.Close(); err != nil {
			b.Fatalf("close: %v", err)
		}
	}
}

func BenchmarkHotPathSetCanonicalID(b *testing.B) {
	vaultPath := testutil.BuildSynthVault(b, benchVaultOptions())
	fullReindex(b, vaultPath)
	target := testutil.SynthPersonID(1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt := newRuntime(b, vaultPath)
		if err := rt.OpenDB(); err != nil {
			b.Fatalf("open db: %v", err)
		}
		email := fieldvalue.String("bench@example.test")
		_, err := objectsvc.SetByReference(rt, objectsvc.SetByReferenceRequest{
			Reference:    target,
			TypedUpdates: map[string]fieldvalue.FieldValue{"email": email},
		})
		rt.Close()
		if err != nil {
			b.Fatalf("set: %v", err)
		}
	}
}

func BenchmarkHotPathReindexIncrementalOneFile(b *testing.B) {
	vaultPath := testutil.BuildSynthVault(b, benchVaultOptions())
	fullReindex(b, vaultPath)
	rel := "people/p0002.md"
	note := filepath.Join(vaultPath, filepath.FromSlash(rel))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		db, err := index.Open(vaultPath)
		if err != nil {
			b.Fatalf("open index: %v", err)
		}
		mtimes, err := db.GetFileMtimes()
		if err != nil {
			_ = db.Close()
			b.Fatalf("GetFileMtimes: %v", err)
		}
		if err := db.Close(); err != nil {
			b.Fatalf("close index: %v", err)
		}
		content, err := os.ReadFile(note)
		if err != nil {
			b.Fatalf("read note: %v", err)
		}
		if err := os.WriteFile(note, append(content, []byte("\n- extra line\n")...), 0o644); err != nil {
			b.Fatalf("touch note: %v", err)
		}
		laterUnix := mtimes[rel] + 5
		nowUnix := time.Now().Unix()
		if laterUnix <= nowUnix {
			laterUnix = nowUnix + 5
		}
		later := time.Unix(laterUnix, 0)
		if err := os.Chtimes(note, later, later); err != nil {
			b.Fatalf("chtimes note: %v", err)
		}
		b.StartTimer()
		rt := newRuntime(b, vaultPath)
		result, err := reindexsvc.Run(rt, reindexsvc.RunRequest{})
		rt.Close()
		if err != nil {
			b.Fatalf("incremental reindex: %v", err)
		}
		if result.FilesIndexed != 1 {
			b.Fatalf("incremental indexed %d files, want 1 (skipped=%d errors=%v indexed_mtime=%d)", result.FilesIndexed, result.FilesSkipped, result.Errors, mtimes[rel])
		}
	}
}
