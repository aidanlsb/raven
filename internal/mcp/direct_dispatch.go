package mcp

import (
	"github.com/aidanlsb/raven/internal/commands"
)

func (s *Server) callToolDirect(name string, args map[string]interface{}) (string, bool, bool) {
	commandID, ok := commands.ResolveToolCommandID(name)
	if !ok {
		return "", false, false
	}

	out, isErr, handled := s.callCanonicalCommand(commandID, args)
	if handled {
		return out, isErr, true
	}

	return errorEnvelope(
		"INTERNAL_ERROR",
		"canonical handler is not configured for command",
		"report this issue with the failing command id",
		map[string]interface{}{"tool_name": name, "command": commandID},
	), true, true
}
