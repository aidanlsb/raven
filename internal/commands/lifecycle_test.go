package commands

import "testing"

func TestResolveCommandID(t *testing.T) {
	tests := []struct {
		path   string
		wantID string
		wantOK bool
	}{
		{path: "edit", wantID: "edit", wantOK: true},
		{path: "schema add field", wantID: "schema_add_field", wantOK: true},
		{path: "query add", wantID: "query_add", wantOK: true},
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

func TestMutatingCommandFlags(t *testing.T) {
	cases := []struct {
		id      string
		mutates bool
	}{
		{id: "edit", mutates: true},
		{id: "new", mutates: true},
		{id: "read", mutates: false},
		{id: "stats", mutates: false},
	}

	for _, tc := range cases {
		meta, ok := Registry[tc.id]
		if !ok {
			t.Fatalf("registry missing %q", tc.id)
		}
		if meta.MutatesVault != tc.mutates {
			t.Fatalf("Registry[%q].MutatesVault=%v, want %v", tc.id, meta.MutatesVault, tc.mutates)
		}
	}
}
