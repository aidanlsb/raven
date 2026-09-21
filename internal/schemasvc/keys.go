package schemasvc

import (
	"maps"
	"slices"
)

// SortedKeys returns map keys in sorted order. Schema-document planning and
// vault-wide apply both use it so preview/apply iteration cannot drift.
func SortedKeys[V any](values map[string]V) []string {
	return slices.Sorted(maps.Keys(values))
}
