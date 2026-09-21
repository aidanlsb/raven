package schemamigratesvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/schemasvc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
)

func buildTypeRenamePlan(
	vaultPath string,
	schemaDoc *schemadoc.Document,
	description, oldName, newName string,
	oldTypeDef *schema.TypeDefinition,
	vaultCfg *config.VaultConfig,
) (*typeRenamePlan, error) {
	oldDefaultPath := ""
	if oldTypeDef != nil {
		oldDefaultPath = oldTypeDef.DefaultPath
	}
	schemaPlan, err := schemasvc.BuildTypeRenamePlan(schemasvc.TypeRenamePlanRequest{
		SchemaDoc:      schemaDoc,
		OldName:        oldName,
		NewName:        newName,
		Description:    description,
		OldDefaultPath: oldDefaultPath,
	})
	if err != nil {
		return nil, err
	}

	plan := &typeRenamePlan{
		SchemaPlan:      schemaPlan,
		MarkdownFiles:   make(map[string][]byte),
		ReferenceFiles:  make(map[string][]byte),
		Changes:         append([]schemasvc.SchemaChange(nil), schemaPlan.Changes...),
		OptionalChanges: append([]schemasvc.SchemaChange(nil), schemaPlan.OptionalChanges...),
	}
	if schemaPlan.DefaultPathOld != "" {
		plan.DefaultPathPlan = &typeDefaultPathRenamePlan{
			OldDefaultPath: schemaPlan.DefaultPathOld,
			NewDefaultPath: schemaPlan.DefaultPathNew,
		}
	}

	movesBySource := make(map[string]typeDirectoryMove)
	err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Error != nil {
			return result.Error
		}
		if result.Document == nil {
			return nil
		}

		hasFileLevelOldType := false
		for _, obj := range result.Document.Objects {
			if obj.Type == oldName && !strings.Contains(obj.ID, "#") {
				hasFileLevelOldType = true
				break
			}
		}
		if !hasFileLevelOldType {
			return nil
		}

		if staged, ok := stageFrontmatterTypeRewrite(result.Document.RawContent, newName); ok {
			plan.MarkdownFiles[result.Path] = staged
			plan.Changes = append(plan.Changes, schemasvc.SchemaChange{
				FilePath:    result.RelativePath,
				ChangeType:  "frontmatter",
				Description: fmt.Sprintf("change type: %s → type: %s", oldName, newName),
				Line:        1,
			})
		}

		if plan.DefaultPathPlan != nil {
			if move, ok := planTypeDirectoryMove(result.RelativePath, newName, plan.DefaultPathPlan, vaultCfg); ok {
				if _, exists := movesBySource[move.SourceRelPath]; !exists {
					movesBySource[move.SourceRelPath] = move
					plan.DefaultPathPlan.Moves = append(plan.DefaultPathPlan.Moves, move)
					plan.OptionalChanges = append(plan.OptionalChanges, schemasvc.SchemaChange{
						FilePath:    move.SourceRelPath,
						ChangeType:  "directory_move",
						Description: fmt.Sprintf("move file '%s' → '%s'", move.SourceRelPath, move.DestinationRelPath),
					})
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInternal, err.Error(), err)
	}

	if plan.DefaultPathPlan != nil && len(plan.DefaultPathPlan.Moves) > 0 {
		if err := plan.stageReferenceUpdates(vaultPath, vaultCfg); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

func buildFieldRenamePlan(
	vaultPath string,
	schemaDoc *schemadoc.Document,
	typeName, oldField, newField string,
	vaultCfg *config.VaultConfig,
) (*fieldRenamePlan, error) {
	tokenOld := "{{field." + oldField + "}}"
	tokenNew := "{{field." + newField + "}}"

	schemaPlan, err := schemasvc.BuildFieldRenamePlan(schemasvc.FieldRenamePlanRequest{
		SchemaDoc: schemaDoc,
		TypeName:  typeName,
		OldField:  oldField,
		NewField:  newField,
	})
	if err != nil {
		return nil, err
	}

	plan := &fieldRenamePlan{
		SchemaYAML:    schemaPlan.SchemaYAML,
		TemplateFiles: make(map[string][]byte),
		MarkdownFiles: make(map[string][]byte),
		Changes:       append([]schemasvc.SchemaChange(nil), schemaPlan.Changes...),
		Conflicts:     make([]FieldRenameConflict, 0),
	}

	if schemaPlan.TemplateSpec != "" && looksLikeTemplatePath(schemaPlan.TemplateSpec) {
		absTemplate := filepath.Join(vaultPath, schemaPlan.TemplateSpec)
		if err := paths.ValidateWithinVault(vaultPath, absTemplate); err != nil {
			if !errors.Is(err, paths.ErrPathOutsideVault) {
				return nil, svcerr.Wrap(codes.ErrFileOutsideVault, err.Error(), err)
			}
		} else {
			templateContent, err := os.ReadFile(absTemplate)
			if err == nil {
				newContent := strings.ReplaceAll(string(templateContent), tokenOld, tokenNew)
				if newContent != string(templateContent) {
					plan.TemplateFiles[absTemplate] = []byte(newContent)
					rel, _ := paths.RelFromVault(vaultPath, absTemplate)
					plan.Changes = append(plan.Changes, schemasvc.SchemaChange{
						FilePath:    rel,
						ChangeType:  "template_file",
						Description: fmt.Sprintf("update template variable %s → %s", tokenOld, tokenNew),
					})
				}
			}
		}
	}

	changedQueries := false
	fieldRefPattern := regexp.MustCompile(`\.` + regexp.QuoteMeta(oldField) + `\b`)
	if vaultCfg != nil && vaultCfg.Queries != nil {
		for _, name := range schemasvc.SortedKeys(vaultCfg.Queries) {
			savedQuery := vaultCfg.Queries[name]
			if savedQuery == nil || savedQuery.Query == "" {
				continue
			}
			parsed, err := query.Parse(savedQuery.Query)
			if err != nil || parsed == nil {
				continue
			}
			if parsed.Type != query.QueryTypeObject || parsed.TypeName != typeName {
				continue
			}
			newQuery := fieldRefPattern.ReplaceAllString(savedQuery.Query, "."+newField)
			if newQuery != savedQuery.Query {
				savedQuery.Query = newQuery
				changedQueries = true
				plan.Changes = append(plan.Changes, schemasvc.SchemaChange{
					FilePath:    "raven.yaml",
					ChangeType:  "saved_query",
					Description: fmt.Sprintf("update saved query '%s': .%s → .%s", name, oldField, newField),
				})
			}
		}
	}
	if changedQueries {
		configOut, err := yaml.Marshal(vaultCfg)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrInternal, err.Error(), err)
		}
		plan.RavenYAML = configOut
	}

	err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
		if result.Error != nil {
			return result.Error
		}
		if result.Document == nil {
			return nil
		}

		original := result.Document.RawContent
		lines := strings.Split(original, "\n")
		startLine, endLine, frontmatterOK := parser.FrontmatterBounds(lines)
		if !frontmatterOK || endLine == -1 {
			return nil
		}

		frontmatterContent := strings.Join(lines[startLine+1:endLine], "\n")
		frontmatter, ok := decodeYAMLMap([]byte(frontmatterContent))
		if !ok {
			return nil
		}
		if objectType, ok := frontmatter["type"].(string); !ok || objectType != typeName {
			return nil
		}

		_, oldPresent := frontmatter[oldField]
		_, newPresent := frontmatter[newField]
		if oldPresent && newPresent {
			plan.Conflicts = append(plan.Conflicts, FieldRenameConflict{
				FilePath:      result.RelativePath,
				ConflictType:  "frontmatter",
				Message:       fmt.Sprintf("frontmatter contains both '%s' and '%s'", oldField, newField),
				Line:          1,
				OldFieldFound: true,
				NewFieldFound: true,
			})
			return nil
		}
		if !oldPresent {
			return nil
		}

		frontmatter[newField] = frontmatter[oldField]
		delete(frontmatter, oldField)
		newFrontmatter, ok := marshalYAMLMap(frontmatter)
		if !ok {
			return nil
		}

		var output strings.Builder
		output.WriteString("---\n")
		output.Write(newFrontmatter)
		output.WriteString("---")
		if endLine+1 < len(lines) {
			output.WriteString("\n")
			output.WriteString(strings.Join(lines[endLine+1:], "\n"))
		}

		plan.MarkdownFiles[result.Path] = []byte(output.String())
		plan.Changes = append(plan.Changes, schemasvc.SchemaChange{
			FilePath:    result.RelativePath,
			ChangeType:  "frontmatter",
			Description: fmt.Sprintf("rename frontmatter key '%s:' → '%s:' for type '%s'", oldField, newField, typeName),
			Line:        1,
		})
		return nil
	})
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInternal, err.Error(), err)
	}

	return plan, nil
}
