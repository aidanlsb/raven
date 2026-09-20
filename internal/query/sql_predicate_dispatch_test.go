package query

import (
	"reflect"
	"testing"
)

func TestPredicateBuilderRegistryCoversLegalCombos(t *testing.T) {
	t.Parallel()
	for _, root := range allRoots {
		for _, kind := range allPredKinds {
			if _, disallowed := disallowedPredicates[root][kind]; disallowed {
				continue
			}
			pred := representativePredicate(kind)
			if pred == nil {
				t.Fatalf("no representative predicate for kind %d", kind)
			}
			key := predicateBuilderKey{predType: reflect.TypeOf(pred), root: root}
			if _, ok := predicateBuilderRegistry[key]; !ok {
				t.Errorf("missing SQL builder for legal combo root=%v kind=%d type=%T", root, kind, pred)
			}
		}
	}
}
