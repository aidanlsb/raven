package query

import (
	"errors"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

// representativePredicate returns a minimal predicate node for each predKind.
// Legality is decided by predicate kind and query root, not by the predicate's
// internal details, so a bare node is enough to exercise the capability matrix.
func representativePredicate(kind predKind) Predicate {
	switch kind {
	case predKindField:
		return &FieldPredicate{Field: "x", Value: "y", CompareOp: CompareEq}
	case predKindStringFunc:
		return &StringFuncPredicate{FuncType: StringFuncIncludes, Field: "x", Value: "y"}
	case predKindArray:
		return &ArrayQuantifierPredicate{
			Quantifier:  ArrayQuantifierAny,
			Field:       "x",
			ElementPred: &ElementEqualityPredicate{Value: "y", CompareOp: CompareEq},
		}
	case predKindHas:
		return &HasPredicate{SubQuery: &Query{Type: QueryTypeTrait, TypeName: "todo"}}
	case predKindContains:
		return &ContainsPredicate{SubQuery: &Query{Type: QueryTypeTrait, TypeName: "todo"}}
	case predKindIn:
		return &InPredicate{Target: "projects/website"}
	case predKindWithin:
		return &WithinPredicate{Target: "projects/website"}
	case predKindRefs:
		return &RefsPredicate{Target: "projects/website"}
	case predKindLinks:
		return &LinksPredicate{
			LinkPredicate: &FieldPredicate{Field: "ext", Value: "pdf", CompareOp: CompareEq},
		}
	case predKindRefd:
		return &RefdPredicate{Target: "projects/website"}
	case predKindContent:
		return &ContentPredicate{SearchTerm: "term"}
	case predKindValue:
		return &ValuePredicate{Value: "y", CompareOp: CompareEq}
	case predKindAt:
		return &AtPredicate{Target: "x"}
	default:
		return nil
	}
}

var allRoots = []QueryType{QueryTypeObject, QueryTypeTrait, QueryTypeAsset, QueryTypeSection, QueryTypeLink}

var allPredKinds = []predKind{
	predKindField, predKindStringFunc, predKindArray, predKindHas, predKindContains,
	predKindIn, predKindWithin, predKindRefs, predKindLinks, predKindRefd, predKindContent, predKindValue, predKindAt,
}

// TestCapabilityMatrix_LegalityIsSingleSourced verifies that predicateAllowedAtRoot
// (the shared matrix) agrees with the disallowedPredicates table for every
// (root, kind) pair. This is the single legality source both the validator and
// executor consult.
func TestCapabilityMatrix_LegalityIsSingleSourced(t *testing.T) {
	t.Parallel()
	for _, root := range allRoots {
		for _, kind := range allPredKinds {
			pred := representativePredicate(kind)
			if pred == nil {
				t.Fatalf("no representative predicate for kind %d", kind)
			}
			_, disallowed := disallowedPredicates[root][kind]
			verr := predicateAllowedAtRoot(root, pred)
			if disallowed && verr == nil {
				t.Fatalf("root=%v kind=%d: expected matrix to reject but it allowed", root, kind)
			}
			if !disallowed && verr != nil {
				t.Fatalf("root=%v kind=%d: expected matrix to allow but got %v", root, kind, verr)
			}
		}
	}
}

// TestCapabilityMatrix_ExecutorMatchesValidatorForIllegalCombos verifies that,
// for every illegal (root, predicate-kind) pair, both the validator and the
// SQL executor reject the predicate with the exact same ValidationError. This
// guards against the validator and executor drifting apart.
func TestCapabilityMatrix_ExecutorMatchesValidatorForIllegalCombos(t *testing.T) {
	t.Parallel()

	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"note": {Fields: map[string]*schema.FieldDefinition{"x": {Type: schema.FieldTypeString}}},
		},
		Traits: map[string]*schema.TraitDefinition{"todo": {}},
	}
	v := NewValidator(sch)

	for root, rules := range disallowedPredicates {
		for kind, want := range rules {
			pred := representativePredicate(kind)
			if pred == nil {
				t.Fatalf("no representative predicate for kind %d", kind)
			}

			// Executor: legality fires before any SQL builder, so a bare
			// executor (no DB) is sufficient.
			exec := &Executor{}
			_, _, execErr := exec.buildPredicateSQL(root, pred, "x", "note")
			var execVE *ValidationError
			if !errors.As(execErr, &execVE) {
				t.Fatalf("root=%v kind=%d: executor error = %v, want *ValidationError", root, kind, execErr)
			}
			if execVE.Message != want.message || execVE.Suggestion != want.suggestion {
				t.Fatalf("root=%v kind=%d: executor error = %q/%q, want %q/%q",
					root, kind, execVE.Message, execVE.Suggestion, want.message, want.suggestion)
			}

			// Validator: reach the predicate check via a full query.
			q := &Query{Type: root, Predicate: pred}
			switch root {
			case QueryTypeObject:
				q.TypeName = "note"
			case QueryTypeTrait:
				q.TypeName = "todo"
			}
			valErr := v.Validate(q)
			var valVE *ValidationError
			if !errors.As(valErr, &valVE) {
				t.Fatalf("root=%v kind=%d: validator error = %v, want *ValidationError", root, kind, valErr)
			}
			if valVE.Message != want.message || valVE.Suggestion != want.suggestion {
				t.Fatalf("root=%v kind=%d: validator error = %q/%q, want %q/%q",
					root, kind, valVE.Message, valVE.Suggestion, want.message, want.suggestion)
			}
		}
	}
}
