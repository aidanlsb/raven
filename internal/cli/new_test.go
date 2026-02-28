package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var captureStdoutMu sync.Mutex

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	captureStdoutMu.Lock()
	defer captureStdoutMu.Unlock()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	os.Stdout = w

	outputCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		var buf bytes.Buffer
		_, copyErr := io.Copy(&buf, r)
		_ = r.Close()
		if copyErr != nil {
			errCh <- copyErr
			return
		}
		outputCh <- buf.String()
	}()

	fn()

	os.Stdout = orig
	_ = w.Close()
	select {
	case err := <-errCh:
		t.Fatalf("io.Copy: %v", err)
		return ""
	case output := <-outputCh:
		return output
	}
}

func TestNewAutoFillsNameFieldFromPositionalTitle(t *testing.T) {
	vaultPath := t.TempDir()

	// Schema with name_field set to 'title' - the positional title should auto-fill it.
	schemaYAML := strings.TrimSpace(`
version: 2
types:
  book:
    default_path: books/
    name_field: title
    fields:
      title:
        type: string
        required: true
`) + "\n"

	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	// Isolate global state used by the CLI package.
	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := newFieldFlags
	prevPath := newPathFlag
	prevPathChanged := newCmd.Flags().Lookup("path").Changed
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		newFieldFlags = prevFields
		newPathFlag = prevPath
		newCmd.Flags().Lookup("path").Changed = prevPathChanged
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	newFieldFlags = nil // simulate MCP/agent that didn't provide --field title=...
	newPathFlag = ""
	newCmd.Flags().Lookup("path").Changed = false

	if err := newCmd.RunE(newCmd, []string{"book", "My Book"}); err != nil {
		t.Fatalf("newCmd.RunE: %v", err)
	}

	created := filepath.Join(vaultPath, "books", "my-book.md")
	b, err := os.ReadFile(created)
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	got := string(b)

	if !strings.Contains(got, "type: book") {
		t.Fatalf("expected type frontmatter, got:\n%s", got)
	}
	if !strings.Contains(got, "title: My Book") {
		t.Fatalf("expected auto-filled title field via name_field, got:\n%s", got)
	}
}

func TestNewDoesNotOverrideExplicitNameField(t *testing.T) {
	vaultPath := t.TempDir()

	// Schema with name_field - explicit --field should take precedence over positional title
	schemaYAML := strings.TrimSpace(`
version: 2
types:
  book:
    default_path: books/
    name_field: title
    fields:
      title:
        type: string
        required: true
`) + "\n"

	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := newFieldFlags
	prevPath := newPathFlag
	prevPathChanged := newCmd.Flags().Lookup("path").Changed
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		newFieldFlags = prevFields
		newPathFlag = prevPath
		newCmd.Flags().Lookup("path").Changed = prevPathChanged
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	newFieldFlags = []string{"title=Override Title"}
	newPathFlag = ""
	newCmd.Flags().Lookup("path").Changed = false

	if err := newCmd.RunE(newCmd, []string{"book", "My Book 2"}); err != nil {
		t.Fatalf("newCmd.RunE: %v", err)
	}

	created := filepath.Join(vaultPath, "books", "my-book-2.md")
	b, err := os.ReadFile(created)
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	got := string(b)

	if strings.Contains(got, "title: My Book 2") {
		t.Fatalf("did not expect positional title to override explicit field, got:\n%s", got)
	}
	if !strings.Contains(got, "title: Override Title") {
		t.Fatalf("expected explicit title field to be preserved, got:\n%s", got)
	}
}

func TestNewFieldJSONPreservesStringType(t *testing.T) {
	vaultPath := t.TempDir()

	schemaYAML := strings.TrimSpace(`
version: 2
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
`) + "\n"
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := newFieldFlags
	prevFieldJSON := newFieldJSON
	prevPath := newPathFlag
	prevPathChanged := newCmd.Flags().Lookup("path").Changed
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		newFieldFlags = prevFields
		newFieldJSON = prevFieldJSON
		newPathFlag = prevPath
		newCmd.Flags().Lookup("path").Changed = prevPathChanged
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	newFieldFlags = nil
	newFieldJSON = `{"email":"true"}`
	newPathFlag = ""
	newCmd.Flags().Lookup("path").Changed = false

	if err := newCmd.RunE(newCmd, []string{"person", "Field Json User"}); err != nil {
		t.Fatalf("newCmd.RunE: %v", err)
	}

	created := filepath.Join(vaultPath, "people", "field-json-user.md")
	b, err := os.ReadFile(created)
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `email: "true"`) {
		t.Fatalf("expected email string literal to be preserved, got:\n%s", got)
	}
}

