package commands

import "testing"

func TestResolveToolCommandID(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		wantID   string
		wantOK   bool
	}{
		{
			name:     "mcp tool name",
			toolName: "raven_query",
			wantID:   "query",
			wantOK:   true,
		},
		{
			name:     "registry command id",
			toolName: "query",
			wantID:   "query",
			wantOK:   true,
		},
		{
			name:     "cli style command with spaces",
			toolName: "schema add type",
			wantID:   "schema_add_type",
			wantOK:   true,
		},
		{
			name:     "unknown tool",
			toolName: "raven_not_a_real_tool",
			wantID:   "",
			wantOK:   false,
		},
		{
			name:     "compatibility alias",
			toolName: "raven_template",
			wantID:   "template_list",
			wantOK:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotID, gotOK := ResolveToolCommandID(tc.toolName)
			if gotOK != tc.wantOK {
				t.Fatalf("ResolveToolCommandID(%q) ok = %v, want %v", tc.toolName, gotOK, tc.wantOK)
			}
			if gotID != tc.wantID {
				t.Fatalf("ResolveToolCommandID(%q) id = %q, want %q", tc.toolName, gotID, tc.wantID)
			}
		})
	}
}
