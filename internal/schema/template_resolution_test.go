package schema

import "testing"

func TestResolveTypeTemplateFile(t *testing.T) {
	t.Run("resolves explicit template ID", func(t *testing.T) {
		sch := &Schema{
			Templates: map[string]*TemplateDefinition{
				"interview_technical": {File: "templates/interview/technical.md"},
			},
			Types: map[string]*TypeDefinition{
				"interview": {
					Templates: []string{"interview_technical"},
				},
			},
		}

		got, err := ResolveTypeTemplateFile(sch, "interview", "interview_technical")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "templates/interview/technical.md" {
			t.Fatalf("expected template path %q, got %q", "templates/interview/technical.md", got)
		}
	})

	t.Run("resolves default template ID", func(t *testing.T) {
		sch := &Schema{
			Templates: map[string]*TemplateDefinition{
				"interview_screen": {File: "templates/interview/screen.md"},
			},
			Types: map[string]*TypeDefinition{
				"interview": {
					Templates:       []string{"interview_screen"},
					DefaultTemplate: "interview_screen",
				},
			},
		}

		got, err := ResolveTypeTemplateFile(sch, "interview", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "templates/interview/screen.md" {
			t.Fatalf("expected template path %q, got %q", "templates/interview/screen.md", got)
		}
	})

	t.Run("returns no template when no default", func(t *testing.T) {
		sch := &Schema{
			Templates: map[string]*TemplateDefinition{
				"interview_screen": {File: "templates/interview/screen.md"},
			},
			Types: map[string]*TypeDefinition{
				"interview": {
					Templates: []string{"interview_screen"},
				},
			},
		}

		got, err := ResolveTypeTemplateFile(sch, "interview", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Fatalf("expected empty template path, got %q", got)
		}
	})

	t.Run("falls back to legacy type template", func(t *testing.T) {
		sch := &Schema{
			Types: map[string]*TypeDefinition{
				"meeting": {Template: "templates/meeting.md"},
			},
		}

		got, err := ResolveTypeTemplateFile(sch, "meeting", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "templates/meeting.md" {
			t.Fatalf("expected template path %q, got %q", "templates/meeting.md", got)
		}
	})

	t.Run("errors on unknown template ID", func(t *testing.T) {
		sch := &Schema{
			Templates: map[string]*TemplateDefinition{
				"interview_technical": {File: "templates/interview/technical.md"},
			},
			Types: map[string]*TypeDefinition{
				"interview": {
					Templates: []string{"interview_technical"},
				},
			},
		}

		if _, err := ResolveTypeTemplateFile(sch, "interview", "interview_screen"); err == nil {
			t.Fatal("expected error for unknown template ID, got nil")
		}
	})
}
