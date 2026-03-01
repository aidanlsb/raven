package config

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestLoadVaultConfig(t *testing.T) {
	t.Run("default config when file missing", func(t *testing.T) {
		// Use a temp directory without a raven.yaml
		tmpDir := t.TempDir()

		cfg, err := LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.GetDailyDirectory() != "daily" {
			t.Errorf("expected directories.daily 'daily', got %q", cfg.GetDailyDirectory())
		}
		if cfg.GetWorkflowDirectory() != "workflows/" {
			t.Errorf("expected default directories.workflow 'workflows/', got %q", cfg.GetWorkflowDirectory())
		}
		if cfg.GetTemplateDirectory() != "templates/" {
			t.Errorf("expected default directories.template 'templates/', got %q", cfg.GetTemplateDirectory())
		}
	})

	t.Run("loads custom config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "raven.yaml")

		content := "directories:\n  daily: journal/\n"
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.GetDailyDirectory() != "journal" {
			t.Errorf("expected directories.daily 'journal', got %q", cfg.GetDailyDirectory())
		}
		if cfg.GetWorkflowDirectory() != "workflows/" {
			t.Errorf("expected default directories.workflow 'workflows/', got %q", cfg.GetWorkflowDirectory())
		}
		if cfg.GetTemplateDirectory() != "templates/" {
			t.Errorf("expected default directories.template 'templates/', got %q", cfg.GetTemplateDirectory())
		}
	})

	t.Run("defaults empty daily directory", func(t *testing.T) {
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

		if cfg.GetDailyDirectory() != "daily" {
			t.Errorf("expected default daily directory 'daily', got %q", cfg.GetDailyDirectory())
		}
		if cfg.GetWorkflowDirectory() != "workflows/" {
			t.Errorf("expected default directories.workflow 'workflows/', got %q", cfg.GetWorkflowDirectory())
		}
		if cfg.GetTemplateDirectory() != "templates/" {
			t.Errorf("expected default directories.template 'templates/', got %q", cfg.GetTemplateDirectory())
		}
	})

	t.Run("normalizes directories.workflow", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "raven.yaml")

		content := "directories:\n  workflow: ./automation/workflows\n"
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := cfg.GetWorkflowDirectory(); got != "automation/workflows/" {
			t.Errorf("expected normalized directories.workflow 'automation/workflows/', got %q", got)
		}
	})

	t.Run("normalizes directories.template", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "raven.yaml")

		content := "directories:\n  template: ./content/templates\n"
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := cfg.GetTemplateDirectory(); got != "content/templates/" {
			t.Errorf("expected normalized directories.template 'content/templates/', got %q", got)
		}
	})

	t.Run("rejects legacy daily_directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "raven.yaml")

		content := "daily_directory: journal\n"
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		if _, err := LoadVaultConfig(tmpDir); err == nil {
			t.Fatal("expected error for legacy daily_directory, got nil")
		}
	})

	t.Run("supports legacy top-level workflow_directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "raven.yaml")

		content := "workflow_directory: automation/workflows\n"
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := cfg.GetWorkflowDirectory(); got != "automation/workflows/" {
			t.Errorf("expected normalized legacy workflow_directory to map to directories.workflow, got %q", got)
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
		expected := path.Join("daily", "2025-02-01")
		if id != expected {
			t.Errorf("got %q, want %q", id, expected)
		}
	})

	t.Run("custom directory", func(t *testing.T) {
		cfg2 := &VaultConfig{DailyDirectory: "journal/daily"}
		id := cfg2.DailyNoteID("2025-02-01")
		expected := path.Join("journal/daily", "2025-02-01")
		if id != expected {
			t.Errorf("got %q, want %q", id, expected)
		}
	})
}

