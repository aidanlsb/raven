package query

import "testing"

func TestPredicateSQLRoutingCoversLegalCombos(t *testing.T) {
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
			if leafPredicateSQLBuilder(root, pred) == nil {
				t.Errorf("missing SQL builder for legal combo root=%v kind=%d type=%T", root, kind, pred)
			}
		}
	}
}

func TestPredicateSQLRoutingOmitsIllegalCombos(t *testing.T) {
	t.Parallel()
	for _, root := range allRoots {
		for _, kind := range allPredKinds {
			if _, disallowed := disallowedPredicates[root][kind]; !disallowed {
				continue
			}
			pred := representativePredicate(kind)
			if pred == nil {
				t.Fatalf("no representative predicate for kind %d", kind)
			}
			if leafPredicateSQLBuilder(root, pred) != nil {
				t.Errorf("SQL builder present for illegal combo root=%v kind=%d type=%T", root, kind, pred)
			}
		}
	}
}
