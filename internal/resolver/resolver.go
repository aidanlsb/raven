// Package resolver handles reference resolution.
package resolver

import (
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/pages"
)

// Resolver resolves short references to full object IDs.
type Resolver struct {
	objectIDs      map[string]struct{} // Set of all known object IDs
	shortMap       map[string][]string // Map from short name to full IDs
	slugMap        map[string]string   // Map from slugified ID to original ID
	aliasMap       map[string]string   // Map from alias to object ID
	nameFieldMap   map[string][]string // Map from name_field value (slugified) to object IDs
	dailyDirectory string              // Directory for daily notes (e.g., "daily")
}

// New creates a new Resolver with the given object IDs.
func New(objectIDs []string) *Resolver {
	return NewWithDailyDir(objectIDs, "daily")
}

// NewWithDailyDir creates a new Resolver with the given object IDs and daily directory.
func NewWithDailyDir(objectIDs []string, dailyDirectory string) *Resolver {
	return NewWithAliases(objectIDs, nil, dailyDirectory)
}

// NewWithAliases creates a new Resolver with object IDs, aliases, and daily directory.
// The aliases map maps alias strings to their target object IDs.
func NewWithAliases(objectIDs []string, aliases map[string]string, dailyDirectory string) *Resolver {
	return NewWithNameFields(objectIDs, aliases, nil, dailyDirectory)
}

// NewWithNameFields creates a new Resolver with object IDs, aliases, name field values, and daily directory.
// The nameFieldMap maps name_field values to their object IDs (for semantic resolution).
func NewWithNameFields(objectIDs []string, aliases map[string]string, nameFieldMap map[string]string, dailyDirectory string) *Resolver {
	r := &Resolver{
		objectIDs:      make(map[string]struct{}, len(objectIDs)),
		shortMap:       make(map[string][]string, len(objectIDs)),
		slugMap:        make(map[string]string, len(objectIDs)),
		dailyDirectory: dailyDirectory,
	}

	for _, id := range objectIDs {
		r.objectIDs[id] = struct{}{}

		// Build short name map
		shortName := shortNameFromID(id)
		r.shortMap[shortName] = append(r.shortMap[shortName], id)

		// Build slugified map for fuzzy matching
		sluggedID := pages.SlugifyPath(id)
		r.slugMap[sluggedID] = id
	}

	// Copy aliases (skip empty ones)
	if len(aliases) > 0 {
		r.aliasMap = make(map[string]string, len(aliases)*2)
	}
	for alias, targetID := range aliases {
		if alias == "" {
			continue
		}
		r.aliasMap[alias] = targetID
		// Also add slugified version of alias
		sluggedAlias := pages.Slugify(alias)
		if sluggedAlias != "" && sluggedAlias != alias {
			r.aliasMap[sluggedAlias] = targetID
		}
	}

	// Build name_field map (both exact and slugified keys for case-insensitive matching)
	if len(nameFieldMap) > 0 {
		r.nameFieldMap = make(map[string][]string, len(nameFieldMap)*3)
	}
	for nameValue, objectID := range nameFieldMap {
		if nameValue == "" {
			continue
		}
		// Store both exact and slugified versions
		r.nameFieldMap[nameValue] = append(r.nameFieldMap[nameValue], objectID)
		sluggedName := pages.Slugify(nameValue)
		if sluggedName != "" && sluggedName != nameValue {
			r.nameFieldMap[sluggedName] = append(r.nameFieldMap[sluggedName], objectID)
		}
		// Also store lowercase for case-insensitive matching
		lowerName := strings.ToLower(nameValue)
		if lowerName != nameValue && lowerName != sluggedName {
			r.nameFieldMap[lowerName] = append(r.nameFieldMap[lowerName], objectID)
		}
	}

	return r
}

// ResolverConfig contains configuration for the resolver.
type ResolverConfig struct {
	DailyDirectory string            // Directory for daily notes
	ObjectsRoot    string            // Root directory for typed objects (e.g., "objects/")
	PagesRoot      string            // Root directory for untyped pages (e.g., "pages/")
	Aliases        map[string]string // Map from alias to object ID
	NameFieldMap   map[string]string // Map from name_field value to object ID (e.g., "The Prose Edda" -> "books/the-prose-edda")
}

