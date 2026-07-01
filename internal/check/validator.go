// Package check handles vault-wide validation.
package check

import (
	"strings"

	"github.com/aidanlsb/raven/internal/index"
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
	duplicateAliases []index.DuplicateAlias     // Aliases used by multiple objects
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

// NewValidator creates a new validator.
func NewValidator(s *schema.Schema, objectIDs []string) *Validator {
	return NewValidatorWithAliases(s, objectIDs, nil)
}

// NewValidatorWithAliases creates a new validator with alias support.
func NewValidatorWithAliases(s *schema.Schema, objectIDs []string, aliases map[string]string) *Validator {
	allIDs := make(map[string]struct{}, len(objectIDs))
	for _, id := range objectIDs {
		allIDs[id] = struct{}{}
	}

	return &Validator{
		schema:          s,
		resolver:        resolver.New(objectIDs, resolver.Options{Aliases: aliases}),
		allIDs:          allIDs,
		objectTypes:     make(map[string]string),
		aliases:         aliases,
		missingRefs:     make(map[string]*MissingRef),
		undefinedTraits: make(map[string]*UndefinedTrait),
		usedTypes:       make(map[string]struct{}),
		usedTraits:      make(map[string]struct{}),
		shortRefs:       make(map[string]string),
		usedShortNames:  make(map[string]struct{}),
	}
}

// NewValidatorWithTypes creates a new validator with object type information.
// objectInfos should contain ID and type for each object in the vault.
func NewValidatorWithTypes(s *schema.Schema, objectInfos []ObjectInfo) *Validator {
	return NewValidatorWithTypesAndAliases(s, objectInfos, nil)
}

// NewValidatorWithTypesAndAliases creates a new validator with type info and aliases.
func NewValidatorWithTypesAndAliases(s *schema.Schema, objectInfos []ObjectInfo, aliases map[string]string) *Validator {
	return NewValidatorWithTypesAliasesAndResolver(s, objectInfos, aliases, nil)
}

// NewValidatorWithTypesAliasesAndResolver creates a new validator with type info,
// aliases, and an optional pre-built resolver.
//
// When resolver is nil, a resolver is constructed from objectInfos and aliases.
// When resolver is provided, it is used as-is (for example: index.Database.Resolver).
func NewValidatorWithTypesAliasesAndResolver(
	s *schema.Schema,
	objectInfos []ObjectInfo,
	aliases map[string]string,
	prebuiltResolver *resolver.Resolver,
) *Validator {
	allIDs := make(map[string]struct{}, len(objectInfos))
	objectTypes := make(map[string]string, len(objectInfos))
	ids := make([]string, 0, len(objectInfos))

	for _, info := range objectInfos {
		allIDs[info.ID] = struct{}{}
		objectTypes[info.ID] = info.Type
		ids = append(ids, info.ID)
	}
	res := prebuiltResolver
	if res == nil {
		res = resolver.New(ids, resolver.Options{Aliases: aliases})
	}

	return &Validator{
		schema:          s,
		resolver:        res,
		allIDs:          allIDs,
		objectTypes:     objectTypes,
		aliases:         aliases,
		missingRefs:     make(map[string]*MissingRef),
		undefinedTraits: make(map[string]*UndefinedTrait),
		usedTypes:       make(map[string]struct{}),
		usedTraits:      make(map[string]struct{}),
		shortRefs:       make(map[string]string),
		usedShortNames:  make(map[string]struct{}),
	}
}

// SetDuplicateAliases sets duplicate alias information for validation.
// This should be called before ValidateSchema to report duplicate aliases.
func (v *Validator) SetDuplicateAliases(duplicates []index.DuplicateAlias) {
	v.duplicateAliases = duplicates
}

// SetDirectoryRoots sets the directory prefixes for typed objects and pages.
// When set, suggestions will strip these prefixes for cleaner display.
func (v *Validator) SetDirectoryRoots(objectsRoot, pagesRoot string) {
	v.objectsRoot = paths.NormalizeDirRoot(objectsRoot)
	v.pagesRoot = paths.NormalizeDirRoot(pagesRoot)
}

// SetDailyDirectory updates the resolver's daily directory.
func (v *Validator) SetDailyDirectory(dailyDir string) {
	v.SetDailyDirectoryForInference(dailyDir)
	v.resolver = resolver.New(v.allIDList(), resolver.Options{
		DailyDirectory: dailyDir,
		Aliases:        v.aliases,
	})
}

// SetDailyDirectoryForInference sets the daily directory used for missing-ref
// type inference without rebuilding the resolver.
func (v *Validator) SetDailyDirectoryForInference(dailyDir string) {
	v.dailyDir = strings.TrimSuffix(paths.NormalizeDirRoot(dailyDir), "/")
}

func (v *Validator) allIDList() []string {
	ids := make([]string, 0, len(v.allIDs))
	for id := range v.allIDs {
		ids = append(ids, id)
	}
	return ids
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
