package vault

import (
	"os"
	"path/filepath"
	"testing"

	ravenignore "github.com/aidanlsb/raven/internal/ignore"
)

func TestWalkMarkdownFiles(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestWalkMarkdownFilesWithOptionsExcludesPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	files := map[string]string{
		"keep.md":                    "# Keep\n",
		"AGENTS.md":                  "# Agents\n",
		".cursor/plans/work.plan.md": "# Plan\n",
		"notes/work.plan.md":         "# Plan\n",
	}
	for relPath, content := range files {
		fullPath := filepath.Join(tmpDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", relPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", relPath, err)
		}
	}

	matcher, err := ravenignore.NewMatcher([]string{"AGENTS.md", ".cursor/", "*.plan.md"})
	if err != nil {
		t.Fatalf("NewMatcher returned error: %v", err)
	}

	var found []string
	err = WalkMarkdownFilesWithOptions(tmpDir, &WalkOptions{ExcludeMatcher: matcher}, func(result WalkResult) error {
		found = append(found, result.RelativePath)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkMarkdownFilesWithOptions returned error: %v", err)
	}
	if len(found) != 1 || found[0] != "keep.md" {
		t.Fatalf("found files = %#v, want only keep.md", found)
	}
}

func TestWalkMarkdownFilesWithOptionsSkipsParseBeforeRead(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "unchanged.md")
	if err := os.WriteFile(filePath, []byte("---\ntype: [invalid yaml\n---\n"), 0o644); err != nil {
		t.Fatalf("write unchanged.md: %v", err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat unchanged.md: %v", err)
	}

	var gateCalls int
	var results []WalkResult
	err = WalkMarkdownFilesWithOptions(tmpDir, &WalkOptions{
		ShouldParse: func(relativePath string, fileMtime int64) bool {
			gateCalls++
			if relativePath != "unchanged.md" {
				t.Fatalf("relative path = %q, want unchanged.md", relativePath)
			}
			if fileMtime != info.ModTime().Unix() {
				t.Fatalf("file mtime = %d, want %d", fileMtime, info.ModTime().Unix())
			}
			return false
		},
	}, func(result WalkResult) error {
		results = append(results, result)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkMarkdownFilesWithOptions returned error: %v", err)
	}
	if gateCalls != 1 {
		t.Fatalf("ShouldParse calls = %d, want 1", gateCalls)
	}
	if len(results) != 1 {
		t.Fatalf("results = %#v, want one skipped result", results)
	}
	if !results[0].ParseSkipped || results[0].Document != nil || results[0].Error != nil {
		t.Fatalf("result = %#v, want parse-skipped result without document or error", results[0])
	}
}

func TestWalkMarkdownFilesParallelMatchesSequential(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	for _, name := range []string{"a.md", "b.md", "c.md", "sub/d.md"} {
		dir := filepath.Join(tmpDir, filepath.Dir(name))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		content := "---\ntype: page\n---\n\n# " + name + "\n\nHello [[other]]\n"
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	collect := func(workers int) []string {
		t.Helper()
		var ids []string
		err := WalkMarkdownFilesWithOptions(tmpDir, &WalkOptions{ParseWorkers: workers}, func(result WalkResult) error {
			if result.Error != nil {
				t.Fatalf("walk error for %s: %v", result.RelativePath, result.Error)
			}
			if result.Document == nil || len(result.Document.Objects) == 0 {
				t.Fatalf("missing document for %s", result.RelativePath)
			}
			ids = append(ids, result.RelativePath+"|"+result.Document.Objects[0].ID)
			return nil
		})
		if err != nil {
			t.Fatalf("walk workers=%d: %v", workers, err)
		}
		return ids
	}

	sequential := collect(1)
	parallel := collect(4)
	if len(sequential) != 4 {
		t.Fatalf("sequential files = %d, want 4: %v", len(sequential), sequential)
	}
	if len(sequential) != len(parallel) {
		t.Fatalf("parallel files = %d, sequential = %d", len(parallel), len(sequential))
	}
	for i := range sequential {
		if sequential[i] != parallel[i] {
			t.Fatalf("order mismatch at %d: sequential=%q parallel=%q\nseq=%v\npar=%v", i, sequential[i], parallel[i], sequential, parallel)
		}
	}
}

func TestCollectDocuments(t *testing.T) {
	t.Parallel()
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