// NewWithConfig creates a new Resolver with full configuration.
// This is the preferred constructor when directory organization is enabled.
func NewWithConfig(objectIDs []string, cfg ResolverConfig) *Resolver {
	dailyDir := cfg.DailyDirectory
	if dailyDir == "" {
		dailyDir = "daily"
	}

	return NewWithNameFields(objectIDs, cfg.Aliases, cfg.NameFieldMap, dailyDir)
}

// ResolveResult represents the result of a reference resolution.
type ResolveResult struct {
	// TargetID is the resolved target object ID (empty if unresolved).
	TargetID string

	// Ambiguous is true if the reference matches multiple objects.
	Ambiguous bool

	// Matches contains all matching IDs (for ambiguous refs).
	Matches []string

	// Error message if resolution failed.
	Error string
}

// Resolve resolves a reference to its target object ID.
// If a reference matches multiple things (alias + object, alias + short name, etc.),
// it is treated as ambiguous and returns an error.
//
// Resolution priority:
//  1. Aliases (exact match)
//  2. Name field values (semantic match by display name)
//  3. Date references (YYYY-MM-DD)
//  4. Object IDs (exact path match)
//  5. Short names (filename match)
func (r *Resolver) Resolve(ref string) ResolveResult {
	ref = strings.TrimSpace(ref)
	sluggedRef := pages.Slugify(ref)
	lowerRef := strings.ToLower(ref)

	// Collect all possible matches
	var matches []string
	matchSources := make(map[string]string) // id -> source (for debugging)

	// Check aliases (exact and slugified)
	if targetID, ok := r.aliasMap[ref]; ok {
		matches = append(matches, targetID)
		matchSources[targetID] = "alias"
	} else if targetID, ok := r.aliasMap[sluggedRef]; ok {
		matches = append(matches, targetID)
		matchSources[targetID] = "alias"
	}

	// Check name_field values (semantic matching by display name)
	// This allows [[The Prose Edda]] to resolve even if the file is the-prose-edda.md
	if nameMatches, ok := r.nameFieldMap[ref]; ok {
		for _, id := range nameMatches {
			if _, exists := matchSources[id]; !exists {
				matches = append(matches, id)
				matchSources[id] = "name_field"
			}
		}
	} else if nameMatches, ok := r.nameFieldMap[sluggedRef]; ok {
		for _, id := range nameMatches {
			if _, exists := matchSources[id]; !exists {
				matches = append(matches, id)
				matchSources[id] = "name_field"
			}
		}
	} else if nameMatches, ok := r.nameFieldMap[lowerRef]; ok {
		for _, id := range nameMatches {
			if _, exists := matchSources[id]; !exists {
				matches = append(matches, id)
				matchSources[id] = "name_field"
			}
		}
	}

	// Check if this is a date reference (YYYY-MM-DD)
	if dates.IsValidDate(ref) {
		// Convert date reference to daily note path
		dateID := filepath.Join(r.dailyDirectory, ref)
		// Date references are special - they always resolve to the daily note path
		// Don't treat as ambiguous with aliases since dates are a distinct concept
		if len(matches) == 0 {
			return ResolveResult{
				TargetID: dateID,
			}
		}
		// If there's an alias that matches a date pattern, that's ambiguous
		if _, exists := matchSources[dateID]; !exists {
			matches = append(matches, dateID)
			matchSources[dateID] = "date"
		}
	}

	// If the ref contains a path separator, treat as full path
	if strings.Contains(ref, "/") || strings.HasPrefix(ref, "#") {
		// Check if it exists exactly
		if _, ok := r.objectIDs[ref]; ok {
			if _, exists := matchSources[ref]; !exists {
				matches = append(matches, ref)
				matchSources[ref] = "object_id"
			}
		}

		// For embedded refs like "file#id", try without extension
		if strings.Contains(ref, "#") {
			parts := strings.SplitN(ref, "#", 2)
			baseID := strings.TrimSuffix(parts[0], ".md")
			fullID := baseID + "#" + parts[1]
			if _, ok := r.objectIDs[fullID]; ok {
				if _, exists := matchSources[fullID]; !exists {
					matches = append(matches, fullID)
					matchSources[fullID] = "object_id"
				}
			}
		}

		// Try slugified match: "people/Sif" -> "people/sif"
		sluggedRefPath := pages.SlugifyPath(ref)
		if originalID, ok := r.slugMap[sluggedRefPath]; ok {
			if _, exists := matchSources[originalID]; !exists {
				matches = append(matches, originalID)
				matchSources[originalID] = "object_id"
			}
		}

		// Try suffix matching: "companies/cursor" -> "objects/companies/cursor"
		// This handles cases where a directories.objects prefix is used
		if len(matches) == 0 {
			suffix := "/" + ref
			sluggedSuffix := "/" + sluggedRefPath
			for id := range r.objectIDs {
				if strings.HasSuffix(id, suffix) || strings.HasSuffix(id, sluggedSuffix) {
					if _, exists := matchSources[id]; !exists {
						matches = append(matches, id)
						matchSources[id] = "suffix_match"
					}
				}
			}
		}
	} else {
		// Short reference - search for matches
		shortMatches := r.shortMap[ref]
		if len(shortMatches) == 0 {
			shortMatches = r.shortMap[sluggedRef]
		}

		if len(shortMatches) == 0 {
			// Try to find partial matches (including slugified)
			for id := range r.objectIDs {
				shortName := shortNameFromID(id)
				if shortName == ref || shortName == sluggedRef ||
					strings.HasSuffix(id, "/"+ref) || strings.HasSuffix(id, "/"+sluggedRef) {
					shortMatches = append(shortMatches, id)
				}
			}
		}

		// Add short name matches (avoiding duplicates)
		for _, id := range shortMatches {
			if _, exists := matchSources[id]; !exists {
				matches = append(matches, id)
				matchSources[id] = "short_name"
			}
		}
	}

	// If we have multiple matches, try to disambiguate by preferring parent objects
	// over their sections. E.g., if we match both "companies/cursor" and
	// "companies/cursor#cursor", prefer the parent "companies/cursor".
	if len(matches) > 1 {
		matches = preferParentOverSections(matches)
	}

	// Return result based on number of unique matches
	switch len(matches) {
	case 0:
		return ResolveResult{
			Error: "reference not found",
		}
	case 1:
		return ResolveResult{TargetID: matches[0]}
	default:
		return ResolveResult{
			Ambiguous: true,
			Matches:   matches,
			Error:     "ambiguous reference, multiple matches found",
		}
	}
}

