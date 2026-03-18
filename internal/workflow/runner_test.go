package workflow

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestRunner_StopsAtAgentStep(t *testing.T) {
	wf := &Workflow{
		Name: "x",
		Steps: []*config.WorkflowStep{
			{
				ID:   "fetch",
				Type: "tool",
				Tool: "raven_vault_stats",
			},
			{
				ID:     "agent",
				Type:   "agent",
				Prompt: "Status:\n{{steps.fetch.data.status}}",
				Outputs: map[string]*config.WorkflowPromptOutput{
					"markdown": {Type: "markdown", Required: true},
				},
			},
			{
				ID:   "after",
				Type: "tool",
				Tool: "raven_query",
				Arguments: map[string]interface{}{
					"query_string": "object:project",
				},
			},
		},
	}

	callCount := 0
	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		callCount++
		return map[string]interface{}{
			"ok": true,
			"data": map[string]interface{}{
				"status": "green",
			},
		}, nil
	}

	result, err := r.Run(wf, map[string]string{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Next == nil {
		t.Fatal("expected agent step in result.Next")
	}
	if result.Next.StepID != "agent" {
		t.Fatalf("unexpected Next.StepID: %s", result.Next.StepID)
	}
	if callCount != 1 {
		t.Fatalf("expected exactly one tool call before agent stop, got %d", callCount)
	}
}

func TestRunner_AgentStepTypedArrayOutputExample(t *testing.T) {
	wf := &Workflow{
		Name: "x",
		Steps: []*config.WorkflowStep{
			{
				ID:   "agent",
				Type: "agent",
				Outputs: map[string]*config.WorkflowPromptOutput{
					"bullets": {Type: "string[]", Required: true},
				},
				Prompt: "Summarize",
			},
		},
	}

	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	result, err := r.Run(wf, map[string]string{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Next == nil || result.Next.Example == nil {
		t.Fatalf("expected agent contract example, got %#v", result.Next)
	}
	outputs, ok := result.Next.Example["outputs"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected outputs object in example, got %#v", result.Next.Example["outputs"])
	}
	items, ok := outputs["bullets"].([]interface{})
	if !ok {
		t.Fatalf("expected bullets to be array in example, got %T", outputs["bullets"])
	}
	if len(items) != 0 {
		t.Fatalf("expected empty bullets example array, got %#v", items)
	}
}

func TestRunner_ReturnsStepSummariesInsteadOfStepPayloads(t *testing.T) {
	wf := &Workflow{
		Name: "x",
		Steps: []*config.WorkflowStep{
			{
				ID:   "fetch",
				Type: "tool",
				Tool: "raven_vault_stats",
			},
			{
				ID:     "compose",
				Type:   "agent",
				Prompt: "Status:\n{{steps.fetch.data.status}}",
				Outputs: map[string]*config.WorkflowPromptOutput{
					"markdown": {Type: "markdown", Required: true},
				},
			},
		},
	}

	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{
			"ok": true,
			"data": map[string]interface{}{
				"status": "green",
			},
		}, nil
	}

	result, err := r.Run(wf, map[string]string{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Next == nil {
		t.Fatal("expected awaiting agent boundary")
	}
	if len(result.StepSummaries) != 2 {
		t.Fatalf("expected 2 step summaries, got %d", len(result.StepSummaries))
	}

	byID := map[string]RunStepSummary{}
	for _, s := range result.StepSummaries {
		byID[s.StepID] = s
	}

	fetch, ok := byID["fetch"]
	if !ok {
		t.Fatal("missing fetch step summary")
	}
	if fetch.Status != "ok" || !fetch.HasOutput {
		t.Fatalf("unexpected fetch summary: %#v", fetch)
	}

	compose, ok := byID["compose"]
	if !ok {
		t.Fatal("missing compose step summary")
	}
	if compose.Status != string(RunStatusAwaitingAgent) {
		t.Fatalf("unexpected compose status: %s", compose.Status)
	}
	if !compose.HasOutput {
		t.Fatalf("expected compose step to have stored prompt state: %#v", compose)
	}
}

func TestRunner_ToolStepTypedInterpolation(t *testing.T) {
	wf := &Workflow{
		Name: "typed",
		Steps: []*config.WorkflowStep{
			{
				ID:   "fetch",
				Type: "tool",
				Tool: "raven_query",
				Arguments: map[string]interface{}{
					"query_string": "object:project",
				},
			},
			{
				ID:   "summarize",
				Type: "tool",
				Tool: "raven_upsert",
				Arguments: map[string]interface{}{
					"title": "Daily Brief",
					"type":  "brief",
					"field": map[string]interface{}{
						"count": "{{steps.fetch.data.count}}",
						"ids":   "{{steps.fetch.data.ids}}",
					},
					"content": "count={{steps.fetch.data.count}}",
				},
			},
			{
				ID:     "agent",
				Type:   "agent",
				Prompt: "Done",
			},
		},
	}

	var secondCallArgs map[string]interface{}
	callIndex := 0

	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		callIndex++
		if callIndex == 1 {
			return map[string]interface{}{
				"ok": true,
				"data": map[string]interface{}{
					"count": 3,
					"ids":   []interface{}{"p1", "p2", "p3"},
				},
			}, nil
		}
		secondCallArgs = args
		return map[string]interface{}{"ok": true, "data": map[string]interface{}{}}, nil
	}

	_, err := r.Run(wf, map[string]string{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if secondCallArgs == nil {
		t.Fatal("expected second tool call args to be captured")
	}

	field, ok := secondCallArgs["field"].(map[string]interface{})
	if !ok {
		t.Fatalf("field should be map[string]interface{}, got %T", secondCallArgs["field"])
	}
	if _, ok := field["count"].(int); !ok {
		t.Fatalf("field.count should preserve numeric type, got %T", field["count"])
	}
	if _, ok := field["ids"].([]interface{}); !ok {
		t.Fatalf("field.ids should preserve array type, got %T", field["ids"])
	}
	if secondCallArgs["content"] != "count=3" {
		t.Fatalf("content interpolation mismatch: got %#v", secondCallArgs["content"])
	}
}

func TestRunner_OptionalInputOmittedIsAddressable(t *testing.T) {
	wf := &Workflow{
		Name: "optional-inputs",
		Inputs: map[string]*config.WorkflowInput{
			"title":   {Type: "string", Required: true},
			"project": {Type: "string", Required: false},
		},
		Steps: []*config.WorkflowStep{
			{
				ID:   "create",
				Type: "tool",
				Tool: "raven_upsert",
				Arguments: map[string]interface{}{
					"type":    "analysis-plan",
					"title":   "{{inputs.title}}",
					"project": "{{inputs.project}}",
				},
			},
		},
	}

	var createArgs map[string]interface{}
	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		createArgs = args
		return map[string]interface{}{"ok": true}, nil
	}

	result, err := r.Run(wf, map[string]string{"title": "New model order"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != RunStatusCompleted {
		t.Fatalf("expected completed status, got %s", result.Status)
	}
	if createArgs == nil {
		t.Fatal("expected create tool args")
	}
	if _, ok := createArgs["project"]; !ok {
		t.Fatal("expected optional project input key to be present")
	}
	if createArgs["project"] != nil {
		t.Fatalf("expected omitted optional project to resolve as nil, got %#v", createArgs["project"])
	}
}

func TestRunner_ForEachExecutesNestedToolsWithScopedInterpolation(t *testing.T) {
	wf := &Workflow{
		Name: "fanout",
		Steps: []*config.WorkflowStep{
			{
				ID:   "seed",
				Type: "tool",
				Tool: "raven_query",
			},
			{
				ID:   "fanout",
				Type: "foreach",
				ForEach: &config.WorkflowForEach{
					Items: "{{steps.seed.data.items}}",
					Steps: []*config.WorkflowStep{
						{
							ID:   "create",
							Type: "tool",
							Tool: "raven_upsert",
							Arguments: map[string]interface{}{
								"title":  "{{item.title}}",
								"rank":   "{{index}}",
								"labels": "{{item.labels}}",
							},
						},
					},
				},
			},
		},
	}

	var createArgs []map[string]interface{}
	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		switch tool {
		case "raven_query":
			return map[string]interface{}{
				"ok": true,
				"data": map[string]interface{}{
					"items": []interface{}{
						map[string]interface{}{"title": "First", "labels": []interface{}{"x", "y"}},
						map[string]interface{}{"title": "Second", "labels": []interface{}{"z"}},
					},
				},
			}, nil
		case "raven_upsert":
			createArgs = append(createArgs, args)
			return map[string]interface{}{"ok": true, "data": map[string]interface{}{}}, nil
		default:
			return nil, fmt.Errorf("unexpected tool: %s", tool)
		}
	}

	state, err := NewRunState(wf, map[string]interface{}{})
	if err != nil {
		t.Fatalf("NewRunState error: %v", err)
	}
	result, err := r.RunWithState(wf, state)
	if err != nil {
		t.Fatalf("RunWithState returned error: %v", err)
	}
	if result.Status != RunStatusCompleted {
		t.Fatalf("expected completed status, got %s", result.Status)
	}
	if len(createArgs) != 2 {
		t.Fatalf("expected 2 fanout tool calls, got %d", len(createArgs))
	}
	if createArgs[0]["title"] != "First" || createArgs[1]["title"] != "Second" {
		t.Fatalf("unexpected titles: %#v", createArgs)
	}
	if _, ok := createArgs[0]["rank"].(int); !ok {
		t.Fatalf("expected rank to preserve numeric type, got %T", createArgs[0]["rank"])
	}
	if _, ok := createArgs[0]["labels"].([]interface{}); !ok {
		t.Fatalf("expected labels to preserve array type, got %T", createArgs[0]["labels"])
	}

	fanout, ok := state.Steps["fanout"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fanout step output map, got %T", state.Steps["fanout"])
	}
	data, ok := fanout["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fanout data map, got %T", fanout["data"])
	}
	if data["success_count"] != 2 || data["error_count"] != 0 {
		t.Fatalf("unexpected fanout summary: %#v", data)
	}
}

func TestRunner_ForEachContinueOnError(t *testing.T) {
	wf := &Workflow{
		Name: "fanout-continue",
		Steps: []*config.WorkflowStep{
			{
				ID:   "seed",
				Type: "tool",
				Tool: "raven_query",
			},
			{
				ID:   "fanout",
				Type: "foreach",
				ForEach: &config.WorkflowForEach{
					Items:   "{{steps.seed.data.items}}",
					OnError: "continue",
					Steps: []*config.WorkflowStep{
						{
							ID:   "create",
							Type: "tool",
							Tool: "raven_upsert",
							Arguments: map[string]interface{}{
								"title": "{{item.title}}",
							},
						},
					},
				},
			},
			{
				ID:   "after",
				Type: "tool",
				Tool: "raven_vault_stats",
			},
		},
	}

	afterCalled := false
	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		switch tool {
		case "raven_query":
			return map[string]interface{}{
				"ok": true,
				"data": map[string]interface{}{
					"items": []interface{}{
						map[string]interface{}{"title": "good-1"},
						map[string]interface{}{"title": "bad"},
						map[string]interface{}{"title": "good-2"},
					},
				},
			}, nil
		case "raven_upsert":
			if args["title"] == "bad" {
				return nil, fmt.Errorf("boom")
			}
			return map[string]interface{}{"ok": true}, nil
		case "raven_vault_stats":
			afterCalled = true
			return map[string]interface{}{"ok": true}, nil
		default:
			return nil, fmt.Errorf("unexpected tool: %s", tool)
		}
	}

	state, err := NewRunState(wf, map[string]interface{}{})
	if err != nil {
		t.Fatalf("NewRunState error: %v", err)
	}
	result, err := r.RunWithState(wf, state)
	if err != nil {
		t.Fatalf("RunWithState returned error: %v", err)
	}
	if result.Status != RunStatusCompleted {
		t.Fatalf("expected completed status, got %s", result.Status)
	}
	if !afterCalled {
		t.Fatal("expected workflow to continue after foreach errors")
	}

	fanout, ok := state.Steps["fanout"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fanout step output map, got %T", state.Steps["fanout"])
	}
	data, ok := fanout["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fanout data map, got %T", fanout["data"])
	}
	if data["success_count"] != 2 || data["error_count"] != 1 {
		t.Fatalf("unexpected fanout summary: %#v", data)
	}
}

func TestRunner_ForEachFailFastReturnsError(t *testing.T) {
	wf := &Workflow{
		Name: "fanout-fail-fast",
		Steps: []*config.WorkflowStep{
			{
				ID:   "seed",
				Type: "tool",
				Tool: "raven_query",
			},
			{
				ID:   "fanout",
				Type: "foreach",
				ForEach: &config.WorkflowForEach{
					Items: "{{steps.seed.data.items}}",
					Steps: []*config.WorkflowStep{
						{
							ID:   "create",
							Type: "tool",
							Tool: "raven_upsert",
							Arguments: map[string]interface{}{
								"title": "{{item.title}}",
							},
						},
					},
				},
			},
			{
				ID:   "after",
				Type: "tool",
				Tool: "raven_vault_stats",
			},
		},
	}

	afterCalled := false
	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		switch tool {
		case "raven_query":
			return map[string]interface{}{
				"ok": true,
				"data": map[string]interface{}{
					"items": []interface{}{
						map[string]interface{}{"title": "ok"},
						map[string]interface{}{"title": "bad"},
					},
				},
			}, nil
		case "raven_upsert":
			if args["title"] == "bad" {
				return nil, fmt.Errorf("tool failed")
			}
			return map[string]interface{}{"ok": true}, nil
		case "raven_vault_stats":
			afterCalled = true
			return map[string]interface{}{"ok": true}, nil
		default:
			return nil, fmt.Errorf("unexpected tool: %s", tool)
		}
	}

	state, err := NewRunState(wf, map[string]interface{}{})
	if err != nil {
		t.Fatalf("NewRunState error: %v", err)
	}
	_, err = r.RunWithState(wf, state)
	if err == nil {
		t.Fatal("expected fail-fast foreach to return error")
	}
	if !strings.Contains(err.Error(), "foreach item 1 nested step 'create'") {
		t.Fatalf("unexpected error: %v", err)
	}
	if afterCalled {
		t.Fatal("after step should not run on fail-fast foreach error")
	}
}

func TestRunner_SwitchRoutesToMatchingCase(t *testing.T) {
	wf := &Workflow{
		Name: "switch-match",
		Steps: []*config.WorkflowStep{
			{
				ID:   "classify",
				Type: "tool",
				Tool: "raven_query",
			},
			{
				ID:   "route",
				Type: "switch",
				Switch: &config.WorkflowSwitch{
					Value: "{{steps.classify.data.route}}",
					Outputs: map[string]*config.WorkflowPromptOutput{
						"action": {Type: "string", Required: true},
						"target": {Type: "string", Required: true},
					},
					Cases: map[string]*config.WorkflowSwitchCase{
						"high": {
							Steps: []*config.WorkflowStep{
								{
									ID:   "create_incident",
									Type: "tool",
									Tool: "raven_upsert",
								},
							},
							Emit: map[string]interface{}{
								"action": "create_incident",
								"target": "{{steps.create_incident.data.object_id}}",
							},
						},
					},
					Default: &config.WorkflowSwitchCase{
						Emit: map[string]interface{}{
							"action": "fallback",
							"target": "none",
						},
					},
				},
			},
		},
	}

	callOrder := make([]string, 0, 2)
	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		callOrder = append(callOrder, tool)
		switch tool {
		case "raven_query":
			return map[string]interface{}{
				"ok": true,
				"data": map[string]interface{}{
					"route": "high",
				},
			}, nil
		case "raven_upsert":
			return map[string]interface{}{
				"ok": true,
				"data": map[string]interface{}{
					"object_id": "incident/critical-db",
				},
			}, nil
		default:
			return nil, fmt.Errorf("unexpected tool: %s", tool)
		}
	}

	state, err := NewRunState(wf, map[string]interface{}{})
	if err != nil {
		t.Fatalf("NewRunState error: %v", err)
	}
	result, err := r.RunWithState(wf, state)
	if err != nil {
		t.Fatalf("RunWithState returned error: %v", err)
	}
	if result.Status != RunStatusCompleted {
		t.Fatalf("expected completed status, got %s", result.Status)
	}
	if len(callOrder) != 2 || callOrder[0] != "raven_query" || callOrder[1] != "raven_upsert" {
		t.Fatalf("unexpected tool call order: %#v", callOrder)
	}

	routeStep, ok := state.Steps["route"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected switch step output map, got %T", state.Steps["route"])
	}
	data, ok := routeStep["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected switch data map, got %T", routeStep["data"])
	}
	if data["selected_case"] != "high" {
		t.Fatalf("expected selected_case=high, got %#v", data["selected_case"])
	}
	output, ok := data["output"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected switch output map, got %T", data["output"])
	}
	if output["action"] != "create_incident" || output["target"] != "incident/critical-db" {
		t.Fatalf("unexpected converged switch output: %#v", output)
	}
}

