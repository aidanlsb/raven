package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// AssertFileExists fails the test if the file does not exist.
func (v *TestVault) AssertFileExists(relPath string) {
	v.t.Helper()
	fullPath := filepath.Join(v.Path, relPath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		v.t.Errorf("expected file to exist: %s", relPath)
	}
}

// AssertFileNotExists fails the test if the file exists.
func (v *TestVault) AssertFileNotExists(relPath string) {
	v.t.Helper()
	fullPath := filepath.Join(v.Path, relPath)
	if _, err := os.Stat(fullPath); err == nil {
		v.t.Errorf("expected file to not exist: %s", relPath)
	}
}

// AssertFileContains fails the test if the file does not contain the substring.
func (v *TestVault) AssertFileContains(relPath, substr string) {
	v.t.Helper()
	content := v.ReadFile(relPath)
	if !strings.Contains(content, substr) {
		v.t.Errorf("expected file %s to contain %q, got:\n%s", relPath, substr, content)
	}
}

// AssertFileNotContains fails the test if the file contains the substring.
func (v *TestVault) AssertFileNotContains(relPath, substr string) {
	v.t.Helper()
	content := v.ReadFile(relPath)
	if strings.Contains(content, substr) {
		v.t.Errorf("expected file %s to not contain %q, got:\n%s", relPath, substr, content)
	}
}

// AssertDirExists fails the test if the directory does not exist.
func (v *TestVault) AssertDirExists(relPath string) {
	v.t.Helper()
	fullPath := filepath.Join(v.Path, relPath)
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		v.t.Errorf("expected directory to exist: %s", relPath)
		return
	}
	if !info.IsDir() {
		v.t.Errorf("expected %s to be a directory, but it's a file", relPath)
	}
}

// AssertObjectExists runs a query to check if an object exists by ID.
func (v *TestVault) AssertObjectExists(objectID string) {
	v.t.Helper()
	// Use the read command to check if the object exists
	result := v.RunCLI("read", objectID)
	if !result.OK {
		v.t.Errorf("expected object to exist: %s, got error: %v", objectID, result.Error)
	}
}

// AssertObjectNotExists runs a query to check that an object does not exist.
func (v *TestVault) AssertObjectNotExists(objectID string) {
	v.t.Helper()
	result := v.RunCLI("read", objectID)
	if result.OK {
		v.t.Errorf("expected object to not exist: %s, but it does", objectID)
	}
}

// AssertQueryCount runs a query and verifies the result count.
func (v *TestVault) AssertQueryCount(query string, expectedCount int) {
	v.t.Helper()
	result := v.RunCLI("query", query)
	result.MustSucceed(v.t)

	// Query results are in "items" field
	results := result.DataList("items")
	if len(results) != expectedCount {
		v.t.Errorf("query %q: expected %d results, got %d\nRaw: %s",
			query, expectedCount, len(results), result.RawJSON)
	}
}

// AssertBacklinks verifies that an object has the expected number of backlinks.
func (v *TestVault) AssertBacklinks(objectID string, expectedCount int) {
	v.t.Helper()
	result := v.RunCLI("backlinks", objectID)
	result.MustSucceed(v.t)

	// Backlinks are in "items" field
	results := result.DataList("items")
	if len(results) != expectedCount {
		v.t.Errorf("backlinks for %s: expected %d, got %d\nRaw: %s",
			objectID, expectedCount, len(results), result.RawJSON)
	}
}

// AssertHasWarning checks that the result contains a warning with the given code.
func (r *CLIResult) AssertHasWarning(t *testing.T, code string) {
	t.Helper()
	for _, w := range r.Warnings {
		if w.Code == code {
			return
		}
	}
	t.Errorf("expected warning with code %s, got warnings: %+v", code, r.Warnings)
}

// AssertNoWarnings checks that the result has no warnings.
func (r *CLIResult) AssertNoWarnings(t *testing.T) {
	t.Helper()
	if len(r.Warnings) > 0 {
		t.Errorf("expected no warnings, got: %+v", r.Warnings)
	}
}

// AssertResultCount checks that a query result has the expected count.
func (r *CLIResult) AssertResultCount(t *testing.T, key string, expected int) {
	t.Helper()
	results := r.DataList(key)
	if len(results) != expected {
		t.Errorf("expected %d %s, got %d\nRaw: %s", expected, key, len(results), r.RawJSON)
	}
}
