package pages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Freya", "freya"},
		{"Sif", "sif"},
		{"My Awesome Project", "my-awesome-project"},
		{"UPPER CASE", "upper-case"},
		{"test.md", "test"},
		{"file-name", "file-name"},
		{"Special: Characters!", "special-characters"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Slugify(tt.input)
			if result != tt.expected {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSlugifyPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"people/Freya", "people/freya"},
		{"people/Sif", "people/sif"},
		{"projects/My Project/docs", "projects/my-project/docs"},
		{"file.md", "file"},
		{"path/to/file.md", "path/to/file"},
		{`game-notes\Competitions`, "game-notes/competitions"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SlugifyPath(tt.input)
			if result != tt.expected {
				t.Errorf("SlugifyPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestResolveDefaultPathWithRoots(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"game_notes": {
				DefaultPath: "game-notes/",
			},
		},
	}

	tests := []struct {
		name       string
		targetPath string
		typeName   string
		objects    string
		pages      string
		want       string
	}{
		{
			name:       "applies type default path",
			targetPath: "Competitions",
			typeName:   "game_notes",
			want:       "game-notes/Competitions",
		},
		{
			name:       "nests default path under objects root",
			targetPath: "Competitions",
			typeName:   "game_notes",
			objects:    "objects/",
			want:       "objects/game-notes/Competitions",
		},
		{
			name:       "normalizes windows separator in explicit path",
			targetPath: `notes\Today`,
			typeName:   "page",
			objects:    "objects/",
			pages:      "pages/",
			want:       "objects/notes/Today",
		},
		{
			name:       "uses pages root for untyped page title",
			targetPath: "Quick Note",
			typeName:   "page",
			objects:    "objects/",
			pages:      "pages/",
			want:       "pages/Quick Note",
		},
		{
			name:       "uses objects root for typed object without default path",
			targetPath: "Freya",
			typeName:   "person",
			objects:    "objects/",
			pages:      "pages/",
			want:       "objects/Freya",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDefaultPathWithRoots(tt.targetPath, tt.typeName, sch, tt.objects, tt.pages)
			if got != tt.want {
				t.Fatalf("resolveDefaultPathWithRoots(%q, %q) = %q, want %q", tt.targetPath, tt.typeName, got, tt.want)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	// Create a temp directory for testing
	tmpDir, err := os.MkdirTemp("", "pages-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("basic page creation", func(t *testing.T) {
		result, err := Create(CreateOptions{
			VaultPath:  tmpDir,
			TypeName:   "person",
			Title:      "Freya",
			TargetPath: "people/freya",
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if result.RelativePath != "people/freya.md" {
			t.Errorf("RelativePath = %q, want %q", result.RelativePath, "people/freya.md")
		}

		// Verify file exists
		if _, err := os.Stat(result.FilePath); os.IsNotExist(err) {
			t.Error("File was not created")
		}

		// Verify content
		content, err := os.ReadFile(result.FilePath)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}
		contentStr := string(content)

		if !strings.Contains(contentStr, "type: person") {
			t.Error("File missing 'type: person' in frontmatter")
		}
		// No default heading should be added (headings create section objects)
		if strings.Contains(contentStr, "# Freya") {
			t.Error("File should NOT have a default heading (headings create section objects)")
		}
	})

	t.Run("slugified path creation", func(t *testing.T) {
		result, err := Create(CreateOptions{
			VaultPath:  tmpDir,
			TypeName:   "person",
			Title:      "Sif",
			TargetPath: "people/Sif", // Not pre-slugified
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		// Path should be slugified
		if result.SlugifiedPath != "people/sif" {
			t.Errorf("SlugifiedPath = %q, want %q", result.SlugifiedPath, "people/sif")
		}

		// No default heading should be added
		content, _ := os.ReadFile(result.FilePath)
		if strings.Contains(string(content), "# Sif") {
			t.Error("File should NOT have a default heading")
		}
	})

	t.Run("windows-style target path keeps directory structure", func(t *testing.T) {
		result, err := Create(CreateOptions{
			VaultPath:  tmpDir,
			TypeName:   "note",
			Title:      "Competitions",
			TargetPath: `game-notes\Competitions`,
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if result.RelativePath != "game-notes/competitions.md" {
			t.Errorf("RelativePath = %q, want %q", result.RelativePath, "game-notes/competitions.md")
		}

		createdPath := filepath.Join(tmpDir, "game-notes", "competitions.md")
		if _, err := os.Stat(createdPath); os.IsNotExist(err) {
			t.Errorf("expected file at %q, but it was not created", createdPath)
		}
	})

	t.Run("with fields", func(t *testing.T) {
		result, err := Create(CreateOptions{
			VaultPath:  tmpDir,
			TypeName:   "project",
			Title:      "Website",
			TargetPath: "projects/website",
			Fields: map[string]string{
				"status": "active",
				"owner":  "freya",
			},
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		content, _ := os.ReadFile(result.FilePath)
		contentStr := string(content)

		if !strings.Contains(contentStr, "status: active") {
			t.Error("File missing 'status: active' field")
		}
		if !strings.Contains(contentStr, "owner: freya") {
			t.Error("File missing 'owner: freya' field")
		}
	})

	t.Run("path escaping blocked", func(t *testing.T) {
		_, err := Create(CreateOptions{
			VaultPath:  tmpDir,
			TypeName:   "page",
			Title:      "Evil",
			TargetPath: "../escaped/evil",
		})
		if err == nil {
			t.Error("Expected error for path escaping, got nil")
		}
	})
}

func TestCreateWithTemplate(t *testing.T) {
	// Create a temp directory for testing
	tmpDir, err := os.MkdirTemp("", "pages-template-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("inline template override rejected", func(t *testing.T) {
		_, err := Create(CreateOptions{
			VaultPath:        tmpDir,
			TypeName:         "meeting",
			Title:            "Team Sync",
			TargetPath:       "meetings/team-sync",
			TemplateOverride: "# {{title}}\n\n## Notes",
			TemplateDir:      "templates/",
		})
		if err == nil {
			t.Fatal("Create expected error for inline template override, got nil")
		}
	})

	t.Run("with file template", func(t *testing.T) {
		// Create template directory and file
		templateDir := filepath.Join(tmpDir, "templates")
		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template dir: %v", err)
		}

		templateContent := "# {{title}}\n\n## Attendees\n\n## Action Items"
		if err := os.WriteFile(filepath.Join(templateDir, "meeting.md"), []byte(templateContent), 0644); err != nil {
			t.Fatalf("Failed to write template: %v", err)
		}

		result, err := Create(CreateOptions{
			VaultPath:        tmpDir,
			TypeName:         "meeting",
			Title:            "Weekly Standup",
			TargetPath:       "meetings/weekly-standup",
			TemplateOverride: "templates/meeting.md",
			TemplateDir:      "templates/",
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		content, err := os.ReadFile(result.FilePath)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}
		contentStr := string(content)

		if !strings.Contains(contentStr, "# {{title}}") {
			t.Error("Expected template content to be copied without interpolation")
		}
		if !strings.Contains(contentStr, "## Attendees") {
			t.Error("Template attendees section not present")
		}
		if !strings.Contains(contentStr, "## Action Items") {
			t.Error("Template action items section not present")
		}
	})

	t.Run("missing template file errors", func(t *testing.T) {
		_, err := Create(CreateOptions{
			VaultPath:        tmpDir,
			TypeName:         "note",
			Title:            "Quick Note",
			TargetPath:       "notes/quick-note",
			TemplateOverride: "templates/missing.md",
			TemplateDir:      "templates/",
		})
		if err == nil {
			t.Fatal("Create expected error for missing template file, got nil")
		}
	})

	t.Run("with field variables", func(t *testing.T) {
		templateDir := filepath.Join(tmpDir, "templates")
		if err := os.MkdirAll(templateDir, 0755); err != nil {
			t.Fatalf("Failed to create template dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(templateDir, "meeting-fields.md"), []byte("# {{title}}\n\n**Time:** {{field.time}}\n**Location:** {{field.location}}"), 0644); err != nil {
			t.Fatalf("Failed to write template: %v", err)
		}

		result, err := Create(CreateOptions{
			VaultPath:        tmpDir,
			TypeName:         "meeting",
			Title:            "Project Review",
			TargetPath:       "meetings/project-review",
			Fields:           map[string]string{"time": "14:00", "location": "Room A"},
			TemplateOverride: "templates/meeting-fields.md",
			TemplateDir:      "templates/",
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		content, err := os.ReadFile(result.FilePath)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}
		contentStr := string(content)

		if !strings.Contains(contentStr, "**Time:** {{field.time}}") {
			t.Error("Expected field placeholders to remain unexpanded")
		}
		if !strings.Contains(contentStr, "**Location:** {{field.location}}") {
			t.Error("Expected field placeholders to remain unexpanded")
		}
	})
}

func TestExists(t *testing.T) {
	// Create a temp directory for testing
	tmpDir, err := os.MkdirTemp("", "pages-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test file
	testDir := filepath.Join(tmpDir, "people")
	os.MkdirAll(testDir, 0755)
	testFile := filepath.Join(testDir, "freya.md")
	os.WriteFile(testFile, []byte("test"), 0644)

	t.Run("file exists", func(t *testing.T) {
		if !Exists(tmpDir, "people/freya") {
			t.Error("Exists should return true for existing file")
		}
	})

	t.Run("file does not exist", func(t *testing.T) {
		if Exists(tmpDir, "people/thor") {
			t.Error("Exists should return false for non-existing file")
		}
	})

	t.Run("slugified path exists", func(t *testing.T) {
		// Create sif.md
		os.WriteFile(filepath.Join(testDir, "sif.md"), []byte("test"), 0644)

		// Should find it via non-slugified path
		if !Exists(tmpDir, "people/Sif") {
			t.Error("Exists should find sif.md via 'Sif' path")
		}
	})
}
