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
	dailyDirectory string              // Directory for daily notes (e.g., "daily")
}

// New creates a new Resolver with the given object IDs.
func New(objectIDs []string) *Resolver {
	return NewWithDailyDir(objectIDs, "daily")
}

// NewWithDailyDir creates a new Resolver with the given object IDs and daily directory.
func NewWithDailyDir(objectIDs []string, dailyDirectory string) *Resolver {
	r := &Resolver{
		objectIDs:      make(map[string]struct{}),
		shortMap:       make(map[string][]string),
		slugMap:        make(map[string]string),
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

	return r
}

// ResolverConfig contains configuration for the resolver.
type ResolverConfig struct {
	DailyDirectory string // Directory for daily notes
	ObjectsRoot    string // Root directory for typed objects (e.g., "objects/")
	PagesRoot      string // Root directory for untyped pages (e.g., "pages/")
}

// NewWithConfig creates a new Resolver with full configuration.
// This is the preferred constructor when directory organization is enabled.
func NewWithConfig(objectIDs []string, cfg ResolverConfig) *Resolver {
	dailyDir := cfg.DailyDirectory
	if dailyDir == "" {
		dailyDir = "daily"
	}

	r := &Resolver{
		objectIDs:      make(map[string]struct{}),
		shortMap:       make(map[string][]string),
		slugMap:        make(map[string]string),
		dailyDirectory: dailyDir,
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

	return r
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
func (r *Resolver) Resolve(ref string) ResolveResult {
	ref = strings.TrimSpace(ref)

	// Check if this is a date reference (YYYY-MM-DD)
	if datePattern.MatchString(ref) {
		// Convert date reference to daily note path
		dateID := filepath.Join(r.dailyDirectory, ref)
		if _, ok := r.objectIDs[dateID]; ok {
			return ResolveResult{TargetID: dateID}
		}
		// Date note doesn't exist yet, but it's a valid date reference
		// Return the expected ID (caller can decide whether to create it)
		return ResolveResult{
			TargetID: dateID,
			// Note: This returns a valid target even if the file doesn't exist.
			// The date object is considered to "exist" conceptually.
		}
	}

	// If the ref contains a path separator, treat as full path
	if strings.Contains(ref, "/") || strings.HasPrefix(ref, "#") {
		// Check if it exists exactly
		if _, ok := r.objectIDs[ref]; ok {
			return ResolveResult{TargetID: ref}
		}

		// For embedded refs like "file#id", try without extension
		if strings.Contains(ref, "#") {
			parts := strings.SplitN(ref, "#", 2)
			baseID := strings.TrimSuffix(parts[0], ".md")
			fullID := baseID + "#" + parts[1]
			if _, ok := r.objectIDs[fullID]; ok {
				return ResolveResult{TargetID: fullID}
			}
		}

		// Try slugified match: "people/Sif" -> "people/sif"
		sluggedRef := pages.SlugifyPath(ref)
		if originalID, ok := r.slugMap[sluggedRef]; ok {
			return ResolveResult{TargetID: originalID}
		}

		return ResolveResult{
			Error: "reference not found",
		}
	}

	// Short reference - search for unique match
	matches := r.shortMap[ref]

	if len(matches) == 0 {
		// Try slugified short name
		sluggedRef := pages.Slugify(ref)
		matches = r.shortMap[sluggedRef]
	}

	if len(matches) == 0 {
		// Try to find partial matches (including slugified)
		var partialMatches []string
		sluggedRef := pages.Slugify(ref)
		for id := range r.objectIDs {
			shortName := shortNameFromID(id)
			// Match exact or slugified
			if shortName == ref || shortName == sluggedRef ||
				strings.HasSuffix(id, "/"+ref) || strings.HasSuffix(id, "/"+sluggedRef) {
				partialMatches = append(partialMatches, id)
			}
		}
		matches = partialMatches
	}

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

// AllObjectIDs returns a slice of all known object IDs.
func (r *Resolver) AllObjectIDs() []string {
	ids := make([]string, 0, len(r.objectIDs))
	for id := range r.objectIDs {
		ids = append(ids, id)
	}
	return ids
}
