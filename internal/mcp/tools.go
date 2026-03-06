// Package mcp provides MCP server functionality.
package mcp

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/commands"
)

// GenerateToolSchemas generates MCP tool schemas from the command registry.
// This ensures MCP tools stay in sync with CLI commands automatically.
func GenerateToolSchemas() []Tool {
	var tools []Tool

	for cmdName, meta := range commands.Registry {
		if meta.HideFromMCP {
			continue
		}

		description := meta.Description
		if meta.LongDesc != "" {
			description = meta.LongDesc
		}
		description = withExampleSection(description, meta.Examples)

		tool := Tool{
			Name:        mcpToolName(cmdName),
			Description: description,
			InputSchema: InputSchema{
				Type:       "object",
				Properties: make(map[string]interface{}),
			},
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
			case commands.FlagTypeJSON:
				// Some MCP clients can only pass primitive args. Accept JSON payloads
				// either as structured objects or as JSON-encoded strings.
				prop["anyOf"] = []map[string]interface{}{
					{"type": "object"},
					{"type": "string"},
				}
				prop["description"] = flag.Description + " (object or JSON string)"
			case commands.FlagTypeStringSlice:
				// Repeatable string flags are represented as arrays in MCP, while
				// still accepting comma-delimited strings for client compatibility.
				prop["anyOf"] = []map[string]interface{}{
					{"type": "string"},
					{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
				}
				prop["description"] = flag.Description + " (string or array of strings)"
			case commands.FlagTypeKeyValue, commands.FlagTypePosKeyValue:
				// Key/value inputs are intentionally flexible for MCP compatibility:
				// object, single "k=v" string, or array of "k=v" strings.
				prop["anyOf"] = []map[string]interface{}{
					{"type": "object"},
					{"type": "string"},
					{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
				}
				prop["description"] = flag.Description + " (object, k=v string, or array of k=v strings)"
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

func withExampleSection(description string, examples []string) string {
	if len(examples) == 0 {
		return strings.TrimSpace(description)
	}

	const maxExamples = 3

	exampleCount := len(examples)
	if exampleCount > maxExamples {
		exampleCount = maxExamples
	}

	b := strings.Builder{}
	b.WriteString(strings.TrimSpace(description))
	b.WriteString("\n\nExamples:")
	for _, example := range examples[:exampleCount] {
		b.WriteString("\n- `")
		b.WriteString(example)
		b.WriteString("`")
	}
	if len(examples) > maxExamples {
		b.WriteString(fmt.Sprintf("\n- ... (%d more in CLI help)", len(examples)-maxExamples))
	}

	return b.String()
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
	cmdID, ok := commands.ResolveToolCommandID(toolName)
	if !ok {
		// Commands MUST be in the registry. No fallback behavior.
		// Return empty to trigger "unknown tool" error upstream.
		return nil
	}
	meta := commands.Registry[cmdID]

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
			if intVal, ok := intFlagValue(val); ok {
				cliArgs = append(cliArgs, "--"+flag.Name, intVal)
			}
		case commands.FlagTypeStringSlice:
			for _, item := range stringSliceValues(val) {
				if item != "" {
					cliArgs = append(cliArgs, "--"+flag.Name, item)
				}
			}
		case commands.FlagTypeJSON:
			switch typed := val.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					cliArgs = append(cliArgs, "--"+flag.Name, typed)
				}
			default:
				b, err := json.Marshal(typed)
				if err == nil {
					cliArgs = append(cliArgs, "--"+flag.Name, string(b))
				}
			}
		case commands.FlagTypeKeyValue:
			if isObjectArg(val) && hasFlag(meta.Flags, flag.Name+"-json") {
				// Prefer JSON companion flags (e.g., --field-json) when available so
				// typed values survive end-to-end without key=value coercion.
				continue
			}
			// Key-value flags are represented as a JSON object in MCP, but some clients may
			// send a single "k=v" string or an array of "k=v" strings. Accept all.
			for _, pair := range keyValuePairs(val) {
				cliArgs = append(cliArgs, "--"+flag.Name, pair)
			}
		case commands.FlagTypePosKeyValue:
			// Positional key=value args are handled in step 5b below.
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
			// Preserve empty strings for positional args (e.g., `edit` new_str="" for deletion).
			if strVal, ok := val.(string); ok {
				cliArgs = append(cliArgs, strVal)
			}
		}
	}

	// Step 5b: Add positional key=value pairs (e.g., "set" command's fields)
	for _, flag := range meta.Flags {
		if flag.Type == commands.FlagTypePosKeyValue {
			if val, ok := normalizedArgs[flag.Name]; ok {
				if isObjectArg(val) && hasFlag(meta.Flags, flag.Name+"-json") {
					continue
				}
				// Like FlagTypeKeyValue: primarily a JSON object, but accept "k=v" strings/arrays too.
				cliArgs = append(cliArgs, keyValuePairs(val)...)
			}
		}
	}

	return cliArgs
}

func intFlagValue(v interface{}) (string, bool) {
	switch val := v.(type) {
	case int:
		return strconv.Itoa(val), true
	case int8:
		return strconv.FormatInt(int64(val), 10), true
	case int16:
		return strconv.FormatInt(int64(val), 10), true
	case int32:
		return strconv.FormatInt(int64(val), 10), true
	case int64:
		return strconv.FormatInt(val, 10), true
	case uint:
		return strconv.FormatUint(uint64(val), 10), true
	case uint8:
		return strconv.FormatUint(uint64(val), 10), true
	case uint16:
		return strconv.FormatUint(uint64(val), 10), true
	case uint32:
		return strconv.FormatUint(uint64(val), 10), true
	case uint64:
		return strconv.FormatUint(val, 10), true
	case float32:
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			return "", false
		}
		return strconv.FormatInt(int64(val), 10), true
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return "", false
		}
		return strconv.FormatInt(int64(val), 10), true
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return strconv.FormatInt(i, 10), true
		}
		if f, err := val.Float64(); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return strconv.FormatInt(int64(f), 10), true
		}
		return "", false
	default:
		return "", false
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

