package query

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
)

// ValidationError represents a query validation error.
type ValidationError struct {
	Message    string
	Suggestion string
}

func (e *ValidationError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("%s. %s", e.Message, e.Suggestion)
	}
	return e.Message
}

// Validator validates queries against a schema.
//
// Validation has two layers:
//
//   - Structural validation is always performed, even when no schema is
//     available: the root/predicate legality matrix (capabilities.go), trait
//     ".value" restrictions, section/link built-in field names, regex
//     validity, and empty content terms. These depend only on the query shape.
//   - Schema-dependent validation (unknown types/traits, unknown or wrongly
//     typed object/trait fields) only runs when a schema is present.
//
// This split lets callers enforce structural legality even before a schema is
// loaded, while still surfacing rich schema diagnostics when one is available.
type Validator struct {
	schema *schema.Schema
}

// NewValidator creates a new query validator. A nil schema is permitted; in
// that case only structural validation is performed.
func NewValidator(sch *schema.Schema) *Validator {
	return &Validator{schema: sch}
}

func (v *Validator) hasSchema() bool { return v.schema != nil }

// Validate checks a parsed query. Structural rules always run; schema-dependent
// rules run only when the validator has a schema.
func (v *Validator) Validate(q *Query) error {
	return v.validateQueryCtx(q, nil)
}

func (v *Validator) validateOrderBy(q *Query, typeDef *schema.TypeDefinition) error {
	if len(q.OrderBy) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(q.OrderBy))
	for _, sf := range q.OrderBy {
		if _, dup := seen[sf.Field]; dup {
			return &ValidationError{Message: fmt.Sprintf("duplicate sort field '.%s'", sf.Field)}
		}
		seen[sf.Field] = struct{}{}

		switch q.Type {
		case QueryTypeObject:
			if err := v.validateObjectSortField(q.TypeName, typeDef, sf); err != nil {
				return err
			}
		case QueryTypeTrait:
			if err := v.validateTraitSortField(q.TypeName, sf); err != nil {
				return err
			}
		case QueryTypeSection:
			if _, ok := sectionFieldColumn("s", sf.Field); !ok {
				return &ValidationError{
					Message:    fmt.Sprintf("section has no field '%s'", sf.Field),
					Suggestion: fmt.Sprintf("Available section fields: %s", strings.Join(availableSectionFields(), ", ")),
				}
			}
		case QueryTypeLink:
			if _, ok := linkColumnExpr("l", sf.Field); !ok {
				return unknownLinkFieldError(sf.Field)
			}
		}
	}
	return nil
}

func (v *Validator) validateObjectSortField(typeName string, typeDef *schema.TypeDefinition, sf SortField) error {
	if isDateVirtualField(typeName, sf.Field) {
		return nil
	}
	if !v.hasSchema() {
		return nil
	}
	fieldDef, err := v.fieldDefinitionForType(typeName, typeDef, sf.Field)
	if err != nil {
		return err
	}
	if fieldDef.Type.IsArray() {
		return &ValidationError{
			Message:    fmt.Sprintf("cannot sort by array field '.%s' of type %s", sf.Field, fieldDef.Type),
			Suggestion: "Sort by a scalar field instead",
		}
	}
	return nil
}

func (v *Validator) validateTraitSortField(traitName string, sf SortField) error {
	if sf.Field != "value" {
		return &ValidationError{
			Message:    fmt.Sprintf("sort() on trait queries only supports .value, got .%s", sf.Field),
			Suggestion: "Use sort(.value) or sort(.value, desc)",
		}
	}
	if !v.hasSchema() {
		return nil
	}
	traitDef := v.schema.Traits[traitName]
	if traitDef != nil && traitDef.Type.IsArray() {
		return &ValidationError{
			Message: fmt.Sprintf("cannot sort by array-valued trait '%s'", traitName),
		}
	}
	return nil
}

func validateContentTerm(p *ContentPredicate) error {
	if p.SearchTerm == "" {
		return &ValidationError{
			Message:    "content search term cannot be empty",
			Suggestion: `Provide a search term: content("search terms")`,
		}
	}
	return nil
}

func validateNumericFieldPredicate(rootName string, p *FieldPredicate) error {
	if p == nil || p.IsExists || p.rhs().Path != nil {
		return nil
	}
	if _, err := strconv.ParseFloat(strings.TrimSpace(p.Value), 64); err != nil {
		return &ValidationError{
			Message:    fmt.Sprintf("%s field '.%s' requires a numeric value, got '%s'", rootName, p.Field, p.Value),
			Suggestion: fmt.Sprintf("Use a numeric literal, for example: .%s==1", p.Field),
		}
	}
	return nil
}

