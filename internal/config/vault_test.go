package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadVaultConfig(t *testing.T) {
	t.Run("default config when file missing", func(t *testing.T) {
		// Use a temp directory without a raven.yaml
		tmpDir := t.TempDir()

		cfg, err := LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.DailyDirectory != "daily" {
			t.Errorf("expected daily_directory 'daily', got %q", cfg.DailyDirectory)
		}
	})

	t.Run("loads custom config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "raven.yaml")

		content := "daily_directory: journal\n"
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.DailyDirectory != "journal" {
			t.Errorf("expected daily_directory 'journal', got %q", cfg.DailyDirectory)
		}
	})

	t.Run("defaults empty daily_directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "raven.yaml")

		// Empty config file
		content := "# empty config\n"
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.DailyDirectory != "daily" {
			t.Errorf("expected daily_directory 'daily', got %q", cfg.DailyDirectory)
		}
	})
}

func TestVaultConfigPaths(t *testing.T) {
	cfg := &VaultConfig{
		DailyDirectory: "daily",
	}

	t.Run("DailyNotePath", func(t *testing.T) {
		path := cfg.DailyNotePath("/vault", "2025-02-01")
		expected := filepath.Join("/vault", "daily", "2025-02-01.md")
		if path != expected {
			t.Errorf("got %q, want %q", path, expected)
		}
	})

	t.Run("DailyNoteID", func(t *testing.T) {
		id := cfg.DailyNoteID("2025-02-01")
		expected := filepath.Join("daily", "2025-02-01")
		if id != expected {
			t.Errorf("got %q, want %q", id, expected)
		}
	})

	t.Run("custom directory", func(t *testing.T) {
		cfg2 := &VaultConfig{DailyDirectory: "journal/daily"}
		id := cfg2.DailyNoteID("2025-02-01")
		expected := filepath.Join("journal/daily", "2025-02-01")
		if id != expected {
			t.Errorf("got %q, want %q", id, expected)
		}
	})
}

func TestCreateDefaultVaultConfig(t *testing.T) {
	tmpDir := t.TempDir()

	created, err := CreateDefaultVaultConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected file to be created")
	}

	// Verify file exists
	configPath := filepath.Join(tmpDir, "raven.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("raven.yaml was not created")
	}

	// Verify it can be loaded
	cfg, err := LoadVaultConfig(tmpDir)
	if err != nil {
		t.Fatalf("failed to load created config: %v", err)
	}

	if cfg.DailyDirectory != "daily" {
		t.Errorf("expected daily_directory 'daily', got %q", cfg.DailyDirectory)
	}

	// Calling again should NOT overwrite - returns false
	created2, err := CreateDefaultVaultConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if created2 {
		t.Error("expected file to NOT be created on second call (already exists)")
	}
}
