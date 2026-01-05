// Package mcp provides MCP server functionality.
package mcp

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/commands"
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
	case "query_add":
		return "query add"
	case "query_remove":
		return "query remove"
	}

	return toolName
}

// BuildCLIArgs builds CLI arguments from MCP tool arguments using the registry.
//
// ARGUMENT ORDERING STANDARD (strictly enforced):
//  1. Command name (e.g., "edit", "schema add type")
//  2. All flags with their values (--flag value)
//  3. "--json" flag (always added for MCP)
//  4. "--" separator (always added to prevent args starting with "-" from being parsed as flags)
//  5. Positional arguments in registry-defined order
//
// This standard ensures consistent, predictable parsing regardless of argument content.
// No special cases allowed - all commands must follow this pattern.
//
// FLAG NAME NORMALIZATION:
// MCP clients may send property names with either hyphens or underscores
// (e.g., "default-path" or "default_path"). This function accepts both forms
// and normalizes them to match the registry's canonical names (which use hyphens).
func BuildCLIArgs(toolName string, args map[string]interface{}) []string {
	cmdName := CLICommandName(toolName)
	meta, ok := commands.Registry[cmdName]
	if !ok {
		// Registry uses underscores (e.g., "schema_add_type"),
		// but CLICommandName returns spaces (e.g., "schema add type").
		// Try with underscores.
		underscoreName := strings.ReplaceAll(cmdName, " ", "_")
		meta, ok = commands.Registry[underscoreName]
		if !ok {
			// Commands MUST be in the registry. No fallback behavior.
			// Return empty to trigger "unknown tool" error upstream.
			return nil
		}
	}

	var cliArgs []string

	// Normalize args to handle both hyphen and underscore variants
	// (e.g., accept both "default-path" and "default_path")
	normalizedArgs := normalizeArgs(args)

	// Step 1: Command name
	cliArgs = strings.Fields(meta.Name)

	// Step 2: Collect all flags
	for _, flag := range meta.Flags {
		val, ok := normalizedArgs[flag.Name]
		if !ok {
			continue
		}

		switch flag.Type {
		case commands.FlagTypeBool:
			if boolVal, ok := val.(bool); ok && boolVal {
				cliArgs = append(cliArgs, "--"+flag.Name)
			}
		case commands.FlagTypeInt:
			if numVal, ok := val.(float64); ok {
				cliArgs = append(cliArgs, "--"+flag.Name, fmt.Sprintf("%d", int(numVal)))
			}
		case commands.FlagTypeStringSlice:
			// Comma-separated list becomes multiple flag invocations
			if strVal, ok := val.(string); ok && strVal != "" {
				for _, item := range strings.Split(strVal, ",") {
					item = strings.TrimSpace(item)
					if item != "" {
						cliArgs = append(cliArgs, "--"+flag.Name, item)
					}
				}
			}
		case commands.FlagTypeKeyValue:
			// Key-value maps become key=value positional args AFTER the separator
			// Handled in step 5 below
			continue
		default: // FlagTypeString
			if strVal := toString(val); strVal != "" {
				cliArgs = append(cliArgs, "--"+flag.Name, strVal)
			}
		}
	}

	// Step 3: Always add --json for MCP
	cliArgs = append(cliArgs, "--json")

	// Step 4: ALWAYS add "--" separator before positional arguments
	// This prevents ANY argument starting with "-" from being interpreted as a flag
	cliArgs = append(cliArgs, "--")

	// Step 5: Add positional arguments in registry-defined order
	for _, arg := range meta.Args {
		if val, ok := normalizedArgs[arg.Name]; ok {
			if strVal, ok := val.(string); ok && strVal != "" {
				cliArgs = append(cliArgs, strVal)
			}
		}
	}

	// Step 5b: Add key-value pairs as positional args (e.g., "set" command's fields)
	for _, flag := range meta.Flags {
		if flag.Type == commands.FlagTypeKeyValue {
			if mapVal, ok := normalizedArgs[flag.Name].(map[string]interface{}); ok {
				for k, v := range mapVal {
					cliArgs = append(cliArgs, fmt.Sprintf("%s=%v", k, v))
				}
			}
		}
	}

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

// normalizeArgs returns a copy of the args map with normalized keys.
// MCP clients may send property names with underscores (e.g., "default_path")
// instead of hyphens (e.g., "default-path"). This creates a lookup map that
// accepts both forms by converting underscores to hyphens.
func normalizeArgs(args map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{}, len(args)*2)
	for k, v := range args {
		// Keep the original key
		normalized[k] = v
		// Also add the hyphenated version if it uses underscores
		hyphenKey := strings.ReplaceAll(k, "_", "-")
		if hyphenKey != k {
			normalized[hyphenKey] = v
		}
	}
	return normalized
}