func TestRunner_SwitchFallsBackToDefaultCase(t *testing.T) {
	wf := &Workflow{
		Name: "switch-default",
		Steps: []*config.WorkflowStep{
			{
				ID:   "classify",
				Type: "tool",
				Tool: "raven_query",
			},
			{
				ID:   "route",
				Type: "switch",
				Switch: &config.WorkflowSwitch{
					Value: "{{steps.classify.data.route}}",
					Cases: map[string]*config.WorkflowSwitchCase{
						"high": {
							Emit: map[string]interface{}{
								"action": "create_incident",
							},
						},
					},
					Default: &config.WorkflowSwitchCase{
						Emit: map[string]interface{}{
							"action": "fallback",
						},
					},
				},
			},
		},
	}

	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		if tool != "raven_query" {
			return nil, fmt.Errorf("unexpected tool: %s", tool)
		}
		return map[string]interface{}{
			"ok": true,
			"data": map[string]interface{}{
				"route": "unknown",
			},
		}, nil
	}

	state, err := NewRunState(wf, map[string]interface{}{})
	if err != nil {
		t.Fatalf("NewRunState error: %v", err)
	}
	_, err = r.RunWithState(wf, state)
	if err != nil {
		t.Fatalf("RunWithState returned error: %v", err)
	}

	routeStep, ok := state.Steps["route"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected switch step output map, got %T", state.Steps["route"])
	}
	data, ok := routeStep["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected switch data map, got %T", routeStep["data"])
	}
	if data["selected_case"] != "default" {
		t.Fatalf("expected selected_case=default, got %#v", data["selected_case"])
	}
}