func TestNewFileExistsEmitsJSONErrorInJSONMode(t *testing.T) {
	vaultPath := t.TempDir()

	schemaYAML := strings.TrimSpace(`
version: 2
types:
  person:
    default_path: people/
`) + "\n"

	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	// Isolate global state used by the CLI package.
	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := newFieldFlags
	prevPath := newPathFlag
	prevPathChanged := newCmd.Flags().Lookup("path").Changed
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		newFieldFlags = prevFields
		newPathFlag = prevPath
		newCmd.Flags().Lookup("path").Changed = prevPathChanged
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	newFieldFlags = nil
	newPathFlag = ""
	newCmd.Flags().Lookup("path").Changed = false

	// First run creates the file successfully.
	_ = captureStdout(t, func() {
		if err := newCmd.RunE(newCmd, []string{"person", "Freya"}); err != nil {
			t.Fatalf("newCmd.RunE (first): %v", err)
		}
	})

	// Second run should emit a structured JSON error (and return nil error in JSON mode).
	out := captureStdout(t, func() {
		if err := newCmd.RunE(newCmd, []string{"person", "Freya"}); err != nil {
			t.Fatalf("newCmd.RunE (second): %v", err)
		}
	})

	var resp struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("expected JSON output, got parse error: %v; out=%s", err, out)
	}
	if resp.OK {
		t.Fatalf("expected ok=false; out=%s", out)
	}
	if resp.Error == nil || resp.Error.Code != ErrFileExists {
		t.Fatalf("expected error.code=%s, got %#v; out=%s", ErrFileExists, resp.Error, out)
	}
}

func TestNewRejectsTitleWithPathSeparator(t *testing.T) {
	vaultPath := t.TempDir()

	schemaYAML := strings.TrimSpace(`
version: 2
types:
  person:
    default_path: people/
`) + "\n"

	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := newFieldFlags
	prevPath := newPathFlag
	prevPathChanged := newCmd.Flags().Lookup("path").Changed
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		newFieldFlags = prevFields
		newPathFlag = prevPath
		newCmd.Flags().Lookup("path").Changed = prevPathChanged
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	newFieldFlags = nil
	newPathFlag = ""
	newCmd.Flags().Lookup("path").Changed = false

	out := captureStdout(t, func() {
		if err := newCmd.RunE(newCmd, []string{"person", "folder/name"}); err != nil {
			t.Fatalf("newCmd.RunE: %v", err)
		}
	})

	var resp struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("expected JSON output, got parse error: %v; out=%s", err, out)
	}
	if resp.OK {
		t.Fatalf("expected ok=false; out=%s", out)
	}
	if resp.Error == nil || resp.Error.Code != ErrInvalidInput {
		t.Fatalf("expected error.code=%s, got %#v; out=%s", ErrInvalidInput, resp.Error, out)
	}
	if !strings.Contains(resp.Error.Message, "title cannot contain path separators") {
		t.Fatalf("expected path separator validation message, got: %q", resp.Error.Message)
	}
}

func TestNewUsesExplicitPathWhenProvided(t *testing.T) {
	vaultPath := t.TempDir()

	schemaYAML := strings.TrimSpace(`
version: 2
types:
  note:
    default_path: note/
`) + "\n"
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := newFieldFlags
	prevPath := newPathFlag
	prevPathChanged := newCmd.Flags().Lookup("path").Changed
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		newFieldFlags = prevFields
		newPathFlag = prevPath
		newCmd.Flags().Lookup("path").Changed = prevPathChanged
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	newFieldFlags = nil
	newPathFlag = "custom/raven-logo-brief"
	newCmd.Flags().Lookup("path").Changed = true

	out := captureStdout(t, func() {
		if err := newCmd.RunE(newCmd, []string{"note", "Raven Move Friction"}); err != nil {
			t.Fatalf("newCmd.RunE: %v", err)
		}
	})

	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			File string `json:"file"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("expected JSON output, got parse error: %v; out=%s", err, out)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true; out=%s", out)
	}
	if resp.Data.File != "custom/raven-logo-brief.md" {
		t.Fatalf("expected explicit path to be used, got %q", resp.Data.File)
	}
}

func TestNewRejectsDirectoryOnlyPath(t *testing.T) {
	vaultPath := t.TempDir()

	schemaYAML := strings.TrimSpace(`
version: 2
types:
  note:
    default_path: note/
`) + "\n"
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := newFieldFlags
	prevPath := newPathFlag
	prevPathChanged := newCmd.Flags().Lookup("path").Changed
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		newFieldFlags = prevFields
		newPathFlag = prevPath
		newCmd.Flags().Lookup("path").Changed = prevPathChanged
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	newFieldFlags = nil
	newPathFlag = "note/"
	newCmd.Flags().Lookup("path").Changed = true

	out := captureStdout(t, func() {
		if err := newCmd.RunE(newCmd, []string{"note", "Raven Move Friction"}); err != nil {
			t.Fatalf("newCmd.RunE: %v", err)
		}
	})

	var resp struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("expected JSON output, got parse error: %v; out=%s", err, out)
	}
	if resp.OK {
		t.Fatalf("expected ok=false; out=%s", out)
	}
	if resp.Error == nil || resp.Error.Code != ErrInvalidInput {
		t.Fatalf("expected error.code=%s, got %#v; out=%s", ErrInvalidInput, resp.Error, out)
	}
}

func TestNewPageUsesObjectRootWhenPageRootOmitted(t *testing.T) {
	vaultPath := t.TempDir()

	schemaYAML := strings.TrimSpace(`
version: 2
types:
  person:
    default_path: people/
`) + "\n"
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	ravenYAML := strings.TrimSpace(`
directories:
  object: objects/
`) + "\n"
	if err := os.WriteFile(filepath.Join(vaultPath, "raven.yaml"), []byte(ravenYAML), 0o644); err != nil {
		t.Fatalf("write raven.yaml: %v", err)
	}

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := newFieldFlags
	prevPath := newPathFlag
	prevPathChanged := newCmd.Flags().Lookup("path").Changed
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		newFieldFlags = prevFields
		newPathFlag = prevPath
		newCmd.Flags().Lookup("path").Changed = prevPathChanged
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	newFieldFlags = nil
	newPathFlag = ""
	newCmd.Flags().Lookup("path").Changed = false

	out := captureStdout(t, func() {
		if err := newCmd.RunE(newCmd, []string{"page", "Quick Note"}); err != nil {
			t.Fatalf("newCmd.RunE: %v", err)
		}
	})

	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			File string `json:"file"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("expected JSON output, got parse error: %v; out=%s", err, out)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true; out=%s", out)
	}
	if resp.Data.File != "objects/quick-note.md" {
		t.Fatalf("expected file path under objects root, got %q", resp.Data.File)
	}

	if _, err := os.Stat(filepath.Join(vaultPath, "objects", "quick-note.md")); err != nil {
		t.Fatalf("expected created page file in objects root: %v", err)
	}
}

