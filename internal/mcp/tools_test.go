package mcp

import "testing"

func TestGenerateToolSchemasCompactSurface(t *testing.T) {
	tools := GenerateToolSchemas()
	if len(tools) != 3 {
		t.Fatalf("expected 3 compact MCP tools, got %d", len(tools))
	}

	toolByName := make(map[string]Tool, len(tools))
	for _, tool := range tools {
		toolByName[tool.Name] = tool
	}

	for _, name := range []string{compactToolDiscover, compactToolDescribe, compactToolInvoke} {
		if _, ok := toolByName[name]; !ok {
			t.Fatalf("missing compact tool %q", name)
		}
	}

	describe := toolByName[compactToolDescribe]
	if len(describe.InputSchema.Required) != 1 || describe.InputSchema.Required[0] != "command" {
		t.Fatalf("expected raven_describe to require command, got %#v", describe.InputSchema.Required)
	}
	invoke := toolByName[compactToolInvoke]
	if len(invoke.InputSchema.Required) != 1 || invoke.InputSchema.Required[0] != "command" {
		t.Fatalf("expected raven_invoke to require command, got %#v", invoke.InputSchema.Required)
	}
}

func TestBuildCommandContractStrictTypes(t *testing.T) {
	queryContract, ok := buildCommandContract("query")
	if !ok {
		t.Fatal("expected query contract")
	}
	if got := queryContract.Parameters["apply"].Type; got != paramTypeStringArray {
		t.Fatalf("query.apply type=%q, want %q", got, paramTypeStringArray)
	}

	newContract, ok := buildCommandContract("new")
	if !ok {
		t.Fatal("expected new contract")
	}
	if got := newContract.Parameters["field"].Type; got != paramTypeObject {
		t.Fatalf("new.field type=%q, want %q", got, paramTypeObject)
	}
	if got := newContract.Parameters["field-json"].Type; got != paramTypeObject {
		t.Fatalf("new.field-json type=%q, want %q", got, paramTypeObject)
	}
}

func TestDiscoverableContractsApplyPolicy(t *testing.T) {
	contracts := discoverableContracts()
	byID := make(map[string]commandContract, len(contracts))
	for _, c := range contracts {
		byID[c.CommandID] = c
	}

	if _, ok := byID["query"]; !ok {
		t.Fatal("expected discoverable contract for query")
	}
	if _, ok := byID["serve"]; ok {
		t.Fatal("did not expect serve to be discoverable")
	}
}

func TestCLICommandName(t *testing.T) {
	tests := []struct {
		toolName string
		wantCLI  string
	}{
		{"raven_new", "new"},
		{"raven_schema_add_type", "schema add type"},
		{"query", "query"},
	}

	for _, tt := range tests {
		if got := CLICommandName(tt.toolName); got != tt.wantCLI {
			t.Fatalf("CLICommandName(%q) = %q, want %q", tt.toolName, got, tt.wantCLI)
		}
	}
}

func TestMCPToolName(t *testing.T) {
	tests := []struct {
		cliName  string
		wantTool string
	}{
		{"new", "raven_new"},
		{"schema add type", "raven_schema_add_type"},
	}

	for _, tt := range tests {
		if got := mcpToolName(tt.cliName); got != tt.wantTool {
			t.Fatalf("mcpToolName(%q) = %q, want %q", tt.cliName, got, tt.wantTool)
		}
	}
}
