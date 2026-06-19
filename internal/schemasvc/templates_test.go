package schemasvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetTemplateRejectsFrontmatter(t *testing.T) {
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("version: 2\n"), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}
	templateDir := filepath.Join(vaultPath, "templates")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "daily.md"), []byte("---\ntype: date\n---\n# {{date}}\n"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	_, err := SetTemplate(SetTemplateRequest{
		VaultPath:   vaultPath,
		TemplateID:  "daily_default",
		File:        "templates/daily.md",
		Description: "Daily template",
	})
	if err == nil {
		t.Fatal("SetTemplate expected error, got nil")
	}
	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("error = %T, want *Error", err)
	}
	if svcErr.Code != ErrorValidation {
		t.Fatalf("error code = %s, want %s", svcErr.Code, ErrorValidation)
	}
	if !strings.Contains(svcErr.Message, "frontmatter") {
		t.Fatalf("expected frontmatter validation message, got %q", svcErr.Message)
	}
}
