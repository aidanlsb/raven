package indexjournal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJournalLifecycle(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	operationID, err := Begin(vaultPath)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}

	snapshot := loadSnapshot(t, vaultPath)
	if !snapshot.Dirty() || !snapshot.RequiresFullScan() {
		t.Fatalf("snapshot = %#v, want unknown dirty operation", snapshot)
	}

	operationID, err = SetPaths(vaultPath, operationID, []string{"notes/two.md", "notes/one.md", "notes/one.md"})
	if err != nil {
		t.Fatalf("SetPaths() error = %v", err)
	}
	snapshot = loadSnapshot(t, vaultPath)
	if snapshot.RequiresFullScan() {
		t.Fatalf("snapshot = %#v, want concrete paths", snapshot)
	}
	if got := snapshot.Paths(); len(got) != 2 || got[0] != "notes/one.md" || got[1] != "notes/two.md" {
		t.Fatalf("Paths() = %#v, want sorted unique paths", got)
	}

	if err := ClearPaths(vaultPath, operationID, "notes/one.md"); err != nil {
		t.Fatalf("ClearPaths(one) error = %v", err)
	}
	if got := loadSnapshot(t, vaultPath).Paths(); len(got) != 1 || got[0] != "notes/two.md" {
		t.Fatalf("remaining paths = %#v, want notes/two.md", got)
	}

	if err := ClearPaths(vaultPath, operationID, "notes/two.md"); err != nil {
		t.Fatalf("ClearPaths(two) error = %v", err)
	}
	if snapshot := loadSnapshot(t, vaultPath); snapshot.Dirty() {
		t.Fatalf("snapshot = %#v, want clean", snapshot)
	}
	if _, err := os.Stat(filepath.Join(vaultPath, ".raven", Filename)); !os.IsNotExist(err) {
		t.Fatalf("journal file still exists, stat error = %v", err)
	}
}

func TestCompleteIfUnchangedPreservesAdvancedOperation(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	operationID, err := Begin(vaultPath)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	unknown := loadSnapshot(t, vaultPath).Operations[0]

	if _, err := SetPaths(vaultPath, operationID, []string{"notes/changed.md"}); err != nil {
		t.Fatalf("SetPaths() error = %v", err)
	}
	if err := CompleteIfUnchanged(vaultPath, unknown); err != nil {
		t.Fatalf("CompleteIfUnchanged() error = %v", err)
	}

	snapshot := loadSnapshot(t, vaultPath)
	if !snapshot.Dirty() || snapshot.RequiresFullScan() {
		t.Fatalf("snapshot = %#v, want advanced concrete operation preserved", snapshot)
	}
	if got := snapshot.Paths(); len(got) != 1 || got[0] != "notes/changed.md" {
		t.Fatalf("Paths() = %#v, want notes/changed.md", got)
	}
}

func TestRecoveryDoesNotClearActiveUnknownOperation(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	operationID, err := Begin(vaultPath)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	t.Cleanup(func() { _ = CancelUnknown(vaultPath, operationID) })
	snapshot := loadSnapshot(t, vaultPath)

	if err := CompleteRecoveredUnknown(vaultPath, snapshot); err != nil {
		t.Fatalf("CompleteRecoveredUnknown() error = %v", err)
	}
	if !loadSnapshot(t, vaultPath).Dirty() {
		t.Fatal("active unknown operation was cleared by recovery")
	}
}

func TestSetPathsWithoutGuardCreatesConcreteOperation(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	operationID, err := SetPaths(vaultPath, "", []string{"note.md"})
	if err != nil {
		t.Fatalf("SetPaths() error = %v", err)
	}
	if operationID == "" {
		t.Fatal("SetPaths() returned empty operation ID")
	}
	snapshot := loadSnapshot(t, vaultPath)
	if snapshot.RequiresFullScan() {
		t.Fatalf("snapshot = %#v, want concrete operation", snapshot)
	}
	if got := snapshot.Paths(); len(got) != 1 || got[0] != "note.md" {
		t.Fatalf("Paths() = %#v, want note.md", got)
	}
}

func TestCancelUnknownDoesNotClearConcreteWork(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	operationID, err := Begin(vaultPath)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if err := CancelUnknown(vaultPath, operationID); err != nil {
		t.Fatalf("CancelUnknown(unknown) error = %v", err)
	}
	if loadSnapshot(t, vaultPath).Dirty() {
		t.Fatal("unknown operation remains after cancellation")
	}

	operationID, err = SetPaths(vaultPath, "", []string{"note.md"})
	if err != nil {
		t.Fatalf("SetPaths() error = %v", err)
	}
	if err := CancelUnknown(vaultPath, operationID); err != nil {
		t.Fatalf("CancelUnknown(concrete) error = %v", err)
	}
	if !loadSnapshot(t, vaultPath).Dirty() {
		t.Fatal("concrete operation was cleared by cancellation")
	}
}

func loadSnapshot(t *testing.T, vaultPath string) Snapshot {
	t.Helper()
	snapshot, err := Load(vaultPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return snapshot
}
