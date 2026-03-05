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

	t.Run("accepts typed array agent outputs", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "typed-output.yaml")
		content := `description: typed output
steps:
  - id: compose
    type: agent
    prompt: "Summarize"
    outputs:
      bullets:
        type: string[]
        required: true
`
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		wf, err := Load(vaultDir, "typed-output", &config.WorkflowRef{File: "workflows/typed-output.yaml"})
		if err != nil {
			t.Fatalf("Load error: %v", err)
		}
		got := wf.Steps[0].Outputs["bullets"]
		if got == nil || got.Type != "string[]" {
			t.Fatalf("expected typed output string[], got %#v", got)
		}
	})

	t.Run("accepts foreach step with nested tool steps", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "foreach-ok.yaml")
		content := `description: foreach
steps:
  - id: seed
    type: tool
    tool: raven_query
  - id: fanout
    type: foreach
    foreach:
      items: "{{steps.seed.data.results}}"
      on_error: continue
      steps:
        - id: create
          type: tool
          tool: raven_upsert
          arguments:
            title: "{{item.title}}"
            ordinal: "{{index}}"
`
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		if _, err := Load(vaultDir, "foreach-ok", &config.WorkflowRef{File: "workflows/foreach-ok.yaml"}); err != nil {
			t.Fatalf("Load error: %v", err)
		}
	})

	t.Run("rejects foreach missing items", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "foreach-missing-items.yaml")
		content := `description: foreach
steps:
  - id: fanout
    type: foreach
    foreach:
      steps:
        - id: create
          type: tool
          tool: raven_upsert
`
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		_, err := Load(vaultDir, "foreach-missing-items", &config.WorkflowRef{File: "workflows/foreach-missing-items.yaml"})
		if err == nil || !strings.Contains(err.Error(), "missing foreach.items") {
			t.Fatalf("expected foreach.items error, got %v", err)
		}
	})

	t.Run("rejects foreach nested non-tool steps", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "foreach-non-tool.yaml")
		content := `description: foreach
steps:
  - id: fanout
    type: foreach
    foreach:
      items: "{{steps.seed.data.results}}"
      steps:
        - id: ask
          type: agent
          prompt: nope
`
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		_, err := Load(vaultDir, "foreach-non-tool", &config.WorkflowRef{File: "workflows/foreach-non-tool.yaml"})
		if err == nil || !strings.Contains(err.Error(), "must be type 'tool'") {
			t.Fatalf("expected nested tool-only error, got %v", err)
		}
	})

	t.Run("rejects foreach invalid on_error", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "foreach-on-error.yaml")
		content := `description: foreach
steps:
  - id: fanout
    type: foreach
    foreach:
      items: "{{steps.seed.data.results}}"
      on_error: maybe
      steps:
        - id: create
          type: tool
          tool: raven_upsert
`
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		_, err := Load(vaultDir, "foreach-on-error", &config.WorkflowRef{File: "workflows/foreach-on-error.yaml"})
		if err == nil || !strings.Contains(err.Error(), "invalid foreach.on_error") {
			t.Fatalf("expected foreach.on_error error, got %v", err)
		}
	})

	t.Run("accepts switch with cases and required default", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "switch-ok.yaml")
		content := `description: switch
steps:
  - id: classify
    type: agent
    prompt: classify
    outputs:
      route:
        type: string
        required: true
  - id: route
    type: switch
    switch:
      value: "{{steps.classify.validated_outputs.route}}"
      outputs:
        action:
          type: string
          required: true
      cases:
        high:
          steps:
            - id: create_incident
              type: tool
              tool: raven_upsert
          emit:
            action: "create_incident"
      default:
        emit:
          action: "fallback"
`
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		if _, err := Load(vaultDir, "switch-ok", &config.WorkflowRef{File: "workflows/switch-ok.yaml"}); err != nil {
			t.Fatalf("Load error: %v", err)
		}
	})

	t.Run("rejects switch missing value", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "switch-missing-value.yaml")
		content := `description: switch
steps:
  - id: route
    type: switch
    switch:
      cases:
        high:
          emit:
            action: high
      default:
        emit:
          action: fallback
`
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		_, err := Load(vaultDir, "switch-missing-value", &config.WorkflowRef{File: "workflows/switch-missing-value.yaml"})
		if err == nil || !strings.Contains(err.Error(), "missing switch.value") {
			t.Fatalf("expected switch.value error, got %v", err)
		}
	})

	t.Run("rejects switch missing default", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "switch-missing-default.yaml")
		content := `description: switch
steps:
  - id: route
    type: switch
    switch:
      value: "high"
      cases:
        high:
          emit:
            action: high
`
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		_, err := Load(vaultDir, "switch-missing-default", &config.WorkflowRef{File: "workflows/switch-missing-default.yaml"})
		if err == nil || !strings.Contains(err.Error(), "must define switch.default") {
			t.Fatalf("expected switch.default error, got %v", err)
		}
	})

	t.Run("rejects switch nested non-deterministic steps", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "switch-nested-agent.yaml")
		content := `description: switch
steps:
  - id: route
    type: switch
    switch:
      value: "high"
      cases:
        high:
          steps:
            - id: ask
              type: agent
              prompt: no
      default:
        emit:
          action: fallback
`
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		_, err := Load(vaultDir, "switch-nested-agent", &config.WorkflowRef{File: "workflows/switch-nested-agent.yaml"})
		if err == nil || !strings.Contains(err.Error(), "must be type 'tool' or 'foreach'") {
			t.Fatalf("expected deterministic nested-step error, got %v", err)
		}
	})

	t.Run("rejects switch outputs without per-branch emit", func(t *testing.T) {
		path := filepath.Join(vaultDir, "workflows", "switch-missing-emit.yaml")
		content := `description: switch
steps:
  - id: route
    type: switch
    switch:
      value: "high"
      outputs:
        action:
          type: string
          required: true
      cases:
        high:
          steps:
            - id: create
              type: tool
              tool: raven_upsert
      default:
        emit:
          action: fallback
`
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		_, err := Load(vaultDir, "switch-missing-emit", &config.WorkflowRef{File: "workflows/switch-missing-emit.yaml"})
		if err == nil || !strings.Contains(err.Error(), "missing emit") {
			t.Fatalf("expected missing emit error, got %v", err)
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

func TestLoad_TestdataForEachFixture(t *testing.T) {
	vaultDir, err := filepath.Abs(filepath.Join("..", "..", "testdata"))
	if err != nil {
		t.Fatalf("resolve testdata path: %v", err)
	}

	vaultCfg, err := config.LoadVaultConfig(vaultDir)
	if err != nil {
		t.Fatalf("LoadVaultConfig error: %v", err)
	}

	wf, err := Get(vaultDir, "bifrost-watch-fanout", vaultCfg)
	if err != nil {
		t.Fatalf("Get workflow error: %v", err)
	}
	if len(wf.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(wf.Steps))
	}

	fanout := wf.Steps[1]
	if fanout == nil || fanout.Type != "foreach" {
		t.Fatalf("expected second step type foreach, got %#v", fanout)
	}
	if fanout.ForEach == nil {
		t.Fatal("expected foreach config")
	}
	if fanout.ForEach.Items != "{{steps.gather_watch_notes.validated_outputs.entries}}" {
		t.Fatalf("unexpected foreach.items: %q", fanout.ForEach.Items)
	}
	if fanout.ForEach.As != "entry" || fanout.ForEach.IndexAs != "slot" {
		t.Fatalf("unexpected foreach aliases: as=%q index_as=%q", fanout.ForEach.As, fanout.ForEach.IndexAs)
	}
	if len(fanout.ForEach.Steps) != 1 || fanout.ForEach.Steps[0] == nil {
		t.Fatalf("expected one nested tool step, got %#v", fanout.ForEach.Steps)
	}
	if fanout.ForEach.Steps[0].Type != "tool" || fanout.ForEach.Steps[0].Tool != "raven_upsert" {
		t.Fatalf("unexpected nested step: %#v", fanout.ForEach.Steps[0])
	}
}

func TestLoad_TestdataSwitchFixture(t *testing.T) {
	vaultDir, err := filepath.Abs(filepath.Join("..", "..", "testdata"))
	if err != nil {
		t.Fatalf("resolve testdata path: %v", err)
	}

	wf, err := Load(vaultDir, "switch-routing", &config.WorkflowRef{File: "workflows/switch-routing.yaml"})
	if err != nil {
		t.Fatalf("Load workflow error: %v", err)
	}
	if len(wf.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(wf.Steps))
	}

	route := wf.Steps[1]
	if route == nil || route.Type != "switch" {
		t.Fatalf("expected second step type switch, got %#v", route)
	}
	if route.Switch == nil {
		t.Fatal("expected switch config")
	}
	if route.Switch.Value != "{{steps.classify.validated_outputs.route}}" {
		t.Fatalf("unexpected switch.value: %q", route.Switch.Value)
	}
	if route.Switch.Default == nil {
		t.Fatal("expected switch default branch")
	}
	if len(route.Switch.Cases) != 2 {
		t.Fatalf("expected 2 switch cases, got %d", len(route.Switch.Cases))
	}
}
