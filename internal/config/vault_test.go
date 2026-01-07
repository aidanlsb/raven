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

func TestDirectoriesConfig(t *testing.T) {
	t.Run("loads directories config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "raven.yaml")

		content := `
daily_directory: daily
directories:
  objects: objects/
  pages: pages/
`
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !cfg.HasDirectoriesConfig() {
			t.Error("expected HasDirectoriesConfig to return true")
		}

		dirs := cfg.GetDirectoriesConfig()
		if dirs.Objects != "objects/" {
			t.Errorf("expected objects 'objects/', got %q", dirs.Objects)
		}
		if dirs.Pages != "pages/" {
			t.Errorf("expected pages 'pages/', got %q", dirs.Pages)
		}
	})

	t.Run("nil when not configured", func(t *testing.T) {
		cfg := &VaultConfig{DailyDirectory: "daily"}

		if cfg.HasDirectoriesConfig() {
			t.Error("expected HasDirectoriesConfig to return false")
		}
		if cfg.GetDirectoriesConfig() != nil {
			t.Error("expected GetDirectoriesConfig to return nil")
		}
	})

	t.Run("FilePathToObjectID with roots", func(t *testing.T) {
		cfg := &VaultConfig{
			DailyDirectory: "daily",
			Directories: &DirectoriesConfig{
				Objects: "objects/",
				Pages:   "pages/",
			},
		}

		tests := []struct {
			filePath string
			expected string
		}{
			{"objects/people/freya.md", "people/freya"},
			{"objects/projects/website.md", "projects/website"},
			{"pages/my-note.md", "my-note"},
			{"daily/2025-01-01.md", "daily/2025-01-01"}, // Not in objects or pages
		}

		for _, tc := range tests {
			got := cfg.FilePathToObjectID(tc.filePath)
			if got != tc.expected {
				t.Errorf("FilePathToObjectID(%q) = %q, want %q", tc.filePath, got, tc.expected)
			}
		}
	})

	t.Run("ObjectIDToFilePath with roots", func(t *testing.T) {
		cfg := &VaultConfig{
			DailyDirectory: "daily",
			Directories: &DirectoriesConfig{
				Objects: "objects/",
				Pages:   "pages/",
			},
		}

		tests := []struct {
			objectID string
			typeName string
			expected string
		}{
			{"people/freya", "person", "objects/people/freya.md"},
			{"projects/website", "project", "objects/projects/website.md"},
			{"my-note", "page", "pages/my-note.md"},
			{"random-note", "", "pages/random-note.md"},
		}

		for _, tc := range tests {
			got := cfg.ObjectIDToFilePath(tc.objectID, tc.typeName)
			if got != tc.expected {
				t.Errorf("ObjectIDToFilePath(%q, %q) = %q, want %q", tc.objectID, tc.typeName, got, tc.expected)
			}
		}
	})

	t.Run("ResolveReferenceToFilePath", func(t *testing.T) {
		cfg := &VaultConfig{
			DailyDirectory: "daily",
			Directories: &DirectoriesConfig{
				Objects: "objects/",
				Pages:   "pages/",
			},
		}

		tests := []struct {
			ref      string
			expected string
		}{
			{"people/freya", "objects/people/freya.md"},
			{"projects/website", "objects/projects/website.md"},
			{"my-note", "pages/my-note.md"},
		}

		for _, tc := range tests {
			got := cfg.ResolveReferenceToFilePath(tc.ref)
			if got != tc.expected {
				t.Errorf("ResolveReferenceToFilePath(%q) = %q, want %q", tc.ref, got, tc.expected)
			}
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
