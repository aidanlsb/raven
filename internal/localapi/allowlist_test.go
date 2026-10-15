package localapi

import "testing"

func TestCommandAllowed(t *testing.T) {
	t.Parallel()
	allowed := []string{"query", "query_saved_list", "query_saved_get", "objects", "read", "backlinks", "outlinks", "search", "daily", "date", "schema", "vault_stats", "set", "unset", "new", "move"}
	for _, id := range allowed {
		if !CommandAllowed(id) {
			t.Errorf("CommandAllowed(%q) = false, want true", id)
		}
	}

	blocked := []string{"delete", "write", "add", "edit", "reindex", "mcp_serve", "api", "section_delete", "query_saved_set"}
	for _, id := range blocked {
		if CommandAllowed(id) {
			t.Errorf("CommandAllowed(%q) = true, want false", id)
		}
	}
}
