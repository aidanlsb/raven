package mcp

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestExecuteToolDirectUnknownTool(t *testing.T) {
	_, err := ExecuteToolDirect("", "raven_not_real", nil)
	if err == nil {
		t.Fatal("expected unknown tool error")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("expected unknown tool error, got: %v", err)
	}
}

func TestExecuteToolDirectSuccess(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	envelope, err := ExecuteToolDirect(v.Path, "raven_new", map[string]interface{}{
		"type":  "person",
		"title": "Alice",
	})
	if err != nil {
		t.Fatalf("ExecuteToolDirect returned error: %v", err)
	}

	ok, _ := envelope["ok"].(bool)
	if !ok {
		t.Fatalf("expected ok=true envelope, got: %#v", envelope)
	}
	if !v.FileExists("people/alice.md") {
		t.Fatal("expected people/alice.md to exist")
	}
}

func TestExecuteToolDirectToolError(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	_, err := ExecuteToolDirect(v.Path, "raven_new", map[string]interface{}{
		"title": "Missing Type",
	})
	if err == nil {
		t.Fatal("expected tool error")
	}
	if !strings.Contains(err.Error(), "returned error") {
		t.Fatalf("expected envelope tool error, got: %v", err)
	}
}
