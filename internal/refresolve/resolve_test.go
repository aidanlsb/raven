package refresolve

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestResolveDoesNotTreatNonMarkdownFileAsIdentity(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("files/paper.pdf", "%PDF").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})

	result, err := Resolve("files/paper.pdf", rt, false)
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	if !IsRefNotFound(err) {
		t.Fatalf("error = %v, want RefNotFoundError", err)
	}
}

func TestResolve_ObjectFound(t *testing.T) {
	t.Parallel()

	content := `---
type: book
---

A book.
`

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("books/dune.md", content).
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})

	result, err := Resolve("books/dune", rt, false)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result == nil {
		t.Fatalf("Resolve() returned nil result")
	}
	if result.ObjectID != "books/dune" {
		t.Errorf("ObjectID = %q, want %q", result.ObjectID, "books/dune")
	}
}

func TestResolve_RefNotFound(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})

	result, err := Resolve("nonexistent", rt, false)
	if result != nil {
		t.Errorf("result = %#v, want nil", result)
	}
	if !IsRefNotFound(err) {
		t.Errorf("error type = %T, want RefNotFoundError", err)
	}
}

// setupIndexedVault creates a test vault with indexed documents for testing.
func setupIndexedVault(t *testing.T, docs map[string]string) (*vaultruntime.Runtime, *index.Database) {
	t.Helper()

	v := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema())
	for path, content := range docs {
		v.WithFile(path, content)
	}
	v.Build()

	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
	if err := rt.OpenDB(); err != nil {
		t.Fatalf("OpenDB: %v", err)
	}

	sch := rt.Schema
	if sch == nil {
		sch = schema.New()
	}

	// Index all documents
	for path := range docs {
		fileContent, err := os.ReadFile(filepath.Join(v.Path, path))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		doc, err := parser.ParseDocument(string(fileContent), path, v.Path)
		if err != nil {
			t.Fatalf("ParseDocument(%s): %v", path, err)
		}
		if err := rt.DB.IndexDocument(doc, sch); err != nil {
			t.Fatalf("IndexDocument(%s): %v", path, err)
		}
	}

	return rt, rt.DB
}

// TestResolve_ExactMatch tests exact object ID resolution.
func TestResolve_ExactMatch(t *testing.T) {
	t.Parallel()

	rt, _ := setupIndexedVault(t, map[string]string{
		"books/dune.md": "---\ntype: book\n---\n\nA book.",
	})

	result, err := Resolve("books/dune", rt, false)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result == nil {
		t.Fatalf("Resolve() returned nil result")
	}
	if result.ObjectID != "books/dune" {
		t.Errorf("ObjectID = %q, want %q", result.ObjectID, "books/dune")
	}
	if result.IsSection {
		t.Errorf("IsSection = true, want false")
	}
}

// TestResolve_SlugMatch tests resolution by file basename.
func TestResolve_SlugMatch(t *testing.T) {
	t.Parallel()

	rt, _ := setupIndexedVault(t, map[string]string{
		"books/dune.md": "---\ntype: book\n---\n\nContent",
	})

	// "dune" should resolve to "books/dune" via slug
	result, err := Resolve("dune", rt, false)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result == nil {
		t.Fatalf("Resolve() returned nil result")
	}
	if result.ObjectID != "books/dune" {
		t.Errorf("ObjectID = %q, want %q (slug match)", result.ObjectID, "books/dune")
	}
}

// TestResolverDirectly_AmbiguousCase tests resolver ambiguous matching.
func TestResolverDirectly_AmbiguousCase(t *testing.T) {
	t.Parallel()

	rt, db := setupIndexedVault(t, map[string]string{
		"people/freya.md": "---\ntype: person\n---\n\nContent",
		"places/freya.md": "---\ntype: place\n---\n\nContent",
	})

	op, err := New(rt)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Resolve by slug - should be ambiguous or pick one
	result, err := op.Resolve("freya", false)
	if result != nil {
		// If it resolved, verify it's one of the two
		if result.ObjectID != "people/freya" && result.ObjectID != "places/freya" {
			t.Errorf("ObjectID = %q, want people/freya or places/freya", result.ObjectID)
		}
	} else if !IsAmbiguousRef(err) {
		// If it didn't resolve, should be ambiguous
		t.Errorf("error type = %T, want AmbiguousRefError", err)
	}

	// Exact match should work
	result, err = op.Resolve("people/freya", false)
	if err != nil {
		t.Fatalf("Resolve(people/freya) error = %v", err)
	}
	if result.ObjectID != "people/freya" {
		t.Errorf("ObjectID = %q, want people/freya", result.ObjectID)
	}

	_ = db
}

// TestResolve_SectionPresent tests resolving a reference with section fragment.
func TestResolve_SectionPresent(t *testing.T) {
	t.Parallel()

	rt, _ := setupIndexedVault(t, map[string]string{
		"guide.md": "---\ntype: page\n---\n\n# Introduction\n\nSome content.",
	})

	result, err := Resolve("guide#introduction", rt, false)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result == nil {
		t.Fatalf("Resolve() returned nil result")
	}
	if !strings.HasPrefix(result.ObjectID, "guide#") {
		t.Errorf("ObjectID = %q, want guide#<slug>", result.ObjectID)
	}
	if !result.IsSection {
		t.Errorf("IsSection = false, want true")
	}
	if result.FileObjectID != "guide" {
		t.Errorf("FileObjectID = %q, want %q", result.FileObjectID, "guide")
	}
}

