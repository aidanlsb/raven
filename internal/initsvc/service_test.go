package initsvc

import "testing"

func TestSetSwitchBackShellQuotesVaultName(t *testing.T) {
	t.Parallel()
	state := postInitState{
		previousActiveName: "work$(touch /tmp/pwn)",
		previousActivePath: "/vault/work",
	}
	setSwitchBack(&state)
	want := "rvn --json vault use -- 'work$(touch /tmp/pwn)'"
	if state.switchBack != want {
		t.Fatalf("switch_back = %q, want %q", state.switchBack, want)
	}
}
