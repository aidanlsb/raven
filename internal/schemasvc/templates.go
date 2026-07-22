package schemasvc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemadoc"
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
		return nil, newError(ErrorInvalidInput, "template_id cannot be empty", "", nil, nil)
	}

	sch, err := runtimeSchema(rt, "Run 'rvn init' to create a schema")
	if err != nil {
		return nil, err
	}

	templateDef, ok := sch.Templates[templateID]
	if !ok || templateDef == nil {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("template '%s' not found", templateID),
			"Use `rvn schema template list` to see available template IDs",
			nil,
			nil,
		)
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
		return nil, newError(ErrorInvalidInput, "template_id cannot be empty", "", nil, nil)
	}
	if strings.TrimSpace(req.File) == "" {
		return nil, newError(ErrorInvalidInput, "--file is required", "Use --file <path-under-directories.template>", nil, nil)
	}

	if rt == nil || rt.VaultCfg == nil {
		return nil, newError(ErrorConfigInvalid, "vault config runtime is required", "Fix raven.yaml and try again", nil, nil)
	}
	vaultPath := rt.VaultPath
	vaultCfg := rt.VaultCfg

	templateDir := vaultCfg.GetTemplateDirectory()
	fileRef, err := template.ResolveFileRef(req.File, templateDir)
	if err != nil {
		return nil, newError(ErrorInvalidInput, err.Error(), fmt.Sprintf("Use a file path under %s", templateDir), nil, err)
	}

	fullPath := filepath.Join(vaultPath, filepath.FromSlash(fileRef))
	if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
		return nil, newError(ErrorFileOutside, "template files must be within the vault", "Template files must be within the vault", nil, err)
	}
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return nil, newError(
			ErrorFileNotFound,
			fmt.Sprintf("template file not found: %s", fileRef),
			"Create the file first under directories.template (for example: templates/...)",
			nil,
			err,
		)
	} else if err != nil {
		return nil, newError(ErrorFileRead, "failed to read template file metadata", "", nil, err)
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, newError(ErrorFileRead, "failed to read template file", "", nil, err)
	}
	if err := template.ValidateContent(string(content)); err != nil {
		return nil, newError(ErrorValidation, err.Error(), "Template files should contain only body Markdown; Raven writes object frontmatter separately", nil, err)
	}

	description := strings.TrimSpace(req.Description)
	err = editSchemaWithLoadError(vaultPath, "", ErrorSchemaInvalid, func(doc *schemadoc.Document) error {
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
		return newError(ErrorInvalidInput, "template_id cannot be empty", "", nil, nil)
	}

	return editSchema(rt.VaultPath, "Run 'rvn init' to create a schema", func(doc *schemadoc.Document) error {
		sch := doc.Schema()
		if _, ok := sch.Templates[templateID]; !ok {
			return newError(ErrorInvalidInput, fmt.Sprintf("template '%s' not found", templateID), "Nothing to remove", nil, nil)
		}

		refs := templateRefs(sch, templateID)
		if len(refs) > 0 {
			return newError(
				ErrorInvalidInput,
				fmt.Sprintf("template '%s' is still referenced by: %s", templateID, strings.Join(refs, ", ")),
				"Remove those bindings first with `rvn schema template unbind <template_id> --type <type>` or `rvn schema template unbind <template_id> --core <core>`",
				nil,
				nil,
			)
		}

		templatesNode, ok := doc.Root()["templates"].(map[string]interface{})
		if !ok {
			return newError(ErrorInvalidInput, "schema has no templates section", "Nothing to remove", nil, nil)
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
