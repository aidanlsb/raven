package query

import (
	"fmt"
	"regexp"
	"sort"
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
//     ".value" restrictions, section/asset built-in field names, regex
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
	return v.validateQuery(q)
}

func (v *Validator) validateQuery(q *Query) error {
	if q == nil {
		return nil
	}

	switch q.Type {
	case QueryTypeObject:
		return v.validateObjectQuery(q)
	case QueryTypeAsset:
		return v.validateAssetQuery(q)
	case QueryTypeSection:
		return v.validateSectionQuery(q)
	case QueryTypeLink:
		return v.validateLinkQuery(q)
	case QueryTypeTrait:
		return v.validateTraitQuery(q)
	default:
		return &ValidationError{Message: fmt.Sprintf("unknown query root %d", q.Type)}
	}
}

func (v *Validator) validateObjectQuery(q *Query) error {
	var typeDef *schema.TypeDefinition
	if v.hasSchema() {
		td, exists := v.schema.Types[q.TypeName]
		if !exists {
			available := v.availableTypes()
			return &ValidationError{
				Message:    fmt.Sprintf("unknown type '%s'", q.TypeName),
				Suggestion: fmt.Sprintf("Available types: %s", strings.Join(available, ", ")),
			}
		}
		typeDef = td
	}

	return v.validatePredicate(QueryTypeObject, q.TypeName, typeDef, q.Predicate)
}

func (v *Validator) validateTraitQuery(q *Query) error {
	if v.hasSchema() {
		if _, exists := v.schema.Traits[q.TypeName]; !exists {
			available := v.availableTraits()
			return &ValidationError{
				Message:    fmt.Sprintf("unknown trait '%s'", q.TypeName),
				Suggestion: fmt.Sprintf("Available traits: %s", strings.Join(available, ", ")),
			}
		}
	}

	return v.validatePredicate(QueryTypeTrait, q.TypeName, nil, q.Predicate)
}

func (v *Validator) validateAssetQuery(q *Query) error {
	return v.validatePredicate(QueryTypeAsset, "", nil, q.Predicate)
}

func (v *Validator) validateSectionQuery(q *Query) error {
	return v.validatePredicate(QueryTypeSection, "", nil, q.Predicate)
}

func (v *Validator) validateLinkQuery(q *Query) error {
	return v.validatePredicate(QueryTypeLink, "", nil, q.Predicate)
}

// validatePredicate is the single validation entry point for predicate nodes.
// It handles boolean composition, then the shared legality matrix, then the
// per-root fine-grained checks.
func (v *Validator) validatePredicate(root QueryType, typeName string, typeDef *schema.TypeDefinition, pred Predicate) error {
	if pred == nil {
		return nil
	}

	switch p := pred.(type) {
	case *OrPredicate:
		for _, subPred := range p.Predicates {
			if err := v.validatePredicate(root, typeName, typeDef, subPred); err != nil {
				return err
			}
		}
		return nil
	case *NotPredicate:
		return v.validatePredicate(root, typeName, typeDef, p.Inner)
	case *GroupPredicate:
		for _, subPred := range p.Predicates {
			if err := v.validatePredicate(root, typeName, typeDef, subPred); err != nil {
				return err
			}
		}
		return nil
	}

	// Coarse legality: shared with the executor via the capability matrix.
	if verr := predicateAllowedAtRoot(root, pred); verr != nil {
		return verr
	}

	// Fine-grained checks for the (now known-legal) predicate kind at this root.
	switch root {
	case QueryTypeObject:
		return v.validateLegalObjectPredicate(typeName, typeDef, pred)
	case QueryTypeTrait:
		return v.validateLegalTraitPredicate(typeName, pred)
	case QueryTypeAsset:
		return v.validateLegalAssetPredicate(pred)
	case QueryTypeSection:
		return v.validateLegalSectionPredicate(pred)
	case QueryTypeLink:
		return v.validateLegalLinkPredicate(pred)
	default:
		return nil
	}
}

