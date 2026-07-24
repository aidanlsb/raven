package readsvc

import (
	"errors"
	"fmt"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
)

const catalogReadAttempts = 3

// ErrCatalogChanging reports that the index changed during every attempt to
// build a generation-consistent catalog.
var ErrCatalogChanging = errors.New("index changed while building read catalog")

// CatalogOptions selects the index-derived values included in a catalog.
type CatalogOptions struct {
	// ObjectIDs requests the lightweight reference-resolution ID list. It
	// includes both object and section IDs.
	ObjectIDs bool

	Objects  bool
	Sections bool
	Assets   bool
	Aliases  bool
	Resolver bool

	// FilePaths requests all distinct object and asset file paths.
	FilePaths bool

	// Consistent brackets all selected reads with the resolver generation and
	// retries when the generation changes.
	Consistent bool
}

// CatalogSnapshot is a read-only view of the requested index-derived values.
// Lookup maps are populated when their corresponding domain slice is selected.
type CatalogSnapshot struct {
	Generation int64

	ObjectIDs []string

	Objects    []model.Object
	ObjectByID map[string]model.Object

	Sections    []model.Section
	SectionByID map[string]model.Section

	Assets    []model.Asset
	AssetByID map[string]model.Asset

	Aliases  map[string]string
	Resolver *resolver.Resolver

	FilePaths []string
}

type catalogReader interface {
	ResolverGeneration() (int64, error)
	AllObjectIDs() ([]string, error)
	AllObjects() ([]model.Object, error)
	AllSections() ([]model.Section, error)
	QueryAssets() ([]model.Asset, error)
	AllAliases() (map[string]string, error)
	Resolver(index.ResolverOptions) (*resolver.Resolver, error)
	AllIndexedFilePaths() ([]string, error)
}

// Catalog loads a selective read-side catalog, opening the runtime database on
// demand. Consistent catalogs are retried when index writes overlap the reads.
func Catalog(rt *Runtime, opts CatalogOptions) (CatalogSnapshot, error) {
	if rt == nil {
		return CatalogSnapshot{}, fmt.Errorf("runtime is required")
	}
	if err := rt.OpenDB(); err != nil {
		return CatalogSnapshot{}, err
	}
	return buildCatalog(rt.DB, opts, catalogResolverOptions(rt))
}

func buildCatalog(db catalogReader, opts CatalogOptions, resolverOpts index.ResolverOptions) (CatalogSnapshot, error) {
	if !opts.Consistent {
		snapshot, err := readCatalog(db, opts, resolverOpts)
		if err != nil {
			return CatalogSnapshot{}, err
		}
		generation, err := db.ResolverGeneration()
		if err != nil {
			return CatalogSnapshot{}, fmt.Errorf("read catalog generation: %w", err)
		}
		snapshot.Generation = generation
		return snapshot, nil
	}

	for range catalogReadAttempts {
		generationBefore, err := db.ResolverGeneration()
		if err != nil {
			return CatalogSnapshot{}, fmt.Errorf("read catalog generation: %w", err)
		}

		snapshot, err := readCatalog(db, opts, resolverOpts)
		if err != nil {
			return CatalogSnapshot{}, err
		}

		generationAfter, err := db.ResolverGeneration()
		if err != nil {
			return CatalogSnapshot{}, fmt.Errorf("read catalog generation: %w", err)
		}
		if generationBefore != generationAfter {
			continue
		}

		snapshot.Generation = generationAfter
		return snapshot, nil
	}

	return CatalogSnapshot{}, ErrCatalogChanging
}

func readCatalog(db catalogReader, opts CatalogOptions, resolverOpts index.ResolverOptions) (CatalogSnapshot, error) {
	var snapshot CatalogSnapshot
	var err error

	if opts.ObjectIDs {
		snapshot.ObjectIDs, err = db.AllObjectIDs()
		if err != nil {
			return CatalogSnapshot{}, fmt.Errorf("list catalog object IDs: %w", err)
		}
	}
	if opts.Objects {
		snapshot.Objects, err = db.AllObjects()
		if err != nil {
			return CatalogSnapshot{}, fmt.Errorf("list catalog objects: %w", err)
		}
		snapshot.ObjectByID = make(map[string]model.Object, len(snapshot.Objects))
		for _, object := range snapshot.Objects {
			snapshot.ObjectByID[object.ID] = object
		}
	}
	if opts.Sections {
		snapshot.Sections, err = db.AllSections()
		if err != nil {
			return CatalogSnapshot{}, fmt.Errorf("list catalog sections: %w", err)
		}
		snapshot.SectionByID = make(map[string]model.Section, len(snapshot.Sections))
		for _, section := range snapshot.Sections {
			snapshot.SectionByID[section.ID] = section
		}
	}
	if opts.Assets {
		snapshot.Assets, err = db.QueryAssets()
		if err != nil {
			return CatalogSnapshot{}, fmt.Errorf("list catalog assets: %w", err)
		}
		snapshot.AssetByID = make(map[string]model.Asset, len(snapshot.Assets))
		for _, asset := range snapshot.Assets {
			snapshot.AssetByID[asset.ID] = asset
		}
	}
	if opts.Aliases {
		snapshot.Aliases, err = db.AllAliases()
		if err != nil {
			return CatalogSnapshot{}, fmt.Errorf("list catalog aliases: %w", err)
		}
	}
	if opts.Resolver {
		snapshot.Resolver, err = db.Resolver(resolverOpts)
		if err != nil {
			return CatalogSnapshot{}, fmt.Errorf("build catalog resolver: %w", err)
		}
	}
	if opts.FilePaths {
		snapshot.FilePaths, err = db.AllIndexedFilePaths()
		if err != nil {
			return CatalogSnapshot{}, fmt.Errorf("list catalog file paths: %w", err)
		}
	}

	return snapshot, nil
}

func catalogResolverOptions(rt *Runtime) index.ResolverOptions {
	sch := rt.Schema
	if sch == nil {
		sch = schema.New()
	}
	return index.ResolverOptions{
		DailyDirectory: rt.VaultCfg.GetDailyDirectory(),
		Schema:         sch,
	}
}
