package mcp

import "testing"

func TestAllGeneratedToolsHaveDirectSemanticMapping(t *testing.T) {
	tools := GenerateToolSchemas()

	missing := make([]string, 0)
	for _, tool := range tools {
		if _, ok := compatibilityToolSemanticMap[tool.Name]; !ok {
			missing = append(missing, tool.Name)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("generated MCP tools missing direct semantic mappings: %v", missing)
	}
}
