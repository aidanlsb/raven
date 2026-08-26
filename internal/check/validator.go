// Package check handles vault-wide validation.
package check

import (
	"strings"

	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
)

// Validator validates documents against a schema.
type Validator struct {
	schema           *schema.Schema
	resolver         *resolver.Resolver
	allIDs           map[string]struct{}
	objectTypes      map[string]string          // Object ID -> type name (for target type validation)
	aliases          map[string]string          // Alias -> object ID
	duplicateAliases []resolver.AliasCollision  // Aliases used by multiple objects
	missingRefs      map[string]*MissingRef     // Keyed by target path to dedupe
	undefinedTraits  map[string]*UndefinedTrait // Keyed by trait name to dedupe
	usedTypes        map[string]struct{}        // Types actually used in documents
	usedTraits       map[string]struct{}        // Traits actually used in documents
	shortRefs        map[string]string          // Short ref -> full path (for suggestions)
	usedShortNames   map[string]struct{}        // Short names actually used in references
	objectsRoot      string                     // Directory prefix for typed objects (e.g., "objects/")
	pagesRoot        string                     // Directory prefix for untyped pages (e.g., "pages/")
	dailyDir         string                     // Directory prefix for daily notes (e.g., "daily")
}

// ObjectInfo contains basic info about an object for validation.
type ObjectInfo struct {
	ID   string
	Type string
}

// Options configures Validator construction.
type Options struct {
	Schema           *schema.Schema
	ObjectInfos      []ObjectInfo
	Aliases          map[string]string
	Resolver         *resolver.Resolver
	DuplicateAliases []resolver.AliasCollision
	ObjectsRoot      string
	PagesRoot        string
	DailyDir         string
}

// New creates a new validator with the given options.
//
// When Resolver is nil, a resolver is constructed from ObjectInfos and Aliases.
// When Resolver is provided, it is used as-is (for example: index.Database.Resolver).
func New(opts Options) *Validator {
	allIDs := make(map[string]struct{}, len(opts.ObjectInfos))
	objectTypes := make(map[string]string, len(opts.ObjectInfos))
	ids := make([]string, 0, len(opts.ObjectInfos))

	for _, info := range opts.ObjectInfos {
		allIDs[info.ID] = struct{}{}
		objectTypes[info.ID] = info.Type
		ids = append(ids, info.ID)
	}

	res := opts.Resolver
	if res == nil {
		res = resolver.New(ids, resolver.Options{Aliases: opts.Aliases})
	}

	return &Validator{
		schema:           opts.Schema,
		resolver:         res,
		allIDs:           allIDs,
		objectTypes:      objectTypes,
		aliases:          opts.Aliases,
		duplicateAliases: opts.DuplicateAliases,
		missingRefs:      make(map[string]*MissingRef),
		undefinedTraits:  make(map[string]*UndefinedTrait),
		usedTypes:        make(map[string]struct{}),
		usedTraits:       make(map[string]struct{}),
		shortRefs:        make(map[string]string),
		usedShortNames:   make(map[string]struct{}),
		objectsRoot:      paths.NormalizeDirRoot(opts.ObjectsRoot),
		pagesRoot:        paths.NormalizeDirRoot(opts.PagesRoot),
		dailyDir:         strings.TrimSuffix(paths.NormalizeDirRoot(opts.DailyDir), "/"),
	}
}

// SetDailyDirectory updates the resolver's daily directory in place.
//
// The resolver retains its full indexed state (object IDs, aliases,
// alias matches, name-field values, etc.) so callers that started from a
// canonical resolver do not silently lose alias-match or name-field
// resolution when the daily directory changes.
func (v *Validator) SetDailyDirectory(dailyDir string) {
	normalized := strings.TrimSuffix(paths.NormalizeDirRoot(dailyDir), "/")
	v.dailyDir = normalized
	v.resolver.SetDailyDirectory(dailyDir)
}

// displayID returns an object ID suitable for display (with directory prefix stripped).
func (v *Validator) displayID(id string) string {
	if v.objectsRoot != "" && strings.HasPrefix(id, v.objectsRoot) {
		return strings.TrimPrefix(id, v.objectsRoot)
	}
	if v.pagesRoot != "" && strings.HasPrefix(id, v.pagesRoot) {
		return strings.TrimPrefix(id, v.pagesRoot)
	}
	return id
}

// MissingRefs returns all missing references collected during validation.
func (v *Validator) MissingRefs() []*MissingRef {
	refs := make([]*MissingRef, 0, len(v.missingRefs))
	for _, ref := range v.missingRefs {
		refs = append(refs, ref)
	}
	return refs
}

// UndefinedTraits returns all undefined traits collected during validation.
func (v *Validator) UndefinedTraits() []*UndefinedTrait {
	traits := make([]*UndefinedTrait, 0, len(v.undefinedTraits))
	for _, trait := range v.undefinedTraits {
		traits = append(traits, trait)
	}
	return traits
}

// ShortRefs returns the short refs that could be full paths.
func (v *Validator) ShortRefs() map[string]string {
	return v.shortRefs
}
