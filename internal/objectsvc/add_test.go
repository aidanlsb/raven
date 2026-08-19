package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/filelock"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestAppendToFileWaitsForExclusiveLock(t *testing.T) {
	t.Parallel()

	destPath := filepath.Join(t.TempDir(), "target.md")
	if err := os.WriteFile(destPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	lockFile, err := os.OpenFile(destPath, os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	defer lockFile.Close()

	if err := filelock.LockExclusive(lockFile); err != nil {
		t.Fatalf("lock file: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := AppendToFile(&vaultruntime.Runtime{}, destPath, "appended", nil, false, "")
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("append completed before lock release: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := filelock.Unlock(lockFile); err != nil {
		t.Fatalf("unlock file: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("append did not complete after lock release")
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if got, want := string(content), "existing\nappended\n"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestAppendToFileReturnsInsertedLineForSectionTarget(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	destPath := filepath.Join(vaultPath, "project.md")
	content := `# Project

### Bugs / Fixes
- Existing item

### Other
- Keep this below
`
	if err := os.WriteFile(destPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	line, err := AppendToFile(&vaultruntime.Runtime{VaultPath: vaultPath}, destPath, "New bug item", nil, false, "project#bugs-fixes")
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if line != 5 {
		t.Fatalf("line = %d, want 5", line)
	}

	updated, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if got := string(updated); got != `# Project

### Bugs / Fixes
- Existing item
New bug item

### Other
- Keep this below
` {
		t.Fatalf("unexpected content:\n%s", got)
	}
}

func TestAppendUnderHeadingUsesDirectSectionRange(t *testing.T) {
	t.Parallel()

	destPath := filepath.Join(t.TempDir(), "project.md")
	content := `# Project
intro
## Child
child body
# Next
next body
`
	if err := os.WriteFile(destPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	line, err := appendUnderHeading(destPath, "direct item", "# Project")
	if err != nil {
		t.Fatalf("appendUnderHeading failed: %v", err)
	}
	if line != 3 {
		t.Fatalf("line = %d, want 3", line)
	}

	updated, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if got := string(updated); got != `# Project
intro
direct item
## Child
child body
# Next
next body
` {
		t.Fatalf("unexpected content:\n%s", got)
	}
}

func TestAppendUnderHeadingRejectsMissingHeading(t *testing.T) {
	t.Parallel()

	destPath := filepath.Join(t.TempDir(), "project.md")
	content := "# Project\n"
	if err := os.WriteFile(destPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := appendUnderHeading(destPath, "item", "## Missing")
	if err == nil {
		t.Fatal("expected missing heading error")
	}
	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if svcErr.Code != codes.ErrRefNotFound {
		t.Fatalf("error code = %q, want %q", svcErr.Code, codes.ErrRefNotFound)
	}
	if got := string(mustReadFile(t, destPath)); got != content {
		t.Fatalf("missing heading add changed file: %q", got)
	}
}

func TestAppendToFileMissingTargetReportsFileNotFound(t *testing.T) {
	t.Parallel()

	destPath := filepath.Join(t.TempDir(), "missing.md")
	_, err := AppendToFile(&vaultruntime.Runtime{}, destPath, "appended", nil, false, "")
	if err == nil {
		t.Fatal("expected missing target error")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if svcErr.Code != codes.ErrFileNotFound {
		t.Fatalf("error code = %q, want %q", svcErr.Code, codes.ErrFileNotFound)
	}
}

func TestAppendUnderHeadingWaitsForAppendLock(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	destPath := filepath.Join(vaultPath, "notes.md")
	initial := "# Notes\n\n## Captures\n"
	if err := os.WriteFile(destPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Hold the coordinating append lock externally, mirroring another writer
	// mid-flight.
	release, err := acquireAppendLock(vaultPath, destPath)
	if err != nil {
		t.Fatalf("acquire append lock: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := AppendToFile(
			&vaultruntime.Runtime{VaultPath: vaultPath},
			destPath,
			"- new capture",
			&config.CaptureConfig{Heading: "## Captures"},
			false,
			"",
		)
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("appendUnderHeading completed before lock release: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	release()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("appendUnderHeading failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("appendUnderHeading did not complete after lock release")
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(content), "- new capture") {
		t.Fatalf("captured line missing: %q", string(content))
	}
}

func TestAppendWithinObjectWaitsForAppendLock(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	destPath := filepath.Join(vaultPath, "project.md")
	initial := "# Project\n\n### Bugs\n- existing\n"
	if err := os.WriteFile(destPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	release, err := acquireAppendLock(vaultPath, destPath)
	if err != nil {
		t.Fatalf("acquire append lock: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := AppendToFile(
			&vaultruntime.Runtime{VaultPath: vaultPath},
			destPath,
			"- new bug",
			nil,
			false,
			"project#bugs",
		)
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("appendWithinObject completed before lock release: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	release()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("appendWithinObject failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("appendWithinObject did not complete after lock release")
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(content), "- new bug") {
		t.Fatalf("captured line missing: %q", string(content))
	}
}

// TestAppendUnderHeadingConcurrentCapturesNoLoss spawns many parallel captures
// under the same heading. Without a lock covering the RMW+atomicfile.WriteFile
// critical section, concurrent writers race and lose lines; with the append
// lock every captured line must be present in the final file.
func TestAppendUnderHeadingConcurrentCapturesNoLoss(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	destPath := filepath.Join(vaultPath, "log.md")
	initial := "# Log\n\n## Captures\n"
	if err := os.WriteFile(destPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	const captures = 32
	var wg sync.WaitGroup
	errs := make(chan error, captures)
	for i := 0; i < captures; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := AppendToFile(
				&vaultruntime.Runtime{VaultPath: vaultPath},
				destPath,
				fmt.Sprintf("- capture %02d", i),
				&config.CaptureConfig{Heading: "## Captures"},
				false,
				"",
			)
			if err != nil {
				errs <- fmt.Errorf("capture %d: %w", i, err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("append error: %v", err)
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	body := string(content)
	var got []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "- capture ") {
			got = append(got, line)
		}
	}
	sort.Strings(got)
	want := make([]string, captures)
	for i := 0; i < captures; i++ {
		want[i] = fmt.Sprintf("- capture %02d", i)
	}
	sort.Strings(want)
	if len(got) != captures {
		t.Fatalf("captured %d lines, want %d\nfile contents:\n%s", len(got), captures, body)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("missing/mismatched capture: got %q, want %q\nfile contents:\n%s", got[i], want[i], body)
		}
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return content
}
