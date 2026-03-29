package commands

import "testing"

func TestPolicyForCommandID(t *testing.T) {
	tests := []struct {
		name           string
		commandID      string
		wantInvokable  bool
		wantDiscover   bool
		wantWorkflowOK bool
	}{
		{
			name:           "default leaf operation",
			commandID:      "query",
			wantInvokable:  true,
			wantDiscover:   true,
			wantWorkflowOK: true,
		},
		{
			name:           "non-invokable runtime command",
			commandID:      "serve",
			wantInvokable:  false,
			wantDiscover:   false,
			wantWorkflowOK: false,
		},
		{
			name:           "workflow disallowed prefix",
			commandID:      "workflow_run",
			wantInvokable:  true,
			wantDiscover:   true,
			wantWorkflowOK: false,
		},
		{
			name:           "workflow disallowed exact command",
			commandID:      "open",
			wantInvokable:  true,
			wantDiscover:   true,
			wantWorkflowOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PolicyForCommandID(tc.commandID)
			if got.Invokable != tc.wantInvokable {
				t.Fatalf("Invokable=%v, want %v", got.Invokable, tc.wantInvokable)
			}
			if got.Discoverable != tc.wantDiscover {
				t.Fatalf("Discoverable=%v, want %v", got.Discoverable, tc.wantDiscover)
			}
			if got.WorkflowAllowed != tc.wantWorkflowOK {
				t.Fatalf("WorkflowAllowed=%v, want %v", got.WorkflowAllowed, tc.wantWorkflowOK)
			}
		})
	}
}

func TestResolveToolPolicy(t *testing.T) {
	commandID, policy, ok := ResolveToolPolicy("raven_query")
	if !ok {
		t.Fatal("expected raven_query to resolve")
	}
	if commandID != "query" {
		t.Fatalf("commandID=%q, want query", commandID)
	}
	if !policy.Invokable || !policy.Discoverable || !policy.WorkflowAllowed {
		t.Fatalf("unexpected policy for query: %+v", policy)
	}

	if _, _, ok := ResolveToolPolicy("raven_not_real"); ok {
		t.Fatal("expected unknown tool to fail policy resolution")
	}
}
