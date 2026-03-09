package mcp

import (
	"encoding/json"
	"sort"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/workflow"
)

type savedQueryResource struct {
	Name        string `json:"name"`
	Query       string `json:"query"`
	Description string `json:"description,omitempty"`
}

func (s *Server) readSavedQueriesResource() (string, error) {
	keepPath, err := s.resolveKeepPath()
	if err != nil {
		return "", err
	}

	keepCfg, err := config.LoadKeepConfig(keepPath)
	if err != nil {
		return "", err
	}

	queries := make([]savedQueryResource, 0, len(keepCfg.Queries))
	for name, q := range keepCfg.Queries {
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

func (s *Server) readWorkflowsListResource() (string, error) {
	keepPath, err := s.resolveKeepPath()
	if err != nil {
		return "", err
	}

	keepCfg, err := config.LoadKeepConfig(keepPath)
	if err != nil {
		return "", err
	}

	items, err := workflow.List(keepPath, keepCfg)
	if err != nil {
		return "", err
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	payload := map[string]interface{}{
		"workflows": items,
	}
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (s *Server) readWorkflowResource(name string) (string, error) {
	keepPath, err := s.resolveKeepPath()
	if err != nil {
		return "", err
	}

	keepCfg, err := config.LoadKeepConfig(keepPath)
	if err != nil {
		return "", err
	}

	wf, err := workflow.Get(keepPath, name, keepCfg)
	if err != nil {
		return "", err
	}

	out := map[string]interface{}{
		"name":        wf.Name,
		"description": wf.Description,
	}
	if len(wf.Inputs) > 0 {
		out["inputs"] = wf.Inputs
	}
	if len(wf.Steps) > 0 {
		out["steps"] = wf.Steps
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
