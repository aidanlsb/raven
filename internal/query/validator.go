package query

import (
	"fmt"
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
type Validator struct {
	schema *schema.Schema
}

// NewValidator creates a new query validator.
func NewValidator(sch *schema.Schema) *Validator {
	return &Validator{schema: sch}
}

// Validate checks a parsed query against the schema.
// Returns a ValidationError if the query references undefined types, traits, or fields.
func (v *Validator) Validate(q *Query) error {
	return v.validateQuery(q)
}

func (v *Validator) validateQuery(q *Query) error {
	if q == nil {
		return nil
	}

	var err error
	if q.Type == QueryTypeObject {
		err = v.validateObjectQuery(q)
	} else {
		err = v.validateTraitQuery(q)
	}
	if err != nil {
		return err
	}

	return nil
}

func (v *Validator) validateObjectQuery(q *Query) error {
	// Check that the type exists in schema
	typeDef, exists := v.schema.Types[q.TypeName]
	if !exists {
		available := v.availableTypes()
		return &ValidationError{
			Message:    fmt.Sprintf("unknown type '%s'", q.TypeName),
			Suggestion: fmt.Sprintf("Available types: %s", strings.Join(available, ", ")),
		}
	}

	// Validate predicate
	if q.Predicate != nil {
		if err := v.validateObjectPredicate(q.Predicate, q.TypeName, typeDef); err != nil {
			return err
		}
	}

	return nil
}

func (v *Validator) validateTraitQuery(q *Query) error {
	// Check that the trait exists in schema
	if _, exists := v.schema.Traits[q.TypeName]; !exists {
		available := v.availableTraits()
		return &ValidationError{
			Message:    fmt.Sprintf("unknown trait '%s'", q.TypeName),
			Suggestion: fmt.Sprintf("Available traits: %s", strings.Join(available, ", ")),
		}
	}

	// Validate predicate
	if q.Predicate != nil {
		if err := v.validateTraitPredicate(q.Predicate); err != nil {
			return err
		}
	}

	return nil
}

func (v *Validator) validateObjectPredicate(pred Predicate, typeName string, typeDef *schema.TypeDefinition) error {
	switch p := pred.(type) {
	case *FieldPredicate:
		return v.validateFieldPredicate(p, typeName, typeDef)
	case *StringFuncPredicate:
		return v.validateObjectStringFuncPredicate(p, typeName, typeDef)
	case *ArrayQuantifierPredicate:
		return v.validateArrayQuantifierPredicate(p, typeName, typeDef)
	case *HasPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery)
		}
	case *ParentPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery)
		}
		// Target-based predicate doesn't need schema validation
	case *AncestorPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery)
		}
	case *ChildPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery)
		}
	case *DescendantPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery)
		}
	case *ContainsPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery)
		}
	case *RefsPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery)
		}
	case *ContentPredicate:
		// Content predicate just needs a non-empty search term
		if p.SearchTerm == "" {
			return &ValidationError{
				Message:    "content search term cannot be empty",
				Suggestion: `Provide a search term: content("search terms")`,
			}
		}
	case *RefdPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery)
		}
	case *ValuePredicate:
		// ValuePredicate is deprecated; the parser now uses FieldPredicate with Field="value"
		return &ValidationError{
			Message:    "value predicate is only valid for trait queries",
			Suggestion: "Use .value==X in trait queries, or use .field==X for object fields",
		}
	case *OnPredicate:
		return &ValidationError{
			Message:    "on: predicate is only valid for trait queries",
			Suggestion: "Use on(object:...) in trait queries",
		}
	case *WithinPredicate:
		return &ValidationError{
			Message:    "within: predicate is only valid for trait queries",
			Suggestion: "Use within(object:...) in trait queries",
		}
	case *AtPredicate:
		// at: is only valid for trait queries
		return &ValidationError{
			Message:    "at: predicate is only valid for trait queries",
			Suggestion: "Use at(trait:...) to find traits co-located with other traits",
		}
	case *OrPredicate:
		for _, subPred := range p.Predicates {
			if err := v.validateObjectPredicate(subPred, typeName, typeDef); err != nil {
				return err
			}
		}
	case *NotPredicate:
		return v.validateObjectPredicate(p.Inner, typeName, typeDef)
	case *GroupPredicate:
		for _, subPred := range p.Predicates {
			if err := v.validateObjectPredicate(subPred, typeName, typeDef); err != nil {
				return err
			}
		}
	}
	return nil
}

