package template

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApply_BasicVariables(t *testing.T) {
	vars := &Variables{
		Title:    "Team Sync",
		Slug:     "team-sync",
		Type:     "meeting",
		Date:     "2026-01-05",
		Datetime: "2026-01-05T14:30",
		Year:     "2026",
		Month:    "01",
		Day:      "05",
		Weekday:  "Monday",
		Fields:   map[string]string{"time": "14:00", "location": "Room A"},
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "title substitution",
			template: "# {{title}}",
			expected: "# Team Sync",
		},
		{
			name:     "slug substitution",
			template: "File: {{slug}}.md",
			expected: "File: team-sync.md",
		},
		{
			name:     "type substitution",
			template: "Type: {{type}}",
			expected: "Type: meeting",
		},
		{
			name:     "date variables",
			template: "Date: {{date}} ({{year}}-{{month}}-{{day}})",
			expected: "Date: 2026-01-05 (2026-01-05)",
		},
		{
			name:     "datetime substitution",
			template: "Created: {{datetime}}",
			expected: "Created: 2026-01-05T14:30",
		},
		{
			name:     "weekday substitution",
			template: "Day: {{weekday}}",
			expected: "Day: Monday",
		},
		{
			name:     "field variables",
			template: "Time: {{field.time}}, Location: {{field.location}}",
			expected: "Time: 14:00, Location: Room A",
		},
		{
			name:     "multiple substitutions",
			template: "# {{title}}\n\nCreated: {{date}}\nType: {{type}}",
			expected: "# Team Sync\n\nCreated: 2026-01-05\nType: meeting",
		},
		{
			name:     "unknown variable preserved",
			template: "Unknown: {{unknown}}",
			expected: "Unknown: {{unknown}}",
		},
		{
			name:     "unknown field preserved",
			template: "Unknown: {{field.missing}}",
			expected: "Unknown: {{field.missing}}",
		},
		{
			name:     "escaped braces",
			template: "Literal: \\{{title}}",
			expected: "Literal: {{title}}",
		},
		{
			name:     "empty template",
			template: "",
			expected: "",
		},
		{
			name:     "no variables",
			template: "Plain text content",
			expected: "Plain text content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Apply(tt.template, vars)
			if result != tt.expected {
				t.Errorf("Apply() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestApply_NilVars(t *testing.T) {
	result := Apply("# {{title}}", nil)
	if result != "# {{title}}" {
		t.Errorf("Apply with nil vars should return original, got %q", result)
	}
}

func TestNewVariables(t *testing.T) {
	fields := map[string]string{"name": "Alice"}
	vars := NewVariables("My Title", "person", "my-title", fields)

	if vars.Title != "My Title" {
		t.Errorf("Title = %q, want %q", vars.Title, "My Title")
	}
	if vars.Type != "person" {
		t.Errorf("Type = %q, want %q", vars.Type, "person")
	}
	if vars.Slug != "my-title" {
		t.Errorf("Slug = %q, want %q", vars.Slug, "my-title")
	}
	if vars.Fields["name"] != "Alice" {
		t.Errorf("Fields[name] = %q, want %q", vars.Fields["name"], "Alice")
	}
	// Date/time should be set to now
	if vars.Date == "" {
		t.Error("Date should not be empty")
	}
	if vars.Year == "" {
		t.Error("Year should not be empty")
	}
}

func TestNewDailyVariables(t *testing.T) {
	date := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	vars := NewDailyVariables(date)

	if vars.Date != "2026-01-05" {
		t.Errorf("Date = %q, want %q", vars.Date, "2026-01-05")
	}
	if vars.Year != "2026" {
		t.Errorf("Year = %q, want %q", vars.Year, "2026")
	}
	if vars.Month != "01" {
		t.Errorf("Month = %q, want %q", vars.Month, "01")
	}
	if vars.Day != "05" {
		t.Errorf("Day = %q, want %q", vars.Day, "05")
	}
	if vars.Weekday != "Monday" {
		t.Errorf("Weekday = %q, want %q", vars.Weekday, "Monday")
	}
	if vars.Type != "date" {
		t.Errorf("Type = %q, want %q", vars.Type, "date")
	}
}

func TestLoad_InlineTemplate(t *testing.T) {
	// Multi-line content should be treated as inline
	content, err := Load("/tmp", "# {{title}}\n\n## Notes")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if content != "# {{title}}\n\n## Notes" {
		t.Errorf("Load() = %q, want inline content", content)
	}
}

func TestLoad_EmptyTemplate(t *testing.T) {
	content, err := Load("/tmp", "")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if content != "" {
		t.Errorf("Load() = %q, want empty string", content)
	}
}

func TestLoad_FileTemplate(t *testing.T) {
	// Create a temp directory and template file
	tmpDir := t.TempDir()
	templateDir := filepath.Join(tmpDir, "templates")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatalf("Failed to create template dir: %v", err)
	}

	templatePath := filepath.Join(templateDir, "meeting.md")
	templateContent := "# {{title}}\n\n## Attendees\n\n## Notes"
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to write template: %v", err)
	}

	// Load the template
	content, err := Load(tmpDir, "templates/meeting.md")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if content != templateContent {
		t.Errorf("Load() = %q, want %q", content, templateContent)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Loading a missing file should return empty string (per spec)
	content, err := Load(tmpDir, "templates/missing.md")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if content != "" {
		t.Errorf("Load() = %q, want empty string for missing file", content)
	}
}

func TestLoad_PathOutsideVault(t *testing.T) {
	tmpDir := t.TempDir()

	// Trying to load a file outside vault should return empty (security)
	content, err := Load(tmpDir, "../etc/passwd")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if content != "" {
		t.Errorf("Load() = %q, want empty string for path outside vault", content)
	}
}

func TestIsPath(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"templates/meeting.md", true},
		{"meeting.md", true},
		{"templates", true},
		{"# {{title}}\n\nContent", false},
		{"This is inline content", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isPath(tt.input)
			if result != tt.expected {
				t.Errorf("isPath(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestApply_FullTemplate(t *testing.T) {
	vars := &Variables{
		Title:    "Weekly Team Sync",
		Slug:     "weekly-team-sync",
		Type:     "meeting",
		Date:     "2026-01-05",
		Datetime: "2026-01-05T10:00",
		Year:     "2026",
		Month:    "01",
		Day:      "05",
		Weekday:  "Monday",
		Fields:   map[string]string{"time": "10:00 AM"},
	}

	template := `# {{title}}

**Date:** {{date}} ({{weekday}})
**Time:** {{field.time}}

## Attendees

## Agenda

## Notes

## Action Items
`

	expected := `# Weekly Team Sync

**Date:** 2026-01-05 (Monday)
**Time:** 10:00 AM

## Attendees

## Agenda

## Notes

## Action Items
`

	result := Apply(template, vars)
	if result != expected {
		t.Errorf("Apply() = %q, want %q", result, expected)
	}
}
