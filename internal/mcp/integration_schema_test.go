//go:build integration

package mcp_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestMCPIntegration_SchemaAddTypeDefaultsPathToTypeName(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	result := server.callTool("schema_add_type", map[string]interface{}{
		"name": "meeting",
	})

	if result.IsError {
		t.Fatalf("tool call failed: %s", result.Text)
	}

	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			DefaultPath string `json:"default_path"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Text), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Data.DefaultPath != "meeting/" {
		t.Fatalf("expected default_path %q, got %q", "meeting/", resp.Data.DefaultPath)
	}

	v.AssertFileContains("schema.yaml", "meeting:")
	v.AssertFileContains("schema.yaml", "default_path: meeting/")
}

func TestMCPIntegration_SchemaIntrospection(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Get schema types
	result := server.callTool("schema", map[string]interface{}{
		"subcommand": "types",
	})

	if result.IsError {
		t.Fatalf("schema introspection failed: %s", result.Text)
	}

	// Verify person and project types are in the output
	if !strings.Contains(result.Text, "person") || !strings.Contains(result.Text, "project") {
		t.Errorf("expected schema to include person and project types, got: %s", result.Text)
	}

	// Get details for one type using explicit positional args.
	typeResult := server.callTool("schema", map[string]interface{}{
		"subcommand": "type",
		"name":       "person",
	})

	if typeResult.IsError {
		t.Fatalf("schema type introspection failed: %s", typeResult.Text)
	}

	var typeResp struct {
		Data struct {
			Type struct {
				Name string `json:"name"`
			} `json:"type"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(typeResult.Text), &typeResp); err != nil {
		t.Fatalf("failed to parse schema type response: %v\n%s", err, typeResult.Text)
	}
	if typeResp.Data.Type.Name != "person" {
		t.Errorf("expected type details for person, got: %s", typeResult.Text)
	}
}

// TestMCPIntegration_SchemaFieldDescriptionsViaToolCall verifies schema field
// descriptions can be added/updated/removed through MCP tools/call.
func TestMCPIntegration_SchemaFieldDescriptionsViaToolCall(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	// Add a new field with a description.
	addFieldResult := server.callTool("schema_add_field", map[string]interface{}{
		"type_name":   "person",
		"field_name":  "website",
		"type":        "string",
		"description": "Primary website URL",
	})
	if addFieldResult.IsError {
		t.Fatalf("schema add field failed: %s", addFieldResult.Text)
	}
	v.AssertFileContains("schema.yaml", "website:")
	v.AssertFileContains("schema.yaml", "description: Primary website URL")

	// Update existing field description.
	updateFieldResult := server.callTool("schema_update_field", map[string]interface{}{
		"type_name":   "person",
		"field_name":  "email",
		"description": "Primary contact email",
	})
	if updateFieldResult.IsError {
		t.Fatalf("schema update field failed: %s", updateFieldResult.Text)
	}
	v.AssertFileContains("schema.yaml", "description: Primary contact email")

	// Remove the description with "-" sentinel.
	removeDescriptionResult := server.callTool("schema_update_field", map[string]interface{}{
		"type_name":   "person",
		"field_name":  "email",
		"description": "-",
	})
	if removeDescriptionResult.IsError {
		t.Fatalf("schema update field remove description failed: %s", removeDescriptionResult.Text)
	}
	v.AssertFileNotContains("schema.yaml", "description: Primary contact email")
}

// TestMCPIntegration_SchemaFieldEnumValuesViaToolCall verifies enum value
// remaps are rejected in favor of the migration-aware convert command.
func TestMCPIntegration_SchemaFieldEnumValuesViaToolCall(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	before := v.ReadFile("schema.yaml")

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	updateFieldResult := server.callTool("schema_update_field", map[string]interface{}{
		"type_name":  "project",
		"field_name": "status",
		"values":     "active,paused,done,archived",
	})
	if !updateFieldResult.IsError {
		t.Fatalf("schema update field values unexpectedly succeeded: %s", updateFieldResult.Text)
	}
	if !strings.Contains(updateFieldResult.Text, "schema convert field") {
		t.Fatalf("schema update field rejection did not direct callers to convert: %s", updateFieldResult.Text)
	}
	if got := v.ReadFile("schema.yaml"); got != before {
		t.Fatalf("rejected update changed schema.yaml:\n%s", got)
	}
}