// TestResolve_SectionMissing tests that a missing section returns not found.
func TestResolve_SectionMissing(t *testing.T) {
	t.Parallel()

	rt, _ := setupIndexedVault(t, map[string]string{
		"guide.md": "---\ntype: page\n---\n\nNo sections.",
	})

	result, err := Resolve("guide#nonexistent", rt, false)
	if result != nil {
		t.Errorf("result = %#v, want nil", result)
	}
	if !IsRefNotFound(err) {
		t.Errorf("error type = %T, want RefNotFoundError", err)
	}
}

// TestResolve_LiteralPath tests that a literal file path resolves correctly.
func TestResolve_LiteralPath(t *testing.T) {
	t.Parallel()

	rt, _ := setupIndexedVault(t, map[string]string{
		"notes/meeting.md": "---\ntype: page\n---\n\nContent",
	})

	// Literal path with .md extension
	result, err := Resolve("notes/meeting.md", rt, false)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if result == nil {
		t.Fatalf("Resolve() returned nil result")
	}
	if result.ObjectID != "notes/meeting" {
		t.Errorf("ObjectID = %q, want %q", result.ObjectID, "notes/meeting")
	}
}

// TestResolve_AllowMissing tests that missing files are tolerated when allowMissing=true.
func TestResolve_AllowMissing(t *testing.T) {
	t.Parallel()

	rt, _ := setupIndexedVault(t, map[string]string{})

	// Daily note that doesn't exist yet
	result, err := Resolve("2026-12-25", rt, true)
	if err != nil {
		t.Fatalf("Resolve(allowMissing=true) error = %v", err)
	}
	if result == nil {
		t.Fatalf("Resolve() returned nil result")
	}
	if result.ObjectID != "2026-12-25" {
		t.Errorf("ObjectID = %q, want %q", result.ObjectID, "2026-12-25")
	}
	if result.FilePath == "" {
		t.Errorf("FilePath empty, want daily note path")
	}
}

// TestResolve_AllowMissingFalse tests that missing files return error when allowMissing=false.
func TestResolve_AllowMissingFalse(t *testing.T) {
	t.Parallel()

	rt, _ := setupIndexedVault(t, map[string]string{})

	result, err := Resolve("2026-12-25", rt, false)
	if result != nil {
		t.Errorf("result = %#v, want nil", result)
	}
	if !IsRefNotFound(err) {
		t.Errorf("error type = %T, want RefNotFoundError", err)
	}
}

// TestResolveDynamic_Today tests resolving "today" as a dynamic date reference.
func TestResolveDynamic_Today(t *testing.T) {
	t.Parallel()

	rt, _ := setupIndexedVault(t, map[string]string{})

	result, err := ResolveDynamic("today", rt, true)
	if err != nil {
		t.Fatalf("ResolveDynamic('today') error = %v", err)
	}
	if result == nil {
		t.Fatalf("ResolveDynamic('today') returned nil result")
	}
	if result.MatchSource != "date" {
		t.Errorf("MatchSource = %q, want %q", result.MatchSource, "date")
	}
	// ObjectID should be today's date in YYYY-MM-DD format
	now := time.Now()
	expectedDate := now.Format("2006-01-02")
	if result.ObjectID != expectedDate {
		t.Errorf("ObjectID = %q, want %q (today)", result.ObjectID, expectedDate)
	}
}

// TestResolveDynamic_Yesterday tests resolving "yesterday".
func TestResolveDynamic_Yesterday(t *testing.T) {
	t.Parallel()

	rt, _ := setupIndexedVault(t, map[string]string{})

	result, err := ResolveDynamic("yesterday", rt, true)
	if err != nil {
		t.Fatalf("ResolveDynamic('yesterday') error = %v", err)
	}
	if result == nil {
		t.Fatalf("ResolveDynamic('yesterday') returned nil result")
	}
	if result.MatchSource != "date" {
		t.Errorf("MatchSource = %q, want %q", result.MatchSource, "date")
	}
	// ObjectID should be yesterday's date
	now := time.Now().AddDate(0, 0, -1)
	expectedDate := now.Format("2006-01-02")
	if result.ObjectID != expectedDate {
		t.Errorf("ObjectID = %q, want %q (yesterday)", result.ObjectID, expectedDate)
	}
}

// TestResolveDynamic_StaticDate tests resolving a static YYYY-MM-DD date.
func TestResolveDynamic_StaticDate(t *testing.T) {
	t.Parallel()

	rt, _ := setupIndexedVault(t, map[string]string{})

	result, err := ResolveDynamic("2026-08-25", rt, true)
	if err != nil {
		t.Fatalf("ResolveDynamic('2026-08-25') error = %v", err)
	}
	if result == nil {
		t.Fatalf("ResolveDynamic('2026-08-25') returned nil result")
	}
	if result.ObjectID != "2026-08-25" {
		t.Errorf("ObjectID = %q, want %q", result.ObjectID, "2026-08-25")
	}
}

