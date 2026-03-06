package cli

import (
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestApplyWorkflowStepPatch_DeepMergesForEachConfig(t *testing.T) {
	existing := &config.WorkflowStep{
		ID:   "fanout",
		Type: "foreach",
		ForEach: &config.WorkflowForEach{
			Items:   "{{steps.seed.data.results}}",
			OnError: "fail_fast",
			Steps: []*config.WorkflowStep{
				{
					ID:   "create",
					Type: "tool",
					Tool: "raven_upsert",
				},
			},
		},
	}

	updated, err := applyWorkflowStepPatch(existing, `{"foreach":{"on_error":"continue"}}`)
	if err != nil {
		t.Fatalf("applyWorkflowStepPatch() error: %v", err)
	}
	if updated.ForEach == nil {
		t.Fatal("expected foreach config to remain present")
	}
	if updated.ForEach.OnError != "continue" {
		t.Fatalf("foreach.on_error = %q, want %q", updated.ForEach.OnError, "continue")
	}
	if updated.ForEach.Items != existing.ForEach.Items {
		t.Fatalf("foreach.items = %q, want %q", updated.ForEach.Items, existing.ForEach.Items)
	}
	if len(updated.ForEach.Steps) != 1 || updated.ForEach.Steps[0] == nil {
		t.Fatalf("foreach.steps should be preserved, got %#v", updated.ForEach.Steps)
	}
}

func TestApplyWorkflowStepPatch_DeepMergesSwitchConfig(t *testing.T) {
	existing := &config.WorkflowStep{
		ID:   "route",
		Type: "switch",
		Switch: &config.WorkflowSwitch{
			Value: "{{steps.classify.validated_outputs.route}}",
			Cases: map[string]*config.WorkflowSwitchCase{
				"high": {
					Emit: map[string]interface{}{
						"action": "escalate",
					},
				},
			},
			Default: &config.WorkflowSwitchCase{
				Emit: map[string]interface{}{
					"action": "backlog",
					"owner":  "triage",
				},
			},
		},
	}

	updated, err := applyWorkflowStepPatch(existing, `{"switch":{"default":{"emit":{"action":"fallback"}}}}`)
	if err != nil {
		t.Fatalf("applyWorkflowStepPatch() error: %v", err)
	}
	if updated.Switch == nil {
		t.Fatal("expected switch config to remain present")
	}
	if updated.Switch.Value != existing.Switch.Value {
		t.Fatalf("switch.value = %q, want %q", updated.Switch.Value, existing.Switch.Value)
	}
	if _, ok := updated.Switch.Cases["high"]; !ok {
		t.Fatalf("switch.cases.high should be preserved, got %#v", updated.Switch.Cases)
	}
	if updated.Switch.Default == nil || updated.Switch.Default.Emit == nil {
		t.Fatalf("switch.default.emit should be preserved, got %#v", updated.Switch.Default)
	}
	if got := updated.Switch.Default.Emit["action"]; got != "fallback" {
		t.Fatalf("switch.default.emit.action = %#v, want %q", got, "fallback")
	}
	if got := updated.Switch.Default.Emit["owner"]; got != "triage" {
		t.Fatalf("switch.default.emit.owner = %#v, want %q", got, "triage")
	}
}
