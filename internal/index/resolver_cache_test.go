package index

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestIndexDocumentAutoResolveReusesIncrementalResolver(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	sch := schema.New()
	if err := db.IndexDocument(resolverTestDocument("people/freya.md", "people/freya"), sch); err != nil {
		t.Fatal(err)
	}
	if db.referenceResolverBuilds != 0 {
		t.Fatalf("resolver builds after ref-free index = %d, want 0", db.referenceResolverBuilds)
	}

	source := resolverTestDocument("notes/source.md", "notes/source")
	source.Refs = []*model.Reference{{
		SourceID:  "notes/source",
		TargetRaw: "freya",
		Line:      model.IntPtr(1),
	}}
	if err := db.IndexDocument(source, sch); err != nil {
		t.Fatal(err)
	}
	assertIndexedRefTarget(t, db, "notes/source.md", "freya", "people/freya")
	if db.referenceResolverBuilds != 1 {
		t.Fatalf("resolver builds after first ref index = %d, want 1", db.referenceResolverBuilds)
	}

	if err := db.IndexDocument(source, sch); err != nil {
		t.Fatal(err)
	}
	assertIndexedRefTarget(t, db, "notes/source.md", "freya", "people/freya")
	if db.referenceResolverBuilds != 1 {
		t.Fatalf("resolver rebuilt for repeated file index: builds = %d, want 1", db.referenceResolverBuilds)
	}

	if err := db.IndexDocument(resolverTestDocument("people/frigg.md", "people/frigg"), sch); err != nil {
		t.Fatal(err)
	}
	source.Refs[0].TargetRaw = "frigg"
	if err := db.IndexDocument(source, sch); err != nil {
		t.Fatal(err)
	}
	assertIndexedRefTarget(t, db, "notes/source.md", "frigg", "people/frigg")
	if db.referenceResolverBuilds != 1 {
		t.Fatalf("resolver rebuilt after incremental short-name addition: builds = %d, want 1", db.referenceResolverBuilds)
	}
}

func TestIndexDocumentIncrementalResolverTracksAliasAndNameFieldChanges(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	sch := schema.New()
	sch.Types["person"] = &schema.TypeDefinition{
		NameField: "name",
		Fields: map[string]*schema.FieldDefinition{
			"name":  {Type: schema.FieldTypeString},
			"alias": {Type: schema.FieldTypeString},
		},
	}

	person := resolverTestDocument("people/freya.md", "people/freya")
	person.Objects[0].Type = "person"
	person.Objects[0].Fields = map[string]schema.FieldValue{
		"name":  schema.String("Freya Old"),
		"alias": schema.String("Old Queen"),
	}
	if err := db.IndexDocument(person, sch); err != nil {
		t.Fatal(err)
	}

	source := resolverTestDocument("notes/source.md", "notes/source")
	source.Refs = []*model.Reference{
		{SourceID: "notes/source", TargetRaw: "Old Queen", Line: model.IntPtr(1)},
		{SourceID: "notes/source", TargetRaw: "Freya Old", Line: model.IntPtr(2)},
	}
	if err := db.IndexDocument(source, sch); err != nil {
		t.Fatal(err)
	}
	assertIndexedRefTarget(t, db, source.FilePath, "Old Queen", "people/freya")
	assertIndexedRefTarget(t, db, source.FilePath, "Freya Old", "people/freya")

	person.Objects[0].Fields = map[string]schema.FieldValue{
		"name":  schema.String("Freya New"),
		"alias": schema.String("New Queen"),
	}
	if err := db.IndexDocument(person, sch); err != nil {
		t.Fatal(err)
	}

	source.Refs = []*model.Reference{
		{SourceID: "notes/source", TargetRaw: "Old Queen", Line: model.IntPtr(1)},
		{SourceID: "notes/source", TargetRaw: "Freya Old", Line: model.IntPtr(2)},
		{SourceID: "notes/source", TargetRaw: "New Queen", Line: model.IntPtr(3)},
		{SourceID: "notes/source", TargetRaw: "Freya New", Line: model.IntPtr(4)},
	}
	if err := db.IndexDocument(source, sch); err != nil {
		t.Fatal(err)
	}

	assertIndexedRefUnresolved(t, db, source.FilePath, "Old Queen")
	assertIndexedRefUnresolved(t, db, source.FilePath, "Freya Old")
	assertIndexedRefTarget(t, db, source.FilePath, "New Queen", "people/freya")
	assertIndexedRefTarget(t, db, source.FilePath, "Freya New", "people/freya")
	if db.referenceResolverBuilds != 1 {
		t.Fatalf("resolver builds = %d, want 1 across alias/name updates", db.referenceResolverBuilds)
	}
}

