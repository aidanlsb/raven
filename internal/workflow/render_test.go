package workflow

import (
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestSubstituteInputs(t *testing.T) {
	inputs := map[string]string{
		"person_id": "people/freya",
		"question":  "How does auth work?",
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "simple substitution",
			template: "read: {{inputs.person_id}}",
			expected: "read: people/freya",
		},
		{
			name:     "multiple substitutions",
			template: "{{inputs.person_id}} and {{inputs.question}}",
			expected: "people/freya and How does auth work?",
		},
		{
			name:     "unknown input left as-is",
			template: "{{inputs.unknown}}",
			expected: "{{inputs.unknown}}",
		},
		{
			name:     "no substitutions",
			template: "just plain text",
			expected: "just plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := substituteInputs(tt.template, inputs)
			if result != tt.expected {
				t.Errorf("substituteInputs() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestValidateInputs(t *testing.T) {
	wf := &Workflow{
		Inputs: map[string]*config.WorkflowInput{
			"required_input": {
				Type:     "string",
				Required: true,
			},
			"optional_input": {
				Type:     "string",
				Required: false,
			},
			"with_default": {
				Type:     "string",
				Required: true,
				Default:  "default_value",
			},
		},
	}

	renderer := &Renderer{}

	tests := []struct {
		name        string
		inputs      map[string]string
		expectError bool
	}{
		{
			name: "all required present",
			inputs: map[string]string{
				"required_input": "value",
			},
			expectError: false,
		},
		{
			name:        "missing required",
			inputs:      map[string]string{},
			expectError: true,
		},
		{
			name: "required with default is ok when missing",
			inputs: map[string]string{
				"required_input": "value",
				// with_default is required but has default, so it's ok
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := renderer.validateInputs(wf, tt.inputs)
			if (err != nil) != tt.expectError {
				t.Errorf("validateInputs() error = %v, expectError %v", err, tt.expectError)
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	wf := &Workflow{
		Inputs: map[string]*config.WorkflowInput{
			"with_default": {
				Type:    "string",
				Default: "default_value",
			},
			"no_default": {
				Type: "string",
			},
		},
	}

	renderer := &Renderer{}

	inputs := map[string]string{
		"no_default": "provided",
	}

	result := renderer.applyDefaults(wf, inputs)

	if result["with_default"] != "default_value" {
		t.Errorf("expected with_default='default_value', got %q", result["with_default"])
	}
	if result["no_default"] != "provided" {
		t.Errorf("expected no_default='provided', got %q", result["no_default"])
	}
}

func TestRenderPrompt(t *testing.T) {
	renderer := &Renderer{}

	inputs := map[string]string{
		"name": "Freya",
	}

	context := map[string]interface{}{
		"person": map[string]interface{}{
			"id":   "people/freya",
			"name": "Freya",
		},
		"count": 42,
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "input substitution",
			template: "Hello {{inputs.name}}!",
			expected: "Hello Freya!",
		},
		{
			name:     "context object",
			template: "Person: {{context.person}}",
			expected: "Person: - **id**: people/freya\n- **name**: Freya",
		},
		{
			name:     "context path",
			template: "ID: {{context.person.id}}",
			expected: "ID: people/freya",
		},
		{
			name:     "escaped braces",
			template: `Use \{{literal\}} braces`,
			expected: "Use {{literal}} braces",
		},
		{
			name:     "mixed",
			template: "{{inputs.name}} at {{context.person.id}}",
			expected: "Freya at people/freya",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderer.renderPrompt(tt.template, inputs, context)
			if result != tt.expected {
				t.Errorf("renderPrompt() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestResolveContextPath(t *testing.T) {
	context := map[string]interface{}{
		"person": map[string]interface{}{
			"id":   "people/freya",
			"name": "Freya",
			"nested": map[string]interface{}{
				"deep": "value",
			},
		},
		"count": 42,
	}

	// Test simple string value
	if result := resolveContextPath(context, "person.id"); result != "people/freya" {
		t.Errorf("resolveContextPath(person.id) = %v, want people/freya", result)
	}

	// Test nested path
	if result := resolveContextPath(context, "person.nested.deep"); result != "value" {
		t.Errorf("resolveContextPath(person.nested.deep) = %v, want value", result)
	}

	// Test number
	if result := resolveContextPath(context, "count"); result != 42 {
		t.Errorf("resolveContextPath(count) = %v, want 42", result)
	}

	// Test missing path returns nil
	if result := resolveContextPath(context, "nonexistent"); result != nil {
		t.Errorf("resolveContextPath(nonexistent) = %v, want nil", result)
	}

	// Test missing nested path returns nil
	if result := resolveContextPath(context, "person.missing"); result != nil {
		t.Errorf("resolveContextPath(person.missing) = %v, want nil", result)
	}

	// Test that resolving to a map returns non-nil
	if result := resolveContextPath(context, "person"); result == nil {
		t.Errorf("resolveContextPath(person) = nil, want non-nil map")
	}
}
