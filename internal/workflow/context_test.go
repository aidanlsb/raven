package workflow

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestRenderer_ContextQueriesAndDefaults(t *testing.T) {
	wf := &Workflow{
		Name:   "test",
		Prompt: "Q={{inputs.q}}\nC={{context.q}}\nS={{context.s}}",
		Inputs: map[string]*config.WorkflowInput{
			"q": {Type: "string", Required: true},
			"term": {
				Type:     "string",
				Required: false,
				Default:  "freya",
			},
		},
		Context: map[string]*config.ContextQuery{
			// Missing QueryFunc should not fail render; error should be embedded in context.
			"q": {Query: `object:person .name=={{inputs.q}}`},
			// Search uses default limit when unset.
			"s": {Search: `{{inputs.term}}`},
		},
	}

	var gotSearchTerm string
	var gotSearchLimit int

	r := &Renderer{
		QueryFunc: nil,
		SearchFunc: func(term string, limit int) (interface{}, error) {
			gotSearchTerm = term
			gotSearchLimit = limit
			return []map[string]interface{}{
				{"title": "Freya", "file_path": "people/freya.md"},
			}, nil
		},
	}

	res, err := r.Render(wf, map[string]string{"q": "Freya"})
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	if gotSearchTerm != "freya" {
		t.Fatalf("Search term = %q, want %q (default input)", gotSearchTerm, "freya")
	}
	if gotSearchLimit != 20 {
		t.Fatalf("Search limit = %d, want 20 (default)", gotSearchLimit)
	}

	// Query context should contain an embedded error.
	qCtx, ok := res.Context["q"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected context.q to be an error map, got %#v", res.Context["q"])
	}
	if errMsg, _ := qCtx["error"].(string); errMsg == "" || !strings.Contains(errMsg, "query function not configured") {
		t.Fatalf("expected query error in context, got %#v", res.Context["q"])
	}

	// Prompt should reflect input substitution and include formatted context.
	if !strings.Contains(res.Prompt, "Q=Freya") {
		t.Fatalf("expected prompt to include input substitution, got:\n%s", res.Prompt)
	}
	if !strings.Contains(res.Prompt, "**error**") {
		t.Fatalf("expected prompt to include formatted error object, got:\n%s", res.Prompt)
	}
	if !strings.Contains(res.Prompt, "Freya") || !strings.Contains(res.Prompt, "people/freya.md") {
		t.Fatalf("expected prompt to include formatted search context, got:\n%s", res.Prompt)
	}
}

func TestExecuteContextQuery_NoRecognizedType(t *testing.T) {
	r := &Renderer{}
	_, err := r.executeContextQuery(&config.ContextQuery{}, map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "no recognized type") {
		t.Fatalf("expected no recognized type error, got %v", err)
	}
}

