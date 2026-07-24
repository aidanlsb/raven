package checkfixsvc

import (
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
)

// MissingRefResolution is a resolved interactive decision to create one missing
// reference target as TypeName. Decisions are collected by the caller's prompt
// layer; this package owns the mutation orchestration.
type MissingRefResolution struct {
	TargetPath string
	TypeName   string
}

// NewTypeResolution is a resolved interactive decision to add a new type to the
// schema before creating any pages that use it.
type NewTypeResolution struct {
	TypeName    string
	DefaultPath string
}

// TraitResolution is a resolved interactive decision to add one undefined trait
// to the schema.
type TraitResolution struct {
	TraitName    string
	TraitType    string
	EnumValues   []string
	DefaultValue string
}

// PageOutcome reports the result of creating one missing referenced page.
// ResolvedPath is the slugified destination (without the .md extension) so the
// caller can render it consistently with the path Raven actually created.
type PageOutcome struct {
	TargetPath   string
	TypeName     string
	ResolvedPath string
	Err          error
}

// TypeOutcome reports the result of adding one new type to the schema.
type TypeOutcome struct {
	TypeName    string
	DefaultPath string
	Err         error
}

// TraitOutcome reports the result of adding one undefined trait to the schema.
type TraitOutcome struct {
	TraitName string
	TraitType string
	Err       error
}

// MissingRefApplyResult bundles the outcomes of applying a batch of interactive
// missing-reference decisions. Types are reported in creation order, followed
// by page creations.
type MissingRefApplyResult struct {
	Types []TypeOutcome
	Pages []PageOutcome
}

// CreatedPages returns the number of pages that were created successfully.
func (r MissingRefApplyResult) CreatedPages() int {
	created := 0
	for _, page := range r.Pages {
		if page.Err == nil {
			created++
		}
	}
	return created
}

// ApplyMissingRefResolutions performs the schema/file mutations for a batch of
// interactive missing-reference decisions. New types are added first (mutating
// the in-memory schema so page path resolution reflects their default_path),
// then one page is created per resolution. Pages whose type could not be
// resolved (e.g. a new-type creation that failed) are skipped, mirroring the
// interactive flow's behavior.
//
// This is the service-side counterpart to the CLI prompt layer: callers gather
// user decisions interactively and hand them here so mutations never originate
// from presentation code.
func ApplyMissingRefResolutions(
	vaultPath string,
	sch *schema.Schema,
	newTypes []NewTypeResolution,
	resolutions []MissingRefResolution,
	vaultCfg *config.VaultConfig,
) MissingRefApplyResult {
	result := MissingRefApplyResult{}

	objectsRoot := vaultCfg.GetObjectsRoot()
	pagesRoot := vaultCfg.GetPagesRoot()
	dailyDir := vaultCfg.GetDailyDirectory()
	templateDir := vaultCfg.GetTemplateDirectory()
	protectedPrefixes := vaultCfg.ProtectedPrefixes

	for _, newType := range newTypes {
		err := AddType(vaultPath, sch, newType.TypeName, newType.DefaultPath)
		result.Types = append(result.Types, TypeOutcome{
			TypeName:    newType.TypeName,
			DefaultPath: newType.DefaultPath,
			Err:         err,
		})
	}

	for _, res := range resolutions {
		typeName := res.TypeName
		if typeName == "" {
			continue
		}
		// Skip pages whose type is not resolvable (e.g. a new-type creation
		// failed earlier); the type failure is already reported separately.
		if _, exists := sch.Types[typeName]; !exists && !schema.IsBuiltinType(typeName) {
			continue
		}

		resolvedPath := ResolveAndSlugifyTargetPath(res.TargetPath, typeName, sch, objectsRoot, pagesRoot, dailyDir)
		err := CreateMissingPage(vaultPath, sch, res.TargetPath, typeName, objectsRoot, pagesRoot, dailyDir, templateDir, protectedPrefixes)
		result.Pages = append(result.Pages, PageOutcome{
			TargetPath:   res.TargetPath,
			TypeName:     typeName,
			ResolvedPath: resolvedPath,
			Err:          err,
		})
	}

	return result
}

// ApplyTraitResolutions adds each resolved undefined trait to the schema,
// returning per-trait outcomes in the same order.
func ApplyTraitResolutions(vaultPath string, sch *schema.Schema, resolutions []TraitResolution) []TraitOutcome {
	outcomes := make([]TraitOutcome, 0, len(resolutions))
	for _, res := range resolutions {
		err := AddTrait(vaultPath, sch, res.TraitName, res.TraitType, res.EnumValues, res.DefaultValue)
		outcomes = append(outcomes, TraitOutcome{
			TraitName: res.TraitName,
			TraitType: res.TraitType,
			Err:       err,
		})
	}
	return outcomes
}
