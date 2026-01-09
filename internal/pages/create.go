// Package pages provides utilities for creating and managing page files.
package pages

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/slugs"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/template"
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

	// TargetPath is the relative path within the vault.
	// Can be just a filename (e.g., "freya") or a full path (e.g., "people/freya").
	// If no directory is specified AND the type has a default_path in the schema,
	// the file will be created in the type's default directory.
	// Will be slugified automatically.
	TargetPath string

	// Fields are additional frontmatter fields to include.
	// Keys are field names, values are the field values.
	Fields map[string]string

	// Schema is used for:
	// 1. Resolving default_path for types
	// 2. Determining required fields for placeholders
	// 3. Loading type templates
	// If nil, no default_path resolution, required field handling, or template loading occurs.
	Schema *schema.Schema

	// IncludeRequiredPlaceholders adds empty placeholders for required fields.
	IncludeRequiredPlaceholders bool

	// TemplateOverride allows overriding the type's template.
	// If set, this is used instead of the schema's template for the type.
	// Can be a file path (relative to vault) or inline template content.
	TemplateOverride string

	// ObjectsRoot is the root directory for typed objects (e.g., "objects/").
	// If set, the type's default_path is nested under this root.
	ObjectsRoot string

	// PagesRoot is the root directory for untyped pages (e.g., "pages/").
	// If set, pages without a type-specific directory go here.
	PagesRoot string
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

	// Extract original title before any path manipulation
	originalBaseName := filepath.Base(opts.TargetPath)
	originalBaseName = strings.TrimSuffix(originalBaseName, ".md")

	// Use provided title or derive from path
	title := opts.Title
	if title == "" {
		title = originalBaseName
	}

	// Resolve target path: apply type's default_path and directory roots
	targetPath := resolveDefaultPathWithRoots(opts.TargetPath, opts.TypeName, opts.Schema, opts.ObjectsRoot, opts.PagesRoot)

	// Slugify the path for the filename
	slugifiedPath := SlugifyPath(targetPath)

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

	// Determine template to use
	templateSpec := opts.TemplateOverride
	if templateSpec == "" && opts.Schema != nil {
		if typeDef, ok := opts.Schema.Types[opts.TypeName]; ok && typeDef != nil {
			templateSpec = typeDef.Template
		}
	}

	// Load and apply template if specified
	if templateSpec != "" {
		templateContent, err := template.Load(opts.VaultPath, templateSpec)
		if err != nil {
			// Log warning but continue without template
			templateContent = ""
		}

		if templateContent != "" {
			// Apply variable substitution
			vars := template.NewVariables(title, opts.TypeName, slugifiedPath, opts.Fields)
			processedContent := template.Apply(templateContent, vars)
			content.WriteString(processedContent)
			// Ensure template ends with newline
			if !strings.HasSuffix(processedContent, "\n") {
				content.WriteString("\n")
			}
		} else {
			// No template - use default heading
			content.WriteString(fmt.Sprintf("# %s\n\n", title))
		}
	} else {
		// No template - use default heading
		content.WriteString(fmt.Sprintf("# %s\n\n", title))
	}

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

// resolveDefaultPath applies the type's default_path if the target doesn't already have a directory.
// This is the single source of truth for default_path resolution.
func resolveDefaultPath(targetPath, typeName string, sch *schema.Schema) string {
	return resolveDefaultPathWithRoots(targetPath, typeName, sch, "", "")
}