func TestNewUsesTemplateIDFromSchema(t *testing.T) {
	vaultPath := t.TempDir()

	schemaYAML := strings.TrimSpace(`
version: 2
templates:
  interview_technical:
    file: templates/interview/technical.md
types:
  interview:
    default_path: interviews/
    templates: [interview_technical]
`) + "\n"
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(vaultPath, "templates", "interview"), 0o755); err != nil {
		t.Fatalf("mkdir templates/interview: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "templates", "interview", "technical.md"), []byte("## Technical Interview\n"), 0o644); err != nil {
		t.Fatalf("write template file: %v", err)
	}

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := newFieldFlags
	prevPath := newPathFlag
	prevTemplate := newTemplate
	prevPathChanged := newCmd.Flags().Lookup("path").Changed
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		newFieldFlags = prevFields
		newPathFlag = prevPath
		newTemplate = prevTemplate
		newCmd.Flags().Lookup("path").Changed = prevPathChanged
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	newFieldFlags = nil
	newPathFlag = ""
	newTemplate = "interview_technical"
	newCmd.Flags().Lookup("path").Changed = false

	if err := newCmd.RunE(newCmd, []string{"interview", "Jane Doe"}); err != nil {
		t.Fatalf("newCmd.RunE: %v", err)
	}

	created := filepath.Join(vaultPath, "interviews", "jane-doe.md")
	contentBytes, err := os.ReadFile(created)
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	content := string(contentBytes)
	if !strings.Contains(content, "## Technical Interview") {
		t.Fatalf("expected selected template content in created file, got:\n%s", content)
	}
}

func TestNewWithoutDefaultTemplateCreatesWithoutTemplate(t *testing.T) {
	vaultPath := t.TempDir()

	schemaYAML := strings.TrimSpace(`
version: 2
templates:
  interview_screen:
    file: templates/interview/screen.md
types:
  interview:
    default_path: interviews/
    templates: [interview_screen]
`) + "\n"
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(vaultPath, "templates", "interview"), 0o755); err != nil {
		t.Fatalf("mkdir templates/interview: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "templates", "interview", "screen.md"), []byte("## Screening Template\n"), 0o644); err != nil {
		t.Fatalf("write template file: %v", err)
	}

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := newFieldFlags
	prevPath := newPathFlag
	prevTemplate := newTemplate
	prevPathChanged := newCmd.Flags().Lookup("path").Changed
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		newFieldFlags = prevFields
		newPathFlag = prevPath
		newTemplate = prevTemplate
		newCmd.Flags().Lookup("path").Changed = prevPathChanged
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	newFieldFlags = nil
	newPathFlag = ""
	newTemplate = ""
	newCmd.Flags().Lookup("path").Changed = false

	if err := newCmd.RunE(newCmd, []string{"interview", "No Template Interview"}); err != nil {
		t.Fatalf("newCmd.RunE: %v", err)
	}

	created := filepath.Join(vaultPath, "interviews", "no-template-interview.md")
	contentBytes, err := os.ReadFile(created)
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	content := string(contentBytes)
	if strings.Contains(content, "## Screening Template") {
		t.Fatalf("did not expect template content when default_template is unset, got:\n%s", content)
	}
}
