package templatesvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertTemplateCode(t *testing.T, err error, want Code) *Error {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %q, got nil", want)
	}
	svcErr, ok := AsError(err)
	if !ok {
		t.Fatalf("expected templatesvc error, got %T: %v", err, err)
	}
	if svcErr.Code != want {
		t.Fatalf("error code = %q, want %q", svcErr.Code, want)
	}
	return svcErr
}

func TestWriteAndListLifecycle(t *testing.T) {
	vaultPath := t.TempDir()

	listResult, err := List(ListRequest{
		VaultPath:   vaultPath,
		TemplateDir: "templates/",
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listResult.Templates) != 0 {
		t.Fatalf("expected empty template list, got %#v", listResult.Templates)
	}

	writeCreated, err := Write(WriteRequest{
		VaultPath:   vaultPath,
		TemplateDir: "templates/",
		Path:        "meeting.md",
		Content:     "# Meeting\n",
	})
	if err != nil {
		t.Fatalf("Write (create) returned error: %v", err)
	}
	if writeCreated.Status != "created" || !writeCreated.Changed {
		t.Fatalf("unexpected create write result: %#v", writeCreated)
	}
	if writeCreated.Path != "templates/meeting.md" {
		t.Fatalf("path = %q, want %q", writeCreated.Path, "templates/meeting.md")
	}

	writeUnchanged, err := Write(WriteRequest{
		VaultPath:   vaultPath,
		TemplateDir: "templates/",
		Path:        "meeting.md",
		Content:     "# Meeting\n",
	})
	if err != nil {
		t.Fatalf("Write (unchanged) returned error: %v", err)
	}
	if writeUnchanged.Status != "unchanged" || writeUnchanged.Changed {
		t.Fatalf("unexpected unchanged write result: %#v", writeUnchanged)
	}

	writeUpdated, err := Write(WriteRequest{
		VaultPath:   vaultPath,
		TemplateDir: "templates/",
		Path:        "meeting.md",
		Content:     "# Meeting\n\n## Notes\n",
	})
	if err != nil {
		t.Fatalf("Write (update) returned error: %v", err)
	}
	if writeUpdated.Status != "updated" || !writeUpdated.Changed {
		t.Fatalf("unexpected updated write result: %#v", writeUpdated)
	}

	updatedContent, err := os.ReadFile(filepath.Join(vaultPath, "templates", "meeting.md"))
	if err != nil {
		t.Fatalf("failed to read updated template: %v", err)
	}
	if string(updatedContent) != "# Meeting\n\n## Notes\n" {
		t.Fatalf("unexpected template content: %q", string(updatedContent))
	}

	finalList, err := List(ListRequest{
		VaultPath:   vaultPath,
		TemplateDir: "templates/",
	})
	if err != nil {
		t.Fatalf("List (after write) returned error: %v", err)
	}
	if len(finalList.Templates) != 1 {
		t.Fatalf("expected 1 template file, got %d", len(finalList.Templates))
	}
	if finalList.Templates[0].Path != "templates/meeting.md" {
		t.Fatalf("template path = %q, want %q", finalList.Templates[0].Path, "templates/meeting.md")
	}
}

func TestDeleteRespectsSchemaReferences(t *testing.T) {
	vaultPath := t.TempDir()

	_, err := Write(WriteRequest{
		VaultPath:   vaultPath,
		TemplateDir: "templates/",
		Path:        "meeting.md",
		Content:     "# Meeting\n",
	})
	if err != nil {
		t.Fatalf("failed to write template fixture: %v", err)
	}

	schemaYAML := `version: 1
templates:
  meeting_template:
    file: templates/meeting.md
`
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0o644); err != nil {
		t.Fatalf("failed to write schema fixture: %v", err)
	}

	_, err = Delete(DeleteRequest{
		VaultPath:   vaultPath,
		TemplateDir: "templates/",
		Path:        "meeting.md",
		Force:       false,
	})
	svcErr := assertTemplateCode(t, err, CodeValidationFailed)
	if !strings.Contains(svcErr.Message, "meeting_template") {
		t.Fatalf("expected schema template id in error message, got %q", svcErr.Message)
	}

	if _, statErr := os.Stat(filepath.Join(vaultPath, "templates", "meeting.md")); statErr != nil {
		t.Fatalf("template should still exist after blocked delete, stat err: %v", statErr)
	}

	deleteResult, err := Delete(DeleteRequest{
		VaultPath:   vaultPath,
		TemplateDir: "templates/",
		Path:        "meeting.md",
		Force:       true,
	})
	if err != nil {
		t.Fatalf("Delete (force) returned error: %v", err)
	}
	if !deleteResult.Forced {
		t.Fatalf("expected forced delete result, got %#v", deleteResult)
	}
	if len(deleteResult.TemplateIDs) != 1 || deleteResult.TemplateIDs[0] != "meeting_template" {
		t.Fatalf("unexpected template ids: %#v", deleteResult.TemplateIDs)
	}
	if deleteResult.DeletedPath != "templates/meeting.md" {
		t.Fatalf("deleted path = %q, want %q", deleteResult.DeletedPath, "templates/meeting.md")
	}
	if !strings.HasPrefix(deleteResult.TrashPath, ".trash/templates/meeting") {
		t.Fatalf("unexpected trash path %q", deleteResult.TrashPath)
	}

	if _, statErr := os.Stat(filepath.Join(vaultPath, "templates", "meeting.md")); !os.IsNotExist(statErr) {
		t.Fatalf("source template should not exist after delete, stat err: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(vaultPath, filepath.FromSlash(deleteResult.TrashPath))); statErr != nil {
		t.Fatalf("trash file should exist, stat err: %v", statErr)
	}
}

func TestWriteRejectsPathOutsideTemplateDirectory(t *testing.T) {
	_, err := Write(WriteRequest{
		VaultPath:   t.TempDir(),
		TemplateDir: "templates/",
		Path:        "other/meeting.md",
		Content:     "# Meeting\n",
	})
	assertTemplateCode(t, err, CodeInvalidInput)
}
