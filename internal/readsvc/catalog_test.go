package readsvc

import (
	"errors"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/resolver"
)

type catalogReaderStub struct {
	generations []int64
	generation  int
	calls       map[string]int

	objectIDs []string
	objects   []model.Object
	sections  []model.Section
	assets    []model.Asset
	aliases   map[string]string
	filePaths []string
}

func (s *catalogReaderStub) record(name string) {
	if s.calls == nil {
		s.calls = make(map[string]int)
	}
	s.calls[name]++
}

func (s *catalogReaderStub) ResolverGeneration() (int64, error) {
	s.record("generation")
	if len(s.generations) == 0 {
		return 0, nil
	}
	index := s.generation
	if index >= len(s.generations) {
		index = len(s.generations) - 1
	}
	s.generation++
	return s.generations[index], nil
}

func (s *catalogReaderStub) AllObjectIDs() ([]string, error) {
	s.record("object_ids")
	return s.objectIDs, nil
}

func (s *catalogReaderStub) AllObjects() ([]model.Object, error) {
	s.record("objects")
	return s.objects, nil
}

func (s *catalogReaderStub) AllSections() ([]model.Section, error) {
	s.record("sections")
	return s.sections, nil
}

func (s *catalogReaderStub) QueryAssets() ([]model.Asset, error) {
	s.record("assets")
	return s.assets, nil
}

func (s *catalogReaderStub) AllAliases() (map[string]string, error) {
	s.record("aliases")
	return s.aliases, nil
}

func (s *catalogReaderStub) Resolver(index.ResolverOptions) (*resolver.Resolver, error) {
	s.record("resolver")
	return resolver.New(s.objectIDs, resolver.Options{Aliases: s.aliases}), nil
}

func (s *catalogReaderStub) AllIndexedFilePaths() ([]string, error) {
	s.record("file_paths")
	return s.filePaths, nil
}

func TestBuildCatalogSelectsDomainValuesAndLookups(t *testing.T) {
	t.Parallel()

	stub := &catalogReaderStub{
		generations: []int64{7},
		objectIDs:   []string{"people/freya", "people/freya#notes"},
		objects: []model.Object{{
			ID:       "people/freya",
			Type:     "person",
			FilePath: "people/freya.md",
		}},
		sections: []model.Section{{
			ID:       "people/freya#notes",
			FilePath: "people/freya.md",
		}},
		assets: []model.Asset{{
			ID:       "assets/portrait.png",
			FilePath: "assets/portrait.png",
		}},
		aliases:   map[string]string{"The Queen": "people/freya"},
		filePaths: []string{"people/freya.md", "assets/portrait.png"},
	}

	snapshot, err := buildCatalog(stub, CatalogOptions{
		ObjectIDs: true,
		Objects:   true,
		Sections:  true,
		Assets:    true,
		Aliases:   true,
		Resolver:  true,
		FilePaths: true,
	}, index.ResolverOptions{})
	if err != nil {
		t.Fatalf("buildCatalog() error = %v", err)
	}

	if snapshot.Generation != 7 {
		t.Errorf("Generation = %d, want 7", snapshot.Generation)
	}
	if got := snapshot.ObjectByID["people/freya"].Type; got != "person" {
		t.Errorf("ObjectByID type = %q, want person", got)
	}
	if got := snapshot.SectionByID["people/freya#notes"].FilePath; got != "people/freya.md" {
		t.Errorf("SectionByID file path = %q, want people/freya.md", got)
	}
	if got := snapshot.AssetByID["assets/portrait.png"].FilePath; got != "assets/portrait.png" {
		t.Errorf("AssetByID file path = %q, want assets/portrait.png", got)
	}
	if got := snapshot.Aliases["The Queen"]; got != "people/freya" {
		t.Errorf("alias target = %q, want people/freya", got)
	}
	if resolved := snapshot.Resolver.Resolve("The Queen"); resolved.TargetID != "people/freya" {
		t.Errorf("resolver target = %q, want people/freya", resolved.TargetID)
	}
	for _, call := range []string{"object_ids", "objects", "sections", "assets", "aliases", "resolver", "file_paths"} {
		if stub.calls[call] != 1 {
			t.Errorf("%s calls = %d, want 1", call, stub.calls[call])
		}
	}
}

func TestBuildCatalogConsistentRetriesWholeSelection(t *testing.T) {
	t.Parallel()

	stub := &catalogReaderStub{
		generations: []int64{1, 2, 2, 2},
		objects:     []model.Object{{ID: "people/freya"}},
	}

	snapshot, err := buildCatalog(stub, CatalogOptions{
		Objects:    true,
		Consistent: true,
	}, index.ResolverOptions{})
	if err != nil {
		t.Fatalf("buildCatalog() error = %v", err)
	}
	if snapshot.Generation != 2 {
		t.Errorf("Generation = %d, want 2", snapshot.Generation)
	}
	if stub.calls["objects"] != 2 {
		t.Errorf("object reads = %d, want 2", stub.calls["objects"])
	}
}

func TestBuildCatalogConsistentRejectsContinuouslyChangingIndex(t *testing.T) {
	t.Parallel()

	stub := &catalogReaderStub{
		generations: []int64{1, 2, 2, 3, 3, 4},
	}

	_, err := buildCatalog(stub, CatalogOptions{Objects: true, Consistent: true}, index.ResolverOptions{})
	if !errors.Is(err, ErrCatalogChanging) {
		t.Fatalf("buildCatalog() error = %v, want ErrCatalogChanging", err)
	}
	if stub.calls["objects"] != catalogReadAttempts {
		t.Errorf("object reads = %d, want %d", stub.calls["objects"], catalogReadAttempts)
	}
}

func TestCatalogOpensRuntimeDatabaseOnDemand(t *testing.T) {
	t.Parallel()

	rt := &Runtime{
		VaultPath: t.TempDir(),
		VaultCfg:  &config.VaultConfig{},
	}
	t.Cleanup(rt.Close)

	_, err := Catalog(rt, CatalogOptions{ObjectIDs: true})
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	if rt.DB == nil {
		t.Fatal("Catalog() did not open the runtime database")
	}
}

func TestCatalogBuildsResolverWithDegradedSchema(t *testing.T) {
	t.Parallel()

	rt, err := NewRuntime(writeCorruptSchemaVault(t), RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	t.Cleanup(rt.Close)
	if rt.SchemaLoadErr == nil {
		t.Fatal("SchemaLoadErr = nil, want degraded schema state")
	}

	snapshot, err := Catalog(rt, CatalogOptions{Resolver: true, Consistent: true})
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	if snapshot.Resolver == nil {
		t.Fatal("Resolver = nil, want built-in-schema resolver")
	}
}
