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
		return nil
	case *ArrayQuantifierPredicate:
		return nil
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
	// Check if field exists on the type
	if typeDef == nil || typeDef.Fields == nil {
		return &ValidationError{
			Message:    fmt.Sprintf("type '%s' has no defined fields", typeName),
			Suggestion: fmt.Sprintf("Add fields to type '%s' in schema.yaml", typeName),
		}
	}

	if _, exists := typeDef.Fields[p.Field]; !exists {
		available := v.availableFields(typeDef)
		return &ValidationError{
			Message:    fmt.Sprintf("type '%s' has no field '%s'", typeName, p.Field),
			Suggestion: fmt.Sprintf("Available fields: %s", strings.Join(available, ", ")),
		}
	}

	return nil
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
