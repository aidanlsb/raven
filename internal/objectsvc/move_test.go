package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestMoveFileUpdatesBacklinksAfterRename(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("notes/ref.md", "See [[people/freya]].\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "people/freya.md", "notes/ref.md")

	result, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       &config.VaultConfig{},
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "people/freya.md"),
		DestinationFile:   filepath.Join(v.Path, "archive/freya.md"),
		SourceObjectID:    "people/freya",
		DestinationObject: "archive/freya",
		UpdateRefs:        true,
	})
	if err != nil {
		t.Fatalf("MoveFile() error = %v", err)
	}
	if len(result.WarningMessages) != 0 {
		t.Fatalf("unexpected warnings: %#v", result.WarningMessages)
	}
	if len(result.UpdatedRefs) != 1 || result.UpdatedRefs[0] != "notes/ref" {
		t.Fatalf("UpdatedRefs = %#v, want [notes/ref]", result.UpdatedRefs)
	}

	content := v.ReadFile("notes/ref.md")
	if !strings.Contains(content, "[[archive/freya]]") {
		t.Fatalf("backlink not updated, content:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(v.Path, "archive/freya.md")); err != nil {
		t.Fatalf("expected moved file to exist: %v", err)
	}
}

func TestMoveFileUpdatesMarkdownFileLinks(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("files/paper(1).pdf", "%PDF test\n").
		WithFile("notes/ref.md", "Read [angle](<../files/paper(1).pdf>) and [escaped](../files/paper\\(1\\).pdf).\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "notes/ref.md")

	preview, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       config.DefaultVaultConfig(),
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "files/paper(1).pdf"),
		DestinationFile:   filepath.Join(v.Path, "files/archive/paper(1).pdf"),
		SourceObjectID:    "files/paper(1).pdf",
		DestinationObject: "files/archive/paper(1).pdf",
		UpdateRefs:        true,
		Preview:           true,
	})
	if err != nil {
		t.Fatalf("MoveFile(preview) error = %v", err)
	}
	if len(preview.UpdatedRefs) != 1 || preview.UpdatedRefs[0] != "notes/ref" {
		t.Fatalf("preview UpdatedRefs = %#v, want [notes/ref]", preview.UpdatedRefs)
	}
	if content := v.ReadFile("notes/ref.md"); strings.Contains(content, "files/archive") {
		t.Fatalf("preview changed source link:\n%s", content)
	}

	result, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       config.DefaultVaultConfig(),
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "files/paper(1).pdf"),
		DestinationFile:   filepath.Join(v.Path, "files/archive/paper(1).pdf"),
		SourceObjectID:    "files/paper(1).pdf",
		DestinationObject: "files/archive/paper(1).pdf",
		UpdateRefs:        true,
	})
	if err != nil {
		t.Fatalf("MoveFile() error = %v", err)
	}
	if len(result.WarningMessages) != 0 {
		t.Fatalf("unexpected warnings: %#v", result.WarningMessages)
	}
	if len(result.UpdatedRefs) != 1 || result.UpdatedRefs[0] != "notes/ref" {
		t.Fatalf("UpdatedRefs = %#v, want [notes/ref]", result.UpdatedRefs)
	}

	content := v.ReadFile("notes/ref.md")
	if !strings.Contains(content, "[angle](<../files/archive/paper(1).pdf>)") {
		t.Fatalf("angle-delimited file link not updated, content:\n%s", content)
	}
	if !strings.Contains(content, `[escaped](../files/archive/paper\(1\).pdf)`) {
		t.Fatalf("escaped file link not updated, content:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(v.Path, "files/archive/paper(1).pdf")); err != nil {
		t.Fatalf("expected moved file to exist: %v", err)
	}
}

