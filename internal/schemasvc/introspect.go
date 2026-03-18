package schemasvc

import (
	"fmt"
	"sort"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
)

type SchemaResult struct {
	Version   int                       `json:"version"`
	Types     map[string]TypeSchema     `json:"types"`
	Core      map[string]CoreTypeSchema `json:"core,omitempty"`
	Traits    map[string]TraitSchema    `json:"traits"`
	Templates map[string]TemplateSchema `json:"templates,omitempty"`
	Queries   map[string]SavedQueryInfo `json:"queries,omitempty"`
}

type TypeSchema struct {
	Name            string                 `json:"name"`
	Builtin         bool                   `json:"builtin"`
	DefaultPath     string                 `json:"default_path,omitempty"`
	Description     string                 `json:"description,omitempty"`
	NameField       string                 `json:"name_field,omitempty"`
	Template        string                 `json:"template,omitempty"`
	Templates       []string               `json:"templates,omitempty"`
	DefaultTemplate string                 `json:"default_template,omitempty"`
	Fields          map[string]FieldSchema `json:"fields,omitempty"`
}

type CoreTypeSchema struct {
	Name            string   `json:"name"`
	Templates       []string `json:"templates,omitempty"`
	DefaultTemplate string   `json:"default_template,omitempty"`
}

type TemplateSchema struct {
	ID          string `json:"id"`
	File        string `json:"file"`
	Description string `json:"description,omitempty"`
}

type FieldSchema struct {
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	Values      []string `json:"values,omitempty"`
	Target      string   `json:"target,omitempty"`
	Description string   `json:"description,omitempty"`
}

type TraitSchema struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Values  []string `json:"values,omitempty"`
	Default string   `json:"default,omitempty"`
}

type SavedQueryInfo struct {
	Name        string   `json:"name"`
	Query       string   `json:"query"`
	Args        []string `json:"args,omitempty"`
	Description string   `json:"description,omitempty"`
}

type TypesHint struct {
	Message               string   `json:"message"`
	TypesWithoutNameField []string `json:"types_without_name_field"`
	FixCommand            string   `json:"fix_command"`
}

type TypesResult struct {
	Types map[string]TypeSchema `json:"types"`
	Hint  *TypesHint            `json:"hint,omitempty"`
}

type TraitsResult struct {
	Traits map[string]TraitSchema `json:"traits"`
}

type CoreResult struct {
	Core map[string]CoreTypeSchema `json:"core"`
}

type TypeResult struct {
	Type TypeSchema `json:"type"`
}

type TraitResult struct {
	Trait TraitSchema `json:"trait"`
}

type CoreTypeResult struct {
	Core CoreTypeSchema `json:"core"`
}

type CommandsResult struct {
	Commands map[string]CommandSchema `json:"commands"`
}

type CommandSchema struct {
	Description string                `json:"description"`
	Args        []string              `json:"args,omitempty"`
	Flags       map[string]FlagSchema `json:"flags,omitempty"`
	Examples    []string              `json:"examples,omitempty"`
	UseCases    []string              `json:"use_cases,omitempty"`
}

type FlagSchema struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Examples    []string `json:"examples,omitempty"`
}

func FullSchema(vaultPath string) (*SchemaResult, error) {
	sch, err := loadSchema(vaultPath, "Run 'rvn init' to create a schema")
	if err != nil {
		return nil, err
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return nil, newError(ErrorConfigInvalid, fmt.Sprintf("failed to load raven.yaml: %v", err), "Fix raven.yaml and try again", nil, err)
	}

	result := &SchemaResult{
		Version: sch.Version,
		Types:   make(map[string]TypeSchema),
		Core:    make(map[string]CoreTypeSchema),
		Traits:  make(map[string]TraitSchema),
	}

	for name, typeDef := range sch.Types {
		result.Types[name] = buildTypeSchema(name, typeDef, false)
	}
	result.Types["page"] = TypeSchema{Name: "page", Builtin: true}
	result.Types["section"] = TypeSchema{Name: "section", Builtin: true}
	result.Types["date"] = TypeSchema{Name: "date", Builtin: true}

	result.Core["date"] = buildCoreTypeSchema("date", sch.Core["date"])
	result.Core["page"] = buildCoreTypeSchema("page", sch.Core["page"])
	result.Core["section"] = buildCoreTypeSchema("section", sch.Core["section"])

	for name, traitDef := range sch.Traits {
		result.Traits[name] = buildTraitSchema(name, traitDef)
	}

	if len(sch.Templates) > 0 {
		result.Templates = make(map[string]TemplateSchema, len(sch.Templates))
		for id, templateDef := range sch.Templates {
			if templateDef == nil {
				continue
			}
			result.Templates[id] = TemplateSchema{
				ID:          id,
				File:        templateDef.File,
				Description: templateDef.Description,
			}
		}
	}

	if vaultCfg != nil && len(vaultCfg.Queries) > 0 {
		result.Queries = make(map[string]SavedQueryInfo)
		for name, q := range vaultCfg.Queries {
			result.Queries[name] = SavedQueryInfo{
				Name:        name,
				Query:       q.Query,
				Args:        q.Args,
				Description: q.Description,
			}
		}
	}

	return result, nil
}