// validateSubquery validates a nested query (its own root re-enters the
// structural/schema split independently).
func (v *Validator) validateSubquery(sub *Query) error {
	if sub == nil {
		return nil
	}
	return v.validateQuery(sub)
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

func (v *Validator) validateLegalObjectPredicate(typeName string, typeDef *schema.TypeDefinition, pred Predicate) error {
	switch p := pred.(type) {
	case *FieldPredicate:
		return v.validateFieldPredicate(p, typeName, typeDef)
	case *StringFuncPredicate:
		return v.validateObjectStringFuncPredicate(p, typeName, typeDef)
	case *ArrayQuantifierPredicate:
		return v.validateArrayQuantifierPredicate(p, typeName, typeDef)
	case *HasPredicate:
		return v.validateSubquery(p.SubQuery)
	case *ContainsPredicate:
		return v.validateSubquery(p.SubQuery)
	case *RefsPredicate:
		return v.validateSubquery(p.SubQuery)
	case *LinksPredicate:
		return validateLinkPredicate(p.LinkPredicate)
	case *RefdPredicate:
		return v.validateSubquery(p.SubQuery)
	case *ContentPredicate:
		return validateContentTerm(p)
	}
	return nil
}

func (v *Validator) validateLegalTraitPredicate(traitName string, pred Predicate) error {
	switch p := pred.(type) {
	case *ValuePredicate:
		return nil
	case *FieldPredicate:
		// Allow .value for traits (the trait's value field).
		if p.Field == "value" {
			return nil
		}
		return &ValidationError{
			Message:    "field predicates other than .value are only valid for type queries",
			Suggestion: "Use .value==X for trait values, or .field==X in type queries",
		}
	case *StringFuncPredicate:
		if p.IsElementRef || p.Field != "value" {
			fieldLabel := "." + p.Field
			if p.IsElementRef {
				fieldLabel = "_"
			}
			return &ValidationError{
				Message:    fmt.Sprintf("string functions on trait queries only support .value, got %s", fieldLabel),
				Suggestion: `Use includes(.value, "..."), startswith(.value, "..."), endswith(.value, "..."), or matches(.value, "..."). Use content("...") to search trait line content.`,
			}
		}
		return validateRegexPattern(p)
	case *ArrayQuantifierPredicate:
		return v.validateTraitArrayQuantifierPredicate(p, traitName)
	case *InPredicate:
		return v.validateSubquery(p.SubQuery)
	case *WithinPredicate:
		return v.validateSubquery(p.SubQuery)
	case *RefsPredicate:
		return v.validateSubquery(p.SubQuery)
	case *LinksPredicate:
		return validateLinkPredicate(p.LinkPredicate)
	case *ContentPredicate:
		return validateContentTerm(p)
	case *AtPredicate:
		if p.SubQuery != nil {
			if p.SubQuery.Type != QueryTypeTrait {
				return &ValidationError{
					Message:    "at: requires a trait subquery",
					Suggestion: "Use at(trait:name) to find traits co-located with other traits",
				}
			}
			return v.validateQuery(p.SubQuery)
		}
	}
	return nil
}

func (v *Validator) validateLegalAssetPredicate(pred Predicate) error {
	switch p := pred.(type) {
	case *FieldPredicate:
		return v.validateAssetFieldPredicate(p)
	case *StringFuncPredicate:
		return v.validateAssetStringFuncPredicate(p)
	case *RefdPredicate:
		return v.validateSubquery(p.SubQuery)
	}
	return nil
}

func (v *Validator) validateLegalSectionPredicate(pred Predicate) error {
	switch p := pred.(type) {
	case *FieldPredicate:
		if _, ok := sectionFieldColumn("s", p.Field); !ok {
			return &ValidationError{
				Message:    fmt.Sprintf("section has no field '%s'", p.Field),
				Suggestion: "Available section fields: id, file_object_id, file_path, slug, title, level, line_start, line_end, direct_line_end, subtree_line_end, parent_section_id",
			}
		}
	case *StringFuncPredicate:
		if p.IsElementRef {
			return &ValidationError{
				Message:    "string function placeholder '_' is not valid for section queries",
				Suggestion: `Use includes(.title, "..."), startswith(.slug, "..."), or matches(.file_path, "...")`,
			}
		}
		if _, ok := sectionFieldColumn("s", p.Field); !ok {
			return &ValidationError{
				Message:    fmt.Sprintf("section has no field '%s'", p.Field),
				Suggestion: "Available section fields: id, file_object_id, file_path, slug, title, level, line_start, line_end, direct_line_end, subtree_line_end, parent_section_id",
			}
		}
		if isNumericSectionField(p.Field) {
			return &ValidationError{
				Message:    fmt.Sprintf("string function predicates are not valid for numeric section field '.%s'", p.Field),
				Suggestion: "Use comparison predicates for numeric section fields",
			}
		}
		return validateRegexPattern(p)
	case *InPredicate:
		return v.validateSubquery(p.SubQuery)
	case *WithinPredicate:
		return v.validateSubquery(p.SubQuery)
	case *HasPredicate:
		return v.validateSubquery(p.SubQuery)
	case *ContainsPredicate:
		return v.validateSubquery(p.SubQuery)
	case *RefsPredicate:
		return v.validateSubquery(p.SubQuery)
	case *LinksPredicate:
		return validateLinkPredicate(p.LinkPredicate)
	case *RefdPredicate:
		return v.validateSubquery(p.SubQuery)
	case *ContentPredicate:
		return validateContentTerm(p)
	}
	return nil
}

func (v *Validator) validateLegalLinkPredicate(pred Predicate) error {
	switch p := pred.(type) {
	case *FieldPredicate, *StringFuncPredicate:
		return validateLinkPredicate(pred)
	case *WithinPredicate:
		return v.validateSubquery(p.SubQuery)
	}
	return nil
}

func (v *Validator) validateTraitArrayQuantifierPredicate(p *ArrayQuantifierPredicate, traitName string) error {
	if p.Field != "value" {
		return &ValidationError{
			Message:    fmt.Sprintf("array predicates on trait queries only support .value, got .%s", p.Field),
			Suggestion: "Use any(.value, ...), all(.value, ...), or none(.value, ...) for array-valued traits",
		}
	}
	if !v.hasSchema() {
		return nil
	}
	traitDef := v.schema.Traits[traitName]
	if traitDef == nil {
		return &ValidationError{
			Message:    fmt.Sprintf("unknown trait '%s'", traitName),
			Suggestion: fmt.Sprintf("Available traits: %s", strings.Join(v.availableTraits(), ", ")),
		}
	}
	elemType, ok := arrayElementType(traitDef.Type)
	if !ok {
		return &ValidationError{
			Message:    fmt.Sprintf("array predicates any()/all()/none() require an array-valued trait, but trait '%s' is %s", traitName, traitDef.Type),
			Suggestion: "Use any()/all()/none() only with [] trait types, or use .value predicates on scalar traits",
		}
	}
	return v.validateArrayElementPredicate(p.ElementPred, elemType)
}

func (v *Validator) validateFieldPredicate(p *FieldPredicate, typeName string, typeDef *schema.TypeDefinition) error {
	if isDateVirtualField(typeName, p.Field) {
		return nil
	}
	if !v.hasSchema() {
		return nil
	}
	_, err := v.fieldDefinitionForType(typeName, typeDef, p.Field)
	return err
}

func (v *Validator) validateAssetFieldPredicate(p *FieldPredicate) error {
	if _, ok := assetFieldTypes[p.Field]; !ok {
		return &ValidationError{
			Message:    fmt.Sprintf("asset has no field '%s'", p.Field),
			Suggestion: fmt.Sprintf("Available asset fields: %s", strings.Join(availableAssetFields(), ", ")),
		}
	}
	return nil
}

func (v *Validator) validateAssetStringFuncPredicate(p *StringFuncPredicate) error {
	if p.IsElementRef {
		return &ValidationError{
			Message:    "string function placeholder '_' is not valid for asset queries",
			Suggestion: `Use includes(.filename, "..."), startswith(.file_path, "..."), or startswith(.media_type, "...")`,
		}
	}
	fieldType, ok := assetFieldTypes[p.Field]
	if !ok {
		return &ValidationError{
			Message:    fmt.Sprintf("asset has no field '%s'", p.Field),
			Suggestion: fmt.Sprintf("Available asset fields: %s", strings.Join(availableAssetFields(), ", ")),
		}
	}
	if fieldType != schema.FieldTypeString {
		return &ValidationError{
			Message:    fmt.Sprintf("string function predicates are not valid for asset field '.%s' of type %s", p.Field, fieldType),
			Suggestion: "Use comparison predicates for non-string asset fields",
		}
	}
	return validateRegexPattern(p)
}

func (v *Validator) validateObjectStringFuncPredicate(p *StringFuncPredicate, typeName string, typeDef *schema.TypeDefinition) error {
	if p.IsElementRef {
		return &ValidationError{
			Message:    "string function placeholder '_' is only valid inside any()/all()/none()",
			Suggestion: `Use includes(.field, "..."), startswith(.field, "..."), endswith(.field, "..."), or matches(.field, "...") for type fields.`,
		}
	}

	if v.hasSchema() {
		fieldDef, err := v.fieldDefinitionForType(typeName, typeDef, p.Field)
		if err != nil {
			return err
		}

		if isArrayFieldType(fieldDef.Type) {
			return &ValidationError{
				Message:    fmt.Sprintf("string function predicates require a scalar field, but '.%s' is %s", p.Field, fieldDef.Type),
				Suggestion: fmt.Sprintf(`Use any(.%s, includes(_, "...")) for array fields`, p.Field),
			}
		}

		if !isStringLikeFieldType(fieldDef.Type) {
			return &ValidationError{
				Message:    fmt.Sprintf("string function predicates are not valid for field '.%s' of type %s", p.Field, fieldDef.Type),
				Suggestion: "Use comparison predicates (.field==value, .field!=value, .field<value, etc.) for non-string fields",
			}
		}
	}

	return validateRegexPattern(p)
}

func (v *Validator) validateArrayQuantifierPredicate(p *ArrayQuantifierPredicate, typeName string, typeDef *schema.TypeDefinition) error {
	if !v.hasSchema() {
		return nil
	}
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
				Suggestion: `Use includes(_, "..."), startswith(_, "..."), endswith(_, "..."), or matches(_, "...")`,
			}
		}
		if !isStringLikeFieldType(elemType) {
			return &ValidationError{
				Message:    fmt.Sprintf("string functions are not valid for array elements of type %s", elemType),
				Suggestion: "Use element comparisons (_==value, _!=value, _<value, etc.) for non-string array element types",
			}
		}
		if err := validateRegexPattern(p); err != nil {
			return err
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
			Suggestion: `Use _==value, _!=value, or string functions like includes(_, "...")`,
		}
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