func TestRunner_SwitchReturnsErrorOnInvalidEmitType(t *testing.T) {
	wf := &Workflow{
		Name: "switch-invalid-emit",
		Steps: []*config.WorkflowStep{
			{
				ID:   "classify",
				Type: "tool",
				Tool: "raven_query",
			},
			{
				ID:   "route",
				Type: "switch",
				Switch: &config.WorkflowSwitch{
					Value: "{{steps.classify.data.route}}",
					Outputs: map[string]*config.WorkflowPromptOutput{
						"action": {Type: "string", Required: true},
					},
					Cases: map[string]*config.WorkflowSwitchCase{
						"high": {
							Emit: map[string]interface{}{
								"action": 123,
							},
						},
					},
					Default: &config.WorkflowSwitchCase{
						Emit: map[string]interface{}{
							"action": "fallback",
						},
					},
				},
			},
		},
	}

	r := NewRunner("/tmp/vault", &config.VaultConfig{})
	r.ToolFunc = func(tool string, args map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{
			"ok": true,
			"data": map[string]interface{}{
				"route": "high",
			},
		}, nil
	}

	state, err := NewRunState(wf, map[string]interface{}{})
	if err != nil {
		t.Fatalf("NewRunState error: %v", err)
	}
	_, err = r.RunWithState(wf, state)
	if err == nil {
		t.Fatal("expected switch emit validation error")
	}
	if !strings.Contains(err.Error(), "emit invalid") {
		t.Fatalf("unexpected error: %v", err)
	}
}
