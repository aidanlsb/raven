// Package schemachange detects and classifies schema mutations to determine
// their index invalidation policy.
//
// Schema writes to schema.yaml can alter how every markdown file in the vault
// is indexed without changing file mtimes. This package analyzes schema
// snapshots before and after mutation to choose the appropriate invalidation
// strategy.
package schemachange

import (
	"github.com/aidanlsb/raven/internal/schema"
)

// InvalidationPolicy describes what index recovery work is required after a
// schema mutation completes.
type InvalidationPolicy string

const (
	// PolicyNone means schema metadata changed but no indexed data is affected.
	// Examples: descriptions, comments, schema.version bumps
	PolicyNone InvalidationPolicy = "none"

	// PolicyResolverRefresh means reference resolution or path inference changed
	// but existing markdown parsing projections remain valid.
	// Examples: type default_path changes (affects inference & backlinks)
	PolicyResolverRefresh InvalidationPolicy = "resolver_refresh"

	// PolicyFullScan means existing markdown must be reparsed into the current
	// database schema. All files are re-indexed regardless of mtime.
	// Examples: trait additions/removals, field additions/removals, type changes
	PolicyFullScan InvalidationPolicy = "full_scan"

	// PolicyFullRebuild is reserved for Raven's internal index storage/version
	// changes, not ordinary vault schema edits.
	// Example: SQLite index schema migration (not user schema.yaml changes)
	PolicyFullRebuild InvalidationPolicy = "full_rebuild"
)

// Diff holds before and after schema snapshots for a mutation.
type Diff struct {
	Before *schema.Schema
	After  *schema.Schema
}

// Classification reports the invalidation policy and human-readable reasons.
type Classification struct {
	Policy  InvalidationPolicy
	Reasons []string
}

// Classify determines the index invalidation policy for a schema mutation.
//
// Classification rules (first match wins, descending severity):
//
//  1. Full scan: types added/removed, traits added/removed, fields added/removed,
//     field type/target/values/required changed, trait type/values changed
//  2. Resolver refresh: type default_path changed (affects inference and backlinks)
//  3. None: version change, description changes, template/name_field changes
//
// A nil Before schema is treated as an empty schema (initial schema creation).
// A nil After schema is invalid and returns PolicyFullScan for safety.
func Classify(d Diff) Classification {
	if d.After == nil {
		return Classification{
			Policy:  PolicyFullScan,
			Reasons: []string{"schema removed or unavailable after mutation"},
		}
	}

	if d.Before == nil {
		// Treat missing Before as empty schema (initial setup).
		d.Before = schema.New()
	}

	var reasons []string

	// Check for indexed structural changes (types, traits, fields)
	if typeChanges := detectIndexedTypeChanges(d.Before, d.After); len(typeChanges) > 0 {
		reasons = append(reasons, typeChanges...)
	}
	if traitChanges := detectIndexedTraitChanges(d.Before, d.After); len(traitChanges) > 0 {
		reasons = append(reasons, traitChanges...)
	}
	if len(reasons) > 0 {
		return Classification{Policy: PolicyFullScan, Reasons: reasons}
	}

	// Check for reference resolution changes
	if resolverChanges := detectResolverChanges(d.Before, d.After); len(resolverChanges) > 0 {
		return Classification{Policy: PolicyResolverRefresh, Reasons: resolverChanges}
	}

	// Metadata-only changes (descriptions, templates, etc.) do not affect index
	return Classification{Policy: PolicyNone, Reasons: []string{"metadata-only change"}}
}

func detectIndexedTypeChanges(before, after *schema.Schema) []string {
	var reasons []string

	// Types added
	for typeName := range after.Types {
		if schema.IsBuiltinType(typeName) {
			continue
		}
		if _, exists := before.Types[typeName]; !exists {
			reasons = append(reasons, "type added: "+typeName)
		}
	}

	// Types removed
	for typeName := range before.Types {
		if schema.IsBuiltinType(typeName) {
			continue
		}
		if _, exists := after.Types[typeName]; !exists {
			reasons = append(reasons, "type removed: "+typeName)
		}
	}

	// Field changes within existing types
	for typeName := range after.Types {
		if schema.IsBuiltinType(typeName) {
			continue
		}
		beforeType, beforeExists := before.Types[typeName]
		afterType, afterExists := after.Types[typeName]
		if !beforeExists || !afterExists {
			continue
		}

		// Fields added
		for fieldName := range afterType.Fields {
			if _, exists := beforeType.Fields[fieldName]; !exists {
				reasons = append(reasons, "field added: "+typeName+"."+fieldName)
			}
		}

		// Fields removed
		for fieldName := range beforeType.Fields {
			if _, exists := afterType.Fields[fieldName]; !exists {
				reasons = append(reasons, "field removed: "+typeName+"."+fieldName)
			}
		}

		// Field properties changed (type, required, target, values)
		for fieldName, afterField := range afterType.Fields {
			beforeField, exists := beforeType.Fields[fieldName]
			if !exists || beforeField == nil || afterField == nil {
				continue
			}
			if beforeField.Type != afterField.Type {
				reasons = append(reasons, "field type changed: "+typeName+"."+fieldName)
			}
			if beforeField.Required != afterField.Required {
				reasons = append(reasons, "field required changed: "+typeName+"."+fieldName)
			}
			if beforeField.Target != afterField.Target {
				reasons = append(reasons, "field target changed: "+typeName+"."+fieldName)
			}
			if !stringSliceEqual(beforeField.Values, afterField.Values) {
				reasons = append(reasons, "field values changed: "+typeName+"."+fieldName)
			}
		}
	}

	return reasons
}

func detectIndexedTraitChanges(before, after *schema.Schema) []string {
	var reasons []string

	// Traits added
	for traitName := range after.Traits {
		if _, exists := before.Traits[traitName]; !exists {
			reasons = append(reasons, "trait added: "+traitName)
		}
	}

	// Traits removed
	for traitName := range before.Traits {
		if _, exists := after.Traits[traitName]; !exists {
			reasons = append(reasons, "trait removed: "+traitName)
		}
	}

	// Trait properties changed (type, values)
	for traitName, afterTrait := range after.Traits {
		beforeTrait, exists := before.Traits[traitName]
		if !exists || beforeTrait == nil || afterTrait == nil {
			continue
		}
		if beforeTrait.Type != afterTrait.Type {
			reasons = append(reasons, "trait type changed: "+traitName)
		}
		if !stringSliceEqual(beforeTrait.Values, afterTrait.Values) {
			reasons = append(reasons, "trait values changed: "+traitName)
		}
	}

	return reasons
}

func detectResolverChanges(before, after *schema.Schema) []string {
	var reasons []string

	// Type default_path changes affect inference and backlinks
	for typeName := range after.Types {
		if schema.IsBuiltinType(typeName) {
			continue
		}
		beforeType, beforeExists := before.Types[typeName]
		afterType, afterExists := after.Types[typeName]
		if beforeExists && afterExists && beforeType != nil && afterType != nil {
			if beforeType.DefaultPath != afterType.DefaultPath {
				reasons = append(reasons, "type default_path changed: "+typeName)
			}
		}
	}

	return reasons
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
