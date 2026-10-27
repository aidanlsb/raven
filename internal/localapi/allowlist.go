package localapi

import "github.com/aidanlsb/raven/internal/commands"

// CommandAllowed reports whether commandID may be invoked through POST /v1/invoke.
// Eligibility is the registry policy, not a companion-specific subset: any
// invokable command is accepted. Companion clients choose which commands to call.
func CommandAllowed(commandID string) bool {
	return commands.IsInvokableCommandID(commandID)
}
