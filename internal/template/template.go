// Package template provides template loading and variable substitution for Raven.
package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/paths"
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
		Date:     now.Format(dates.DateLayout),
		Datetime: now.Format(dates.DatetimeLayout),
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
		Title:    date.Format(dates.DateLayout),
		Slug:     date.Format(dates.DateLayout),
		Type:     "date",
		Date:     date.Format(dates.DateLayout),
		Datetime: date.Format(dates.DatetimeLayout),
		Year:     date.Format("2006"),
		Month:    date.Format("01"),
		Day:      date.Format("02"),
		Weekday:  date.Weekday().String(),
		Fields:   make(map[string]string),
	}
}

// Load loads a template from a file path and enforces template directory policy.
// Inline template content is not supported.
func Load(vaultPath, templateSpec, templateDir string) (string, error) {
	if templateSpec == "" {
		return "", nil
	}

	if strings.Contains(templateSpec, "\n") {
		return "", fmt.Errorf("inline template content is not supported; use a template file path")
	}

	fileRef, err := ResolveFileRef(templateSpec, templateDir)
	if err != nil {
		return "", err
	}

	return loadFromFile(vaultPath, fileRef)
}

func normalizeFileRef(filePath string) (string, error) {
	trimmed := strings.TrimSpace(filePath)
	trimmed = strings.ReplaceAll(trimmed, "\\", "/")
	normalized := filepath.ToSlash(filepath.Clean(trimmed))
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	if normalized == "" || normalized == "." {
		return "", fmt.Errorf("template declaration must include a non-empty file path")
	}
	if normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("template file path cannot escape the vault")
	}
	if strings.Contains(normalized, "\n") || strings.Contains(normalized, "\r") {
		return "", fmt.Errorf("template file path cannot contain newlines")
	}
	return normalized, nil
}

// ResolveFileRef normalizes a template file path and enforces directories.template
// policy. Bare filenames are resolved under templateDir.
func ResolveFileRef(filePath, templateDir string) (string, error) {
	normalized, err := normalizeFileRef(filePath)
	if err != nil {
		return "", err
	}
	if templateDir != "" && !strings.HasPrefix(normalized, templateDir) && !strings.Contains(normalized, "/") {
		normalized = templateDir + normalized
	}
	if templateDir != "" && !strings.HasPrefix(normalized, templateDir) {
		return "", fmt.Errorf(
			"template file must be under directories.template %q: got %q",
			templateDir,
			normalized,
		)
	}
	return normalized, nil
}

// loadFromFile loads template content from a file.
func loadFromFile(vaultPath, templatePath string) (string, error) {
	fullPath := filepath.Join(vaultPath, templatePath)

	// Security check: ensure path is within vault
	if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
		return "", fmt.Errorf("template file must be within vault")
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("template file not found: %s", templatePath)
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
