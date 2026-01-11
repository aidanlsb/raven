package workflow

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
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
	// Use safe placeholder strings instead of null bytes to avoid editor issues
	result = strings.ReplaceAll(result, "\\{{", "«RAVEN_ESC_OPEN»")
	result = strings.ReplaceAll(result, "\\}}", "«RAVEN_ESC_CLOSE»")

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
	result = strings.ReplaceAll(result, "«RAVEN_ESC_OPEN»", "{{")
	result = strings.ReplaceAll(result, "«RAVEN_ESC_CLOSE»", "}}")

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
	case []interface{}:
		// Format arrays more readably
		return formatArray(v)
	case []map[string]interface{}:
		// Format arrays of maps (common from query results)
		return formatMapArray(v)
	case map[string]interface{}:
		// Format objects more readably
		return formatObject(v)
	default:
		// For other complex types, return pretty JSON
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// formatMapArray formats an array of maps for readable output.
func formatMapArray(arr []map[string]interface{}) string {
	if len(arr) == 0 {
		return "(none)"
	}

	var lines []string
	for i, item := range arr {
		line := formatObjectSummary(item, i+1)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// formatArray formats an array for readable output in prompts.
func formatArray(arr []interface{}) string {
	if len(arr) == 0 {
		return "(none)"
	}

	var lines []string
	for i, item := range arr {
		switch v := item.(type) {
		case map[string]interface{}:
			// For objects, try to extract meaningful info
			line := formatObjectSummary(v, i+1)
			lines = append(lines, line)
		case string:
			lines = append(lines, fmt.Sprintf("- %s", v))
		default:
			b, _ := json.Marshal(v)
			lines = append(lines, fmt.Sprintf("- %s", string(b)))
		}
	}
	return strings.Join(lines, "\n")
}

// formatObjectSummary creates a readable one-line summary of an object.
func formatObjectSummary(obj map[string]interface{}, num int) string {
	// Try to find the most useful fields to display
	var parts []string

	// First, try to get a good identifier
	identifier := ""
	// Prefer title/name over IDs for readability
	for _, key := range []string{"title", "name"} {
		if val, ok := obj[key]; ok && val != nil {
			if s, ok := val.(string); ok && s != "" {
				identifier = s
				break
			}
		}
	}
	// Fall back to ID fields
	if identifier == "" {
		for _, key := range []string{"id", "object_id", "source_id"} {
			if val, ok := obj[key]; ok && val != nil {
				if s, ok := val.(string); ok && s != "" {
					identifier = s
					break
				}
			}
		}
	}
	if identifier != "" {
		parts = append(parts, identifier)
	}

	// Add type if present
	if t, ok := obj["type"].(string); ok && t != "" {
		parts = append(parts, fmt.Sprintf("(%s)", t))
	}

	// Add file path if present and not already shown via identifier
	if fp, ok := obj["file_path"].(string); ok && fp != "" {
		hasPath := false
		for _, p := range parts {
			if strings.Contains(p, "/") {
				hasPath = true
				break
			}
		}
		if !hasPath {
			parts = append(parts, fmt.Sprintf("[%s]", fp))
		}
	}

	// Add line number if present
	if line, ok := obj["line"].(float64); ok {
		parts = append(parts, fmt.Sprintf("line %d", int(line)))
	}

	// Add snippet if present (for search results)
	if snippet, ok := obj["snippet"].(string); ok && snippet != "" {
		// Truncate long snippets
		if len(snippet) > 100 {
			snippet = snippet[:97] + "..."
		}
		// Clean up snippet for single line
		snippet = strings.ReplaceAll(snippet, "\n", " ")
		parts = append(parts, fmt.Sprintf("- %s", snippet))
	}

	if len(parts) == 0 {
		// Fallback to JSON
		b, _ := json.Marshal(obj)
		return fmt.Sprintf("%d. %s", num, string(b))
	}

	return fmt.Sprintf("%d. %s", num, strings.Join(parts, " "))
}

// formatObject formats a single object for readable output.
func formatObject(obj map[string]interface{}) string {
	// If it has content, prefer showing that
	if content, ok := obj["content"].(string); ok && content != "" {
		return content
	}

	// Otherwise, format as readable key-value pairs
	var lines []string
	for key, val := range obj {
		if val == nil {
			continue
		}
		switch v := val.(type) {
		case string:
			if v != "" {
				lines = append(lines, fmt.Sprintf("- **%s**: %s", key, v))
			}
		case float64:
			lines = append(lines, fmt.Sprintf("- **%s**: %v", key, v))
		case bool:
			lines = append(lines, fmt.Sprintf("- **%s**: %v", key, v))
		case map[string]interface{}:
			b, _ := json.Marshal(v)
			lines = append(lines, fmt.Sprintf("- **%s**: %s", key, string(b)))
		default:
			b, _ := json.Marshal(v)
			lines = append(lines, fmt.Sprintf("- **%s**: %s", key, string(b)))
		}
	}

	if len(lines) == 0 {
		return "(empty)"
	}

	// Sort for consistent output
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}
