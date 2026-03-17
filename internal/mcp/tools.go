// Package mcp provides MCP server functionality.
package mcp

import (
	"fmt"
	"sort"
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
			Description: "Fetch the strict invocation contract for one Raven command.",
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
			Description: "Invoke any registry command with strict typed validation and policy checks.",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Command identifier (e.g. query, raven_query, or schema add type)",
					},
					"args": map[string]interface{}{
						"type":        "object",
						"description": "Strict arguments object matching raven_describe parameter schema",
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
