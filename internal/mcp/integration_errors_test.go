//go:build integration

package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestMCPIntegration_ErrorHandling(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Create a file first
	server.callTool("new", map[string]interface{}{
		"type":  "person",
		"title": "Alice",
	})

	// Try to create a duplicate (should fail)
	result := server.callTool("new", map[string]interface{}{
		"type":  "person",
		"title": "Alice",
	})

	// Should be an error
	if !result.IsError {
		t.Fatalf("expected error for duplicate object, got success: %s", result.Text)
	}

	// Parse the error response
	var resp struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.Text), &resp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if resp.Error.Code != "FILE_EXISTS" {
		t.Errorf("expected error code FILE_EXISTS, got %s", resp.Error.Code)
	}
}

func TestMCPIntegration_QueryParseErrorsIncludeSuggestion(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	result := server.callTool("query", map[string]interface{}{
		"query_string": "from issue where",
	})
	if !result.IsError {
		t.Fatalf("expected query parse failure, got success: %s", result.Text)
	}

	env := parseMCPEnvelope(t, result.Text)
	if env.Error == nil || env.Error.Code != "QUERY_INVALID" {
		t.Fatalf("expected QUERY_INVALID, got: %s", result.Text)
	}
	if env.Error.Suggestion != "RQL does not use 'where'. Put predicates directly after the query root, for example: type:issue .status==open" {
		t.Fatalf("unexpected query suggestion: %q", env.Error.Suggestion)
	}
}

func TestMCPIntegration_SearchSyntaxErrorsReturnInvalidInput(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	result := server.callTool("search", map[string]interface{}{
		"query": "@broken",
	})
	if !result.IsError {
		t.Fatalf("expected search syntax failure, got success: %s", result.Text)
	}

	env := parseMCPEnvelope(t, result.Text)
	if env.Error == nil || env.Error.Code != "INVALID_INPUT" {
		t.Fatalf("expected INVALID_INPUT, got: %s", result.Text)
	}
	if env.Error.Message != "invalid search query" {
		t.Fatalf("unexpected search error message: %q", env.Error.Message)
	}
	if env.Error.Suggestion != "Quote special characters or use a simpler full-text query and retry." {
		t.Fatalf("unexpected search suggestion: %q", env.Error.Suggestion)
	}
}
