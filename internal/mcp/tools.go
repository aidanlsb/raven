// Package mcp provides MCP server functionality.
package mcp

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/commands"
)

const (
	compactToolDiscover = "raven_discover"
	compactToolDescribe = "raven_describe"
	compactToolInvoke   = "raven_invoke"
)

// GenerateToolSchemas returns the compact MCP surface.
func GenerateToolSchemas() []Tool {
	return []Tool{
		{
			Name:        compactToolDiscover,
			Description: "Search and browse discoverable Raven commands with compact metadata.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Filter by keyword against command id/name/summary/category",
					},
					"category": map[string]interface{}{
						"type":        "string",
						"description": "Filter by category (query, content, schema, workflow, vault, navigation, maintenance)",
					},
					"mode": map[string]interface{}{
						"type":        "string",
						"description": "Filter by mode (read, write)",
					},
					"risk": map[string]interface{}{
						"type":        "string",
						"description": "Filter by risk (safe, mutating, destructive)",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum results to return (default 25, max 200)",
					},
					"cursor": map[string]interface{}{
						"type":        "string",
						"description": "Pagination cursor returned by a previous discover call",
					},
				},
			},
		},
		{
			Name:        compactToolDescribe,
			Description: "Fetch the compact invocation contract for one Raven command.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Command identifier (e.g. query, raven_query, or schema add type)",
					},
				},
				Required: []string{"command"},
			},
		},
		{
			Name:        compactToolInvoke,
			Description: "Invoke any registry command with strict typed validation and policy checks (command args must be nested inside args).",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Command identifier (e.g. query, raven_query, or schema add type)",
					},
					"args": map[string]interface{}{
						"type":        "object",
						"description": "Command-specific arguments. Put parameters from raven_describe here.",
					},
					"schema_hash": map[string]interface{}{
						"type":        "string",
						"description": "Optional schema hash returned by raven_describe",
					},
					"strict_schema": map[string]interface{}{
						"type":        "boolean",
						"description": "When true (default), reject invocation if provided schema_hash is stale",
					},
				},
				Required: []string{"command"},
			},
		},
	}
}

// mcpToolName converts a CLI command name to an MCP tool name.
// e.g., "new" -> "raven_new", "schema add type" -> "raven_schema_add_type"
func mcpToolName(cmdName string) string {
	// Replace spaces with underscores
	name := strings.ReplaceAll(cmdName, " ", "_")
	return "raven_" + name
}

// CLICommandName converts an MCP tool name back to CLI command name.
// e.g., "raven_new" -> "new", "raven_schema_add_type" -> "schema add type"
func CLICommandName(toolName string) string {
	if id, ok := commands.ResolveToolCommandID(toolName); ok {
		if meta, ok := commands.Registry[id]; ok {
			return meta.Name
		}
		return strings.ReplaceAll(id, "_", " ")
	}

	raw := strings.TrimPrefix(toolName, "raven_")
	return strings.ReplaceAll(raw, "_", " ")
}

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%v", val)
	case bool:
		if val {
			return "true"
		}
		return ""
	default:
		return ""
	}
}
