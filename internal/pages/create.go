// Package pages provides utilities for creating and managing page files.
package pages

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gosimple/slug"
	"github.com/ravenscroftj/raven/internal/schema"
)

// CreateOptions configures page creation behavior.
type CreateOptions struct {
	// VaultPath is the root path of the vault.
	VaultPath string

	// TypeName is the type of the page (e.g., "person", "project").
	TypeName string

	// Title is the display title for the page (used in heading).
	// If empty, derived from the target path.
	Title string

	// TargetPath is the relative path within the vault (e.g., "people/alice").
	// Will be slugified automatically.
	TargetPath string

	// Fields are additional frontmatter fields to include.
	// Keys are field names, values are the field values.
	Fields map[string]string

	// Schema is used to determine required fields for the type.
	// If nil, only basic frontmatter is generated.
	Schema *schema.Schema

	// IncludeRequiredPlaceholders adds empty placeholders for required fields.
	IncludeRequiredPlaceholders bool
}

// CreateResult contains information about the created page.
type CreateResult struct {
	// FilePath is the absolute path to the created file.
	FilePath string

	// RelativePath is the path relative to the vault.
	RelativePath string

	// Slugified path used for the file.
	SlugifiedPath string
}

// Create creates a new page file with the given options.
func Create(opts CreateOptions) (*CreateResult, error) {
	if opts.VaultPath == "" {
		return nil, fmt.Errorf("vault path is required")
	}
	if opts.TypeName == "" {
		return nil, fmt.Errorf("type name is required")
	}
	if opts.TargetPath == "" {
		return nil, fmt.Errorf("target path is required")
	}

	// Extract original title before slugifying
	originalBaseName := filepath.Base(opts.TargetPath)
	originalBaseName = strings.TrimSuffix(originalBaseName, ".md")

	// Use provided title or derive from path
	title := opts.Title
	if title == "" {
		title = originalBaseName
	}

	// Slugify the path for the filename
	slugifiedPath := SlugifyPath(opts.TargetPath)

	// Build the file path with slugified name
	filePath := filepath.Join(opts.VaultPath, slugifiedPath)
	if !strings.HasSuffix(filePath, ".md") {
		filePath += ".md"
	}

	// Security: verify path is within vault
	absVault, err := filepath.Abs(opts.VaultPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve vault path: %w", err)
	}
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve file path: %w", err)
	}
	if !strings.HasPrefix(absFile, absVault+string(filepath.Separator)) && absFile != absVault {
		return nil, fmt.Errorf("cannot create file outside vault")
	}

	// Create parent directories
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Build frontmatter
	var content strings.Builder
	content.WriteString("---\n")
	content.WriteString(fmt.Sprintf("type: %s\n", opts.TypeName))

	// Collect all fields to write
	allFields := make(map[string]string)

	// Add provided fields
	for k, v := range opts.Fields {
		allFields[k] = v
	}

	// Add required field placeholders if requested
	if opts.IncludeRequiredPlaceholders && opts.Schema != nil {
		if typeDef, ok := opts.Schema.Types[opts.TypeName]; ok && typeDef != nil {
			// Add required fields
			for fieldName, fieldDef := range typeDef.Fields {
				if fieldDef != nil && fieldDef.Required {
					if _, exists := allFields[fieldName]; !exists {
						allFields[fieldName] = "" // Empty placeholder
					}
				}
			}

			// Add required traits
			for _, traitName := range typeDef.Traits.List() {
				if typeDef.Traits.IsRequired(traitName) {
					if _, exists := allFields[traitName]; !exists {
						allFields[traitName] = "" // Empty placeholder
					}
				}
			}
		}
	}

	// Sort field names for consistent output
	var sortedFields []string
	for name := range allFields {
		sortedFields = append(sortedFields, name)
	}
	sort.Strings(sortedFields)

	for _, fieldName := range sortedFields {
		value := allFields[fieldName]
		if value == "" {
			content.WriteString(fmt.Sprintf("%s: \n", fieldName))
		} else if strings.ContainsAny(value, ":\n\"'") {
			// Quote values that need it
			content.WriteString(fmt.Sprintf("%s: \"%s\"\n", fieldName, strings.ReplaceAll(value, "\"", "\\\"")))
		} else {
			content.WriteString(fmt.Sprintf("%s: %s\n", fieldName, value))
		}
	}

	content.WriteString("---\n\n")
	content.WriteString(fmt.Sprintf("# %s\n\n", title))

	// Write the file
	if err := os.WriteFile(filePath, []byte(content.String()), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	relPath, _ := filepath.Rel(opts.VaultPath, filePath)

	return &CreateResult{
		FilePath:      filePath,
		RelativePath:  relPath,
		SlugifiedPath: slugifiedPath,
	}, nil
}

// Exists checks if a page already exists at the given path.
func Exists(vaultPath, targetPath string) bool {
	slugifiedPath := SlugifyPath(targetPath)
	filePath := filepath.Join(vaultPath, slugifiedPath)
	if !strings.HasSuffix(filePath, ".md") {
		filePath += ".md"
	}
	_, err := os.Stat(filePath)
	return err == nil
}

// SlugifyPath slugifies each component of a path.
// "people/Emily Jia" -> "people/emily-jia"
// Also handles embedded object IDs: "daily/2025-02-01#Team Sync" -> "daily/2025-02-01#team-sync"
func SlugifyPath(path string) string {
	// Remove .md extension if present
	path = strings.TrimSuffix(path, ".md")

	parts := strings.Split(path, "/")
	for i, part := range parts {
		// Handle embedded object IDs (file#id)
		if strings.Contains(part, "#") {
			subParts := strings.SplitN(part, "#", 2)
			parts[i] = Slugify(subParts[0]) + "#" + Slugify(subParts[1])
		} else {
			parts[i] = Slugify(part)
		}
	}
	return strings.Join(parts, "/")
}

// Slugify converts a string to a URL-safe slug.
func Slugify(s string) string {
	s = strings.TrimSuffix(s, ".md")
	slugged := slug.Make(s)
	if slugged == "" {
		slugged = strings.ToLower(strings.ReplaceAll(s, " ", "-"))
	}
	return slugged
}

// CreateDailyNote creates a daily note for the given date.
func CreateDailyNote(vaultPath, dailyDir, dateStr, friendlyTitle string) (*CreateResult, error) {
	targetPath := filepath.Join(dailyDir, dateStr)

	return Create(CreateOptions{
		VaultPath:  vaultPath,
		TypeName:   "date",
		Title:      friendlyTitle,
		TargetPath: targetPath,
	})
}
