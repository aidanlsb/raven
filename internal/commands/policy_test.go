package commands

import "testing"

func TestPolicyForCommandID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		commandID     string
		wantInvokable bool
	}{
		{
			name:          "default leaf operation",
			commandID:     "query",
			wantInvokable: true,
		},
		{
			name:          "non-invokable runtime command",
			commandID:     "mcp_serve",
			wantInvokable: false,
		},
		{
			name:          "non-invokable grouping command",
			commandID:     "template",
			wantInvokable: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PolicyForCommandID(tc.commandID)
			if got.Invokable != tc.wantInvokable {
				t.Fatalf("Invokable=%v, want %v", got.Invokable, tc.wantInvokable)
			}
		})
	}
}

func TestRemovedVaultCompatibilityAliases(t *testing.T) {
	t.Parallel()
	for _, commandID := range []string{"vault_current", "vault_path"} {
		if _, ok := Registry[commandID]; ok {
			t.Errorf("%s must not remain registered", commandID)
		}
	}
}

func TestInvokableRegistryCommandsAreDiscoverable(t *testing.T) {
	t.Parallel()

	for commandID := range Registry {
		policy := PolicyForCommandID(commandID)
		_, blocked := nonInvokableCommandIDs[commandID]
		if policy.Invokable == blocked {
			t.Errorf("%s Invokable=%v, blocked=%v", commandID, policy.Invokable, blocked)
		}
	}

	contracts := DiscoverableContracts()
	seen := make(map[string]struct{}, len(contracts))
	for _, contract := range contracts {
		seen[contract.CommandID] = struct{}{}
		if !IsInvokableCommandID(contract.CommandID) {
			t.Errorf("discoverable contract %q is not invokable", contract.CommandID)
		}
	}
	for commandID := range Registry {
		_, listed := seen[commandID]
		if IsInvokableCommandID(commandID) != listed {
			t.Errorf("%s discoverable=%v, invokable=%v", commandID, listed, IsInvokableCommandID(commandID))
		}
	}
}