// preferParentOverSections filters out section matches when their parent file
// also matches. For example, if both "companies/cursor" and "companies/cursor#cursor"
// match, only "companies/cursor" is returned.
func preferParentOverSections(matches []string) []string {
	// Build a set of parent IDs (non-section matches)
	parents := make(map[string]bool)
	for _, id := range matches {
		if !strings.Contains(id, "#") {
			parents[id] = true
		}
	}

	// If we have no parents, return all matches as-is
	if len(parents) == 0 {
		return matches
	}

	// Filter: keep non-sections, and only keep sections if their parent isn't matched
	var filtered []string
	for _, id := range matches {
		if !strings.Contains(id, "#") {
			// Keep parent objects
			filtered = append(filtered, id)
		} else {
			// Only keep section if its parent file isn't also a match
			parts := strings.SplitN(id, "#", 2)
			parentID := parts[0]
			if !parents[parentID] {
				filtered = append(filtered, id)
			}
		}
	}

	return filtered
}

// Exists checks if an object ID exists.
func (r *Resolver) Exists(id string) bool {
	_, ok := r.objectIDs[id]
	return ok
}

// shortNameFromID extracts the short name from an object ID.
// For "people/freya" -> "freya"
// For "daily/2025-02-01#standup" -> "standup"
func shortNameFromID(id string) string {
	// Handle embedded IDs
	if idx := strings.LastIndex(id, "#"); idx >= 0 {
		return id[idx+1:]
	}

	// Get filename without path
	base := filepath.Base(id)
	return strings.TrimSuffix(base, ".md")
}

