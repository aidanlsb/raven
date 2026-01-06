package workflow

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
)

// Renderer handles workflow rendering with context gathering.
type Renderer struct {
	vaultPath string
	vaultCfg  *config.VaultConfig

	// QueryFunc executes a Raven query and returns JSON results.
	QueryFunc func(query string) (interface{}, error)

	// ReadFunc reads a single object by ID and returns JSON.
	ReadFunc func(id string) (interface{}, error)

	// BacklinksFunc gets backlinks for a target and returns JSON.
	BacklinksFunc func(target string) (interface{}, error)

	// SearchFunc performs full-text search and returns JSON.
	SearchFunc func(term string, limit int) (interface{}, error)
}

// NewRenderer creates a new workflow renderer.
func NewRenderer(vaultPath string, vaultCfg *config.VaultConfig) *Renderer {
	return &Renderer{
		vaultPath: vaultPath,
		vaultCfg:  vaultCfg,
	}
}

// Render executes a workflow with the given inputs.
func (r *Renderer) Render(wf *Workflow, inputs map[string]string) (*RenderResult, error) {
	// Validate inputs
	if err := r.validateInputs(wf, inputs); err != nil {
		return nil, err
	}

	// Apply defaults for missing optional inputs
	resolvedInputs := r.applyDefaults(wf, inputs)

	// Execute context queries with input substitution
	context, err := r.gatherContext(wf, resolvedInputs)
	if err != nil {
		return nil, fmt.Errorf("failed to gather context: %w", err)
	}

	// Render prompt with input and context substitution
	prompt := r.renderPrompt(wf.Prompt, resolvedInputs, context)

	return &RenderResult{
		Name:    wf.Name,
		Prompt:  prompt,
		Context: context,
	}, nil
}

// validateInputs checks that all required inputs are provided.
func (r *Renderer) validateInputs(wf *Workflow, inputs map[string]string) error {
	if wf.Inputs == nil {
		return nil
	}

	var missing []string
	for name, def := range wf.Inputs {
		if def.Required {
			if _, ok := inputs[name]; !ok {
				if def.Default == "" {
					missing = append(missing, name)
				}
			}
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required inputs: %s", strings.Join(missing, ", "))
	}

	return nil
}

// applyDefaults fills in default values for missing optional inputs.
func (r *Renderer) applyDefaults(wf *Workflow, inputs map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range inputs {
		result[k] = v
	}

	if wf.Inputs != nil {
		for name, def := range wf.Inputs {
			if _, ok := result[name]; !ok && def.Default != "" {
				result[name] = def.Default
			}
		}
	}

	return result
}

// gatherContext executes all context queries.
func (r *Renderer) gatherContext(wf *Workflow, inputs map[string]string) (map[string]interface{}, error) {
	if wf.Context == nil {
		return make(map[string]interface{}), nil
	}

	context := make(map[string]interface{})

	for name, query := range wf.Context {
		result, err := r.executeContextQuery(query, inputs)
		if err != nil {
			// Include error in context rather than failing entirely
			context[name] = map[string]interface{}{
				"error": err.Error(),
			}
		} else {
			context[name] = result
		}
	}

	return context, nil
}

// executeContextQuery runs a single context query.
func (r *Renderer) executeContextQuery(query *config.ContextQuery, inputs map[string]string) (interface{}, error) {
	// Substitute input variables in the query
	if query.Read != "" {
		id := substituteInputs(query.Read, inputs)
		if r.ReadFunc == nil {
			return nil, fmt.Errorf("read function not configured")
		}
		return r.ReadFunc(id)
	}

	if query.Query != "" {
		q := substituteInputs(query.Query, inputs)
		if r.QueryFunc == nil {
			return nil, fmt.Errorf("query function not configured")
		}
		return r.QueryFunc(q)
	}

	if query.Backlinks != "" {
		target := substituteInputs(query.Backlinks, inputs)
		if r.BacklinksFunc == nil {
			return nil, fmt.Errorf("backlinks function not configured")
		}
		return r.BacklinksFunc(target)
	}

	if query.Search != "" {
		term := substituteInputs(query.Search, inputs)
		if r.SearchFunc == nil {
			return nil, fmt.Errorf("search function not configured")
		}
		limit := query.Limit
		if limit == 0 {
			limit = 20
		}
		return r.SearchFunc(term, limit)
	}

	return nil, fmt.Errorf("context query has no recognized type (read, query, backlinks, or search)")
}

// substituteInputs replaces {{inputs.X}} with input values.
func substituteInputs(template string, inputs map[string]string) string {
	// Match {{inputs.name}}
	re := regexp.MustCompile(`\{\{inputs\.([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)
	return re.ReplaceAllStringFunc(template, func(match string) string {
		// Extract the input name
		name := re.FindStringSubmatch(match)[1]
		if val, ok := inputs[name]; ok {
			return val
		}
		return match // Leave unsubstituted if not found
	})
}

// renderPrompt substitutes both input and context variables.
func (r *Renderer) renderPrompt(template string, inputs map[string]string, context map[string]interface{}) string {
	result := template

	// First, handle escaped braces
	result = strings.ReplaceAll(result, "\\{{", "\x00ESCAPED_OPEN\x00")
	result = strings.ReplaceAll(result, "\\}}", "\x00ESCAPED_CLOSE\x00")

	// Substitute {{inputs.X}}
	result = substituteInputs(result, inputs)

	// Substitute {{context.X}} and {{context.X.Y}}
	re := regexp.MustCompile(`\{\{context\.([a-zA-Z_][a-zA-Z0-9_.]*)\}\}`)
	result = re.ReplaceAllStringFunc(result, func(match string) string {
		path := re.FindStringSubmatch(match)[1]
		val := resolveContextPath(context, path)
		if val == nil {
			return match // Leave unsubstituted
		}
		return formatValue(val)
	})

	// Restore escaped braces
	result = strings.ReplaceAll(result, "\x00ESCAPED_OPEN\x00", "{{")
	result = strings.ReplaceAll(result, "\x00ESCAPED_CLOSE\x00", "}}")

	return result
}

// resolveContextPath resolves a dotted path like "meeting.title" from context.
func resolveContextPath(context map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	var current interface{} = context

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = v[part]
			if !ok {
				return nil
			}
		default:
			return nil
		}
	}

	return current
}

// formatValue converts a value to a string for template substitution.
func formatValue(val interface{}) string {
	switch v := val.(type) {
	case string:
		return v
	case float64:
		// Format without unnecessary decimals
		if v == float64(int(v)) {
			return fmt.Sprintf("%d", int(v))
		}
		return fmt.Sprintf("%v", v)
	case bool:
		return fmt.Sprintf("%v", v)
	case nil:
		return ""
	default:
		// For complex types, return JSON
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}
