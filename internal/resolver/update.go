package resolver

import (
	"strings"

	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/slugs"
)

// Update describes resolver-index entries added and removed by an index
// mutation. It lets a long-lived resolver stay current without rebuilding its
// vault-wide maps after every changed file.
type Update struct {
	AddedObjectIDs   []string
	RemovedObjectIDs []string

	AddedAliases   map[string][]string
	RemovedAliases map[string][]string

	AddedNameFields   map[string][]string
	RemovedNameFields map[string][]string
}

// ApplyUpdate incrementally applies index changes to the resolver.
func (r *Resolver) ApplyUpdate(update Update) {
	if r == nil {
		return
	}

	for value, ids := range update.RemovedAliases {
		for _, id := range ids {
			r.removeAliasTarget(value, id)
		}
	}
	for value, ids := range update.RemovedNameFields {
		for _, id := range ids {
			r.removeNameFieldTarget(value, id)
		}
	}
	for _, id := range update.RemovedObjectIDs {
		r.removeObjectID(id)
	}

	for _, id := range update.AddedObjectIDs {
		r.addObjectID(id)
	}
	for value, ids := range update.AddedAliases {
		for _, id := range ids {
			addAliasTarget(r.aliasMap, value, id)
		}
	}
	for value, ids := range update.AddedNameFields {
		for _, id := range ids {
			r.addNameFieldTarget(value, id)
		}
	}
}

func (r *Resolver) addObjectID(id string) {
	if id == "" {
		return
	}
	if _, exists := r.objectIDs[id]; exists {
		return
	}
	r.addCommonID(id)
}

func (r *Resolver) removeObjectID(id string) {
	if _, exists := r.objectIDs[id]; !exists {
		return
	}
	r.removeCommonID(id)
}

func (r *Resolver) addCommonID(id string) {
	r.objectIDs[id] = struct{}{}

	shortName := paths.ShortNameFromID(id)
	addShortMapEntry(r.shortMap, shortName, id)

	sluggedID := slugs.PathSlug(id)
	r.slugMatches[sluggedID] = insertSortedUnique(r.slugMatches[sluggedID], id)
	r.slugMap[sluggedID] = r.slugMatches[sluggedID][len(r.slugMatches[sluggedID])-1]

	indexResolverSuffixes(r.suffixMap, id)
}

func (r *Resolver) removeCommonID(id string) {
	delete(r.objectIDs, id)

	shortName := paths.ShortNameFromID(id)
	removeMapEntry(r.shortMap, shortName, id)

	sluggedID := slugs.PathSlug(id)
	r.slugMatches[sluggedID] = removeSortedValue(r.slugMatches[sluggedID], id)
	if len(r.slugMatches[sluggedID]) == 0 {
		delete(r.slugMatches, sluggedID)
		delete(r.slugMap, sluggedID)
	} else {
		r.slugMap[sluggedID] = r.slugMatches[sluggedID][len(r.slugMatches[sluggedID])-1]
	}

	removeResolverSuffixes(r.suffixMap, id)
}

func removeResolverSuffixes(suffixMap map[string][]string, id string) {
	remaining := id
	for {
		slash := strings.IndexByte(remaining, '/')
		if slash == -1 || slash+1 >= len(remaining) {
			return
		}
		remaining = remaining[slash+1:]
		removeMapEntry(suffixMap, "/"+remaining, id)
		sluggedSuffix := "/" + slugs.PathSlug(remaining)
		if sluggedSuffix != "/"+remaining {
			removeMapEntry(suffixMap, sluggedSuffix, id)
		}
	}
}

func (r *Resolver) removeAliasTarget(alias, targetID string) {
	if alias == "" || targetID == "" {
		return
	}
	removeMapEntry(r.aliasMap, alias, targetID)
	sluggedAlias := slugs.ComponentSlug(alias)
	if sluggedAlias != "" && sluggedAlias != alias {
		removeMapEntry(r.aliasMap, sluggedAlias, targetID)
	}
}

func (r *Resolver) addNameFieldTarget(nameValue, objectID string) {
	if nameValue == "" || objectID == "" {
		return
	}
	for _, key := range nameFieldKeys(nameValue) {
		r.nameFieldMap[key] = insertSortedUnique(r.nameFieldMap[key], objectID)
	}
}

func (r *Resolver) removeNameFieldTarget(nameValue, objectID string) {
	if nameValue == "" || objectID == "" {
		return
	}
	for _, key := range nameFieldKeys(nameValue) {
		removeMapEntry(r.nameFieldMap, key, objectID)
	}
}

func nameFieldKeys(nameValue string) []string {
	keys := []string{nameValue}
	sluggedName := slugs.ComponentSlug(nameValue)
	if sluggedName != "" && sluggedName != nameValue {
		keys = append(keys, sluggedName)
	}
	lowerName := strings.ToLower(nameValue)
	if lowerName != nameValue && lowerName != sluggedName {
		keys = append(keys, lowerName)
	}
	return keys
}

func removeMapEntry(entries map[string][]string, key, id string) {
	values := removeSortedValue(entries[key], id)
	if len(values) == 0 {
		delete(entries, key)
		return
	}
	entries[key] = values
}

func removeSortedValue(values []string, target string) []string {
	for i, value := range values {
		if value != target {
			continue
		}
		copy(values[i:], values[i+1:])
		return values[:len(values)-1]
	}
	return values
}
