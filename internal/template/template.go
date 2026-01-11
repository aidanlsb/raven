// Package template provides template loading and variable substitution for Raven.
package template

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Variables holds the available template variables for substitution.
type Variables struct {
	// Title is the title passed to rvn new
	Title string
	// Slug is the slugified title
	Slug string
	// Type is the type name
	Type string
	// Date is today's date (YYYY-MM-DD)
	Date string
	// Datetime is the current datetime (YYYY-MM-DDTHH:MM)
	Datetime string
	// Year is the current year
	Year string
	// Month is the current month (2 digit)
	Month string
	// Day is the current day (2 digit)
	Day string
	// Weekday is the day name (Monday, Tuesday, etc.)
	Weekday string
	// Fields are field values from --field flags
	Fields map[string]string
}

// NewVariables creates a Variables struct with the given title, type, and fields.
// Date/time fields are populated with current time.
func NewVariables(title, typeName, slug string, fields map[string]string) *Variables {
	now := time.Now()
	return &Variables{
		Title:    title,
		Slug:     slug,
		Type:     typeName,
		Date:     now.Format("2006-01-02"),
		Datetime: now.Format("2006-01-02T15:04"),
		Year:     now.Format("2006"),
		Month:    now.Format("01"),
		Day:      now.Format("02"),
		Weekday:  now.Weekday().String(),
		Fields:   fields,
	}
}

// NewDailyVariables creates Variables for a daily note with a specific date.
func NewDailyVariables(date time.Time) *Variables {
	return &Variables{
		Title:    date.Format("2006-01-02"),
		Slug:     date.Format("2006-01-02"),
		Type:     "date",
		Date:     date.Format("2006-01-02"),
		Datetime: date.Format("2006-01-02T15:04"),
		Year:     date.Format("2006"),
		Month:    date.Format("01"),
		Day:      date.Format("02"),
		Weekday:  date.Weekday().String(),
		Fields:   make(map[string]string),
	}
}

// Load loads a template from either a file path or returns inline content.
// If templateSpec starts with a path-like pattern, it's treated as a file path
// relative to vaultPath. Otherwise, it's treated as inline template content.
func Load(vaultPath, templateSpec string) (string, error) {
	if templateSpec == "" {
		return "", nil
	}

	// Heuristic: if it looks like a path (contains "/" or ends with ".md"),
	// treat it as a file path. Otherwise, treat as inline content.
	if isPath(templateSpec) {
		return loadFromFile(vaultPath, templateSpec)
	}

	// Inline template content
	return templateSpec, nil
}

// isPath determines if a string looks like a file path.
func isPath(s string) bool {
	// If it contains a slash, it's a path
	if strings.Contains(s, "/") {
		return true
	}
	// If it ends with .md, it's a path
	if strings.HasSuffix(s, ".md") {
		return true
	}
	// If it starts with "templates" or similar directory patterns
	if strings.HasPrefix(s, "templates") {
		return true
	}
	// If it has multiple lines, it's likely inline content, not a path
	if strings.Contains(s, "\n") {
		return false
	}
	// Single line without slashes - could be a simple filename
	// Check if it looks like a filename (word characters, dots, hyphens)
	matched, _ := regexp.MatchString(`^[\w.-]+$`, s)
	return matched && len(s) < 100 // Paths are usually short
}

// loadFromFile loads template content from a file.
func loadFromFile(vaultPath, templatePath string) (string, error) {
	fullPath := filepath.Join(vaultPath, templatePath)

	// Security check: ensure path is within vault
	absVault, err := filepath.Abs(vaultPath)
	if err != nil {
		return "", err
	}
	absTemplate, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absTemplate, absVault+string(filepath.Separator)) {
		return "", nil // Silent fail for security - return empty
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		// Return empty string on missing file (as per spec)
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	return string(content), nil
}

// Apply substitutes template variables in the content.
// Variables use {{name}} syntax. Unknown variables are left as-is.
// Escaped variables \{{name}} are converted to literal {{name}}.
func Apply(content string, vars *Variables) string {
	if content == "" || vars == nil {
		return content
	}

	// First, temporarily replace escaped sequences
	// Use safe placeholder strings instead of null bytes to avoid editor issues
	content = strings.ReplaceAll(content, "\\{{", "«RAVEN_ESC_OPEN»")
	content = strings.ReplaceAll(content, "\\}}", "«RAVEN_ESC_CLOSE»")

	// Build replacement map
	replacements := map[string]string{
		"{{title}}":    vars.Title,
		"{{slug}}":     vars.Slug,
		"{{type}}":     vars.Type,
		"{{date}}":     vars.Date,
		"{{datetime}}": vars.Datetime,
		"{{year}}":     vars.Year,
		"{{month}}":    vars.Month,
		"{{day}}":      vars.Day,
		"{{weekday}}":  vars.Weekday,
	}

	// Apply standard variable replacements
	for placeholder, value := range replacements {
		content = strings.ReplaceAll(content, placeholder, value)
	}

	// Apply field variables: {{field.X}}
	if vars.Fields != nil {
		for fieldName, fieldValue := range vars.Fields {
			placeholder := "{{field." + fieldName + "}}"
			content = strings.ReplaceAll(content, placeholder, fieldValue)
		}
	}

	// Restore escaped sequences as literals
	content = strings.ReplaceAll(content, "«RAVEN_ESC_OPEN»", "{{")
	content = strings.ReplaceAll(content, "«RAVEN_ESC_CLOSE»", "}}")

	return content
}
