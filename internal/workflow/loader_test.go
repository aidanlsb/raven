package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

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
			Steps: []*config.WorkflowStep{{ID: "q", Type: "tool", Tool: "raven_query"}},
		})
		if err == nil || !strings.Contains(err.Error(), "must contain only 'file'") {
			t.Fatalf("expected conflict error, got %v", err)
		}
	})

	t.Run("inline workflow declarations are rejected", func(t *testing.T) {
		_, err := Load(vaultDir, "x", &config.WorkflowRef{Description: "d"})
		if err == nil || !strings.Contains(err.Error(), "inline workflow definitions are not supported") {
			t.Fatalf("expected inline unsupported error, got %v", err)
		}
	})

	t.Run("legacy top-level keys are rejected in inline definitions", func(t *testing.T) {
		var ref config.WorkflowRef
		err := yaml.Unmarshal([]byte("description: d\nprompt: hi\n"), &ref)
		if err == nil || !strings.Contains(err.Error(), "legacy top-level key 'prompt'") {
			t.Fatalf("expected legacy prompt key error, got %v", err)
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
		if err := os.WriteFile(path, []byte("description: test\nsteps:\n  - id: q\n    type: tool\n    tool: raven_query\n"), 0o644); err != nil {
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

	t.Run("resolves bare filename under default workflow directory", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "w1-bare.yaml")
		if err := os.WriteFile(path, []byte("description: bare\nsteps:\n  - id: q\n    type: tool\n    tool: raven_query\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		wf, err := Load(vaultDir, "w1-bare", &config.WorkflowRef{File: "w1-bare.yaml"})
		if err != nil {
			t.Fatalf("Load error: %v", err)
		}
		if wf.Name != "w1-bare" {
			t.Fatalf("expected workflow name w1-bare, got %s", wf.Name)
		}
	})

	t.Run("enforces configured workflow directory", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(vaultDir, "automation", "flows"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		path := filepath.Join(vaultDir, "automation", "flows", "w2.yaml")
		if err := os.WriteFile(path, []byte("description: test\nsteps:\n  - id: q\n    type: tool\n    tool: raven_query\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		_, err := LoadWithConfig(vaultDir, "w2", &config.WorkflowRef{File: "automation/flows/w2.yaml"}, &config.VaultConfig{
			Directories: &config.DirectoriesConfig{
				Workflow: "workflows/",
			},
		})
		if err == nil || !strings.Contains(err.Error(), "workflow file must be under directories.workflow") {
			t.Fatalf("expected directories.workflow enforcement error, got %v", err)
		}
	})

	t.Run("loads from custom workflow directory", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(vaultDir, "automation", "flows"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		path := filepath.Join(vaultDir, "automation", "flows", "w3.yaml")
		if err := os.WriteFile(path, []byte("description: test\nsteps:\n  - id: q\n    type: tool\n    tool: raven_query\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		wf, err := LoadWithConfig(vaultDir, "w3", &config.WorkflowRef{File: "automation/flows/w3.yaml"}, &config.VaultConfig{
			Directories: &config.DirectoriesConfig{
				Workflow: "automation/flows/",
			},
		})
		if err != nil {
			t.Fatalf("LoadWithConfig error: %v", err)
		}
		if wf.Name != "w3" {
			t.Fatalf("expected workflow name w3, got %s", wf.Name)
		}
	})

	t.Run("resolves bare filename under custom workflow directory", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(vaultDir, "automation", "flows"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		path := filepath.Join(vaultDir, "automation", "flows", "w4.yaml")
		if err := os.WriteFile(path, []byte("description: test\nsteps:\n  - id: q\n    type: tool\n    tool: raven_query\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		wf, err := LoadWithConfig(vaultDir, "w4", &config.WorkflowRef{File: "w4.yaml"}, &config.VaultConfig{
			Directories: &config.DirectoriesConfig{
				Workflow: "automation/flows/",
			},
		})
		if err != nil {
			t.Fatalf("LoadWithConfig error: %v", err)
		}
		if wf.Name != "w4" {
			t.Fatalf("expected workflow name w4, got %s", wf.Name)
		}
	})

	t.Run("legacy top-level keys are rejected in workflow files", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "shorthand.yaml")
		if err := os.WriteFile(path, []byte("description: test\nprompt: hi\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		_, err := Load(vaultDir, "shorthand", &config.WorkflowRef{File: "workflows/shorthand.yaml"})
		if err == nil || !strings.Contains(err.Error(), "legacy top-level key 'prompt'") {
			t.Fatalf("expected legacy key error, got %v", err)
		}
	})

	t.Run("file must be within vault", func(t *testing.T) {
		outside := filepath.Join(filepath.Dir(vaultDir), "outside.yaml")
		if err := os.WriteFile(outside, []byte("prompt: hi\n"), 0o644); err != nil {
			t.Fatalf("write outside: %v", err)
		}

		_, err := Load(vaultDir, "bad", &config.WorkflowRef{File: "../outside.yaml"})
		if err == nil || !strings.Contains(err.Error(), "cannot escape the vault") {
			t.Fatalf("expected security error, got %v", err)
		}
	})

	t.Run("file must include steps", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "noprompt.yaml")
		if err := os.WriteFile(path, []byte("description: x\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		_, err := Load(vaultDir, "noprompt", &config.WorkflowRef{File: "workflows/noprompt.yaml"})
		if err == nil || !strings.Contains(err.Error(), "must define 'steps'") {
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
				"a": {File: "workflows/a.yaml"},
			},
		}
		_, err := Get(vaultDir, "missing", vc)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not found error, got %v", err)
		}
	})

	t.Run("List includes errors in description rather than failing", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(vaultDir, "workflows"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(vaultDir, "workflows", "good.yaml"), []byte("description: ok\nsteps:\n  - id: q\n    type: tool\n    tool: raven_query\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		vc := &config.VaultConfig{
			Workflows: map[string]*config.WorkflowRef{
				"good": {File: "workflows/good.yaml"},
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

func TestResolveWorkflowFileRef(t *testing.T) {
	t.Run("keeps explicit workflow-relative path", func(t *testing.T) {
		got, err := ResolveWorkflowFileRef("workflows/brief.yaml", "workflows/")
		if err != nil {
			t.Fatalf("ResolveWorkflowFileRef error: %v", err)
		}
		if got != "workflows/brief.yaml" {
			t.Fatalf("expected workflows/brief.yaml, got %q", got)
		}
	})

	t.Run("resolves bare filename under workflow directory", func(t *testing.T) {
		got, err := ResolveWorkflowFileRef("brief.yaml", "workflows/")
		if err != nil {
			t.Fatalf("ResolveWorkflowFileRef error: %v", err)
		}
		if got != "workflows/brief.yaml" {
			t.Fatalf("expected workflows/brief.yaml, got %q", got)
		}
	})

	t.Run("normalizes windows separators", func(t *testing.T) {
		got, err := ResolveWorkflowFileRef(`workflows\brief.yaml`, "workflows/")
		if err != nil {
			t.Fatalf("ResolveWorkflowFileRef error: %v", err)
		}
		if got != "workflows/brief.yaml" {
			t.Fatalf("expected workflows/brief.yaml, got %q", got)
		}
	})

	t.Run("rejects path outside configured workflow directory", func(t *testing.T) {
		_, err := ResolveWorkflowFileRef("automation/brief.yaml", "workflows/")
		if err == nil || !strings.Contains(err.Error(), "workflow file must be under directories.workflow") {
			t.Fatalf("expected directories.workflow enforcement error, got %v", err)
		}
	})
}