func TestIncrementalResolverMatchesColdResolverAcrossShortNameRemoval(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	sch := schema.New()
	if err := db.IndexDocument(resolverTestDocument("people/freya.md", "people/freya"), sch); err != nil {
		t.Fatal(err)
	}
	source := resolverTestDocument("notes/source.md", "notes/source")
	source.Refs = []*model.Reference{{SourceID: "notes/source", TargetRaw: "freya", Line: model.IntPtr(1)}}
	if err := db.IndexDocument(source, sch); err != nil {
		t.Fatal(err)
	}
	if err := db.IndexDocument(resolverTestDocument("gods/freya.md", "gods/freya"), sch); err != nil {
		t.Fatal(err)
	}

	assertCachedResolverMatchesCold(t, db, sch, "freya")
	if err := db.RemoveFile("gods/freya.md"); err != nil {
		t.Fatal(err)
	}
	assertCachedResolverMatchesCold(t, db, sch, "freya")

	source.Refs[0].TargetRaw = "freya"
	if err := db.IndexDocument(source, sch); err != nil {
		t.Fatal(err)
	}
	assertIndexedRefTarget(t, db, source.FilePath, "freya", "people/freya")
}

func BenchmarkIndexDocumentWithWarmResolver(b *testing.B) {
	db, err := OpenInMemory()
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()

	sch := schema.New()
	targets := resolverTestDocument("targets.md", "targets/root")
	for i := 0; i < 5000; i++ {
		targets.Objects = append(targets.Objects, &model.Object{
			ID:        fmt.Sprintf("people/person-%05d", i),
			Type:      "person",
			Fields:    map[string]schema.FieldValue{},
			LineStart: i + 2,
		})
	}
	if err := db.IndexDocument(targets, sch); err != nil {
		b.Fatal(err)
	}

	source := resolverTestDocument("notes/source.md", "notes/source")
	source.Refs = []*model.Reference{{
		SourceID:  "notes/source",
		TargetRaw: "person-04999",
		Line:      model.IntPtr(1),
	}}
	if err := db.IndexDocument(source, sch); err != nil {
		b.Fatal(err)
	}

	buildsBefore := db.referenceResolverBuilds
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := db.IndexDocument(source, sch); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(db.referenceResolverBuilds-buildsBefore), "full-resolver-builds")
}

func resolverTestDocument(filePath, objectID string) *parser.ParsedDocument {
	return &parser.ParsedDocument{
		FilePath: filePath,
		Objects: []*model.Object{{
			ID:        objectID,
			Type:      "page",
			Fields:    map[string]schema.FieldValue{},
			LineStart: 1,
		}},
	}
}

func assertIndexedRefTarget(t *testing.T, db *Database, filePath, targetRaw, wantTarget string) {
	t.Helper()
	var target sql.NullString
	if err := db.db.QueryRow(
		`SELECT target_id FROM refs WHERE file_path = ? AND target_raw = ?`,
		filePath,
		targetRaw,
	).Scan(&target); err != nil {
		t.Fatalf("query ref %q: %v", targetRaw, err)
	}
	if !target.Valid || target.String != wantTarget {
		t.Fatalf("target_id for %q = %q (valid %v), want %q", targetRaw, target.String, target.Valid, wantTarget)
	}
}

func assertIndexedRefUnresolved(t *testing.T, db *Database, filePath, targetRaw string) {
	t.Helper()
	var target sql.NullString
	if err := db.db.QueryRow(
		`SELECT target_id FROM refs WHERE file_path = ? AND target_raw = ?`,
		filePath,
		targetRaw,
	).Scan(&target); err != nil {
		t.Fatalf("query ref %q: %v", targetRaw, err)
	}
	if target.Valid {
		t.Fatalf("target_id for %q = %q, want NULL", targetRaw, target.String)
	}
}

func assertCachedResolverMatchesCold(t *testing.T, db *Database, sch *schema.Schema, ref string) {
	t.Helper()
	if db.referenceResolverCache == nil {
		t.Fatal("reference resolver cache is nil")
	}
	cached := db.referenceResolverCache.resolver.Resolve(ref)
	cold, err := db.Resolver(ResolverOptions{DailyDirectory: "daily", Schema: sch})
	if err != nil {
		t.Fatal(err)
	}
	want := cold.Resolve(ref)
	if cached.TargetID != want.TargetID || cached.Ambiguous != want.Ambiguous ||
		!sameStringSet(cached.Matches, want.Matches) {
		t.Fatalf("cached Resolve(%q) = %+v, cold = %+v", ref, cached, want)
	}
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	values := make(map[string]int, len(a))
	for _, value := range a {
		values[value]++
	}
	for _, value := range b {
		values[value]--
		if values[value] < 0 {
			return false
		}
	}
	return true
}