func (v *Validator) validateTraitPredicate(pred Predicate) error {
	switch p := pred.(type) {
	case *ValuePredicate:
		return nil
	case *StringFuncPredicate:
		if p.IsElementRef || p.Field != "value" {
			fieldLabel := "." + p.Field
			if p.IsElementRef {
				fieldLabel = "_"
			}
			return &ValidationError{
				Message:    fmt.Sprintf("string functions on trait queries only support .value, got %s", fieldLabel),
				Suggestion: `Use contains(.value, "..."), startswith(.value, "..."), endswith(.value, "..."), or matches(.value, "..."). Use content("...") to search trait line content.`,
			}
		}
		return nil
	case *OnPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery)
		}
		// Target-based predicate doesn't need schema validation
	case *WithinPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery)
		}
	case *RefsPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery)
		}
	case *ContentPredicate:
		// Content predicate just needs a non-empty search term
		if p.SearchTerm == "" {
			return &ValidationError{
				Message:    "content search term cannot be empty",
				Suggestion: `Provide a search term: content("search terms")`,
			}
		}
	case *AtPredicate:
		// at: is only valid for trait queries (which we're in)
		if p.SubQuery != nil {
			if p.SubQuery.Type != QueryTypeTrait {
				return &ValidationError{
					Message:    "at: requires a trait subquery",
					Suggestion: "Use at(trait:name) to find traits co-located with other traits",
				}
			}
			return v.validateQuery(p.SubQuery)
		}
	case *RefdPredicate:
		return &ValidationError{
			Message:    "refd: predicate is only valid for object queries",
			Suggestion: "Use refd: with object queries, or use refs: in trait queries",
		}
	case *FieldPredicate:
		// Allow .value for traits (the trait's value field)
		if p.Field == "value" {
			return nil
		}
		return &ValidationError{
			Message:    "field predicates other than .value are only valid for object queries",
			Suggestion: "Use .value==X for trait values, or .field==X in object queries",
		}
	case *ArrayQuantifierPredicate:
		return &ValidationError{
			Message:    "array predicates are only valid for object queries",
			Suggestion: "Use any()/all()/none() on object array fields",
		}
	case *HasPredicate:
		return &ValidationError{
			Message:    "has: predicate is only valid for object queries",
			Suggestion: "Use has(trait:...) in object queries",
		}
	case *ContainsPredicate:
		return &ValidationError{
			Message:    "contains: predicate is only valid for object queries",
			Suggestion: "Use encloses(trait:...) in object queries",
		}
	case *ParentPredicate:
		return &ValidationError{
			Message:    "parent: predicate is only valid for object queries",
			Suggestion: "Use parent(object:...) in object queries",
		}
	case *AncestorPredicate:
		return &ValidationError{
			Message:    "ancestor: predicate is only valid for object queries",
			Suggestion: "Use ancestor(object:...) in object queries",
		}
	case *ChildPredicate:
		return &ValidationError{
			Message:    "child: predicate is only valid for object queries",
			Suggestion: "Use child(object:...) in object queries",
		}
	case *DescendantPredicate:
		return &ValidationError{
			Message:    "descendant: predicate is only valid for object queries",
			Suggestion: "Use descendant(object:...) in object queries",
		}
	case *OrPredicate:
		for _, subPred := range p.Predicates {
			if err := v.validateTraitPredicate(subPred); err != nil {
				return err
			}
		}
	case *NotPredicate:
		return v.validateTraitPredicate(p.Inner)
	case *GroupPredicate:
		for _, subPred := range p.Predicates {
			if err := v.validateTraitPredicate(subPred); err != nil {
				return err
			}
		}
	}
	return nil
}

func (v *Validator) validateFieldPredicate(p *FieldPredicate, typeName string, typeDef *schema.TypeDefinition) error {
	_, err := v.fieldDefinitionForType(typeName, typeDef, p.Field)
	return err
}

func (v *Validator) validateObjectStringFuncPredicate(p *StringFuncPredicate, typeName string, typeDef *schema.TypeDefinition) error {
	if p.IsElementRef {
		return &ValidationError{
			Message:    "string function placeholder '_' is only valid inside any()/all()/none()",
			Suggestion: `Use contains(.field, "..."), startswith(.field, "..."), endswith(.field, "..."), or matches(.field, "...") for object fields.`,
		}
	}

	fieldDef, err := v.fieldDefinitionForType(typeName, typeDef, p.Field)
	if err != nil {
		return err
	}

	if isArrayFieldType(fieldDef.Type) {
		return &ValidationError{
			Message:    fmt.Sprintf("string function predicates require a scalar field, but '.%s' is %s", p.Field, fieldDef.Type),
			Suggestion: fmt.Sprintf(`Use any(.%s, contains(_, "...")) for array fields`, p.Field),
		}
	}

	if !isStringLikeFieldType(fieldDef.Type) {
		return &ValidationError{
			Message:    fmt.Sprintf("string function predicates are not valid for field '.%s' of type %s", p.Field, fieldDef.Type),
			Suggestion: "Use comparison predicates (.field==value, .field!=value, .field<value, etc.) for non-string fields",
		}
	}

	return nil
}

