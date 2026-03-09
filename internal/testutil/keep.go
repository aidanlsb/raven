// Package testutil provides reusable test utilities for Raven integration tests.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TestKeep represents a temporary keep for testing.
type TestKeep struct {
	Path   string
	t      *testing.T
	schema string
	files  map[string]string
}

// NewTestKeep creates a new test keep builder.
// Call Build() to create the actual keep directory.
func NewTestKeep(t *testing.T) *TestKeep {
	t.Helper()
	return &TestKeep{
		t:     t,
		files: make(map[string]string),
	}
}

// WithSchema sets the schema.yaml content for the keep.
func (v *TestKeep) WithSchema(yaml string) *TestKeep {
	v.schema = yaml
	return v
}

// WithFile adds a file to the keep.
// The path is relative to the keep root.
func (v *TestKeep) WithFile(path, content string) *TestKeep {
	v.files[path] = content
	return v
}

// WithRavenYAML sets the raven.yaml content for the keep.
func (v *TestKeep) WithRavenYAML(yaml string) *TestKeep {
	v.files["raven.yaml"] = yaml
	return v
}

// Build creates the keep directory and all configured files.
// Returns the TestKeep for method chaining.
func (v *TestKeep) Build() *TestKeep {
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

// writeFile writes a file to the keep, creating directories as needed.
func (v *TestKeep) writeFile(relPath, content string) {
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

// WriteFile writes a file to the keep, creating directories as needed.
func (v *TestKeep) WriteFile(relPath, content string) {
	v.t.Helper()
	v.writeFile(relPath, content)
}

// ReadFile reads a file from the keep.
// Returns the content as a string.
func (v *TestKeep) ReadFile(relPath string) string {
	v.t.Helper()
	fullPath := filepath.Join(v.Path, relPath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		v.t.Fatalf("failed to read file %s: %v", fullPath, err)
	}
	return string(content)
}

// FileExists checks if a file exists in the keep.
func (v *TestKeep) FileExists(relPath string) bool {
	v.t.Helper()
	fullPath := filepath.Join(v.Path, relPath)
	_, err := os.Stat(fullPath)
	return err == nil
}

// MinimalSchema returns a minimal valid schema.yaml content.
func MinimalSchema() string {
	return `version: 1
types: {}
`
}

// PersonProjectSchema returns a schema with person and project types.
func PersonProjectSchema() string {
	return `version: 1
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
