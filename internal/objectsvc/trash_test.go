package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/svcerr"
)

func TestListTrashReturnsMirroredEntriesAndFilters(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTrashTestFile(t, vaultPath, ".trash/people/freya.md", "---\ntype: person\nname: Freya\n---\n")
	writeTrashTestFile(t, vaultPath, ".trash/files/paper.pdf", "pdf")
	vaultCfg := config.DefaultVaultConfig()

	result, err := ListTrash(ListTrashRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
	})
	if err != nil {
		t.Fatalf("ListTrash() error = %v", err)
	}
	if result.TrashDir != ".trash" || result.DeletionBehavior != "trash" {
		t.Fatalf("ListTrash() config = %#v", result)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entries = %#v, want 2", result.Entries)
	}
	if got := result.Entries[0]; got.Reference != "files/paper.pdf" ||
		got.TrashPath != ".trash/files/paper.pdf" ||
		got.RestorePath != "files/paper.pdf" ||
		got.Kind != TrashKindFile {
		t.Fatalf("file entry = %#v", got)
	}
	if got := result.Entries[1]; got.Reference != "people/freya" ||
		got.TrashPath != ".trash/people/freya.md" ||
		got.RestorePath != "people/freya.md" ||
		got.Kind != TrashKindMarkdown {
		t.Fatalf("markdown entry = %#v", got)
	}

	filtered, err := ListTrash(ListTrashRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		Reference:   "people/freya",
		Kind:        TrashKindMarkdown,
	})
	if err != nil {
		t.Fatalf("ListTrash(filtered) error = %v", err)
	}
	if len(filtered.Entries) != 1 || filtered.Entries[0].Reference != "people/freya" {
		t.Fatalf("filtered entries = %#v", filtered.Entries)
	}
}

func TestListTrashUsesConfiguredDirectory(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTrashTestFile(t, vaultPath, "archive/trash/files/paper.pdf", "pdf")
	vaultCfg := &config.VaultConfig{Deletion: &config.DeletionConfig{
		Behavior: "permanent",
		TrashDir: "archive/trash",
	}}

	result, err := ListTrash(ListTrashRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
	})
	if err != nil {
		t.Fatalf("ListTrash() error = %v", err)
	}
	if result.TrashDir != "archive/trash" || result.DeletionBehavior != "permanent" {
		t.Fatalf("ListTrash() config = %#v", result)
	}
	if len(result.Entries) != 1 || result.Entries[0].TrashPath != "archive/trash/files/paper.pdf" {
		t.Fatalf("entries = %#v", result.Entries)
	}
}

func TestListTrashPreservesOriginalReferenceForVersionedCollisions(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTrashTestFile(t, vaultPath, ".trash/people/freya.md", "oldest")
	versionPath := ".trash/people/freya.raven-trash-" + trashCollisionTag("people/freya.md") + "-2026-03-10-112233-1.md"
	writeTrashTestFile(t, vaultPath, versionPath, "newest")
	vaultCfg := config.DefaultVaultConfig()

	result, err := ListTrash(ListTrashRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		Reference:   "people/freya",
	})
	if err != nil {
		t.Fatalf("ListTrash() error = %v", err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entries = %#v, want two versions", result.Entries)
	}
	for _, entry := range result.Entries {
		if entry.Reference != "people/freya" || entry.RestorePath != "people/freya.md" {
			t.Fatalf("versioned entry lost original identity: %#v", entry)
		}
	}

	_, err = PreviewRestoreByReference(RestoreByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		Reference:   "people/freya",
	})
	requireTrashServiceErrorCode(t, err, codes.ErrRefAmbiguous)

	restored, err := RestoreByReference(RestoreByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		Reference:   versionPath,
	})
	if err != nil {
		t.Fatalf("RestoreByReference(exact path) error = %v", err)
	}
	if restored.Entry.Reference != "people/freya" || restored.Entry.RestorePath != "people/freya.md" {
		t.Fatalf("restored entry = %#v", restored.Entry)
	}
	requireTrashTestPath(t, vaultPath, "people/freya.md", true)
	requireTrashTestPath(t, vaultPath, ".trash/people/freya.md", true)
	requireTrashTestPath(t, vaultPath, versionPath, false)
}

func TestListTrashDoesNotDecodeUnverifiedCollisionMarker(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	literalPath := ".trash/people/freya.raven-trash-deadbeefdead-2026-03-10-112233-1.md"
	writeTrashTestFile(t, vaultPath, literalPath, "literal")

	result, err := ListTrash(ListTrashRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
	})
	if err != nil {
		t.Fatalf("ListTrash() error = %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("entries = %#v, want one", result.Entries)
	}
	if got := result.Entries[0]; got.Reference != "people/freya.raven-trash-deadbeefdead-2026-03-10-112233-1" ||
		got.RestorePath != "people/freya.raven-trash-deadbeefdead-2026-03-10-112233-1.md" {
		t.Fatalf("literal marker-shaped entry was decoded: %#v", got)
	}
}

func TestListTrashPreservesTimestampedLiteralPath(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	timestampedPath := ".trash/people/freya-2026-03-10-112233.md"
	writeTrashTestFile(t, vaultPath, timestampedPath, "literal")

	result, err := ListTrash(ListTrashRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
	})
	if err != nil {
		t.Fatalf("ListTrash() error = %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("entries = %#v, want one", result.Entries)
	}
	if got := result.Entries[0]; got.Reference != "people/freya-2026-03-10-112233" ||
		got.RestorePath != "people/freya-2026-03-10-112233.md" {
		t.Fatalf("timestamped literal entry was decoded: %#v", got)
	}
}

