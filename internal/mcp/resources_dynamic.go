package mcp

import (
	"encoding/json"
	"sort"

	"github.com/aidanlsb/raven/internal/config"
)

type savedQueryResource struct {
	Name        string `json:"name"`
	Query       string `json:"query"`
	Description string `json:"description,omitempty"`
}

func (s *Server) readSavedQueriesResource(vaultName, vaultPath string) (string, error) {
	vaultPath, err := s.resolveVaultPathForInvocation(vaultName, vaultPath)
	if err != nil {
		return "", err
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return "", err
	}

	queries := make([]savedQueryResource, 0, len(vaultCfg.Queries))
	for name, q := range vaultCfg.Queries {
		if q == nil {
			continue
		}
		queries = append(queries, savedQueryResource{
			Name:        name,
			Query:       q.Query,
			Description: q.Description,
		})
	}
	sort.Slice(queries, func(i, j int) bool {
		return queries[i].Name < queries[j].Name
	})

	payload := map[string]interface{}{
		"queries": queries,
	}
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}
