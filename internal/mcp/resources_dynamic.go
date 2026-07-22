package mcp

import (
	"encoding/json"

	"github.com/aidanlsb/raven/internal/querysvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// readSavedQueriesResourceAt renders the raven://queries/saved resource. It
// delegates to querysvc.List (the same service behind the query_saved_list
// command) and reuses the shared SavedQueryInfo.Payload shaping, so the resource
// content stays identical to the command's --json data.
func (s *Server) readSavedQueriesResourceAt(vaultPath string) (string, error) {
	rt, err := vaultruntime.New(vaultPath, vaultruntime.Options{SkipSchema: true})
	if err != nil {
		return "", err
	}
	defer rt.Close()
	result, err := querysvc.List(rt, querysvc.ListRequest{VaultPath: vaultPath})
	if err != nil {
		return "", err
	}

	queries := make([]map[string]interface{}, 0, len(result.Queries))
	for _, q := range result.Queries {
		queries = append(queries, q.Payload())
	}

	payload := map[string]interface{}{
		"queries": queries,
	}
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}