// resolveDefaultPathWithRoots applies directory roots and type default_path.
func resolveDefaultPathWithRoots(targetPath, typeName string, sch *schema.Schema, objectsRoot, pagesRoot string) string {
	// Normalize roots
	objectsRoot = paths.NormalizeDirRoot(objectsRoot)
	pagesRoot = paths.NormalizeDirRoot(pagesRoot)

	// If target already has a directory component, just add the appropriate root
	if strings.Contains(targetPath, "/") {
		// Check if it starts with a leading / (absolute within vault)
		if strings.HasPrefix(targetPath, "/") {
			return strings.TrimPrefix(targetPath, "/")
		}
		// Has directory - add objects root if configured
		if objectsRoot != "" {
			return filepath.Join(objectsRoot, targetPath)
		}
		return targetPath
	}

	// No directory in target - look up type's default_path
	var defaultPath string
	if sch != nil {
		if typeDef, ok := sch.Types[typeName]; ok && typeDef != nil && typeDef.DefaultPath != "" {
			defaultPath = typeDef.DefaultPath
			// Security: validate default_path doesn't escape vault
			defaultPath = filepath.Clean(defaultPath)
			if strings.Contains(defaultPath, "..") || filepath.IsAbs(defaultPath) {
				defaultPath = ""
			}
		}
	}

	if defaultPath != "" {
		// Type has a default_path - nest under objects root
		if objectsRoot != "" {
			return filepath.Join(objectsRoot, defaultPath, targetPath)
		}
		return filepath.Join(defaultPath, targetPath)
	}

	// No default_path - use pages root for untyped pages, or objects root for typed objects
	if typeName == "" || typeName == "page" {
		if pagesRoot != "" {
			return filepath.Join(pagesRoot, targetPath)
		}
	} else {
		// Typed but no default_path - put in objects root
		if objectsRoot != "" {
			return filepath.Join(objectsRoot, targetPath)
		}
	}

	return targetPath
}

// ResolveTargetPath applies the type's default_path if the target doesn't already have a directory.
// This is exported for use by other packages that need to compute the resolved path
// (e.g., for display purposes) without creating the file.
func ResolveTargetPath(targetPath, typeName string, sch *schema.Schema) string {
	return resolveDefaultPath(targetPath, typeName, sch)
}

// ResolveTargetPathWithRoots applies directory roots and type default_path.
// This is the full resolution including configured directory organization.
func ResolveTargetPathWithRoots(targetPath, typeName string, sch *schema.Schema, objectsRoot, pagesRoot string) string {
	return resolveDefaultPathWithRoots(targetPath, typeName, sch, objectsRoot, pagesRoot)
}

// Exists checks if a page already exists at the given path.
// Note: This does NOT apply default_path resolution. Pass the already-resolved path
// or use ExistsWithSchema for type-aware checking.
func Exists(vaultPath, targetPath string) bool {
	slugifiedPath := SlugifyPath(targetPath)
	filePath := filepath.Join(vaultPath, slugifiedPath)
	if !strings.HasSuffix(filePath, ".md") {
		filePath += ".md"
	}
	_, err := os.Stat(filePath)
	return err == nil
}

// ExistsWithSchema checks if a page already exists, applying default_path resolution.
func ExistsWithSchema(vaultPath, targetPath, typeName string, sch *schema.Schema) bool {
	resolved := resolveDefaultPath(targetPath, typeName, sch)
	return Exists(vaultPath, resolved)
}

// SlugifyPath slugifies each component of a path.
// "people/Sif" -> "people/sif"
// Also handles embedded object IDs: "daily/2025-02-01#Team Sync" -> "daily/2025-02-01#team-sync"
func SlugifyPath(path string) string {
	return slugs.PathSlug(path)
}

// Slugify converts a string to a URL-safe slug.
func Slugify(s string) string {
	return slugs.ComponentSlug(s)
}

// CreateDailyNote creates a daily note for the given date.
func CreateDailyNote(vaultPath, dailyDir, dateStr, friendlyTitle string) (*CreateResult, error) {
	return CreateDailyNoteWithTemplate(vaultPath, dailyDir, dateStr, friendlyTitle, "")
}

// CreateDailyNoteWithTemplate creates a daily note with an optional template.
func CreateDailyNoteWithTemplate(vaultPath, dailyDir, dateStr, friendlyTitle, dailyTemplate string) (*CreateResult, error) {
	targetPath := filepath.Join(dailyDir, dateStr)

	return Create(CreateOptions{
		VaultPath:        vaultPath,
		TypeName:         "date",
		Title:            friendlyTitle,
		TargetPath:       targetPath,
		TemplateOverride: dailyTemplate,
	})
}
