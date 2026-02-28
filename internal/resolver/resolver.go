// Package resolver handles reference resolution.
package resolver

import (
	"path"
	"strings"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/paths"
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

// Options configures the resolver.
type Options struct {
	// DailyDirectory is the directory for daily notes (default: "daily").
	DailyDirectory string

	// Aliases maps alias strings to their target object IDs.
	// For example: {"The Queen": "people/freya"}
	Aliases map[string]string

	// NameFieldMap maps name_field values to object IDs for semantic resolution.
	// For example: {"The Prose Edda": "books/the-prose-edda"}
	NameFieldMap map[string]string
}

// New creates a new Resolver with the given object IDs and options.
func New(objectIDs []string, opts Options) *Resolver {
	dailyDir := opts.DailyDirectory
	if dailyDir == "" {
		dailyDir = "daily"
	}

	r := &Resolver{
		objectIDs:      make(map[string]struct{}, len(objectIDs)),
		shortMap:       make(map[string][]string, len(objectIDs)),
		slugMap:        make(map[string]string, len(objectIDs)),
		dailyDirectory: dailyDir,
	}

	for _, id := range objectIDs {
		r.objectIDs[id] = struct{}{}

		// Build short name map
		shortName := paths.ShortNameFromID(id)
		r.shortMap[shortName] = append(r.shortMap[shortName], id)

		// Build slugified map for fuzzy matching
		sluggedID := pages.SlugifyPath(id)
		r.slugMap[sluggedID] = id
	}

	// Copy aliases (skip empty ones)
	if len(opts.Aliases) > 0 {
		r.aliasMap = make(map[string]string, len(opts.Aliases)*2)
	}
	for alias, targetID := range opts.Aliases {
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
	if len(opts.NameFieldMap) > 0 {
		r.nameFieldMap = make(map[string][]string, len(opts.NameFieldMap)*3)
	}
	for nameValue, objectID := range opts.NameFieldMap {
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

// ResolveResult represents the result of a reference resolution.
type ResolveResult struct {
	// TargetID is the resolved target object ID (empty if unresolved).
	TargetID string

	// Ambiguous is true if the reference matches multiple objects.
	Ambiguous bool

	// Matches contains all matching IDs (for ambiguous refs).
	Matches []string

	// MatchSources maps matched IDs to their match source.
	MatchSources map[string]string

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

	c := newMatchCollector()

	addAliasMatches(r, c, ref, sluggedRef)
	addNameFieldMatches(r, c, ref, sluggedRef, lowerRef)

	// Date references are special - they always resolve to the daily note path
	// unless there are already other matches, in which case they participate
	// in ambiguity detection.
	if res, done := maybeResolveDateRef(r, c, ref); done {
		return res
	}

	if isPathLikeRef(ref) {
		addPathMatches(r, c, ref)
	} else {
		addShortMatches(r, c, ref, sluggedRef)
	}

	matches := c.matches

	// If we have multiple matches, try to disambiguate by preferring parent objects
	// over their sections. E.g., if we match both "companies/cursor" and
	// "companies/cursor#cursor", prefer the parent "companies/cursor".
	if len(matches) > 1 {
		matches = preferParentOverSections(matches)
	}

	matchSources := filterMatchSources(c.sources, matches)
	return buildResolveResult(matches, matchSources)
}

type matchCollector struct {
	matches []string
	sources map[string]string // id -> source (for debugging)
}

func newMatchCollector() *matchCollector {
	return &matchCollector{
		sources: make(map[string]string),
	}
}

func (c *matchCollector) add(id, source string) {
	if id == "" {
		return
	}
	if _, exists := c.sources[id]; exists {
		return
	}
	c.matches = append(c.matches, id)
	c.sources[id] = source
}

func addAliasMatches(r *Resolver, c *matchCollector, ref, sluggedRef string) {
	// Check aliases (exact and slugified)
	if targetID, ok := r.aliasMap[ref]; ok {
		c.add(targetID, "alias")
	} else if targetID, ok := r.aliasMap[sluggedRef]; ok {
		c.add(targetID, "alias")
	}
}

func addNameFieldMatches(r *Resolver, c *matchCollector, ref, sluggedRef, lowerRef string) {
	// Check name_field values (semantic matching by display name)
	// This allows [[The Prose Edda]] to resolve even if the file is the-prose-edda.md
	var nameMatches []string
	if m, ok := r.nameFieldMap[ref]; ok {
		nameMatches = m
	} else if m, ok := r.nameFieldMap[sluggedRef]; ok {
		nameMatches = m
	} else if m, ok := r.nameFieldMap[lowerRef]; ok {
		nameMatches = m
	}
	for _, id := range nameMatches {
		c.add(id, "name_field")
	}
}

func maybeResolveDateRef(r *Resolver, c *matchCollector, ref string) (ResolveResult, bool) {
	// Check if this is a date reference (YYYY-MM-DD)
	if !dates.IsValidDate(ref) {
		return ResolveResult{}, false
	}

	// Convert date reference to daily note path
	dateID := path.Join(r.dailyDirectory, ref)

	// Date references are special - they always resolve to the daily note path
	// Don't treat as ambiguous with aliases since dates are a distinct concept
	if len(c.matches) == 0 {
		return ResolveResult{
			TargetID:     dateID,
			MatchSources: map[string]string{dateID: "date"},
		}, true
	}

	// If there's an alias that matches a date pattern, that's ambiguous
	c.add(dateID, "date")
	return ResolveResult{}, false
}

func isPathLikeRef(ref string) bool {
	// Treat embedded refs like "file#section" as path-like so they can resolve via
	// slugified/suffix matching and embedded-ID logic.
	return strings.Contains(ref, "/") || strings.HasPrefix(ref, "#") || strings.Contains(ref, "#")
}

func addPathMatches(r *Resolver, c *matchCollector, ref string) {
	// Check if it exists exactly
	if _, ok := r.objectIDs[ref]; ok {
		c.add(ref, "object_id")
	}

	// For embedded refs like "file#id", try without extension
	if baseID, fragment, isEmbedded := paths.ParseEmbeddedID(ref); isEmbedded {
		baseID = strings.TrimSuffix(baseID, ".md")
		fullID := baseID + "#" + fragment
		if _, ok := r.objectIDs[fullID]; ok {
			c.add(fullID, "object_id")
		}
	}

	// Try slugified match: "people/Sif" -> "people/sif"
	sluggedRefPath := pages.SlugifyPath(ref)
	if originalID, ok := r.slugMap[sluggedRefPath]; ok {
		c.add(originalID, "object_id")
	}

	// Try suffix matching: "companies/cursor" -> "objects/companies/cursor"
	// This handles cases where a directories.objects prefix is used
	if len(c.matches) == 0 {
		suffix := "/" + ref
		sluggedSuffix := "/" + sluggedRefPath
		for id := range r.objectIDs {
			if strings.HasSuffix(id, suffix) || strings.HasSuffix(id, sluggedSuffix) {
				c.add(id, "suffix_match")
			}
		}
	}
}

func addShortMatches(r *Resolver, c *matchCollector, ref, sluggedRef string) {
	// Short reference - search for matches
	shortMatches := r.shortMap[ref]
	if len(shortMatches) == 0 {
		shortMatches = r.shortMap[sluggedRef]
	}

	if len(shortMatches) == 0 {
		// Try to find partial matches (including slugified)
		for id := range r.objectIDs {
			shortName := paths.ShortNameFromID(id)
			if shortName == ref || shortName == sluggedRef ||
				strings.HasSuffix(id, "/"+ref) || strings.HasSuffix(id, "/"+sluggedRef) {
				shortMatches = append(shortMatches, id)
			}
		}
	}

	// Add short name matches (avoiding duplicates)
	for _, id := range shortMatches {
		c.add(id, "short_name")
	}
}

func buildResolveResult(matches []string, matchSources map[string]string) ResolveResult {
	// Return result based on number of unique matches
	switch len(matches) {
	case 0:
		return ResolveResult{
			Error: "reference not found",
		}
	case 1:
		return ResolveResult{
			TargetID:     matches[0],
			MatchSources: matchSources,
		}
	default:
		return ResolveResult{
			Ambiguous:    true,
			Matches:      matches,
			MatchSources: matchSources,
			Error:        "ambiguous reference, multiple matches found",
		}
	}
}

func filterMatchSources(matchSources map[string]string, matches []string) map[string]string {
	if matchSources == nil || len(matches) == 0 {
		return nil
	}
	filtered := make(map[string]string, len(matches))
	for _, match := range matches {
		if source, ok := matchSources[match]; ok {
			filtered[match] = source
		}
	}
	return filtered
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
		parentID, _, isEmbedded := paths.ParseEmbeddedID(id)
		if !isEmbedded {
			// Keep parent objects
			filtered = append(filtered, id)
		} else {
			// Only keep section if its parent file isn't also a match
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
		if parent, _, isEmbedded := paths.ParseEmbeddedID(id); isEmbedded {
			if parent != parentFile {
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
