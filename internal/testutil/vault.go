// Package testutil provides reusable test utilities for Raven integration tests.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TestVault represents a temporary vault for testing.
type TestVault struct {
	Path   string
	t      *testing.T
	schema string
	files  map[string]string
}

// NewTestVault creates a new test vault builder.
// Call Build() to create the actual vault directory.
func NewTestVault(t *testing.T) *TestVault {
	t.Helper()
	return &TestVault{
		t:     t,
		files: make(map[string]string),
	}
}

// WithSchema sets the schema.yaml content for the vault.
func (v *TestVault) WithSchema(yaml string) *TestVault {
	v.schema = yaml
	return v
}

// WithFile adds a file to the vault.
// The path is relative to the vault root.
func (v *TestVault) WithFile(path, content string) *TestVault {
	v.files[path] = content
	return v
}

// WithRavenYAML sets the raven.yaml content for the vault.
func (v *TestVault) WithRavenYAML(yaml string) *TestVault {
	v.files["raven.yaml"] = yaml
	return v
}

// Build creates the vault directory and all configured files.
// Returns the TestVault for method chaining.
func (v *TestVault) Build() *TestVault {
	v.t.Helper()

	// Create temp directory
	v.Path = v.t.TempDir()

	// Write schema.yaml if provided
	if v.schema != "" {
		v.writeFile("schema.yaml", v.schema)
	}

	// Write all configured files
	for path, content := range v.files {
		v.writeFile(path, content)
	}

	return v
}

// writeFile writes a file to the vault, creating directories as needed.
func (v *TestVault) writeFile(relPath, content string) {
	v.t.Helper()
	fullPath := filepath.Join(v.Path, relPath)

	// Create parent directories
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		v.t.Fatalf("failed to create directory %s: %v", dir, err)
	}

	// Write file
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		v.t.Fatalf("failed to write file %s: %v", fullPath, err)
	}
}

// ReadFile reads a file from the vault.
// Returns the content as a string.
func (v *TestVault) ReadFile(relPath string) string {
	v.t.Helper()
	fullPath := filepath.Join(v.Path, relPath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		v.t.Fatalf("failed to read file %s: %v", fullPath, err)
	}
	return string(content)
}

// FileExists checks if a file exists in the vault.
func (v *TestVault) FileExists(relPath string) bool {
	v.t.Helper()
	fullPath := filepath.Join(v.Path, relPath)
	_, err := os.Stat(fullPath)
	return err == nil
}

// MinimalSchema returns a minimal valid schema.yaml content.
func MinimalSchema() string {
	return `version: 2
types:
  page:
    default_path: ""
`
}

// PersonProjectSchema returns a schema with person and project types.
func PersonProjectSchema() string {
	return `version: 2
types:
  person:
    default_path: people/
    name_field: name
    fields:
      name:
        type: string
        required: true
      email:
        type: string
  project:
    default_path: projects/
    name_field: title
    fields:
      title:
        type: string
        required: true
      status:
        type: enum
        values: [active, paused, done]
      owner:
        type: ref
        target: person
traits:
  due:
    type: date
  priority:
    type: enum
    values: [low, medium, high]
`
}
