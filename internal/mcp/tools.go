// Package mcp provides MCP server functionality.
package mcp

import (
	"fmt"
	"strings"

	"github.com/ravenscroftj/raven/internal/commands"
)

// GenerateToolSchemas generates MCP tool schemas from the command registry.
// This ensures MCP tools stay in sync with CLI commands automatically.
func GenerateToolSchemas() []Tool {
	var tools []Tool

	for cmdName, meta := range commands.Registry {
		tool := Tool{
			Name:        mcpToolName(cmdName),
			Description: meta.Description,
			InputSchema: InputSchema{
				Type:       "object",
				Properties: make(map[string]interface{}),
			},
		}

		// Add long description if available
		if meta.LongDesc != "" {
			tool.Description = meta.LongDesc
		}

		// Add arguments as properties
		var required []string
		for _, arg := range meta.Args {
			tool.InputSchema.Properties[arg.Name] = map[string]interface{}{
				"type":        "string",
				"description": arg.Description,
			}
			if arg.Required {
				required = append(required, arg.Name)
			}
		}

		// Add flags as properties
		for _, flag := range meta.Flags {
			prop := map[string]interface{}{
				"description": flag.Description,
			}

			switch flag.Type {
			case commands.FlagTypeBool:
				prop["type"] = "boolean"
			case commands.FlagTypeInt:
				prop["type"] = "integer"
			case commands.FlagTypeKeyValue:
				prop["type"] = "object"
				prop["description"] = flag.Description + " (key-value object)"
			default:
				prop["type"] = "string"
			}

			if len(flag.Examples) > 0 {
				prop["examples"] = flag.Examples
			}

			tool.InputSchema.Properties[flag.Name] = prop
		}

		if len(required) > 0 {
			tool.InputSchema.Required = required
		}

		tools = append(tools, tool)
	}

	return tools
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
	// Remove "raven_" prefix
	if len(toolName) > 6 && toolName[:6] == "raven_" {
		toolName = toolName[6:]
	}

	// Handle special cases where underscores are part of the command structure
	// These are subcommands that use spaces, not underscores
	switch toolName {
	case "schema_add_type":
		return "schema add type"
	case "schema_add_trait":
		return "schema add trait"
	case "schema_add_field":
		return "schema add field"
	case "schema_validate":
		return "schema validate"
	case "schema_update_type":
		return "schema update type"
	case "schema_update_trait":
		return "schema update trait"
	case "schema_update_field":
		return "schema update field"
	case "schema_remove_type":
		return "schema remove type"
	case "schema_remove_trait":
		return "schema remove trait"
	case "schema_remove_field":
		return "schema remove field"
	}

	return toolName
}

// BuildCLIArgs builds CLI arguments from MCP tool arguments using the registry.
func BuildCLIArgs(toolName string, args map[string]interface{}) []string {
	cmdName := CLICommandName(toolName)
	meta, ok := commands.Registry[cmdName]
	if !ok {
		// Try with underscore version for subcommands
		underscoreName := strings.ReplaceAll(cmdName, " ", "_")
		meta, ok = commands.Registry[underscoreName]
		if !ok {
			// Fall back to simple command name
			return buildArgsSimple(cmdName, args)
		}
	}

	var cliArgs []string

	// Start with command name parts
	cliArgs = strings.Fields(meta.Name)

	// Add positional arguments in order
	for _, arg := range meta.Args {
		if val, ok := args[arg.Name]; ok {
			if strVal, ok := val.(string); ok && strVal != "" {
				cliArgs = append(cliArgs, strVal)
			}
		}
	}

	// Add flags
	for _, flag := range meta.Flags {
		val, ok := args[flag.Name]
		if !ok {
			continue
		}

		switch flag.Type {
		case commands.FlagTypeBool:
			if boolVal, ok := val.(bool); ok && boolVal {
				cliArgs = append(cliArgs, "--"+flag.Name)
			}
		case commands.FlagTypeKeyValue:
			// Handle map of key-value pairs
			if mapVal, ok := val.(map[string]interface{}); ok {
				// Special case for 'set' command: fields are positional args
				if cmdName == "set" && flag.Name == "fields" {
					for k, v := range mapVal {
						cliArgs = append(cliArgs, fmt.Sprintf("%s=%v", k, v))
					}
				} else {
					// Default: use --flag key=value format
					for k, v := range mapVal {
						cliArgs = append(cliArgs, "--"+flag.Name, fmt.Sprintf("%s=%v", k, v))
					}
				}
			}
		default:
			if strVal := toString(val); strVal != "" {
				cliArgs = append(cliArgs, "--"+flag.Name, strVal)
			}
		}
	}

	// Always add --json for MCP
	cliArgs = append(cliArgs, "--json")

	return cliArgs
}

// buildArgsSimple is a fallback for commands not in the registry.
func buildArgsSimple(cmdName string, args map[string]interface{}) []string {
	cliArgs := strings.Fields(cmdName)

	// Add all string args
	for key, val := range args {
		if strVal := toString(val); strVal != "" {
			if len(key) == 1 {
				cliArgs = append(cliArgs, "-"+key, strVal)
			} else {
				cliArgs = append(cliArgs, "--"+key, strVal)
			}
		}
	}

	cliArgs = append(cliArgs, "--json")
	return cliArgs
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