func Types(vaultPath string) (*TypesResult, error) {
	sch, err := loadSchema(vaultPath, "Run 'rvn init' to create a schema")
	if err != nil {
		return nil, err
	}

	types := make(map[string]TypeSchema)
	for name, typeDef := range sch.Types {
		types[name] = buildTypeSchema(name, typeDef, false)
	}
	types["page"] = TypeSchema{Name: "page", Builtin: true}
	types["section"] = TypeSchema{Name: "section", Builtin: true}
	types["date"] = TypeSchema{Name: "date", Builtin: true}

	out := &TypesResult{Types: types}

	var typesWithoutNameField []string
	for name, typeDef := range sch.Types {
		if typeDef == nil || typeDef.NameField != "" || schema.IsBuiltinType(name) {
			continue
		}
		for _, fieldDef := range typeDef.Fields {
			if fieldDef != nil && fieldDef.Required && fieldDef.Type == schema.FieldTypeString {
				typesWithoutNameField = append(typesWithoutNameField, name)
				break
			}
		}
	}
	if len(typesWithoutNameField) > 0 {
		sort.Strings(typesWithoutNameField)
		out.Hint = &TypesHint{
			Message:               "Some types have required string fields but no name_field configured. Setting name_field enables auto-population from the title argument in raven_new.",
			TypesWithoutNameField: typesWithoutNameField,
			FixCommand:            "rvn schema update type <type_name> --name-field <field_name>",
		}
	}

	return out, nil
}

func Traits(vaultPath string) (*TraitsResult, error) {
	sch, err := loadSchema(vaultPath, "Run 'rvn init' to create a schema")
	if err != nil {
		return nil, err
	}

	traits := make(map[string]TraitSchema)
	for name, traitDef := range sch.Traits {
		traits[name] = buildTraitSchema(name, traitDef)
	}

	return &TraitsResult{Traits: traits}, nil
}

func CoreList(vaultPath string) (*CoreResult, error) {
	sch, err := loadSchema(vaultPath, "Run 'rvn init' to create a schema")
	if err != nil {
		return nil, err
	}

	return &CoreResult{Core: map[string]CoreTypeSchema{
		"date":    buildCoreTypeSchema("date", sch.Core["date"]),
		"page":    buildCoreTypeSchema("page", sch.Core["page"]),
		"section": buildCoreTypeSchema("section", sch.Core["section"]),
	}}, nil
}

func CoreByName(vaultPath, coreTypeName string) (*CoreTypeResult, error) {
	sch, err := loadSchema(vaultPath, "Run 'rvn init' to create a schema")
	if err != nil {
		return nil, err
	}
	if !schema.IsBuiltinType(coreTypeName) {
		return nil, newError(ErrorTypeNotFound, fmt.Sprintf("core type '%s' not found", coreTypeName), "Available core types: date, page, section", nil, nil)
	}
	return &CoreTypeResult{Core: buildCoreTypeSchema(coreTypeName, sch.Core[coreTypeName])}, nil
}

func TypeByName(vaultPath, typeName string) (*TypeResult, error) {
	sch, err := loadSchema(vaultPath, "Run 'rvn init' to create a schema")
	if err != nil {
		return nil, err
	}

	if schema.IsBuiltinType(typeName) {
		return &TypeResult{Type: TypeSchema{Name: typeName, Builtin: true}}, nil
	}

	typeDef, ok := sch.Types[typeName]
	if !ok {
		return nil, newError(ErrorTypeNotFound, fmt.Sprintf("type '%s' not found", typeName), "Run 'rvn schema types' to see available types", nil, nil)
	}

	return &TypeResult{Type: buildTypeSchema(typeName, typeDef, false)}, nil
}

func TraitByName(vaultPath, traitName string) (*TraitResult, error) {
	sch, err := loadSchema(vaultPath, "Run 'rvn init' to create a schema")
	if err != nil {
		return nil, err
	}

	traitDef, ok := sch.Traits[traitName]
	if !ok {
		return nil, newError(ErrorTraitNotFound, fmt.Sprintf("trait '%s' not found", traitName), "Run 'rvn schema traits' to see available traits", nil, nil)
	}

	return &TraitResult{Trait: buildTraitSchema(traitName, traitDef)}, nil
}

func buildTypeSchema(name string, typeDef *schema.TypeDefinition, builtin bool) TypeSchema {
	result := TypeSchema{Name: name, Builtin: builtin}
	if typeDef == nil {
		return result
	}

	result.DefaultPath = typeDef.DefaultPath
	result.Description = typeDef.Description
	result.NameField = typeDef.NameField
	result.Template = typeDef.Template
	result.Templates = append([]string(nil), typeDef.Templates...)
	result.DefaultTemplate = typeDef.DefaultTemplate

	if len(typeDef.Fields) > 0 {
		result.Fields = make(map[string]FieldSchema)
		for fieldName, fieldDef := range typeDef.Fields {
			if fieldDef == nil {
				continue
			}
			defaultStr := ""
			if fieldDef.Default != nil {
				defaultStr = fmt.Sprintf("%v", fieldDef.Default)
			}
			result.Fields[fieldName] = FieldSchema{
				Type:        string(fieldDef.Type),
				Required:    fieldDef.Required,
				Default:     defaultStr,
				Values:      fieldDef.Values,
				Target:      fieldDef.Target,
				Description: fieldDef.Description,
			}
		}
	}

	return result
}

func buildCoreTypeSchema(name string, coreDef *schema.CoreTypeDefinition) CoreTypeSchema {
	result := CoreTypeSchema{Name: name}
	if coreDef == nil {
		return result
	}
	result.Templates = append([]string(nil), coreDef.Templates...)
	result.DefaultTemplate = coreDef.DefaultTemplate
	return result
}

func buildTraitSchema(name string, traitDef *schema.TraitDefinition) TraitSchema {
	result := TraitSchema{Name: name}
	if traitDef == nil {
		return result
	}
	result.Type = string(traitDef.Type)
	result.Values = traitDef.Values
	if traitDef.Default != nil {
		result.Default = fmt.Sprintf("%v", traitDef.Default)
	}
	return result
}
