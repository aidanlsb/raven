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
			name:          "non-invokable grouping command",
			commandID:     "template",
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
