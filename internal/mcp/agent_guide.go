package mcp

import (
	_ "embed"
)

// agentGuide is the embedded agent guide content from the docs directory.
// This ensures the MCP resource always matches the documentation.
//
//go:embed agent-guide.md
var agentGuide string

// getAgentGuide returns the embedded agent guide content.
// This guide helps AI agents understand how to effectively use Raven.
func getAgentGuide() string {
	return agentGuide
}
