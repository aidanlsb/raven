package cli

import (
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