func TestDirectoriesConfig(t *testing.T) {
	t.Run("loads directories config with singular keys", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "raven.yaml")

		content := `
directories:
  object: object/
  page: page/
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
		if dirs.Object != "object/" {
			t.Errorf("expected object 'object/', got %q", dirs.Object)
		}
		if dirs.Page != "page/" {
			t.Errorf("expected page 'page/', got %q", dirs.Page)
		}
	})

	t.Run("defaults page root to object root when omitted", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "raven.yaml")

		content := `
directories:
  object: objects/
`
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		dirs := cfg.GetDirectoriesConfig()
		if dirs.Object != "objects/" {
			t.Errorf("expected object root 'objects/', got %q", dirs.Object)
		}
		if dirs.Page != "objects/" {
			t.Errorf("expected page root to default to object root 'objects/', got %q", dirs.Page)
		}
	})

	t.Run("backwards compatibility with plural keys", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "raven.yaml")

		// Old plural key format should still work
		content := `
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
		// Should be normalized to the new singular field names
		if dirs.Object != "objects/" {
			t.Errorf("expected object 'objects/' (from plural key), got %q", dirs.Object)
		}
		if dirs.Page != "pages/" {
			t.Errorf("expected page 'pages/' (from plural key), got %q", dirs.Page)
		}
	})

	t.Run("singular keys take precedence over plural", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "raven.yaml")

		// Both singular and plural keys - singular should win
		content := `
directories:
  object: new-object/
  objects: old-objects/
  page: new-page/
  pages: old-pages/
`
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		dirs := cfg.GetDirectoriesConfig()
		if dirs.Object != "new-object/" {
			t.Errorf("expected singular 'new-object/' to take precedence, got %q", dirs.Object)
		}
		if dirs.Page != "new-page/" {
			t.Errorf("expected singular 'new-page/' to take precedence, got %q", dirs.Page)
		}
	})

	t.Run("workflow key uses singular and falls back to plural", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "raven.yaml")

		content := `
directories:
  workflow: automation/workflows
  workflows: old/workflows
`
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := cfg.GetWorkflowDirectory(); got != "automation/workflows/" {
			t.Errorf("expected singular workflow to take precedence, got %q", got)
		}

		content = `
directories:
  workflows: old/workflows
`
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}
		cfg, err = LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := cfg.GetWorkflowDirectory(); got != "old/workflows/" {
			t.Errorf("expected plural workflows fallback, got %q", got)
		}
	})

	t.Run("template key uses singular and falls back to plural", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "raven.yaml")

		content := `
directories:
  template: content/templates
  templates: old/templates
`
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}

		cfg, err := LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := cfg.GetTemplateDirectory(); got != "content/templates/" {
			t.Errorf("expected singular template to take precedence, got %q", got)
		}

		content = `
directories:
  templates: old/templates
`
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write config: %v", err)
		}
		cfg, err = LoadVaultConfig(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := cfg.GetTemplateDirectory(); got != "old/templates/" {
			t.Errorf("expected plural templates fallback, got %q", got)
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
				Object: "object/",
				Page:   "page/",
			},
		}

		tests := []struct {
			filePath string
			expected string
		}{
			{"object/person/freya.md", "person/freya"},
			{"object/project/website.md", "project/website"},
			{"page/my-note.md", "my-note"},
			{"daily/2025-01-01.md", "daily/2025-01-01"}, // Not in object or page
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
				Object: "object/",
				Page:   "page/",
			},
		}

		tests := []struct {
			objectID string
			typeName string
			expected string
		}{
			{"person/freya", "person", "object/person/freya.md"},
			{"project/website", "project", "object/project/website.md"},
			{"my-note", "page", "page/my-note.md"},
			{"random-note", "", "page/random-note.md"},
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
				Object: "object/",
				Page:   "page/",
			},
		}

		tests := []struct {
			ref      string
			expected string
		}{
			{"person/freya", "object/person/freya.md"},
			{"project/website", "object/project/website.md"},
			{"my-note", "page/my-note.md"},
			{"object/person/freya", "object/person/freya.md"},
			{"page/my-note", "page/my-note.md"},
			{"object/person/freya.md", "object/person/freya.md"},
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

	if cfg.GetDailyDirectory() != "daily" {
		t.Errorf("expected directories.daily 'daily', got %q", cfg.GetDailyDirectory())
	}
	if cfg.GetWorkflowDirectory() != "workflows/" {
		t.Errorf("expected default directories.workflow 'workflows/', got %q", cfg.GetWorkflowDirectory())
	}
	if cfg.GetObjectsRoot() != "object/" {
		t.Errorf("expected default directories.object 'object/', got %q", cfg.GetObjectsRoot())
	}
	if cfg.GetPagesRoot() != "page/" {
		t.Errorf("expected default directories.page 'page/', got %q", cfg.GetPagesRoot())
	}
	if cfg.GetTemplateDirectory() != "templates/" {
		t.Errorf("expected default directories.template 'templates/', got %q", cfg.GetTemplateDirectory())
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "workflows", "onboard.yaml")); os.IsNotExist(err) {
		t.Error("expected default onboard workflow file to be created")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "daily")); os.IsNotExist(err) {
		t.Error("expected default daily directory to be created")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "object")); os.IsNotExist(err) {
		t.Error("expected default object directory to be created")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "page")); os.IsNotExist(err) {
		t.Error("expected default page directory to be created")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "templates")); os.IsNotExist(err) {
		t.Error("expected default template directory to be created")
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

func TestDefaultVaultConfigSavedQueriesMatchDefaultSchema(t *testing.T) {
	tmpDir := t.TempDir()

	if _, err := CreateDefaultVaultConfig(tmpDir); err != nil {
		t.Fatalf("failed to create default vault config: %v", err)
	}
	if _, err := schema.CreateDefault(tmpDir); err != nil {
		t.Fatalf("failed to create default schema: %v", err)
	}

	cfg, err := LoadVaultConfig(tmpDir)
	if err != nil {
		t.Fatalf("failed to load default vault config: %v", err)
	}
	sch, err := schema.Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to load default schema: %v", err)
	}

	validator := query.NewValidator(sch)
	for name, savedQuery := range cfg.Queries {
		t.Run(name, func(t *testing.T) {
			if savedQuery == nil {
				t.Fatalf("saved query %q is nil", name)
			}
			parsed, err := query.Parse(savedQuery.Query)
			if err != nil {
				t.Fatalf("saved query %q failed to parse: %v", name, err)
			}
			if err := validator.Validate(parsed); err != nil {
				t.Fatalf("saved query %q does not match default schema: %v", name, err)
			}
		})
	}
}
