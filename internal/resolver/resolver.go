// Package resolver handles reference resolution.
package resolver

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aidanlsb/raven/internal/pages"
)

// datePattern matches YYYY-MM-DD date format
var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Resolver resolves short references to full object IDs.
type Resolver struct {
	objectIDs      map[string]struct{} // Set of all known object IDs
	shortMap       map[string][]string // Map from short name to full IDs
	slugMap        map[string]string   // Map from slugified ID to original ID
	aliasMap       map[string]string   // Map from alias to object ID
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
	r := &Resolver{
		objectIDs:      make(map[string]struct{}),
		shortMap:       make(map[string][]string),
		slugMap:        make(map[string]string),
		aliasMap:       make(map[string]string),
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

	return r
}

// ResolverConfig contains configuration for the resolver.
type ResolverConfig struct {
	DailyDirectory string            // Directory for daily notes
	ObjectsRoot    string            // Root directory for typed objects (e.g., "objects/")
	PagesRoot      string            // Root directory for untyped pages (e.g., "pages/")
	Aliases        map[string]string // Map from alias to object ID
}

// NewWithConfig creates a new Resolver with full configuration.
// This is the preferred constructor when directory organization is enabled.
func NewWithConfig(objectIDs []string, cfg ResolverConfig) *Resolver {
	dailyDir := cfg.DailyDirectory
	if dailyDir == "" {
		dailyDir = "daily"
	}

	return NewWithAliases(objectIDs, cfg.Aliases, dailyDir)
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
func (r *Resolver) Resolve(ref string) ResolveResult {
	ref = strings.TrimSpace(ref)
	sluggedRef := pages.Slugify(ref)

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

	// Check if this is a date reference (YYYY-MM-DD)
	if datePattern.MatchString(ref) {
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
func (r *Resolver) FindCollisions() []IDCollision {
	var collisions []IDCollision
	for shortName, ids := range r.shortMap {
		if len(ids) > 1 {
			collisions = append(collisions, IDCollision{
				ShortName: shortName,
				ObjectIDs: ids,
			})
		}
	}
	return collisions
}

// AliasCollision represents a collision where an alias conflicts with something else.
type AliasCollision struct {
	Alias       string   // The alias that collides
	ObjectIDs   []string // Object IDs that share this alias (if multiple objects use same alias)
	ConflictsWith string // What it conflicts with: "alias", "short_name", or "object_id"
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
