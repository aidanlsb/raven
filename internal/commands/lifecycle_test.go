package commands

import "testing"

func TestResolveCommandID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path   string
		wantID string
		wantOK bool
	}{
		{path: "edit", wantID: "edit", wantOK: true},
		{path: "schema add field", wantID: "schema_add_field", wantOK: true},
		{path: "query add", wantID: "query_add", wantOK: true},
		{path: "vault path", wantID: "vault_path", wantOK: true},
		{path: "vault stats", wantID: "vault_stats", wantOK: true},
		{path: "not a real command", wantID: "", wantOK: false},
	}

	for _, tt := range tests {
		gotID, gotOK := ResolveCommandID(tt.path)
		if gotOK != tt.wantOK {
			t.Fatalf("ResolveCommandID(%q) ok=%v, want %v", tt.path, gotOK, tt.wantOK)
		}
		if gotID != tt.wantID {
			t.Fatalf("ResolveCommandID(%q) id=%q, want %q", tt.path, gotID, tt.wantID)
		}
	}
}