// stringSliceValues normalizes repeatable string flag inputs.
//
// Supported forms:
// - string:        "a,b,c" or "a"
// - []interface{}: ["a","b"]
// - []string:      ["a","b"]
func stringSliceValues(v interface{}) []string {
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return nil
		}
		parts := strings.Split(s, ",")
		values := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				values = append(values, part)
			}
		}
		return values
	case []interface{}:
		values := make([]string, 0, len(val))
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != "" {
				values = append(values, s)
			}
		}
		return values
	case []string:
		values := make([]string, 0, len(val))
		for _, item := range val {
			item = strings.TrimSpace(item)
			if item != "" {
				values = append(values, item)
			}
		}
		return values
	default:
		return nil
	}
}

// keyValuePairs normalizes a key-value input into one or more "k=v" strings.
//
// Supported forms:
// - map/object: {"name":"Freya","email":"a@b.com"}  -> ["email=a@b.com","name=Freya"] (sorted by key)
// - string:     "name=Freya"                       -> ["name=Freya"]
// - array:      ["name=Freya","email=a@b.com"]     -> ["name=Freya","email=a@b.com"]
//
// This is intentionally permissive to accommodate variations in MCP clients.
func keyValuePairs(v interface{}) []string {
	switch val := v.(type) {
	case map[string]interface{}:
		if len(val) == 0 {
			return nil
		}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, fmt.Sprintf("%s=%v", k, val[k]))
		}
		return pairs
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return nil
		}
		// If it looks like "k=v,k2=v2", accept it as multiple pairs (best-effort).
		if strings.Contains(s, ",") {
			parts := strings.Split(s, ",")
			var pairs []string
			allKV := true
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				if !strings.Contains(p, "=") {
					allKV = false
					break
				}
				pairs = append(pairs, p)
			}
			if allKV && len(pairs) > 0 {
				return pairs
			}
		}
		if !strings.Contains(s, "=") {
			return nil
		}
		return []string{s}
	case []interface{}:
		var pairs []string
		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s == "" || !strings.Contains(s, "=") {
				continue
			}
			pairs = append(pairs, s)
		}
		return pairs
	default:
		return nil
	}
}

func hasFlag(flags []commands.FlagMeta, name string) bool {
	for _, flag := range flags {
		if flag.Name == name {
			return true
		}
	}
	return false
}

func isObjectArg(v interface{}) bool {
	if v == nil {
		return false
	}
	_, ok := v.(map[string]interface{})
	return ok
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

	// Compatibility aliases:
	// - Some clients may send `fields` where Raven expects `field` (and vice versa).
	if v, ok := normalized["fields"]; ok {
		if _, exists := normalized["field"]; !exists {
			normalized["field"] = v
		}
	}
	if v, ok := normalized["field"]; ok {
		if _, exists := normalized["fields"]; !exists {
			normalized["fields"] = v
		}
	}

	// Prefer typed JSON companions when key-value inputs are provided as objects.
	if v, ok := normalized["field"]; ok && isObjectArg(v) {
		if _, exists := normalized["field-json"]; !exists {
			normalized["field-json"] = v
		}
	}
	if v, ok := normalized["fields"]; ok && isObjectArg(v) {
		if _, exists := normalized["fields-json"]; !exists {
			normalized["fields-json"] = v
		}
	}

	return normalized
}