// ResolveAll resolves all references and returns a map from raw ref to result.
func (r *Resolver) ResolveAll(refs []string) map[string]ResolveResult {
	results := make(map[string]ResolveResult, len(refs))
	for _, ref := range refs {
		results[ref] = r.Resolve(ref)
	}
	return results
}

// IDCollision represents a collision between object IDs with the same short name.
type IDCollision struct {
	ShortName string   // The short name that collides (e.g., "freya")
	ObjectIDs []string // The full object IDs that share this short name
}

// FindCollisions finds object IDs that share the same short name.
// This is useful for `rvn check` to warn about potential reference ambiguity.
//
// Note: Collisions between a file and sections within that same file are NOT
// reported, since these resolve unambiguously (the file takes precedence).
// For example, `people/freya` and `people/freya#freya` sharing short name "freya"
// is fine - [[freya]] resolves to the file.
func (r *Resolver) FindCollisions() []IDCollision {
	var collisions []IDCollision
	for shortName, ids := range r.shortMap {
		if len(ids) > 1 {
			// Filter out "false" collisions where a file collides only with
			// sections within that same file
			if isFileSectionCollisionOnly(ids) {
				continue
			}
			collisions = append(collisions, IDCollision{
				ShortName: shortName,
				ObjectIDs: ids,
			})
		}
	}
	return collisions
}

// isFileSectionCollisionOnly returns true if all the colliding IDs are:
// - One parent file, and
// - Sections within that same file
// This is not a real collision because the file takes precedence.
func isFileSectionCollisionOnly(ids []string) bool {
	// Find the parent file(s) - IDs without "#"
	var parentFiles []string
	for _, id := range ids {
		if !strings.Contains(id, "#") {
			parentFiles = append(parentFiles, id)
		}
	}

	// Must have exactly one parent file for this to be a non-collision
	if len(parentFiles) != 1 {
		return false
	}

	parentFile := parentFiles[0]

	// All section IDs must belong to this parent file
	for _, id := range ids {
		if strings.Contains(id, "#") {
			// Extract parent from section ID
			parts := strings.SplitN(id, "#", 2)
			if parts[0] != parentFile {
				// Section belongs to a different file - real collision
				return false
			}
		}
	}

	// All sections belong to the single parent file - not a real collision
	return true
}

// AliasCollision represents a collision where an alias conflicts with something else.
type AliasCollision struct {
	Alias         string   // The alias that collides
	ObjectIDs     []string // Object IDs that share this alias (if multiple objects use same alias)
	ConflictsWith string   // What it conflicts with: "alias", "short_name", or "object_id"
}

// FindAliasCollisions finds alias conflicts:
// 1. Multiple objects using the same alias
// 2. An alias that matches an existing object's short name
// 3. An alias that matches an existing object ID
func (r *Resolver) FindAliasCollisions() []AliasCollision {
	var collisions []AliasCollision

	// Build reverse map: alias -> list of object IDs that use it
	// (aliasMap only stores one, but we need to detect duplicates at index time)
	// For now, we can detect conflicts with short names and object IDs

	for alias, targetID := range r.aliasMap {
		// Check if alias matches any short name (other than the target's own short name)
		if shortMatches, ok := r.shortMap[alias]; ok {
			// Filter out the target object itself
			var conflicts []string
			for _, id := range shortMatches {
				if id != targetID {
					conflicts = append(conflicts, id)
				}
			}
			if len(conflicts) > 0 {
				collisions = append(collisions, AliasCollision{
					Alias:         alias,
					ObjectIDs:     append([]string{targetID}, conflicts...),
					ConflictsWith: "short_name",
				})
			}
		}

		// Check if alias matches any object ID exactly
		if _, exists := r.objectIDs[alias]; exists && alias != targetID {
			collisions = append(collisions, AliasCollision{
				Alias:         alias,
				ObjectIDs:     []string{targetID, alias},
				ConflictsWith: "object_id",
			})
		}
	}

	return collisions
}

// AllObjectIDs returns a slice of all known object IDs.
func (r *Resolver) AllObjectIDs() []string {
	ids := make([]string, 0, len(r.objectIDs))
	for id := range r.objectIDs {
		ids = append(ids, id)
	}
	return ids
}