func TestMoveFileDoesNotRewriteDifferentNormalizedLink(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("files/paper.pdf", "%PDF test\n").
		WithFile("notes/ref.md", "This source-relative link points elsewhere: [paper](files/paper.pdf).\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "notes/ref.md")

	result, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       config.DefaultVaultConfig(),
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "files/paper.pdf"),
		DestinationFile:   filepath.Join(v.Path, "files/archive/paper.pdf"),
		SourceObjectID:    "files/paper.pdf",
		DestinationObject: "files/archive/paper.pdf",
		UpdateRefs:        true,
	})
	if err != nil {
		t.Fatalf("MoveFile() error = %v", err)
	}
	if len(result.UpdatedRefs) != 0 {
		t.Fatalf("UpdatedRefs = %#v, want none for non-matching normalized key", result.UpdatedRefs)
	}
	v.AssertFileContains("notes/ref.md", "[paper](files/paper.pdf)")
}

func TestRewriteIndexedLinkTargetRelocatesStalePosition(t *testing.T) {
	t.Parallel()

	content := []byte("prefix inserted [paper](../assets/paper.pdf)\n")
	link := model.Link{
		Line:          1,
		PositionStart: 0,
		PositionEnd:   len("[paper](../assets/paper.pdf)"),
		RawTarget:     "../assets/paper.pdf",
	}
	updated, changed := rewriteIndexedLinkTarget(content, link, "../assets/archive/paper.pdf")
	if !changed {
		t.Fatal("rewriteIndexedLinkTarget() did not relocate stale indexed span")
	}
	if got := string(updated); !strings.Contains(got, "[paper](../assets/archive/paper.pdf)") {
		t.Fatalf("rewriteIndexedLinkTarget() = %q", got)
	}
}

func TestMoveFileRenameFailureDoesNotRewriteBacklinks(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("notes/ref.md", "See [[people/freya]].\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "people/freya.md", "notes/ref.md")

	destPath := filepath.Join(v.Path, "archive/freya.md")
	if err := os.MkdirAll(destPath, 0o755); err != nil {
		t.Fatalf("mkdir conflicting destination: %v", err)
	}

	_, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       &config.VaultConfig{},
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "people/freya.md"),
		DestinationFile:   destPath,
		SourceObjectID:    "people/freya",
		DestinationObject: "archive/freya",
		UpdateRefs:        true,
	})
	if err == nil {
		t.Fatal("expected MoveFile() to fail")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != ErrorFileWrite {
		t.Fatalf("error code = %s, want %s", svcErr.Code, ErrorFileWrite)
	}

	content := v.ReadFile("notes/ref.md")
	if !strings.Contains(content, "[[people/freya]]") {
		t.Fatalf("backlink changed despite rename failure, content:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(v.Path, "people/freya.md")); err != nil {
		t.Fatalf("expected source file to remain in place: %v", err)
	}
}

func TestMoveFileWarnsWhenRefRewriteFails(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("notes/ref.md", "See [[people/freya]].\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "people/freya.md", "notes/ref.md")

	if err := os.Remove(filepath.Join(v.Path, "notes/ref.md")); err != nil {
		t.Fatalf("remove backlink source: %v", err)
	}

	result, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       &config.VaultConfig{},
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "people/freya.md"),
		DestinationFile:   filepath.Join(v.Path, "archive/freya.md"),
		SourceObjectID:    "people/freya",
		DestinationObject: "archive/freya",
		UpdateRefs:        true,
	})
	if err != nil {
		t.Fatalf("MoveFile() error = %v", err)
	}
	if len(result.UpdatedRefs) != 0 {
		t.Fatalf("UpdatedRefs = %#v, want empty", result.UpdatedRefs)
	}
	if len(result.WarningMessages) != 1 {
		t.Fatalf("WarningMessages = %#v, want one warning", result.WarningMessages)
	}
	if !strings.Contains(result.WarningMessages[0], "notes/ref") {
		t.Fatalf("warning = %q, want notes/ref context", result.WarningMessages[0])
	}
	if _, err := os.Stat(filepath.Join(v.Path, "archive/freya.md")); err != nil {
		t.Fatalf("expected moved file to exist: %v", err)
	}
}

