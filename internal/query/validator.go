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
	if q.Type == QueryTypeObject {
		return v.validateObjectQuery(q)
	}
	return v.validateTraitQuery(q)
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

	// Validate predicates
	for _, pred := range q.Predicates {
		if err := v.validateObjectPredicate(pred, q.TypeName, typeDef); err != nil {
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

	// Validate predicates
	for _, pred := range q.Predicates {
		if err := v.validateTraitPredicate(pred); err != nil {
			return err
		}
	}

	return nil
}

func (v *Validator) validateObjectPredicate(pred Predicate, typeName string, typeDef *schema.TypeDefinition) error {
	switch p := pred.(type) {
	case *FieldPredicate:
		return v.validateFieldPredicate(p, typeName, typeDef)
	case *HasPredicate:
		return v.validateQuery(p.SubQuery)
	case *ParentPredicate:
		return v.validateQuery(p.SubQuery)
	case *AncestorPredicate:
		return v.validateQuery(p.SubQuery)
	case *ChildPredicate:
		return v.validateQuery(p.SubQuery)
	case *RefsPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery)
		}
	case *ContentPredicate:
		// Content predicate just needs a non-empty search term
		if p.SearchTerm == "" {
			return &ValidationError{
				Message:    "content search term cannot be empty",
				Suggestion: "Provide a search term: content:\"search terms\"",
			}
		}
	case *OrPredicate:
		if err := v.validateObjectPredicate(p.Left, typeName, typeDef); err != nil {
			return err
		}
		return v.validateObjectPredicate(p.Right, typeName, typeDef)
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
	case *OnPredicate:
		return v.validateQuery(p.SubQuery)
	case *WithinPredicate:
		return v.validateQuery(p.SubQuery)
	case *RefsPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery)
		}
	case *OrPredicate:
		if err := v.validateTraitPredicate(p.Left); err != nil {
			return err
		}
		return v.validateTraitPredicate(p.Right)
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
