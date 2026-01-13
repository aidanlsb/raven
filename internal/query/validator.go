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
	var err error
	if q.Type == QueryTypeObject {
		err = v.validateObjectQuery(q)
	} else {
		err = v.validateTraitQuery(q)
	}
	if err != nil {
		return err
	}

	// Validate sort/group clauses
	if q.Sort != nil {
		if err := v.validateSortSpec(q.Sort, q.Type); err != nil {
			return err
		}
	}
	if q.Group != nil {
		if err := v.validateGroupSpec(q.Group, q.Type); err != nil {
			return err
		}
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
		if err := v.validateQuery(s.SubQuery); err != nil {
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
	case *HasPredicate:
		return v.validateQuery(p.SubQuery)
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
	case *RefdPredicate:
		if p.SubQuery != nil {
			return v.validateQuery(p.SubQuery)
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
			return v.validateQuery(p.SubQuery)
		}
	case *RefdPredicate:
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

// validateSortSpec validates a sort specification.
func (v *Validator) validateSortSpec(spec *SortSpec, queryType QueryType) error {
	if spec.Path != nil {
		if err := v.validatePathExpr(spec.Path, queryType); err != nil {
			return &ValidationError{
				Message:    fmt.Sprintf("invalid sort path: %s", err.Error()),
				Suggestion: "Valid path steps: _.value, _.parent, _.fieldname, _.refs:type, _.ancestor:type",
			}
		}
	}

	if spec.SubQuery != nil {
		if err := v.validateQuery(spec.SubQuery); err != nil {
			return &ValidationError{
				Message:    fmt.Sprintf("invalid sort subquery: %s", err.Error()),
				Suggestion: "Sort subqueries must be valid trait or object queries",
			}
		}

		// Check that subquery contains at least one _ reference
		if !containsSelfRef(spec.SubQuery) {
			return &ValidationError{
				Message:    "sort subquery should contain a _ reference to bind to the current result",
				Suggestion: "Use predicates like on:_, within:_, refs:_, at:_ to relate the subquery to each result. Example: sort:{trait:due within:_}",
			}
		}
	}

	if spec.Path == nil && spec.SubQuery == nil {
		return &ValidationError{
			Message:    "sort spec must have either a path or subquery",
			Suggestion: "Use sort:_.value or sort:{trait:due within:_}",
		}
	}

	return nil
}

// validateGroupSpec validates a group specification.
func (v *Validator) validateGroupSpec(spec *GroupSpec, queryType QueryType) error {
	if spec.Path != nil {
		if err := v.validatePathExpr(spec.Path, queryType); err != nil {
			return &ValidationError{
				Message:    fmt.Sprintf("invalid group path: %s", err.Error()),
				Suggestion: "Valid path steps: _.parent, _.refs:type, _.ancestor:type",
			}
		}
	}

	if spec.SubQuery != nil {
		if err := v.validateQuery(spec.SubQuery); err != nil {
			return &ValidationError{
				Message:    fmt.Sprintf("invalid group subquery: %s", err.Error()),
				Suggestion: "Group subqueries must be valid trait or object queries",
			}
		}

		// Check that subquery contains at least one _ reference
		if !containsSelfRef(spec.SubQuery) {
			return &ValidationError{
				Message:    "group subquery should contain a _ reference to bind to the current result",
				Suggestion: "Use predicates like on:_, within:_, refs:_, refd:_ to relate the subquery to each result. Example: group:{object:project refd:_}",
			}
		}
	}

	if spec.Path == nil && spec.SubQuery == nil {
		return &ValidationError{
			Message:    "group spec must have either a path or subquery",
			Suggestion: "Use group:_.parent or group:{object:project refd:_}",
		}
	}

	return nil
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
		if p.SubQuery != nil && containsSelfRef(p.SubQuery) {
			return true
		}
	case *ContainsPredicate:
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

// validatePathExpr validates a path expression.
func (v *Validator) validatePathExpr(path *PathExpr, queryType QueryType) error {
	if len(path.Steps) == 0 {
		return fmt.Errorf("empty path expression")
	}

	for i, step := range path.Steps {
		switch step.Kind {
		case PathStepValue:
			// _.value is only valid for trait queries
			if queryType != QueryTypeTrait {
				return fmt.Errorf("_.value is only valid for trait queries")
			}
			// value must be the only or last step
			if i != len(path.Steps)-1 {
				return fmt.Errorf("_.value cannot be followed by other path steps")
			}

		case PathStepParent:
			// parent is valid for both, no additional validation needed

		case PathStepAncestor:
			// ancestor requires a type name
			if step.Name == "" {
				return fmt.Errorf("_.ancestor requires a type name (_.ancestor:type)")
			}
			// Validate the type exists
			if _, exists := v.schema.Types[step.Name]; !exists {
				return fmt.Errorf("unknown type '%s' in _.ancestor:%s", step.Name, step.Name)
			}

		case PathStepRefs:
			// refs requires a type name
			if step.Name == "" {
				return fmt.Errorf("_.refs requires a type name (_.refs:type)")
			}
			// Validate the type exists
			if _, exists := v.schema.Types[step.Name]; !exists {
				return fmt.Errorf("unknown type '%s' in _.refs:%s", step.Name, step.Name)
			}

		case PathStepField:
			// Field access - we can't easily validate this because it depends on
			// what object we're accessing (could be parent, ancestor, etc.)
			// For now, just ensure it has a name
			if step.Name == "" {
				return fmt.Errorf("field step requires a field name")
			}
		}
	}

	return nil
}