func TestMoveFileRollsBackWhenRefRewriteWriteFails(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("notes/ref.md", "See [[people/freya]].\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "people/freya.md", "notes/ref.md")

	failPath := filepath.Join(v.Path, "notes/ref.md")
	restoreWriter := swapMoveFileWriterForTest(func(path string, data []byte, perm os.FileMode) error {
		if path == failPath {
			return fmt.Errorf("injected ref rewrite failure")
		}
		return atomicfile.WriteFile(path, data, perm)
	})
	defer restoreWriter()

	_, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       &config.VaultConfig{},
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "people/freya.md"),
		DestinationFile:   filepath.Join(v.Path, "archive/freya.md"),
		SourceObjectID:    "people/freya",
		DestinationObject: "archive/freya",
		UpdateRefs:        true,
	})
	if err == nil {
		t.Fatal("expected MoveFile() to fail")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != ErrorValidationFailed {
		t.Fatalf("error code = %s, want %s", svcErr.Code, ErrorValidationFailed)
	}

	content := v.ReadFile("notes/ref.md")
	if !strings.Contains(content, "[[people/freya]]") {
		t.Fatalf("backlink changed despite rollback, content:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(v.Path, "people/freya.md")); err != nil {
		t.Fatalf("expected source file to be restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(v.Path, "archive/freya.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected destination file to be removed, got %v", err)
	}
}

func TestMoveFileReturnsChangeSetBeforeIndexProjection(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "people/freya.md")

	result, err := MoveFile(MoveFileRequest{
		VaultPath:          v.Path,
		VaultConfig:        &config.VaultConfig{},
		Schema:             sch,
		SourceFile:         filepath.Join(v.Path, "people/freya.md"),
		DestinationFile:    filepath.Join(v.Path, "archive/freya.md"),
		SourceObjectID:     "people/freya",
		DestinationObject:  "archive/freya",
		ReplacementContent: []byte("---\ntype: person\nname: [\n---\n"),
		UpdateRefs:         true,
	})
	if err != nil {
		t.Fatalf("MoveFile() error = %v", err)
	}
	if len(result.ChangeSet.Moved) != 1 {
		t.Fatalf("ChangeSet.Moved = %#v, want one move", result.ChangeSet.Moved)
	}
	if move := result.ChangeSet.Moved[0]; move.From != "people/freya.md" || move.To != "archive/freya.md" {
		t.Fatalf("ChangeSet.Moved[0] = %#v, want people/freya.md -> archive/freya.md", move)
	}

	if _, err := os.Stat(filepath.Join(v.Path, "people/freya.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected source file to be removed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(v.Path, "archive/freya.md")); err != nil {
		t.Fatalf("expected destination file to remain durable before index projection: %v", err)
	}
}

// swapMoveFileWriterForTest mutates package-global state, so tests that use it
// must not call t.Parallel().
func swapMoveFileWriterForTest(writer func(path string, data []byte, perm os.FileMode) error) func() {
	moveFileWriterMu.Lock()
	previous := moveFileWriter
	moveFileWriter = writer
	moveFileWriterMu.Unlock()

	return func() {
		moveFileWriterMu.Lock()
		moveFileWriter = previous
		moveFileWriterMu.Unlock()
	}
}

func TestMoveFileUpdatesSelfRefsAfterRename(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n\nSee [[people/freya]].\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "people/freya.md")

	result, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       &config.VaultConfig{},
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "people/freya.md"),
		DestinationFile:   filepath.Join(v.Path, "archive/freya.md"),
		SourceObjectID:    "people/freya",
		DestinationObject: "archive/freya",
		UpdateRefs:        true,
	})
	if err != nil {
		t.Fatalf("MoveFile() error = %v", err)
	}
	if len(result.WarningMessages) != 0 {
		t.Fatalf("unexpected warnings: %#v", result.WarningMessages)
	}
	if len(result.UpdatedRefs) != 1 || result.UpdatedRefs[0] != "archive/freya" {
		t.Fatalf("UpdatedRefs = %#v, want [archive/freya]", result.UpdatedRefs)
	}

	content := v.ReadFile("archive/freya.md")
	if !strings.Contains(content, "[[archive/freya]]") {
		t.Fatalf("self-ref not updated, content:\n%s", content)
	}
}

