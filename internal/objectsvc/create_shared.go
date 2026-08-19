package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parseopts"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/refresolve"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/slugs"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// deriveCreateTargetPath computes the path seed used to build a new object's
// filename/slug and (path-derived) object ID.
//
// An explicit target path (from --path) is honored verbatim so its "/" segments
// map to directories. When no explicit path is given, the title is treated purely
// as a display name and slugified into a single filename component. This lets
// titles contain "/" (and other unsafe path characters) without being interpreted
// as directory separators, while the display title is still persisted unchanged in
// frontmatter via name_field.
func deriveCreateTargetPath(targetPath, title string) string {
	if strings.TrimSpace(targetPath) != "" {
		return targetPath
	}
	return slugs.ComponentSlug(title)
}

type requiredFieldGap struct {
	Name   string
	Type   schema.FieldType
	Values []string
}

type createPageRequest struct {
	VaultPath        string
	TypeName         string
	Title            string
	TargetPath       string
	Fields           map[string]fieldvalue.FieldValue
	Schema           *schema.Schema
	TemplateOverride string
	TemplateDir      string
	VaultConfig      *config.VaultConfig
	ObjectsRoot      string
	PagesRoot        string
}

func lookupTypeDefinitionForCreate(sch *schema.Schema, typeName string) (*schema.TypeDefinition, error) {
	typeDef, typeExists := sch.Types[typeName]
	if typeExists || schema.IsBuiltinType(typeName) {
		return typeDef, nil
	}

	typeNames := make([]string, 0, len(sch.Types))
	for name := range sch.Types {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)

	return nil, svcerr.New(codes.ErrTypeNotFound, fmt.Sprintf("type '%s' not found", typeName)).WithSuggestion(fmt.Sprintf("Available types: %s", strings.Join(typeNames, ", "))).WithDetails(map[string]interface{}{"available_types": typeNames})
}

func normalizedCreateFieldValues(values map[string]fieldvalue.FieldValue, typeDef *schema.TypeDefinition, title string) map[string]fieldvalue.FieldValue {
	fieldValues := cloneFieldValues(values)
	ensureNameFieldValue(fieldValues, typeDef, title)
	return fieldValues
}

func requiredFieldGaps(typeDef *schema.TypeDefinition, fields map[string]fieldvalue.FieldValue) []requiredFieldGap {
	if typeDef == nil {
		return nil
	}

	fieldNames := make([]string, 0, len(typeDef.Fields))
	for fieldName := range typeDef.Fields {
		fieldNames = append(fieldNames, fieldName)
	}
	sort.Strings(fieldNames)

	missing := make([]requiredFieldGap, 0)
	for _, fieldName := range fieldNames {
		fieldDef := typeDef.Fields[fieldName]
		if fieldDef == nil || !fieldDef.Required {
			continue
		}
		if _, ok := fields[fieldName]; ok {
			continue
		}
		if fieldDef.Default != nil {
			fields[fieldName] = parser.FieldValueFromYAML(fieldDef.Default)
			continue
		}

		gap := requiredFieldGap{
			Name: fieldName,
			Type: fieldDef.Type,
		}
		if len(fieldDef.Values) > 0 {
			gap.Values = append([]string(nil), fieldDef.Values...)
		}
		missing = append(missing, gap)
	}

	return missing
}

func requiredFieldGapNames(gaps []requiredFieldGap) []string {
	names := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		names = append(names, gap.Name)
	}
	return names
}

func requiredFieldGapDetails(gaps []requiredFieldGap) []map[string]interface{} {
	details := make([]map[string]interface{}, 0, len(gaps))
	for _, gap := range gaps {
		detail := map[string]interface{}{
			"name":     gap.Name,
			"type":     string(gap.Type),
			"required": true,
		}
		if len(gap.Values) > 0 {
			detail["values"] = gap.Values
		}
		details = append(details, detail)
	}
	return details
}

func createRefValidationContext(
	rt *vaultruntime.Runtime,
	parseOptions *parser.ParseOptions,
) *fieldmutation.RefValidationContext {
	if rt == nil {
		return nil
	}
	if parseOptions == nil {
		parseOptions = rt.ParseOptions
	}
	if parseOptions == nil {
		parseOptions = parseopts.FromVaultConfig(rt.VaultCfg)
	}

	return &fieldmutation.RefValidationContext{
		Prepare: func() error {
			if err := rt.OpenDB(); errors.Is(err, index.ErrIndexRebuildRequired) {
				return fieldmutation.ErrRefValidationIndexRebuildRequired
			}
			return nil
		},
		ResolveTargetType: func(rawReference string) (string, error) {
			return resolveReferenceType(rt, parseOptions, rawReference)
		},
	}
}

func resolveReferenceType(rt *vaultruntime.Runtime, parseOptions *parser.ParseOptions, rawReference string) (string, error) {
	resolved, err := refresolve.Resolve(rawReference, rt, false)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(resolved.FilePath)
	if err != nil {
		return "", err
	}

	doc, err := parser.ParseDocumentWithOptions(string(content), resolved.FilePath, rt.VaultPath, parseOptions)
	if err != nil {
		return "", err
	}

	for _, obj := range doc.Objects {
		if obj.ID == resolved.ObjectID {
			return obj.Type, nil
		}
	}

	return "", fmt.Errorf("resolved object %q not found in parsed document", resolved.ObjectID)
}

func validateCreateFieldValues(
	typeName string,
	fields map[string]fieldvalue.FieldValue,
	sch *schema.Schema,
	allowedUnknown map[string]bool,
	refCtx *fieldmutation.RefValidationContext,
) (map[string]fieldvalue.FieldValue, []string, error) {
	return fieldmutation.PrepareValidatedFieldMutationValues(typeName, nil, fields, sch, allowedUnknown, refCtx)
}

func createObjectPage(req createPageRequest) (*pages.CreateResult, error) {
	result, err := pages.Create(pages.CreateOptions{
		VaultPath:         req.VaultPath,
		TypeName:          req.TypeName,
		Title:             req.Title,
		TargetPath:        req.TargetPath,
		Fields:            req.Fields,
		Schema:            req.Schema,
		TemplateOverride:  req.TemplateOverride,
		TemplateDir:       req.TemplateDir,
		ProtectedPrefixes: protectedPrefixes(req.VaultConfig),
		ObjectsRoot:       req.ObjectsRoot,
		PagesRoot:         req.PagesRoot,
	})
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create object", err)
	}
	return result, nil
}

func protectedPrefixes(vaultCfg *config.VaultConfig) []string {
	if vaultCfg == nil {
		return nil
	}
	return vaultCfg.ProtectedPrefixes
}
