// Package mcp provides MCP server functionality.
package mcp

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/commands"
)

const (
	compactToolDiscover = "raven_discover"
	compactToolDescribe = "raven_describe"
	compactToolInvoke   = "raven_invoke"
)

// GenerateToolSchemas returns the compact MCP surface. Each tool's input schema
// is generated from the same registry-backed parameter specs that drive strict
// argument validation (see surface.go), so the advertised schema and the
// enforced contract cannot drift apart.
func GenerateToolSchemas() []Tool {
	return []Tool{
		{
			Name:        compactToolDiscover,
			Description: "List all discoverable Raven commands with compact metadata.",
			InputSchema: inputSchemaFromParamSpecs(discoverParamOrder, discoverParamSpec()),
		},
		{
			Name:        compactToolDescribe,
			Description: "Fetch the compact invocation contract for one Raven command.",
			InputSchema: inputSchemaFromParamSpecs(describeParamOrder, describeParamSpec()),
		},
		{
			Name:        compactToolInvoke,
			Description: "Invoke any registry command with strict typed validation and policy checks (command args must be nested inside args).",
			InputSchema: inputSchemaFromParamSpecs(invokeWrapperParamOrder, invokeWrapperParamSpec()),
		},
	}
}

// inputSchemaFromParamSpecs builds an MCP tool InputSchema from an ordered set
// of parameter specs, reusing the shared commands helpers so the JSON schema
// matches the validation contract exactly.
func inputSchemaFromParamSpecs(order []string, specs map[string]parameterSpec) InputSchema {
	return InputSchema{
		Type:       "object",
		Properties: commands.ParameterProperties(order, specs),
		Required:   commands.RequiredParameterNames(order, specs),
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
