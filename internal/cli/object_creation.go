package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/schema"
)

// objectCreationContext centralizes path resolution and object creation behavior.
// All CLI entry points that create new objects should use this helper.
type objectCreationContext struct {
	vaultPath   string
	schema      *schema.Schema
	objectsRoot string
	pagesRoot   string
	templateDir string
}

type objectCreateParams struct {
	typeName                    string
	title                       string
	targetPath                  string
	fields                      map[string]string
	includeRequiredPlaceholders bool
	templateOverride            string
}

func validateObjectTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("title cannot be empty")
	}
	if strings.ContainsAny(title, "/\\") {
		return fmt.Errorf("title cannot contain path separators")
	}
	return nil
}

func validateObjectTargetPath(targetPath string) error {
	normalized := strings.TrimSpace(targetPath)
	if normalized == "" {
		return fmt.Errorf("path cannot be empty")
	}
	normalized = strings.ReplaceAll(filepath.ToSlash(normalized), "\\", "/")
	if strings.HasSuffix(normalized, "/") {
		return fmt.Errorf("path must include a filename, not just a directory")
	}

	base := strings.TrimSuffix(filepath.Base(normalized), ".md")
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == ".." {
		return fmt.Errorf("path must include a valid filename")
	}

	return nil
}

func newObjectCreationContext(vaultPath string, sch *schema.Schema, objectsRoot, pagesRoot, templateDir string) objectCreationContext {
	return objectCreationContext{
		vaultPath:   vaultPath,
		schema:      sch,
		objectsRoot: objectsRoot,
		pagesRoot:   pagesRoot,
		templateDir: templateDir,
	}
}

func (c objectCreationContext) resolveTargetPath(targetPath, typeName string) string {
	return pages.ResolveTargetPathWithRoots(targetPath, typeName, c.schema, c.objectsRoot, c.pagesRoot)
}

func (c objectCreationContext) resolveAndSlugifyTargetPath(targetPath, typeName string) string {
	return pages.SlugifyPath(c.resolveTargetPath(targetPath, typeName))
}

func (c objectCreationContext) exists(targetPath, typeName string) bool {
	return pages.Exists(c.vaultPath, c.resolveTargetPath(targetPath, typeName))
}

func (c objectCreationContext) create(params objectCreateParams) (*pages.CreateResult, error) {
	return pages.Create(pages.CreateOptions{
		VaultPath:                   c.vaultPath,
		TypeName:                    params.typeName,
		Title:                       params.title,
		TargetPath:                  params.targetPath,
		Fields:                      params.fields,
		Schema:                      c.schema,
		IncludeRequiredPlaceholders: params.includeRequiredPlaceholders,
		TemplateOverride:            params.templateOverride,
		TemplateDir:                 c.templateDir,
		ObjectsRoot:                 c.objectsRoot,
		PagesRoot:                   c.pagesRoot,
	})
}
