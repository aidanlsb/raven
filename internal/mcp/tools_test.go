package mcp

import (
	"encoding/json"
	"testing"
)

func TestGenerateToolSchemasCompactSurface(t *testing.T) {
	t.Parallel()
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
	discover := toolByName[compactToolDiscover]
	for _, unwanted := range []string{"query", "limit", "cursor", "category", "mode", "risk"} {
		if _, ok := discover.InputSchema.Properties[unwanted]; ok {
			t.Fatalf("did not expect raven_discover to expose %q", unwanted)
		}
	}
	invoke := toolByName[compactToolInvoke]
	if len(invoke.InputSchema.Required) != 1 || invoke.InputSchema.Required[0] != "command" {
		t.Fatalf("expected raven_invoke to require command, got %#v", invoke.InputSchema.Required)
	}
	if _, ok := invoke.InputSchema.Properties["vault"]; !ok {
		t.Fatal("expected raven_invoke to expose wrapper-level vault override")
	}
	if _, ok := invoke.InputSchema.Properties["vault_path"]; !ok {
		t.Fatal("expected raven_invoke to expose wrapper-level vault_path override")
	}
}

func TestBuildCommandContractStrictTypes(t *testing.T) {
	t.Parallel()
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

	reclassifyContract, ok := buildCommandContract("reclassify")
	if !ok {
		t.Fatal("expected reclassify contract")
	}
	if got := reclassifyContract.Parameters["field-json"].Type; got != paramTypeObject {
		t.Fatalf("reclassify.field-json type=%q, want %q", got, paramTypeObject)
	}
}

func TestDiscoverableContractsApplyPolicy(t *testing.T) {
	t.Parallel()
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

func TestCompactDiscoverReturnsFullCatalogByDefault(t *testing.T) {
	t.Parallel()
	s := &Server{}

	out, isErr := s.callCompactDiscover(nil)
	if isErr {
		t.Fatalf("discover failed: %s", out)
	}

	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Matches []struct {
				Command string `json:"command"`
			} `json:"matches"`
			Total      int      `json:"total"`
			Returned   int      `json:"returned"`
			Categories []string `json:"categories"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal discover response: %v", err)
	}

	contracts := discoverableContracts()
	if got, want := len(resp.Data.Matches), len(contracts); got != want {
		t.Fatalf("discover returned %d matches, want %d", got, want)
	}
	if resp.Data.Total != len(contracts) {
		t.Fatalf("discover total=%d, want %d", resp.Data.Total, len(contracts))
	}
	if resp.Data.Returned != len(contracts) {
		t.Fatalf("discover returned=%d, want %d", resp.Data.Returned, len(contracts))
	}
	for i, contract := range contracts {
		if resp.Data.Matches[i].Command != contract.CommandID {
			t.Fatalf("discover match %d command=%q, want %q", i, resp.Data.Matches[i].Command, contract.CommandID)
		}
	}
	if len(resp.Data.Categories) == 0 {
		t.Fatal("expected discover categories")
	}
}

func TestCompactDiscoverRejectsLegacySearchAndPaginationArgs(t *testing.T) {
	t.Parallel()
	s := &Server{}

	for _, args := range []map[string]interface{}{
		{"query": "edit"},
		{"limit": 5},
		{"cursor": "10"},
		{"category": "schema"},
		{"mode": "read"},
		{"risk": "safe"},
	} {
		out, isErr := s.callCompactDiscover(args)
		if !isErr {
			t.Fatalf("expected discover args %#v to fail, got: %s", args, out)
		}
		if !json.Valid([]byte(out)) {
			t.Fatalf("expected valid json error for args %#v, got: %s", args, out)
		}
	}
}