// TestResolveDynamic_DateWithFragment tests resolving date reference with section.
func TestResolveDynamic_DateWithFragment(t *testing.T) {
	t.Parallel()

	rt, _ := setupIndexedVault(t, map[string]string{
		"daily/2026-08-25.md": "---\ntype: page\n---\n\n# Meeting Notes\n\nDiscussion.",
	})

	// Resolve with fragment
	result, err := ResolveDynamic("2026-08-25#meeting-notes", rt, false)
	if err != nil {
		t.Fatalf("ResolveDynamic() error = %v", err)
	}
	if result == nil {
		t.Fatalf("ResolveDynamic() returned nil result")
	}
	if !result.IsSection {
		t.Errorf("IsSection = false, want true")
	}
	// The actual file object ID depends on daily directory prefix
	if !strings.Contains(result.FileObjectID, "2026-08-25") {
		t.Errorf("FileObjectID = %q, should contain date", result.FileObjectID)
	}
}

// TestResolveDynamic_DateWithoutFragment tests date without section fragment.
func TestResolveDynamic_DateWithoutFragment(t *testing.T) {
	t.Parallel()

	rt, _ := setupIndexedVault(t, map[string]string{})

	// Dynamic date without fragment should work
	result, err := ResolveDynamic("today", rt, true)
	if err != nil {
		t.Fatalf("ResolveDynamic('today') error = %v", err)
	}
	if result == nil {
		t.Fatalf("ResolveDynamic('today') returned nil result")
	}
	if result.IsSection {
		t.Errorf("IsSection = true, want false (no fragment)")
	}
}

// TestOperation_ReuseResolver tests that Operation reuses the resolver across calls.
func TestOperation_ReuseResolver(t *testing.T) {
	t.Parallel()

	rt, _ := setupIndexedVault(t, map[string]string{
		"file1.md": "---\ntype: page\n---\n\nContent",
		"file2.md": "---\ntype: page\n---\n\nContent",
	})

	op, err := New(rt)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Multiple resolves should work
	result1, err := op.Resolve("file1", false)
	if err != nil {
		t.Fatalf("Resolve(file1) error = %v", err)
	}
	if result1 == nil || result1.ObjectID != "file1" {
		t.Errorf("first Resolve() = %v, want file1", result1)
	}

	result2, err := op.Resolve("file2", false)
	if err != nil {
		t.Fatalf("Resolve(file2) error = %v", err)
	}
	if result2 == nil || result2.ObjectID != "file2" {
		t.Errorf("second Resolve() = %v, want file2", result2)
	}
}

// TestOperation_NilRuntime tests that New returns error with nil runtime.
func TestOperation_NilRuntime(t *testing.T) {
	t.Parallel()

	_, err := New(nil)
	if err == nil {
		t.Fatalf("New(nil) succeeded, want error")
	}
}

// TestErrorTypes tests error type checking helpers.
func TestErrorTypes(t *testing.T) {
	t.Parallel()

	t.Run("RefNotFoundError", func(t *testing.T) {
		err := &RefNotFoundError{Reference: "test"}
		want := "reference 'test' not found"
		if err.Error() != want {
			t.Errorf("Error() = %q, want %q", err.Error(), want)
		}
	})

	t.Run("RefNotFoundError with detail", func(t *testing.T) {
		err := &RefNotFoundError{Reference: "test", Detail: "file missing"}
		if !strings.Contains(err.Error(), "file missing") {
			t.Errorf("Error() should include detail")
		}
	})

	t.Run("AmbiguousRefError", func(t *testing.T) {
		err := &AmbiguousRefError{
			Reference: "test",
			Matches:   []string{"a", "b"},
		}
		errStr := err.Error()
		if errStr == "" {
			t.Errorf("Error() returned empty string")
		}
		if !strings.Contains(errStr, "test") {
			t.Errorf("Error() should mention reference")
		}
	})

	t.Run("IsRefNotFound", func(t *testing.T) {
		err := &RefNotFoundError{Reference: "test"}
		if !IsRefNotFound(err) {
			t.Errorf("IsRefNotFound(&RefNotFoundError{}) = false, want true")
		}
		if IsRefNotFound(nil) {
			t.Errorf("IsRefNotFound(nil) = true, want false")
		}
		if IsRefNotFound(errors.New("other")) {
			t.Errorf("IsRefNotFound(other error) = true, want false")
		}
	})

	t.Run("IsAmbiguousRef", func(t *testing.T) {
		err := &AmbiguousRefError{Reference: "test"}
		if !IsAmbiguousRef(err) {
			t.Errorf("IsAmbiguousRef(&AmbiguousRefError{}) = false, want true")
		}
		if IsAmbiguousRef(nil) {
			t.Errorf("IsAmbiguousRef(nil) = true, want false")
		}
		if IsAmbiguousRef(errors.New("other")) {
			t.Errorf("IsAmbiguousRef(other error) = true, want false")
		}
	})
}
