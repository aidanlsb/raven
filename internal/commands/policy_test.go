package commands

import "testing"

func TestPolicyForCommandID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		commandID     string
		wantInvokable bool
		wantDiscover  bool
	}{
		{
			name:          "default leaf operation",
			commandID:     "query",
			wantInvokable: true,
			wantDiscover:  true,
		},
		{
			name:          "non-invokable runtime command",
			commandID:     "serve",
			wantInvokable: false,
			wantDiscover:  false,
		},
		{
			name:          "non-invokable compatibility alias",
			commandID:     "schema_add",
			wantInvokable: false,
			wantDiscover:  false,
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
		})
	}
}

func TestResolveToolPolicy(t *testing.T) {
	t.Parallel()
	commandID, policy, ok := ResolveToolPolicy("raven_query")
	if !ok {
		t.Fatal("expected raven_query to resolve")
	}
	if commandID != "query" {
		t.Fatalf("commandID=%q, want query", commandID)
	}
	if !policy.Invokable || !policy.Discoverable {
		t.Fatalf("unexpected policy for query: %+v", policy)
	}

	if _, _, ok := ResolveToolPolicy("raven_not_real"); ok {
		t.Fatal("expected unknown tool to fail policy resolution")
	}
}
