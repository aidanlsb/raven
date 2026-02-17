package mcp

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/commands"
)

// TestMCPToolsMatchRegistry verifies that all registry commands
// have corresponding MCP tools with matching schemas.
func TestMCPToolsMatchRegistry(t *testing.T) {
	tools := GenerateToolSchemas()

	// Build a map of tools by name
	toolMap := make(map[string]Tool)
	for _, tool := range tools {
		toolMap[tool.Name] = tool
	}

	// Check each registry command has a corresponding tool
	for cmdName, meta := range commands.Registry {
		toolName := mcpToolName(cmdName)
		tool, ok := toolMap[toolName]
		if !ok {
			t.Errorf("Command %q missing MCP tool %q", cmdName, toolName)
			continue
		}

		// Verify required args match
		for _, arg := range meta.Args {
			if arg.Required {
				found := false
				for _, req := range tool.InputSchema.Required {
					if req == arg.Name {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Tool %q missing required arg %q", toolName, arg.Name)
				}
			}

			// Verify arg is in properties
			if _, ok := tool.InputSchema.Properties[arg.Name]; !ok {
				t.Errorf("Tool %q missing property for arg %q", toolName, arg.Name)
			}
		}

		// Verify flags are in properties
		for _, flag := range meta.Flags {
			if _, ok := tool.InputSchema.Properties[flag.Name]; !ok {
				t.Errorf("Tool %q missing property for flag %q", toolName, flag.Name)
			}
		}
	}

	// Verify we have the expected number of tools
	if len(tools) != len(commands.Registry) {
		t.Errorf("Tool count mismatch: got %d tools, expected %d from registry",
			len(tools), len(commands.Registry))
	}
}

func TestSchemaFieldDescriptionFlagsPresentInMCP(t *testing.T) {
	tools := GenerateToolSchemas()
	toolMap := make(map[string]Tool, len(tools))
	for _, tool := range tools {
		toolMap[tool.Name] = tool
	}

	tests := []string{
		"raven_schema_add_field",
		"raven_schema_update_field",
	}

	for _, toolName := range tests {
		tool, ok := toolMap[toolName]
		if !ok {
			t.Fatalf("expected tool %q to exist", toolName)
		}
		prop, ok := tool.InputSchema.Properties["description"]
		if !ok {
			t.Fatalf("expected tool %q to expose description flag", toolName)
		}

		propMap, ok := prop.(map[string]interface{})
		if !ok {
			t.Fatalf("expected description property for %q to be an object, got %T", toolName, prop)
		}
		if got := propMap["type"]; got != "string" {
			t.Fatalf("expected description property type=string for %q, got %#v", toolName, got)
		}
	}
}

// TestBuildCLIArgsRoundtrip verifies that BuildCLIArgs produces valid CLI commands.
func TestBuildCLIArgsRoundtrip(t *testing.T) {
	tests := []struct {
		toolName string
		args     map[string]interface{}
		wantCmd  string   // Expected first command part
		wantArgs []string // Expected to be present (order may vary)
	}{
		{
			toolName: "raven_stats",
			args:     map[string]interface{}{},
			wantCmd:  "stats",
			wantArgs: []string{"--json"},
		},
		{
			toolName: "raven_query",
			args:     map[string]interface{}{"query_string": "trait:due .value==today"},
			wantCmd:  "query",
			wantArgs: []string{"trait:due .value==today", "--json"},
		},
		{
			toolName: "raven_new",
			args:     map[string]interface{}{"type": "person", "title": "Freya"},
			wantCmd:  "new",
			wantArgs: []string{"person", "Freya", "--json"},
		},
		{
			toolName: "raven_new",
			args: map[string]interface{}{
				"type":  "person",
				"title": "Freya",
				"field": map[string]interface{}{"name": "Freya", "email": "freya@asgard.realm"},
			},
			wantCmd:  "new",
			wantArgs: []string{"--field", "email=freya@asgard.realm", "--field", "name=Freya", "person", "Freya", "--json"},
		},
		{
			toolName: "raven_new",
			args: map[string]interface{}{
				"type":   "person",
				"title":  "Freya",
				"fields": map[string]interface{}{"name": "Freya"}, // alias for `field`
			},
			wantCmd:  "new",
			wantArgs: []string{"--field", "name=Freya", "person", "Freya", "--json"},
		},
		{
			toolName: "raven_new",
			args: map[string]interface{}{
				"type":  "person",
				"title": "Freya",
				"field": []interface{}{"name=Freya", "email=freya@asgard.realm"},
			},
			wantCmd:  "new",
			wantArgs: []string{"--field", "name=Freya", "--field", "email=freya@asgard.realm", "person", "Freya", "--json"},
		},
		{
			toolName: "raven_add",
			args:     map[string]interface{}{"text": "Hello world", "to": "inbox.md"},
			wantCmd:  "add",
			wantArgs: []string{"Hello world", "--to", "inbox.md", "--json"},
		},
		{
			toolName: "raven_upsert",
			args: map[string]interface{}{
				"type":    "brief",
				"title":   "Daily Brief 2026-02-14",
				"content": "# Daily Brief",
				"field": map[string]interface{}{
					"status": "ready",
					"owner":  "people/freya",
				},
			},
			wantCmd:  "upsert",
			wantArgs: []string{"brief", "Daily Brief 2026-02-14", "--content", "# Daily Brief", "--field", "owner=people/freya", "--field", "status=ready", "--json"},
		},
		{
			toolName: "raven_delete",
			args:     map[string]interface{}{"object_id": "people/loki", "force": true},
			wantCmd:  "delete",
			wantArgs: []string{"people/loki", "--force", "--json"},
		},
		{
			toolName: "raven_set",
			args: map[string]interface{}{
				"object_id": "people/freya",
				"fields":    map[string]interface{}{"status": "active", "priority": "high"},
			},
			wantCmd:  "set",
			wantArgs: []string{"people/freya", "priority=high", "status=active", "--json"},
		},
		{
			toolName: "raven_set",
			args: map[string]interface{}{
				"object_id": "people/freya",
				"field":     []interface{}{"status=active"}, // alias for `fields`
			},
			wantCmd:  "set",
			wantArgs: []string{"people/freya", "status=active", "--json"},
		},
		{
			toolName: "raven_schema_add_type",
			args:     map[string]interface{}{"name": "event", "default-path": "events/"},
			wantCmd:  "schema",
			wantArgs: []string{"add", "type", "event", "--default-path", "events/", "--json"},
		},
		{
			toolName: "raven_schema_add_type",
			args: map[string]interface{}{
				"name":        "event",
				"description": "Calendar events",
			},
			wantCmd:  "schema",
			wantArgs: []string{"add", "type", "event", "--description", "Calendar events", "--json"},
		},
		{
			toolName: "raven_schema_add_field",
			args: map[string]interface{}{
				"type_name":   "person",
				"field_name":  "email",
				"type":        "string",
				"description": "Primary contact email",
			},
			wantCmd:  "schema",
			wantArgs: []string{"add", "field", "person", "email", "--type", "string", "--description", "Primary contact email", "--json"},
		},
		{
			toolName: "raven_schema_update_field",
			args: map[string]interface{}{
				"type_name":   "person",
				"field_name":  "email",
				"description": "Primary contact email",
			},
			wantCmd:  "schema",
			wantArgs: []string{"update", "field", "person", "email", "--description", "Primary contact email", "--json"},
		},
		// Test that underscore variants of flag names also work (MCP clients may normalize names)
		{
			toolName: "raven_schema_add_type",
			args:     map[string]interface{}{"name": "meeting", "default_path": "meetings/"},
			wantCmd:  "schema",
			wantArgs: []string{"add", "type", "meeting", "--default-path", "meetings/", "--json"},
		},
		{
			toolName: "raven_workflow_run",
			args: map[string]interface{}{
				"name":       "daily-brief",
				"input-json": map[string]interface{}{"date": "2026-02-14"},
			},
			wantCmd:  "workflow",
			wantArgs: []string{"run", "daily-brief", "--input-json", "--json"},
		},
		{
			toolName: "raven_workflow_continue",
			args: map[string]interface{}{
				"run-id":            "wrf_abc123",
				"agent-output-json": map[string]interface{}{"outputs": map[string]interface{}{"markdown": "done"}},
				"expected-revision": float64(2),
			},
			wantCmd:  "workflow",
			wantArgs: []string{"continue", "wrf_abc123", "--agent-output-json", "--expected-revision", "2", "--json"},
		},
		{
			toolName: "raven_workflow_add",
			args: map[string]interface{}{
				"name": "daily-brief",
				"file": "workflows/daily-brief.yaml",
			},
			wantCmd:  "workflow",
			wantArgs: []string{"add", "daily-brief", "--file", "workflows/daily-brief.yaml", "--json"},
		},
		{
			toolName: "raven_workflow_scaffold",
			args: map[string]interface{}{
				"name":        "daily-brief",
				"description": "Daily brief scaffold",
			},
			wantCmd:  "workflow",
			wantArgs: []string{"scaffold", "daily-brief", "--description", "Daily brief scaffold", "--json"},
		},
		{
			toolName: "raven_workflow_remove",
			args: map[string]interface{}{
				"name": "daily-brief",
			},
			wantCmd:  "workflow",
			wantArgs: []string{"remove", "daily-brief", "--json"},
		},
		{
			toolName: "raven_workflow_validate",
			args: map[string]interface{}{
				"name": "daily-brief",
			},
			wantCmd:  "workflow",
			wantArgs: []string{"validate", "daily-brief", "--json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			cliArgs := BuildCLIArgs(tt.toolName, tt.args)

			// Check command name
			if len(cliArgs) == 0 {
				t.Fatal("BuildCLIArgs returned empty slice")
			}
			if cliArgs[0] != tt.wantCmd {
				t.Errorf("First arg = %q, want %q", cliArgs[0], tt.wantCmd)
			}

			// Check all expected args are present
			argsStr := strings.Join(cliArgs, " ")
			for _, want := range tt.wantArgs {
				if !strings.Contains(argsStr, want) {
					t.Errorf("Args %v missing expected %q", cliArgs, want)
				}
			}
		})
	}
}

func TestBuildCLIArgs_PreservesEmptyPositionalArgs(t *testing.T) {
	cliArgs := BuildCLIArgs("raven_edit", map[string]interface{}{
		"path":    "daily/2026-01-02.md",
		"old_str": "- old task",
		"new_str": "",
		"confirm": true,
	})

	want := []string{
		"edit",
		"--confirm",
		"--json",
		"--",
		"daily/2026-01-02.md",
		"- old task",
		"",
	}

	if !reflect.DeepEqual(cliArgs, want) {
		t.Fatalf("BuildCLIArgs() = %#v, want %#v", cliArgs, want)
	}
}

// TestCLICommandNameConversion verifies tool name <-> CLI name conversion.
func TestCLICommandNameConversion(t *testing.T) {
	tests := []struct {
		toolName string
		wantCLI  string
	}{
		{"raven_new", "new"},
		{"raven_add", "add"},
		{"raven_query", "query"},
		{"raven_schema_add_type", "schema add type"},
		{"raven_schema_add_trait", "schema add trait"},
		{"raven_schema_add_field", "schema add field"},
		{"raven_schema_validate", "schema validate"},
		{"raven_template_get", "template get"},
		{"raven_template_set", "template set"},
		{"raven_workflow_add", "workflow add"},
		{"raven_workflow_scaffold", "workflow scaffold"},
		{"raven_workflow_remove", "workflow remove"},
		{"raven_workflow_validate", "workflow validate"},
		{"raven_workflow_continue", "workflow continue"},
		{"raven_workflow_runs_list", "workflow runs list"},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			got := CLICommandName(tt.toolName)
			if got != tt.wantCLI {
				t.Errorf("CLICommandName(%q) = %q, want %q", tt.toolName, got, tt.wantCLI)
			}
		})
	}
}

// TestMCPToolNameConversion verifies CLI name -> tool name conversion.
func TestMCPToolNameConversion(t *testing.T) {
	tests := []struct {
		cliName  string
		wantTool string
	}{
		{"new", "raven_new"},
		{"add", "raven_add"},
		{"query", "raven_query"},
		{"schema_add_type", "raven_schema_add_type"},
	}

	for _, tt := range tests {
		t.Run(tt.cliName, func(t *testing.T) {
			got := mcpToolName(tt.cliName)
			if got != tt.wantTool {
				t.Errorf("mcpToolName(%q) = %q, want %q", tt.cliName, got, tt.wantTool)
			}
		})
	}
}
