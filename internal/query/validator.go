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
	// Navigation functions like refs(_), refd(_) are always connected, but only support count().
	if s.NavFunc != nil {
		if s.AggField != "" {
			return &ValidationError{
				Message:    fmt.Sprintf("count() does not accept a field argument (got .%s) for navigation function %s(_)", s.AggField, s.NavFunc.Name),
				Suggestion: fmt.Sprintf("Use %s = count(%s(_))", s.Name, s.NavFunc.Name),
			}
		}
		if s.Aggregation != AggCount {
			return &ValidationError{
				Message:    fmt.Sprintf("navigation functions only support count(), not %s()", aggNameForType(s.Aggregation)),
				Suggestion: fmt.Sprintf("Use %s = count(%s(_))", s.Name, s.NavFunc.Name),
			}
		}
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

		// Aggregation-specific typing rules
		switch s.Aggregation {
		case AggCount:
			// count({subquery}) should not take a field arg
			if s.AggField != "" {
				return &ValidationError{
					Message:    fmt.Sprintf("count() does not accept a field argument (got .%s)", s.AggField),
					Suggestion: "Use count({object:...}) or count({trait:...}) without a field argument",
				}
			}
		case AggMin, AggMax, AggSum:
			if s.AggField == "" {
				return &ValidationError{
					Message:    fmt.Sprintf("%s() requires a field argument", aggNameForType(s.Aggregation)),
					Suggestion: "Use min(.field, {object:...}) or min(.value, {trait:...}) (similarly for max/sum)",
				}
			}

			switch s.SubQuery.Type {
			case QueryTypeTrait:
				// Trait aggregates only support .value
				if s.AggField != "value" {
					return &ValidationError{
						Message:    fmt.Sprintf("%s() on trait subqueries only supports .value (got .%s)", aggNameForType(s.Aggregation), s.AggField),
						Suggestion: fmt.Sprintf("Use %s(.value, {trait:%s ...})", aggNameForType(s.Aggregation), s.SubQuery.TypeName),
					}
				}
				td := v.schema.Traits[s.SubQuery.TypeName]
				if td == nil || td.IsBoolean() {
					return &ValidationError{
						Message:    fmt.Sprintf("cannot use %s() on boolean trait '%s' (it has no value)", aggNameForType(s.Aggregation), s.SubQuery.TypeName),
						Suggestion: "Use count({trait:...}) instead, or choose a valued trait",
					}
				}
				if s.Aggregation == AggSum {
					if td.Type != schema.FieldTypeNumber {
						return &ValidationError{
							Message:    fmt.Sprintf("sum() on trait subqueries requires a numeric trait (trait '%s' is type '%s')", s.SubQuery.TypeName, td.Type),
							Suggestion: "Use a numeric trait, or use count()/min()/max() instead",
						}
					}
				}

			case QueryTypeObject:
				typeDef := v.schema.Types[s.SubQuery.TypeName]
				if typeDef == nil || typeDef.Fields == nil {
					return &ValidationError{
						Message:    fmt.Sprintf("type '%s' has no defined fields (cannot aggregate .%s)", s.SubQuery.TypeName, s.AggField),
						Suggestion: fmt.Sprintf("Add the field to type '%s' in schema.yaml, or choose an existing field", s.SubQuery.TypeName),
					}
				}
				fd, ok := typeDef.Fields[s.AggField]
				if !ok || fd == nil {
					available := v.availableFields(typeDef)
					return &ValidationError{
						Message:    fmt.Sprintf("type '%s' has no field '%s' for %s()", s.SubQuery.TypeName, s.AggField, aggNameForType(s.Aggregation)),
						Suggestion: fmt.Sprintf("Available fields: %s", strings.Join(available, ", ")),
					}
				}
				if !isAllowedAggregateFieldType(fd.Type, s.Aggregation) {
					return &ValidationError{
						Message:    fmt.Sprintf("cannot use %s() on field '%s.%s' of type '%s'", aggNameForType(s.Aggregation), s.SubQuery.TypeName, s.AggField, fd.Type),
						Suggestion: "Use count() for non-scalar fields, or aggregate a comparable scalar (string/number/date/datetime/enum) and sum() only on number",
					}
				}
			}
		}
	}

	return nil
}

func aggNameForType(a AggregationType) string {
	switch a {
	case AggMin:
		return "min"
	case AggMax:
		return "max"
	case AggSum:
		return "sum"
	default:
		return "count"
	}
}

func isAllowedAggregateFieldType(ft schema.FieldType, agg AggregationType) bool {
	// Disallow arrays for all aggregates (including min/max) - ambiguous semantics.
	if strings.HasSuffix(string(ft), "[]") {
		return false
	}
	// Disallow refs and booleans
	switch ft {
	case schema.FieldTypeRef, schema.FieldTypeBool:
		return false
	}
	// sum requires number
	if agg == AggSum {
		return ft == schema.FieldTypeNumber
	}
	// min/max allow comparable scalars (including enum)
	switch ft {
	case schema.FieldTypeString, schema.FieldTypeNumber, schema.FieldTypeDate, schema.FieldTypeDatetime, schema.FieldTypeEnum:
		return true
	default:
		return false
	}
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
		// ValuePredicate is deprecated; the parser now uses FieldPredicate with Field="value"
		return &ValidationError{
			Message:    "value predicate is only valid for trait queries",
			Suggestion: "Use .value==X in trait queries, or use .field==X for object fields",
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
