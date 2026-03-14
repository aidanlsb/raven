package checksvc

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
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

func CreateMissingPage(vaultPath string, sch *schema.Schema, targetPath, typeName, objectsRoot, pagesRoot, templateDir string) error {
	_, err := pages.Create(pages.CreateOptions{
		VaultPath:                   vaultPath,
		TypeName:                    typeName,
		TargetPath:                  targetPath,
		Schema:                      sch,
		IncludeRequiredPlaceholders: true,
		TemplateDir:                 templateDir,
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
	schemaPath := paths.SchemaPath(vaultPath)

	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema: %w", err)
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	traits, ok := schemaDoc["traits"].(map[string]interface{})
	if !ok {
		traits = make(map[string]interface{})
		schemaDoc["traits"] = traits
	}

	newTrait := make(map[string]interface{})
	newTrait["type"] = traitType

	if len(enumValues) > 0 {
		newTrait["values"] = enumValues
	}

	if defaultValue != "" {
		if traitType == "boolean" || traitType == "bool" {
			if defaultValue == "true" {
				newTrait["default"] = true
			} else if defaultValue == "false" {
				newTrait["default"] = false
			} else {
				newTrait["default"] = defaultValue
			}
		} else {
			newTrait["default"] = defaultValue
		}
	}

	traits[traitName] = newTrait

	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
		return fmt.Errorf("failed to write schema: %w", err)
	}

	trimmedValues := make([]string, 0, len(enumValues))
	for _, value := range enumValues {
		v := strings.TrimSpace(value)
		if v != "" {
			trimmedValues = append(trimmedValues, v)
		}
	}

	s.Traits[traitName] = &schema.TraitDefinition{
		Type:   schema.FieldType(traitType),
		Values: trimmedValues,
	}
	if defaultValue != "" {
		if traitType == "boolean" || traitType == "bool" {
			s.Traits[traitName].Default = defaultValue == "true"
		} else {
			s.Traits[traitName].Default = defaultValue
		}
	}

	return nil
}

func AddType(vaultPath string, s *schema.Schema, typeName, defaultPath string) error {
	schemaPath := paths.SchemaPath(vaultPath)

	if schema.IsBuiltinType(typeName) {
		return fmt.Errorf("'%s' is a built-in type", typeName)
	}

	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("failed to read schema: %w", err)
	}

	var schemaDoc map[string]interface{}
	if err := yaml.Unmarshal(data, &schemaDoc); err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	types, ok := schemaDoc["types"].(map[string]interface{})
	if !ok {
		types = make(map[string]interface{})
		schemaDoc["types"] = types
	}

	newType := make(map[string]interface{})
	if defaultPath != "" {
		newType["default_path"] = defaultPath
	}

	types[typeName] = newType

	output, err := yaml.Marshal(schemaDoc)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	if err := atomicfile.WriteFile(schemaPath, output, 0o644); err != nil {
		return fmt.Errorf("failed to write schema: %w", err)
	}

	s.Types[typeName] = &schema.TypeDefinition{
		DefaultPath: defaultPath,
	}

	return nil
}
