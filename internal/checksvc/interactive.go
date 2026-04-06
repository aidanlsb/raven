package checksvc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemasvc"
)

type MissingRefGroups struct {
	Certain  []*check.MissingRef
	Inferred []*check.MissingRef
	Unknown  []*check.MissingRef
}

// GroupMissingRefsForInteractive buckets and sorts missing refs for deterministic prompt order.
func GroupMissingRefsForInteractive(refs []*check.MissingRef) MissingRefGroups {
	groups := MissingRefGroups{}
	for _, ref := range refs {
		switch ref.Confidence {
		case check.ConfidenceCertain:
			groups.Certain = append(groups.Certain, ref)
		case check.ConfidenceInferred:
			groups.Inferred = append(groups.Inferred, ref)
		default:
			groups.Unknown = append(groups.Unknown, ref)
		}
	}

	sortRefs := func(items []*check.MissingRef) {
		sort.Slice(items, func(i, j int) bool {
			return items[i].TargetPath < items[j].TargetPath
		})
	}
	sortRefs(groups.Certain)
	sortRefs(groups.Inferred)
	sortRefs(groups.Unknown)

	return groups
}

func ResolveAndSlugifyTargetPath(targetPath, typeName string, sch *schema.Schema, objectsRoot, pagesRoot string) string {
	resolvedPath := pages.ResolveTargetPathWithRoots(targetPath, typeName, sch, objectsRoot, pagesRoot)
	return pages.SlugifyPath(resolvedPath)
}

func CreateMissingPage(vaultPath string, sch *schema.Schema, targetPath, typeName, objectsRoot, pagesRoot, templateDir string, protectedPrefixes []string) error {
	_, err := pages.Create(pages.CreateOptions{
		VaultPath:                   vaultPath,
		TypeName:                    typeName,
		TargetPath:                  targetPath,
		Schema:                      sch,
		IncludeRequiredPlaceholders: true,
		TemplateDir:                 templateDir,
		ProtectedPrefixes:           protectedPrefixes,
		ObjectsRoot:                 objectsRoot,
		PagesRoot:                   pagesRoot,
	})
	return err
}

func AvailableTypeNames(s *schema.Schema) []string {
	var typeNames []string
	for name := range s.Types {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)
	return typeNames
}

func AddTrait(vaultPath string, s *schema.Schema, traitName, traitType string, enumValues []string, defaultValue string) error {
	trimmedValues := make([]string, 0, len(enumValues))
	for _, value := range enumValues {
		v := strings.TrimSpace(value)
		if v != "" {
			trimmedValues = append(trimmedValues, v)
		}
	}

	_, err := schemasvc.AddTrait(schemasvc.AddTraitRequest{
		VaultPath: vaultPath,
		TraitName: traitName,
		TraitType: traitType,
		Values:    strings.Join(trimmedValues, ","),
		Default:   strings.TrimSpace(defaultValue),
	})
	if err != nil {
		return err
	}

	loaded, err := schema.Load(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to reload schema after adding trait: %w", err)
	}
	traitDef, ok := loaded.Traits[traitName]
	if !ok {
		return fmt.Errorf("added trait '%s' was not found after reload", traitName)
	}

	if s.Traits == nil {
		s.Traits = make(map[string]*schema.TraitDefinition)
	}
	s.Traits[traitName] = traitDef

	return nil
}

func AddType(vaultPath string, s *schema.Schema, typeName, defaultPath string) error {
	_, err := schemasvc.AddType(schemasvc.AddTypeRequest{
		VaultPath:   vaultPath,
		TypeName:    typeName,
		DefaultPath: strings.TrimSpace(defaultPath),
	})
	if err != nil {
		return err
	}

	loaded, err := schema.Load(vaultPath)
	if err != nil {
		return fmt.Errorf("failed to reload schema after adding type: %w", err)
	}
	typeDef, ok := loaded.Types[typeName]
	if !ok {
		return fmt.Errorf("added type '%s' was not found after reload", typeName)
	}

	if s.Types == nil {
		s.Types = make(map[string]*schema.TypeDefinition)
	}
	s.Types[typeName] = typeDef

	return nil
}
