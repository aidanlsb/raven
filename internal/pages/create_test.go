package pages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		if !strings.Contains(contentStr, "# Freya") {
			t.Error("File missing '# Freya' heading")
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

		// But heading should preserve original
		content, _ := os.ReadFile(result.FilePath)
		if !strings.Contains(string(content), "# Sif") {
			t.Error("File should preserve original title in heading")
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
