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
	return v.validateQuery(q, false)
}

func (v *Validator) validateQuery(q *Query, allowSelfRef bool) error {
	if q == nil {
		return nil
	}
	if !allowSelfRef && containsSelfRef(q) {
		return &ValidationError{
			Message:    "self-reference '_' is only valid inside pipeline subqueries (and array quantifiers)",
			Suggestion: "Move '_' into a pipeline assignment subquery like count({trait:todo within:_})",
		}
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

	// Validate pipeline stages
	if q.Pipeline != nil {
		if err := v.validatePipeline(q.Pipeline, q.Type); err != nil {
			return err
		}
	}

	return nil
}

// validatePipeline validates all stages in a pipeline.
func (v *Validator) validatePipeline(p *Pipeline, queryType QueryType) error {
	for _, stage := range p.Stages {
		switch s := stage.(type) {
		case *AssignmentStage:
			if err := v.validateAssignmentStage(s, queryType); err != nil {
				return err
			}
			// FilterStage and SortStage don't contain subqueries, just computed/field references
		}
	}
	return nil
}

// validateAssignmentStage validates that assignment subqueries contain _ references.
func (v *Validator) validateAssignmentStage(s *AssignmentStage, queryType QueryType) error {
	// Navigation functions like refs(_), refd(_) are always connected
	if s.NavFunc != nil {
		return nil
	}

	// Subqueries must contain a _ reference to be meaningful
	if s.SubQuery != nil {
		if err := v.validateQuery(s.SubQuery, true); err != nil {
			return &ValidationError{
				Message:    fmt.Sprintf("invalid subquery in assignment '%s': %s", s.Name, err.Error()),
				Suggestion: "Ensure the subquery references valid types/traits",
			}
		}

		if !containsSelfRef(s.SubQuery) {
			return &ValidationError{
				Message:    fmt.Sprintf("pipeline subquery in '%s' must reference _ to connect to the query result", s.Name),
				Suggestion: "Use predicates like within:_, on:_, refs:_, at:_ to relate the subquery to each result. Example: count({trait:todo within:_})",
			}
		}
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
	case *StringFuncPredicate:
		return nil
	case *ArrayQuantifierPredicate:
		return nil
	case *HasPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery, false)
		}
	case *ParentPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery, false)
		}
		// Target-based predicate doesn't need schema validation
	case *AncestorPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery, false)
		}
	case *ChildPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery, false)
		}
	case *DescendantPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery, false)
		}
	case *ContainsPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery, false)
		}
	case *RefsPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery, false)
		}
	case *ContentPredicate:
		// Content predicate just needs a non-empty search term
		if p.SearchTerm == "" {
			return &ValidationError{
				Message:    "content search term cannot be empty",
				Suggestion: "Provide a search term: content:\"search terms\"",
			}
		}
	case *RefdPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery, false)
		}
	case *ValuePredicate:
		return &ValidationError{
			Message:    "value: predicate is only valid for trait queries",
			Suggestion: "Use value==... in trait queries, or use .field==... for object fields",
		}
	case *OnPredicate:
		return &ValidationError{
			Message:    "on: predicate is only valid for trait queries",
			Suggestion: "Use on:{object:...} in trait queries",
		}
	case *WithinPredicate:
		return &ValidationError{
			Message:    "within: predicate is only valid for trait queries",
			Suggestion: "Use within:{object:...} in trait queries",
		}
	case *AtPredicate:
		// at: is only valid for trait queries
		return &ValidationError{
			Message:    "at: predicate is only valid for trait queries",
			Suggestion: "Use at: to find traits co-located with other traits",
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
	case *ValuePredicate:
		return nil
	case *StringFuncPredicate:
		return nil
	case *OnPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery, false)
		}
		// Target-based predicate doesn't need schema validation
	case *WithinPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery, false)
		}
	case *RefsPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery, false)
		}
	case *ContentPredicate:
		// Content predicate just needs a non-empty search term
		if p.SearchTerm == "" {
			return &ValidationError{
				Message:    "content search term cannot be empty",
				Suggestion: "Provide a search term: content:\"search terms\"",
			}
		}
	case *AtPredicate:
		// at: is only valid for trait queries (which we're in)
		if p.SubQuery != nil {
			if p.SubQuery.Type != QueryTypeTrait {
				return &ValidationError{
					Message:    "at: requires a trait subquery",
					Suggestion: "Use at:{trait:name} to find traits co-located with other traits",
				}
			}
			return v.validateQuery(p.SubQuery, false)
		}
	case *RefdPredicate:
		return &ValidationError{
			Message:    "refd: predicate is only valid for object queries",
			Suggestion: "Use refd: with object queries, or use refs: in trait queries",
		}
	case *FieldPredicate:
		return &ValidationError{
			Message:    "field predicates are only valid for object queries",
			Suggestion: "Use .field==value in object queries, or value==... in trait queries",
		}
	case *ArrayQuantifierPredicate:
		return &ValidationError{
			Message:    "array predicates are only valid for object queries",
			Suggestion: "Use any()/all()/none() on object array fields",
		}
	case *HasPredicate:
		return &ValidationError{
			Message:    "has: predicate is only valid for object queries",
			Suggestion: "Use has:{trait:...} in object queries",
		}
	case *ContainsPredicate:
		return &ValidationError{
			Message:    "contains: predicate is only valid for object queries",
			Suggestion: "Use contains:{trait:...} in object queries",
		}
	case *ParentPredicate:
		return &ValidationError{
			Message:    "parent: predicate is only valid for object queries",
			Suggestion: "Use parent:{object:...} in object queries",
		}
	case *AncestorPredicate:
		return &ValidationError{
			Message:    "ancestor: predicate is only valid for object queries",
			Suggestion: "Use ancestor:{object:...} in object queries",
		}
	case *ChildPredicate:
		return &ValidationError{
			Message:    "child: predicate is only valid for object queries",
			Suggestion: "Use child:{object:...} in object queries",
		}
	case *DescendantPredicate:
		return &ValidationError{
			Message:    "descendant: predicate is only valid for object queries",
			Suggestion: "Use descendant:{object:...} in object queries",
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

// containsSelfRef checks if a query contains any _ (self-reference) predicates.
func containsSelfRef(q *Query) bool {
	for _, pred := range q.Predicates {
		if predicateContainsSelfRef(pred) {
			return true
		}
	}
	return false
}

// predicateContainsSelfRef checks if a predicate or its subqueries contain _ references.
func predicateContainsSelfRef(pred Predicate) bool {
	switch p := pred.(type) {
	case *OnPredicate:
		if p.IsSelfRef {
			return true
		}
		if p.SubQuery != nil && containsSelfRef(p.SubQuery) {
			return true
		}
	case *WithinPredicate:
		if p.IsSelfRef {
			return true
		}
		if p.SubQuery != nil && containsSelfRef(p.SubQuery) {
			return true
		}
	case *AtPredicate:
		if p.IsSelfRef {
			return true
		}
		if p.SubQuery != nil && containsSelfRef(p.SubQuery) {
			return true
		}
	case *RefsPredicate:
		if p.IsSelfRef {
			return true
		}
		if p.SubQuery != nil && containsSelfRef(p.SubQuery) {
			return true
		}
	case *RefdPredicate:
		if p.IsSelfRef {
			return true
		}
		if p.SubQuery != nil && containsSelfRef(p.SubQuery) {
			return true
		}
	case *ParentPredicate:
		if p.IsSelfRef {
			return true
		}
		if p.SubQuery != nil && containsSelfRef(p.SubQuery) {
			return true
		}
	case *AncestorPredicate:
		if p.IsSelfRef {
			return true
		}
		if p.SubQuery != nil && containsSelfRef(p.SubQuery) {
			return true
		}
	case *ChildPredicate:
		if p.IsSelfRef {
			return true
		}
		if p.SubQuery != nil && containsSelfRef(p.SubQuery) {
			return true
		}
	case *DescendantPredicate:
		if p.IsSelfRef {
			return true
		}
		if p.SubQuery != nil && containsSelfRef(p.SubQuery) {
			return true
		}
	case *HasPredicate:
		if p.IsSelfRef {
			return true
		}
		if p.SubQuery != nil && containsSelfRef(p.SubQuery) {
			return true
		}
	case *ContainsPredicate:
		if p.IsSelfRef {
			return true
		}
		if p.SubQuery != nil && containsSelfRef(p.SubQuery) {
			return true
		}
	case *OrPredicate:
		if predicateContainsSelfRef(p.Left) || predicateContainsSelfRef(p.Right) {
			return true
		}
	case *GroupPredicate:
		for _, subPred := range p.Predicates {
			if predicateContainsSelfRef(subPred) {
				return true
			}
		}
	}
	return false
}
