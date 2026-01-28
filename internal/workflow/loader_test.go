package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestLoad_InlineAndErrors(t *testing.T) {
	vaultDir := t.TempDir()

	t.Run("nil ref", func(t *testing.T) {
		_, err := Load(vaultDir, "x", nil)
		if err == nil || !strings.Contains(err.Error(), "workflow reference is nil") {
			t.Fatalf("expected nil ref error, got %v", err)
		}
	})

	t.Run("conflicting file and inline fields", func(t *testing.T) {
		_, err := Load(vaultDir, "x", &config.WorkflowRef{
			File:  "wf.yaml",
			Steps: []*config.WorkflowStep{{ID: "q", Type: "query", RQL: "object:project"}},
		})
		if err == nil || !strings.Contains(err.Error(), "both 'file' and inline") {
			t.Fatalf("expected conflict error, got %v", err)
		}
	})

	t.Run("missing steps for inline workflow", func(t *testing.T) {
		_, err := Load(vaultDir, "x", &config.WorkflowRef{Description: "d"})
		if err == nil || !strings.Contains(err.Error(), "must have either 'file' or 'steps'") {
			t.Fatalf("expected missing steps error, got %v", err)
		}
	})

	t.Run("inline workflow loads", func(t *testing.T) {
		ref := &config.WorkflowRef{
			Description: "desc",
			Inputs: map[string]*config.WorkflowInput{
				"name": {Type: "string", Required: true},
			},
			Steps: []*config.WorkflowStep{
				{ID: "q", Type: "query", RQL: "object:project"},
			},
		}
		wf, err := Load(vaultDir, "greet", ref)
		if err != nil {
			t.Fatalf("Load error: %v", err)
		}
		if wf.Name != "greet" || wf.Description != "desc" || len(wf.Steps) != 1 {
			t.Fatalf("unexpected workflow: %+v", wf)
		}
		if wf.Inputs == nil || wf.Inputs["name"] == nil || wf.Inputs["name"].Type != "string" {
			t.Fatalf("expected inputs to be preserved, got %+v", wf.Inputs)
		}
	})
}

func TestLoad_FromFile(t *testing.T) {
	vaultDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(vaultDir, "workflows"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Run("loads from file", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "w1.yaml")
		if err := os.WriteFile(path, []byte("description: test\nsteps:\n  - id: q\n    type: query\n    rql: object:project\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		wf, err := Load(vaultDir, "w1", &config.WorkflowRef{File: "workflows/w1.yaml"})
		if err != nil {
			t.Fatalf("Load error: %v", err)
		}
		if wf.Name != "w1" || wf.Description != "test" || len(wf.Steps) != 1 {
			t.Fatalf("unexpected workflow: %+v", wf)
		}
	})

	t.Run("file must be within vault", func(t *testing.T) {
		outside := filepath.Join(filepath.Dir(vaultDir), "outside.yaml")
		if err := os.WriteFile(outside, []byte("prompt: hi\n"), 0o644); err != nil {
			t.Fatalf("write outside: %v", err)
		}

		_, err := Load(vaultDir, "bad", &config.WorkflowRef{File: "../outside.yaml"})
		if err == nil || !strings.Contains(err.Error(), "within vault") {
			t.Fatalf("expected security error, got %v", err)
		}
	})

	t.Run("file must include prompt", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "noprompt.yaml")
		if err := os.WriteFile(path, []byte("description: x\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		_, err := Load(vaultDir, "noprompt", &config.WorkflowRef{File: "workflows/noprompt.yaml"})
		if err == nil || !strings.Contains(err.Error(), "must have 'steps' field") {
			t.Fatalf("expected missing steps error, got %v", err)
		}
	})
}

func TestGetAndList(t *testing.T) {
	vaultDir := t.TempDir()

	t.Run("Get fails when no workflows configured", func(t *testing.T) {
		_, err := Get(vaultDir, "x", &config.VaultConfig{})
		if err == nil || !strings.Contains(err.Error(), "no workflows defined") {
			t.Fatalf("expected no workflows error, got %v", err)
		}
	})

	t.Run("Get fails when workflow missing", func(t *testing.T) {
		vc := &config.VaultConfig{
			Workflows: map[string]*config.WorkflowRef{
				"a": {Steps: []*config.WorkflowStep{{ID: "q", Type: "query", RQL: "object:project"}}},
			},
		}
		_, err := Get(vaultDir, "missing", vc)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not found error, got %v", err)
		}
	})

	t.Run("List includes errors in description rather than failing", func(t *testing.T) {
		vc := &config.VaultConfig{
			Workflows: map[string]*config.WorkflowRef{
				"good": {Steps: []*config.WorkflowStep{{ID: "q", Type: "query", RQL: "object:project"}}},
				"bad":  {File: "workflows/does-not-exist.yaml"},
			},
		}

		items, err := List(vaultDir, vc)
		if err != nil {
			t.Fatalf("List error: %v", err)
		}
		if len(items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(items))
		}

		var badDesc string
		for _, it := range items {
			if it.Name == "bad" {
				badDesc = it.Description
			}
		}
		if badDesc == "" || !strings.Contains(badDesc, "(error:") {
			t.Fatalf("expected bad workflow to include error description, got %q", badDesc)
		}
	})
}
