package mcp

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestExecuteToolDirectUnknownTool(t *testing.T) {
	_, err := ExecuteToolDirect("", "not_real", nil)
	if err == nil {
		t.Fatal("expected unknown command error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got: %v", err)
	}
}

func TestExecuteToolDirectSuccess(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	envelope, err := ExecuteToolDirect(v.Path, "new", map[string]interface{}{
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

	_, err := ExecuteToolDirect(v.Path, "new", map[string]interface{}{
		"title": "Missing Type",
	})
	if err == nil {
		t.Fatal("expected tool error")
	}
	if !strings.Contains(err.Error(), "returned error") {
		t.Fatalf("expected envelope tool error, got: %v", err)
	}
}

func TestExecuteWorkflowToolDirectRejectsDisallowedTool(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	_, err := ExecuteWorkflowToolDirect(v.Path, "raven_open", map[string]interface{}{
		"reference": "missing",
	})
	if err == nil {
		t.Fatal("expected disallowed workflow tool error")
	}
	if !strings.Contains(err.Error(), "not allowed in workflow steps") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteWorkflowToolDirectNormalizesToolAliases(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	envelope, err := ExecuteWorkflowToolDirect(v.Path, "new", map[string]interface{}{
		"type":  "person",
		"title": "Bob",
	})
	if err != nil {
		t.Fatalf("ExecuteWorkflowToolDirect returned error: %v", err)
	}

	ok, _ := envelope["ok"].(bool)
	if !ok {
		t.Fatalf("expected ok=true envelope, got: %#v", envelope)
	}
	if !v.FileExists("people/bob.md") {
		t.Fatal("expected people/bob.md to exist")
	}
}
