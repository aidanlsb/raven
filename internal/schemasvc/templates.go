package schemasvc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/template"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type TemplateDefinition struct {
	ID          string `json:"id"`
	File        string `json:"file"`
	Description string `json:"description,omitempty"`
}

type SetTemplateRequest struct {
	VaultPath   string
	TemplateID  string
	File        string
	Description string
}

func ListTemplates(rt *vaultruntime.Runtime) ([]TemplateDefinition, error) {
	sch, err := runtimeSchema(rt, "Run 'rvn init' to create a schema")
	if err != nil {
		return nil, err
	}

	items := make([]TemplateDefinition, 0, len(sch.Templates))
	for id, def := range sch.Templates {
		if def == nil {
			continue
		}
		items = append(items, TemplateDefinition{
			ID:          id,
			File:        def.File,
			Description: def.Description,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func GetTemplate(rt *vaultruntime.Runtime, templateID string) (*TemplateDefinition, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "template_id cannot be empty")
	}

	sch, err := runtimeSchema(rt, "Run 'rvn init' to create a schema")
	if err != nil {
		return nil, err
	}

	templateDef, ok := sch.Templates[templateID]
	if !ok || templateDef == nil {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("template '%s' not found", templateID)).WithSuggestion("Use `rvn schema template list` to see available template IDs")
	}

	return &TemplateDefinition{
		ID:          templateID,
		File:        templateDef.File,
		Description: templateDef.Description,
	}, nil
}

func SetTemplate(rt *vaultruntime.Runtime, req SetTemplateRequest) (*TemplateDefinition, error) {
	templateID := strings.TrimSpace(req.TemplateID)
	if templateID == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "template_id cannot be empty")
	}
	if strings.TrimSpace(req.File) == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "--file is required").WithSuggestion("Use --file <path-under-directories.template>")
	}

	if rt == nil || rt.VaultCfg == nil {
		return nil, svcerr.New(codes.ErrConfigInvalid, "vault config runtime is required").WithSuggestion("Fix raven.yaml and try again")
	}
	vaultPath := rt.VaultPath
	vaultCfg := rt.VaultCfg

	templateDir := vaultCfg.GetTemplateDirectory()
	fileRef, err := template.ResolveFileRef(req.File, templateDir)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err).WithSuggestion(fmt.Sprintf("Use a file path under %s", templateDir))
	}

	fullPath := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
	if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileOutsideVault, "template files must be within the vault", err).WithSuggestion("Template files must be within the vault")
	}
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return nil, svcerr.Wrap(codes.ErrFileNotFound, fmt.Sprintf("template file not found: %s", fileRef), err).WithSuggestion("Create the file first under directories.template (for example: templates/...)")
	} else if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to read template file metadata", err)
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to read template file", err)
	}
	if err := template.ValidateContent(string(content)); err != nil {
		return nil, svcerr.Wrap(codes.ErrValidationFailed, err.Error(), err).WithSuggestion("Template files should contain only body Markdown; Raven writes object frontmatter separately")
	}

	description := strings.TrimSpace(req.Description)
	err = editRuntimeSchemaWithLoadError(rt, "", codes.ErrSchemaInvalid, func(doc *schemadoc.Document) error {
		templatesNode := schemadoc.EnsureMap(doc.Root(), "templates")
		templateNode := schemadoc.EnsureMap(templatesNode, templateID)
		templateNode["file"] = fileRef

		if req.Description == "-" {
			delete(templateNode, "description")
			description = ""
		} else if description != "" {
			templateNode["description"] = description
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &TemplateDefinition{
		ID:          templateID,
		File:        fileRef,
		Description: description,
	}, nil
}

func RemoveTemplate(rt *vaultruntime.Runtime, templateID string) error {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return svcerr.New(codes.ErrInvalidInput, "template_id cannot be empty")
	}

	return editRuntimeSchema(rt, "Run 'rvn init' to create a schema", func(doc *schemadoc.Document) error {
		sch := doc.Schema()
		if _, ok := sch.Templates[templateID]; !ok {
			return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("template '%s' not found", templateID)).WithSuggestion("Nothing to remove")
		}

		refs := templateRefs(sch, templateID)
		if len(refs) > 0 {
			return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("template '%s' is still referenced by: %s", templateID, strings.Join(refs, ", "))).WithSuggestion("Remove those bindings first with `rvn schema template unbind <template_id> --type <type>` or `rvn schema template unbind <template_id> --core <core>`")
		}

		templatesNode, ok := doc.Root()["templates"].(map[string]interface{})
		if !ok {
			return svcerr.New(codes.ErrInvalidInput, "schema has no templates section").WithSuggestion("Nothing to remove")
		}
		delete(templatesNode, templateID)
		if len(templatesNode) == 0 {
			delete(doc.Root(), "templates")
		}
		return nil
	})
}

func templateRefs(sch *schema.Schema, templateID string) []string {
	refs := make(map[string]struct{})
	for typeName, typeDef := range sch.Types {
		if typeDef == nil {
			continue
		}
		if containsTemplateID(typeDef.Templates, templateID) {
			refs[typeName] = struct{}{}
		}
	}
	for coreType, coreDef := range sch.Core {
		if coreDef == nil {
			continue
		}
		if containsTemplateID(coreDef.Templates, templateID) {
			refs["core."+coreType] = struct{}{}
		}
	}
	out := make([]string, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}