func TestMoveFileSkipsRefsInsideInlineCode(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("notes/ref.md", "Real [[people/freya]] and code `[[people/freya]]`.\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "people/freya.md", "notes/ref.md")

	if _, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       &config.VaultConfig{},
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "people/freya.md"),
		DestinationFile:   filepath.Join(v.Path, "archive/freya.md"),
		SourceObjectID:    "people/freya",
		DestinationObject: "archive/freya",
		UpdateRefs:        true,
	}); err != nil {
		t.Fatalf("MoveFile() error = %v", err)
	}

	content := v.ReadFile("notes/ref.md")
	if !strings.Contains(content, "Real [[archive/freya]]") {
		t.Fatalf("real backlink not updated, content:\n%s", content)
	}
	if !strings.Contains(content, "`[[people/freya]]`") {
		t.Fatalf("code-span ref must not be rewritten, content:\n%s", content)
	}
	if got := strings.Count(content, "archive/freya"); got != 1 {
		t.Fatalf("expected exactly one rewritten ref, got %d, content:\n%s", got, content)
	}
}

func TestMoveFilePreservesFragmentAndDisplayText(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n\n## Bio\n\nDetails.\n").
		WithFile("notes/ref.md", "See [[people/freya#bio|Freya]] for details.\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "people/freya.md", "notes/ref.md")

	result, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       &config.VaultConfig{},
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "people/freya.md"),
		DestinationFile:   filepath.Join(v.Path, "archive/freya.md"),
		SourceObjectID:    "people/freya",
		DestinationObject: "archive/freya",
		UpdateRefs:        true,
	})
	if err != nil {
		t.Fatalf("MoveFile() error = %v", err)
	}
	if len(result.UpdatedRefs) != 1 || result.UpdatedRefs[0] != "notes/ref" {
		t.Fatalf("UpdatedRefs = %#v, want [notes/ref]", result.UpdatedRefs)
	}

	content := v.ReadFile("notes/ref.md")
	if !strings.Contains(content, "[[archive/freya#bio|Freya]]") {
		t.Fatalf("fragment/display not preserved, content:\n%s", content)
	}
}

func TestMoveFileUpdatesFrontmatterBareRef(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("projects/site.md", "---\ntype: project\ntitle: Site\nowner: people/freya\n---\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "people/freya.md", "projects/site.md")
	resolveVaultRefs(t, v.Path, sch)

	result, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       &config.VaultConfig{},
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "people/freya.md"),
		DestinationFile:   filepath.Join(v.Path, "archive/freya.md"),
		SourceObjectID:    "people/freya",
		DestinationObject: "archive/freya",
		UpdateRefs:        true,
	})
	if err != nil {
		t.Fatalf("MoveFile() error = %v", err)
	}
	if len(result.UpdatedRefs) != 1 || result.UpdatedRefs[0] != "projects/site" {
		t.Fatalf("UpdatedRefs = %#v, want [projects/site]", result.UpdatedRefs)
	}

	content := v.ReadFile("projects/site.md")
	if !strings.Contains(content, "owner: archive/freya") {
		t.Fatalf("frontmatter ref not updated, content:\n%s", content)
	}
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

func resolveVaultRefs(t *testing.T, vaultPath string, sch *schema.Schema) {
	t.Helper()
	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	defer db.Close()
	if _, err := db.ResolveReferencesWithSchema("daily", sch); err != nil {
		t.Fatalf("resolve refs: %v", err)
	}
}
