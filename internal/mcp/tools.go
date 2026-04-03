// Package mcp provides MCP server functionality.
package mcp

import (
	"fmt"
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
			Description: "List all discoverable Raven commands with compact metadata.",
			InputSchema: InputSchema{
				Type:       "object",
				Properties: map[string]interface{}{},
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
						"description": "Command identifier (e.g. query or schema add type)",
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
						"description": "Command identifier (e.g. query or schema add type)",
					},
					"args": map[string]interface{}{
						"type":        "object",
						"description": "Command-specific arguments. Put parameters from raven_describe here.",
					},
					"vault": map[string]interface{}{
						"type":        "string",
						"description": "Optional configured vault name to use for this invocation only.",
					},
					"vault_path": map[string]interface{}{
						"type":        "string",
						"description": "Optional absolute vault path to use for this invocation only.",
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