func TestMCPIntegration_SchemaUpdateTypeAndTraitViaToolCall(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	updateTypeResult := server.callTool("schema_update_type", map[string]interface{}{
		"name":        "project",
		"description": "Tracked work items",
		"add-trait":   "priority",
	})
	if updateTypeResult.IsError {
		t.Fatalf("schema update type failed: %s", updateTypeResult.Text)
	}
	v.AssertFileContains("schema.yaml", "description: Tracked work items")
	v.AssertFileContains("schema.yaml", "- priority")

	updateTraitResult := server.callTool("schema_update_trait", map[string]interface{}{
		"name":    "priority",
		"default": "high",
	})
	if updateTraitResult.IsError {
		t.Fatalf("schema update trait failed: %s", updateTraitResult.Text)
	}
	v.AssertFileContains("schema.yaml", "default: high")
}

func TestMCPIntegration_SchemaRemoveTypeAndTraitWarningsViaToolCall(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	createProject := server.callTool("new", map[string]interface{}{
		"type":  "project",
		"title": "Apollo",
	})
	if createProject.IsError {
		t.Fatalf("schema remove setup (project create) failed: %s", createProject.Text)
	}

	addTraitUsage := server.callTool("add", map[string]interface{}{
		"text": "@priority(high)",
		"to":   "projects/apollo.md",
	})
	if addTraitUsage.IsError {
		t.Fatalf("schema remove setup (trait usage) failed: %s", addTraitUsage.Text)
	}

	removeType := server.callTool("schema_remove_type", map[string]interface{}{
		"name": "project",
	})
	if removeType.IsError {
		t.Fatalf("schema remove type failed: %s", removeType.Text)
	}

	var removeTypeResp struct {
		OK       bool `json:"ok"`
		Warnings []struct {
			Code string `json:"code"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(removeType.Text), &removeTypeResp); err != nil {
		t.Fatalf("failed to parse schema remove type response: %v", err)
	}
	if !removeTypeResp.OK {
		t.Fatalf("expected ok=true in schema remove type response: %s", removeType.Text)
	}
	if len(removeTypeResp.Warnings) == 0 || removeTypeResp.Warnings[0].Code != "ORPHANED_FILES" {
		t.Fatalf("expected ORPHANED_FILES warning, got: %s", removeType.Text)
	}
	v.AssertFileNotContains("schema.yaml", "project:")

	removeTrait := server.callTool("schema_remove_trait", map[string]interface{}{
		"name": "priority",
	})
	if removeTrait.IsError {
		t.Fatalf("schema remove trait failed: %s", removeTrait.Text)
	}

	var removeTraitResp struct {
		OK       bool `json:"ok"`
		Warnings []struct {
			Code string `json:"code"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(removeTrait.Text), &removeTraitResp); err != nil {
		t.Fatalf("failed to parse schema remove trait response: %v", err)
	}
	if !removeTraitResp.OK {
		t.Fatalf("expected ok=true in schema remove trait response: %s", removeTrait.Text)
	}
	if len(removeTraitResp.Warnings) == 0 || removeTraitResp.Warnings[0].Code != "ORPHANED_TRAITS" {
		t.Fatalf("expected ORPHANED_TRAITS warning, got: %s", removeTrait.Text)
	}
	v.AssertFileNotContains("schema.yaml", "priority:")
}

// TestMCPIntegration_SchemaRenameTypeWithDefaultPathRename verifies MCP JSON
// preview/apply behavior for type rename with optional default_path directory
// migration.
func TestMCPIntegration_SchemaRenameTypeWithDefaultPathRename(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  event:
    default_path: events/
    fields:
      title: { type: string }
  project:
    default_path: projects/
    fields:
      kickoff:
        type: ref
        target: event
traits: {}
`).
		WithFile("events/kickoff.md", `---
type: event
title: Kickoff
---
# Kickoff
`).
		WithFile("events/planning.md", `---
type: event
title: Planning
---
# Planning
`).
		WithFile("projects/roadmap.md", `---
type: project
kickoff: events/kickoff
---
# Roadmap

Kickoff: [[events/kickoff]]
Planning: [[events/planning|Planning]]
`).
		Build()

	binary := testutil.BuildCLI(t)
	server := newTestServer(t, v.Path, binary)

	preview := server.callTool("schema_rename_type", map[string]interface{}{
		"old_name": "event",
		"new_name": "meeting",
	})
	if preview.IsError {
		t.Fatalf("schema rename type preview failed: %s", preview.Text)
	}

	var previewResp struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(preview.Text), &previewResp); err != nil {
		t.Fatalf("failed to parse preview response: %v\nraw: %s", err, preview.Text)
	}
	if !previewResp.OK {
		t.Fatalf("expected preview ok=true, got: %s", preview.Text)
	}
	if got, _ := previewResp.Data["preview"].(bool); !got {
		t.Fatalf("expected preview=true, got: %#v", previewResp.Data["preview"])
	}
	if got, _ := previewResp.Data["default_path_rename_available"].(bool); !got {
		t.Fatalf("expected default_path_rename_available=true, got: %#v", previewResp.Data["default_path_rename_available"])
	}
	if got, _ := previewResp.Data["default_path_old"].(string); got != "events/" {
		t.Fatalf("expected default_path_old=events/, got: %#v", previewResp.Data["default_path_old"])
	}
	if got, _ := previewResp.Data["default_path_new"].(string); got != "meetings/" {
		t.Fatalf("expected default_path_new=meetings/, got: %#v", previewResp.Data["default_path_new"])
	}

	v.AssertFileExists("events/kickoff.md")
	v.AssertFileExists("events/planning.md")

	apply := server.callTool("schema_rename_type", map[string]interface{}{
		"old_name":            "event",
		"new_name":            "meeting",
		"confirm":             true,
		"rename_default_path": true, // underscore variant should normalize
	})
	if apply.IsError {
		t.Fatalf("schema rename type apply failed: %s", apply.Text)
	}

	var applyResp struct {
		OK   bool `json:"ok"`
		Data struct {
			DefaultPathRenamed    bool   `json:"default_path_renamed"`
			DefaultPathOld        string `json:"default_path_old"`
			DefaultPathNew        string `json:"default_path_new"`
			FilesMoved            int    `json:"files_moved"`
			ReferenceFilesUpdated int    `json:"reference_files_updated"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(apply.Text), &applyResp); err != nil {
		t.Fatalf("failed to parse apply response: %v\nraw: %s", err, apply.Text)
	}
	if !applyResp.OK {
		t.Fatalf("expected apply ok=true, got: %s", apply.Text)
	}
	if !applyResp.Data.DefaultPathRenamed {
		t.Fatalf("expected default_path_renamed=true, got false")
	}
	if applyResp.Data.DefaultPathOld != "events/" {
		t.Fatalf("expected default_path_old=events/, got %q", applyResp.Data.DefaultPathOld)
	}
	if applyResp.Data.DefaultPathNew != "meetings/" {
		t.Fatalf("expected default_path_new=meetings/, got %q", applyResp.Data.DefaultPathNew)
	}
	if applyResp.Data.FilesMoved != 2 {
		t.Fatalf("expected files_moved=2, got %d", applyResp.Data.FilesMoved)
	}
	if applyResp.Data.ReferenceFilesUpdated < 1 {
		t.Fatalf("expected reference_files_updated>=1, got %d", applyResp.Data.ReferenceFilesUpdated)
	}

	v.AssertFileContains("schema.yaml", "meeting:")
	v.AssertFileContains("schema.yaml", "default_path: meetings/")
	v.AssertFileContains("schema.yaml", "target: meeting")
	v.AssertFileNotContains("schema.yaml", "\n  event:\n")

	v.AssertFileExists("meetings/kickoff.md")
	v.AssertFileExists("meetings/planning.md")
	v.AssertFileNotExists("events/kickoff.md")
	v.AssertFileNotExists("events/planning.md")
	v.AssertFileContains("meetings/kickoff.md", "type: meeting")
	v.AssertFileContains("meetings/planning.md", "type: meeting")

	v.AssertFileContains("projects/roadmap.md", "kickoff: meetings/kickoff")
	v.AssertFileContains("projects/roadmap.md", "[[meetings/kickoff]]")
	v.AssertFileContains("projects/roadmap.md", "[[meetings/planning|Planning]]")
	v.AssertFileNotContains("projects/roadmap.md", "events/kickoff")
	v.AssertFileNotContains("projects/roadmap.md", "events/planning")
}

// TestMCPIntegration_Check tests vault check via MCP tool call.
