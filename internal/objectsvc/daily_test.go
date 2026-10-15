package objectsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/vault"
)

func TestResolveDailyDoesNotCreateFile(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	cfg := &config.VaultConfig{DailyDirectory: "journal"}

	got, err := ResolveDaily(vaultPath, cfg, "today")
	if err != nil {
		t.Fatalf("ResolveDaily: %v", err)
	}
	today := todayISO(t)
	if got.Directory != "journal" {
		t.Fatalf("directory = %q, want journal", got.Directory)
	}
	if got.ID != today || got.Date != today {
		t.Fatalf("id/date = %q/%q, want %q", got.ID, got.Date, today)
	}
	if got.RelativePath != "journal/"+today+".md" {
		t.Fatalf("relative path = %q", got.RelativePath)
	}
	if got.Exists {
		t.Fatal("today_exists = true, want false before the file exists")
	}
	if _, err := os.Stat(filepath.Join(vaultPath, "journal", today+".md")); !os.IsNotExist(err) {
		t.Fatalf("ResolveDaily created a file: %v", err)
	}
}

func TestResolveDailyReportsExistingNote(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	cfg := config.DefaultVaultConfig()
	today := todayISO(t)
	dir := filepath.Join(vaultPath, "daily")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, today+".md"), []byte("---\ntype: date\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveDaily(vaultPath, cfg, "")
	if err != nil {
		t.Fatalf("ResolveDaily: %v", err)
	}
	if !got.Exists {
		t.Fatal("exists = false, want true")
	}
	if got.ID != today {
		t.Fatalf("id = %q, want %q", got.ID, today)
	}
}

func todayISO(t *testing.T) string {
	t.Helper()
	day, err := vault.ParseDateArg("today")
	if err != nil {
		t.Fatal(err)
	}
	return vault.FormatDateISO(day)
}