func (v *Validator) validateScopeSubquery(root QueryType, kind string, sub *Query, outer *validateCtx) error {
	if sub == nil {
		return nil
	}
	switch sub.Type {
	case QueryTypeObject, QueryTypeSection:
		return v.validateQueryCtx(sub, outer)
	case QueryTypeTrait:
		if root == QueryTypeLink && kind == "within" {
			return &ValidationError{
				Message:    fmt.Sprintf("within(trait:%s) is not valid for link queries", sub.TypeName),
				Suggestion: fmt.Sprintf("Use trait:%s links(...) to filter trait lines by outgoing links", sub.TypeName),
			}
		}
		return &ValidationError{
			Message:    fmt.Sprintf("%s() expects a type or section query, not trait:%s", kind, sub.TypeName),
			Suggestion: fmt.Sprintf("Use %s(type:...) or %s(section ...)", kind, kind),
		}
	case QueryTypeLink:
		return &ValidationError{
			Message:    fmt.Sprintf("link queries cannot be %s() targets", kind),
			Suggestion: "Link rows are outgoing edges, not Raven scopes; use a type or section query as the scope target",
		}
	default:
		return &ValidationError{Message: fmt.Sprintf("%s() expects a type or section query", kind)}
	}
}

func (v *Validator) validateRefdSubquery(sub *Query, outer *validateCtx) error {
	if sub == nil {
		return nil
	}
	switch sub.Type {
	case QueryTypeObject, QueryTypeTrait, QueryTypeSection:
		return v.validateQueryCtx(sub, outer)
	case QueryTypeLink:
		return &ValidationError{
			Message:    "refd() only supports type:..., trait:..., or section subqueries",
			Suggestion: "Link rows are outgoing edges and cannot be refd() sources",
		}
	default:
		return &ValidationError{Message: "refd() expects a type, trait, or section query"}
	}
}

func (v *Validator) validateRefsSubquery(sub *Query, outer *validateCtx) error {
	if sub == nil {
		return nil
	}
	switch sub.Type {
	case QueryTypeObject, QueryTypeSection:
		return v.validateQueryCtx(sub, outer)
	case QueryTypeTrait:
		return &ValidationError{
			Message:    fmt.Sprintf("refs() cannot target a trait query (trait:%s)", sub.TypeName),
			Suggestion: fmt.Sprintf("To filter trait lines by outgoing references, use trait:%s refs(...)", sub.TypeName),
		}
	case QueryTypeLink:
		return &ValidationError{
			Message:    "refs() cannot target a link query",
			Suggestion: "External file and URL links have no Raven reference identity; use links(...) to filter outgoing links",
		}
	default:
		return &ValidationError{Message: "refs() expects a type or section query"}
	}
}

func validateRegexPattern(p *StringFuncPredicate) error {
	if p == nil || p.FuncType != StringFuncMatches {
		return nil
	}
	if _, err := regexp.Compile(p.Value); err != nil {
		return &ValidationError{
			Message:    fmt.Sprintf("invalid regex pattern %q: %v", p.Value, err),
			Suggestion: `Fix the regex passed to matches() and retry.`,
		}
	}
	return nil
}

func (v *Validator) fieldDefinitionForType(typeName string, typeDef *schema.TypeDefinition, fieldName string) (*schema.FieldDefinition, error) {
	if typeDef == nil || typeDef.Fields == nil {
		return nil, &ValidationError{
			Message:    fmt.Sprintf("type '%s' has no defined fields", typeName),
			Suggestion: fmt.Sprintf("Add fields to type '%s' in schema.yaml", typeName),
		}
	}

	fieldDef, exists := typeDef.Fields[fieldName]
	if !exists {
		available := v.availableFields(typeDef)
		return nil, &ValidationError{
			Message:    fmt.Sprintf("type '%s' has no field '%s'", typeName, fieldName),
			Suggestion: fmt.Sprintf("Available fields: %s", strings.Join(available, ", ")),
		}
	}

	return fieldDef, nil
}

func (v *Validator) availableTypes() []string {
	types := make([]string, 0, len(v.schema.Types))
	for name := range v.schema.Types {
		types = append(types, name)
	}
	sort.Strings(types)
	return types
}

func (v *Validator) availableTraits() []string {
	traits := make([]string, 0, len(v.schema.Traits))
	for name := range v.schema.Traits {
		traits = append(traits, name)
	}
	sort.Strings(traits)
	return traits
}

func (v *Validator) availableFields(typeDef *schema.TypeDefinition) []string {
	var fields []string
	if typeDef.Fields != nil {
		for name := range typeDef.Fields {
			fields = append(fields, name)
		}
	}
	sort.Strings(fields)
	return fields
}