func TestListTrashSkipsNonRegularEntries(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTrashTestFile(t, vaultPath, ".trash/files/regular.txt", "regular")
	if err := os.Symlink("../../outside.txt", filepath.Join(vaultPath, ".trash/files/link.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	result, err := ListTrash(ListTrashRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
	})
	if err != nil {
		t.Fatalf("ListTrash() error = %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].Reference != "files/regular.txt" {
		t.Fatalf("entries = %#v, want only regular file", result.Entries)
	}
}

func TestTrashRoundTripPreservesSafeVaultSymlink(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTrashTestFile(t, vaultPath, "files/target.txt", "target")
	sourcePath := filepath.Join(vaultPath, "files/link.txt")
	if err := os.Symlink("target.txt", sourcePath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	deleted, err := DeleteFile(DeleteFileRequest{
		VaultPath: vaultPath,
		FilePath:  sourcePath,
		Behavior:  "trash",
		TrashDir:  ".trash",
	})
	if err != nil {
		t.Fatalf("DeleteFile() error = %v", err)
	}
	if info, statErr := os.Lstat(deleted.TrashPath); statErr != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("trashed symlink info = %#v, error = %v", info, statErr)
	}

	listed, err := ListTrash(ListTrashRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
	})
	if err != nil {
		t.Fatalf("ListTrash() error = %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Reference != "files/link.txt" {
		t.Fatalf("entries = %#v, want safe symlink", listed.Entries)
	}

	if _, err := RestoreByReference(RestoreByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
		Reference:   "files/link.txt",
	}); err != nil {
		t.Fatalf("RestoreByReference() error = %v", err)
	}
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read restored symlink: %v", err)
	}
	if string(content) != "target" {
		t.Fatalf("restored symlink content = %q, want target", content)
	}
}

func TestRestoreByReferencePreviewAndApply(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTrashTestFile(t, vaultPath, ".trash/people/freya.md", "restored")
	req := RestoreByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
		Reference:   "people/freya",
	}

	preview, err := PreviewRestoreByReference(req)
	if err != nil {
		t.Fatalf("PreviewRestoreByReference() error = %v", err)
	}
	if preview.Entry.RestorePath != "people/freya.md" {
		t.Fatalf("preview entry = %#v", preview.Entry)
	}
	requireTrashTestPath(t, vaultPath, ".trash/people/freya.md", true)
	requireTrashTestPath(t, vaultPath, "people/freya.md", false)

	result, err := RestoreByReference(req)
	if err != nil {
		t.Fatalf("RestoreByReference() error = %v", err)
	}
	requireTrashTestPath(t, vaultPath, ".trash/people/freya.md", false)
	requireTrashTestPath(t, vaultPath, "people/freya.md", true)
	if len(result.ChangeSet.Moved) != 1 ||
		result.ChangeSet.Moved[0].From != ".trash/people/freya.md" ||
		result.ChangeSet.Moved[0].To != "people/freya.md" {
		t.Fatalf("ChangeSet.Moved = %#v", result.ChangeSet.Moved)
	}
}

func TestRestoreByReferenceRejectsOccupiedDestination(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTrashTestFile(t, vaultPath, ".trash/people/freya.md", "trashed")
	writeTrashTestFile(t, vaultPath, "people/freya.md", "current")

	_, err := RestoreByReference(RestoreByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: config.DefaultVaultConfig(),
		Reference:   "people/freya",
	})
	requireTrashServiceErrorCode(t, err, codes.ErrFileExists)
	requireTrashTestPath(t, vaultPath, ".trash/people/freya.md", true)

	content, readErr := os.ReadFile(filepath.Join(vaultPath, "people/freya.md"))
	if readErr != nil {
		t.Fatalf("read destination: %v", readErr)
	}
	if string(content) != "current" {
		t.Fatalf("destination content = %q, want current", content)
	}
}

func TestRestoreByReferenceRejectsMissingEntry(t *testing.T) {
	t.Parallel()

	_, err := PreviewRestoreByReference(RestoreByReferenceRequest{
		VaultPath:   t.TempDir(),
		VaultConfig: config.DefaultVaultConfig(),
		Reference:   "people/missing",
	})
	requireTrashServiceErrorCode(t, err, codes.ErrFileNotFound)
}

func writeTrashTestFile(t *testing.T, vaultPath, relativePath, content string) {
	t.Helper()
	filePath := filepath.Join(vaultPath, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", relativePath, err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relativePath, err)
	}
}

func requireTrashTestPath(t *testing.T, vaultPath, relativePath string, exists bool) {
	t.Helper()
	_, err := os.Stat(filepath.Join(vaultPath, filepath.FromSlash(relativePath)))
	if exists && err != nil {
		t.Fatalf("expected %s to exist: %v", relativePath, err)
	}
	if !exists && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be missing, stat error = %v", relativePath, err)
	}
}

func requireTrashServiceErrorCode(t *testing.T, err error, code codes.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s error", code)
	}
	var serviceErr *svcerr.Error
	if !errors.As(err, &serviceErr) {
		t.Fatalf("error = %T %v, want *svcerr.Error", err, err)
	}
	if serviceErr.Code != code {
		t.Fatalf("error code = %s, want %s", serviceErr.Code, code)
	}
}
