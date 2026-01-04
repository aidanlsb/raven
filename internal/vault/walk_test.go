package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWalkMarkdownFiles(t *testing.T) {
	// Create a temp directory with test files
	tmpDir, err := os.MkdirTemp("", "walk-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test structure:
	// tmpDir/
	//   page1.md
	//   subdir/
	//     page2.md
	//   .raven/
	//     index.db (should be skipped)
	//   .trash/
	//     deleted.md (should be skipped)
	//   readme.txt (should be skipped)

	// Create page1.md
	page1Content := `---
type: page
---

# Page 1
`
	if err := os.WriteFile(filepath.Join(tmpDir, "page1.md"), []byte(page1Content), 0644); err != nil {
		t.Fatalf("Failed to write page1.md: %v", err)
	}

	// Create subdir/page2.md
	if err := os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}
	page2Content := `---
type: person
---

# Freya
`
	if err := os.WriteFile(filepath.Join(tmpDir, "subdir", "page2.md"), []byte(page2Content), 0644); err != nil {
		t.Fatalf("Failed to write page2.md: %v", err)
	}

	// Create .raven/index.db (should be skipped)
	if err := os.MkdirAll(filepath.Join(tmpDir, ".raven"), 0755); err != nil {
		t.Fatalf("Failed to create .raven: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".raven", "index.db"), []byte("fake db"), 0644); err != nil {
		t.Fatalf("Failed to write index.db: %v", err)
	}

	// Create .trash/deleted.md (should be skipped - trash directory)
	if err := os.MkdirAll(filepath.Join(tmpDir, ".trash"), 0755); err != nil {
		t.Fatalf("Failed to create .trash: %v", err)
	}
	trashedContent := `---
type: person
---

# Deleted Person
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".trash", "deleted.md"), []byte(trashedContent), 0644); err != nil {
		t.Fatalf("Failed to write deleted.md: %v", err)
	}

	// Create readme.txt (should be skipped - not .md)
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("readme"), 0644); err != nil {
		t.Fatalf("Failed to write readme.txt: %v", err)
	}

	// Run walk
	var foundFiles []string
	var foundDocs int

	err = WalkMarkdownFiles(tmpDir, func(result WalkResult) error {
		foundFiles = append(foundFiles, result.RelativePath)
		if result.Document != nil {
			foundDocs++
		}
		return nil
	})

	if err != nil {
		t.Fatalf("WalkMarkdownFiles failed: %v", err)
	}

	// Should find exactly 2 markdown files
	if len(foundFiles) != 2 {
		t.Errorf("Found %d files, want 2. Files: %v", len(foundFiles), foundFiles)
	}

	// Should have parsed 2 documents
	if foundDocs != 2 {
		t.Errorf("Parsed %d documents, want 2", foundDocs)
	}

	// Verify .raven was skipped
	for _, f := range foundFiles {
		if filepath.Dir(f) == ".raven" {
			t.Errorf("Should not have found files in .raven: %s", f)
		}
	}

	// Verify .trash was skipped
	for _, f := range foundFiles {
		if filepath.Dir(f) == ".trash" {
			t.Errorf("Should not have found files in .trash: %s", f)
		}
	}

	// Verify .txt was skipped
	for _, f := range foundFiles {
		if filepath.Ext(f) == ".txt" {
			t.Errorf("Should not have found .txt files: %s", f)
		}
	}
}

func TestWalkMarkdownFilesWithErrors(t *testing.T) {
	// Create a temp directory with an invalid markdown file
	tmpDir, err := os.MkdirTemp("", "walk-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a valid file
	validContent := `---
type: page
---

# Valid
`
	if err := os.WriteFile(filepath.Join(tmpDir, "valid.md"), []byte(validContent), 0644); err != nil {
		t.Fatalf("Failed to write valid.md: %v", err)
	}

	// Create an invalid file (bad frontmatter)
	invalidContent := `---
type: [invalid yaml
---
`
	if err := os.WriteFile(filepath.Join(tmpDir, "invalid.md"), []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to write invalid.md: %v", err)
	}

	var validCount, errorCount int

	err = WalkMarkdownFiles(tmpDir, func(result WalkResult) error {
		if result.Error != nil {
			errorCount++
		} else {
			validCount++
		}
		return nil
	})

	if err != nil {
		t.Fatalf("WalkMarkdownFiles failed: %v", err)
	}

	if validCount != 1 {
		t.Errorf("Valid count = %d, want 1", validCount)
	}

	if errorCount != 1 {
		t.Errorf("Error count = %d, want 1", errorCount)
	}
}

func TestCollectDocuments(t *testing.T) {
	// Create a temp directory with test files
	tmpDir, err := os.MkdirTemp("", "collect-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create two valid files
	for _, name := range []string{"page1.md", "page2.md"} {
		content := `---
type: page
---

# ` + name + `
`
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", name, err)
		}
	}

	docs, errors, err := CollectDocuments(tmpDir)
	if err != nil {
		t.Fatalf("CollectDocuments failed: %v", err)
	}

	if len(docs) != 2 {
		t.Errorf("Got %d documents, want 2", len(docs))
	}

	if len(errors) != 0 {
		t.Errorf("Got %d errors, want 0", len(errors))
	}
}
