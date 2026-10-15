package readsvc

import (
	"testing"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestListObjectsUsesNameFieldAndPaginates(t *testing.T) {
	t.Parallel()
	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.DB().Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields) VALUES
			('people/freya', 'people/freya.md', 'person', 1, '{"name":"Freya"}'),
			('projects/website', 'projects/website.md', 'project', 1, '{"title":"Website","status":"active"}')
	`); err != nil {
		t.Fatal(err)
	}

	rt := &vaultruntime.Runtime{
		VaultPath: t.TempDir(),
		DB:        db,
		Schema: &schema.Schema{
			Types: map[string]*schema.TypeDefinition{
				"person":  {NameField: "name"},
				"project": {NameField: "title"},
			},
		},
	}

	all, err := ListObjects(rt, 0, 0)
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	if all.Total != 2 || len(all.Items) != 2 {
		t.Fatalf("unlimited = %#v", all)
	}
	byID := map[string]ObjectSummary{}
	for _, item := range all.Items {
		byID[item.ID] = item
	}
	if byID["people/freya"].Name != "Freya" || byID["people/freya"].Type != "person" {
		t.Fatalf("freya = %#v", byID["people/freya"])
	}
	if byID["projects/website"].Name != "Website" {
		t.Fatalf("website = %#v", byID["projects/website"])
	}

	page, err := ListObjects(rt, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 {
		t.Fatalf("page = %#v", page)
	}
}

func TestListObjectsRejectsNegativePaging(t *testing.T) {
	t.Parallel()
	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	rt := &vaultruntime.Runtime{VaultPath: t.TempDir(), DB: db}
	if _, err := ListObjects(rt, -1, 0); err == nil {
		t.Fatal("expected limit error")
	} else if svc, ok := svcerr.AsError(err); !ok || svc.Code != "INVALID_INPUT" {
		t.Fatalf("limit error = %v", err)
	}
	if _, err := ListObjects(rt, 0, -1); err == nil {
		t.Fatal("expected offset error")
	}
}

func TestListObjectsFallsBackToTitleThenName(t *testing.T) {
	t.Parallel()
	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.DB().Exec(`
		INSERT INTO objects (id, file_path, type, line_start, fields) VALUES
			('pages/note', 'pages/note.md', 'page', 1, '{"title":"A note"}')
	`); err != nil {
		t.Fatal(err)
	}
	rt := &vaultruntime.Runtime{VaultPath: t.TempDir(), DB: db}
	got, err := ListObjects(rt, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Name != "A note" {
		t.Fatalf("fallback name = %#v", got.Items)
	}
}