func (v *Validator) validateArrayQuantifierPredicate(p *ArrayQuantifierPredicate, typeName string, typeDef *schema.TypeDefinition) error {
	fieldDef, err := v.fieldDefinitionForType(typeName, typeDef, p.Field)
	if err != nil {
		return err
	}

	elemType, ok := arrayElementType(fieldDef.Type)
	if !ok {
		return &ValidationError{
			Message:    fmt.Sprintf("array predicates any()/all()/none() require an array field, but '.%s' is %s", p.Field, fieldDef.Type),
			Suggestion: "Use any()/all()/none() only with [] fields, or use scalar field predicates on non-array fields",
		}
	}

	return v.validateArrayElementPredicate(p.ElementPred, elemType)
}

func (v *Validator) validateArrayElementPredicate(pred Predicate, elemType schema.FieldType) error {
	switch p := pred.(type) {
	case *ElementEqualityPredicate:
		if p.IsRefValue && elemType != schema.FieldTypeRef {
			return &ValidationError{
				Message:    fmt.Sprintf("reference element comparison is only valid for ref[] fields, not %s[]", elemType),
				Suggestion: "Use [[target]] comparisons only for ref[] fields",
			}
		}
		return nil
	case *StringFuncPredicate:
		if !p.IsElementRef {
			return &ValidationError{
				Message:    "array element string functions must use '_' as the first argument",
				Suggestion: `Use contains(_, "..."), startswith(_, "..."), endswith(_, "..."), or matches(_, "...")`,
			}
		}
		if !isStringLikeFieldType(elemType) {
			return &ValidationError{
				Message:    fmt.Sprintf("string functions are not valid for array elements of type %s", elemType),
				Suggestion: "Use element comparisons (_==value, _!=value, _<value, etc.) for non-string array element types",
			}
		}
		return nil
	case *OrPredicate:
		for _, subPred := range p.Predicates {
			if err := v.validateArrayElementPredicate(subPred, elemType); err != nil {
				return err
			}
		}
		return nil
	case *NotPredicate:
		return v.validateArrayElementPredicate(p.Inner, elemType)
	case *GroupPredicate:
		for _, subPred := range p.Predicates {
			if err := v.validateArrayElementPredicate(subPred, elemType); err != nil {
				return err
			}
		}
		return nil
	default:
		return &ValidationError{
			Message:    fmt.Sprintf("unsupported array element predicate type %T", pred),
			Suggestion: `Use _==value, _!=value, or string functions like contains(_, "...")`,
		}
	}
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

func isStringLikeFieldType(fieldType schema.FieldType) bool {
	switch fieldType {
	case schema.FieldTypeString,
		schema.FieldTypeURL,
		schema.FieldTypeDate,
		schema.FieldTypeDatetime,
		schema.FieldTypeEnum,
		schema.FieldTypeRef:
		return true
	default:
		return false
	}
}

func isArrayFieldType(fieldType schema.FieldType) bool {
	_, ok := arrayElementType(fieldType)
	return ok
}

func arrayElementType(fieldType schema.FieldType) (schema.FieldType, bool) {
	switch fieldType {
	case schema.FieldTypeStringArray:
		return schema.FieldTypeString, true
	case schema.FieldTypeNumberArray:
		return schema.FieldTypeNumber, true
	case schema.FieldTypeURLArray:
		return schema.FieldTypeURL, true
	case schema.FieldTypeDateArray:
		return schema.FieldTypeDate, true
	case schema.FieldTypeDatetimeArray:
		return schema.FieldTypeDatetime, true
	case schema.FieldTypeEnumArray:
		return schema.FieldTypeEnum, true
	case schema.FieldTypeBoolArray:
		return schema.FieldTypeBool, true
	case schema.FieldTypeRefArray:
		return schema.FieldTypeRef, true
	default:
		return "", false
	}
}

func (v *Validator) availableTypes() []string {
	types := make([]string, 0, len(v.schema.Types))
	for name := range v.schema.Types {
		types = append(types, name)
	}
	return types
}

func (v *Validator) availableTraits() []string {
	traits := make([]string, 0, len(v.schema.Traits))
	for name := range v.schema.Traits {
		traits = append(traits, name)
	}
	return traits
}

func (v *Validator) availableFields(typeDef *schema.TypeDefinition) []string {
	var fields []string
	if typeDef.Fields != nil {
		for name := range typeDef.Fields {
			fields = append(fields, name)
		}
	}
	return fields
}
