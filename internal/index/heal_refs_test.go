package index

import (
	"testing"

	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

// TestIndexDocumentHealsPendingInboundRefs verifies that indexing a file which
// adds new resolution candidates re-resolves refs elsewhere in the vault that
// previously failed to resolve (e.g. a ref written before its target existed).
func TestIndexDocumentHealsPendingInboundRefs(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.New()
	sch.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"company": {Type: schema.FieldTypeRef, Target: "company"},
		},
	}
	sch.Types["company"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}

	line := 3
	personDoc := &parser.ParsedDocument{
		FilePath: "people/ada.md",
		Objects: []*model.Object{
			{
				ID:   "people/ada",
				Type: "person",
				Fields: map[string]fieldvalue.FieldValue{
					"company": fieldvalue.String("cursor"),
				},
				LineStart: 1,
			},
		},
		Refs: []*model.Reference{
			{
				SourceID:   "people/ada",
				SourceType: "object",
				TargetRaw:  "cursor",
				FilePath:   "people/ada.md",
				Line:       &line,
			},
		},
	}
	if err := db.IndexDocument(personDoc, sch); err != nil {
		t.Fatalf("failed to index person: %v", err)
	}

	var status string
	err = db.db.QueryRow(`
		SELECT resolution_status FROM field_refs WHERE source_id = ? AND field_name = ?
	`, "people/ada", "company").Scan(&status)
	if err != nil {
		t.Fatalf("failed to query field_refs before heal: %v", err)
	}
	if status != "missing" {
		t.Fatalf("expected pre-heal status 'missing', got %q", status)
	}

	// Indexing the target later must heal the pending refs without any
	// explicit reindex.
	companyDoc := &parser.ParsedDocument{
		FilePath: "companies/cursor.md",
		Objects: []*model.Object{
			{
				ID:        "companies/cursor",
				Type:      "company",
				Fields:    map[string]fieldvalue.FieldValue{},
				LineStart: 1,
			},
		},
	}
	if err := db.IndexDocument(companyDoc, sch); err != nil {
		t.Fatalf("failed to index company: %v", err)
	}

	var targetID string
	err = db.db.QueryRow(`
		SELECT target_id, resolution_status FROM field_refs WHERE source_id = ? AND field_name = ?
	`, "people/ada", "company").Scan(&targetID, &status)
	if err != nil {
		t.Fatalf("failed to query field_refs after heal: %v", err)
	}
	if targetID != "companies/cursor" {
		t.Errorf("field ref target_id = %q, want %q", targetID, "companies/cursor")
	}
	if status != "resolved" {
		t.Errorf("field ref status = %q, want %q", status, "resolved")
	}

	var bodyTargetID string
	err = db.db.QueryRow(`
		SELECT target_id FROM refs WHERE file_path = ? AND target_raw = ?
	`, "people/ada.md", "cursor").Scan(&bodyTargetID)
	if err != nil {
		t.Fatalf("failed to query refs after heal: %v", err)
	}
	if bodyTargetID != "companies/cursor" {
		t.Errorf("body ref target_id = %q, want %q", bodyTargetID, "companies/cursor")
	}
}

// TestIndexDocumentWithoutNewCandidatesKeepsFileScope verifies the scope
// guard: re-indexing a file that adds no new resolution candidates keeps the
// resolve pass file-scoped, so pending refs in other files stay untouched.
func TestIndexDocumentWithoutNewCandidatesKeepsFileScope(t *testing.T) {
	t.Parallel()
	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	sch := schema.New()
	sch.Types["note"] = &schema.TypeDefinition{Fields: map[string]*schema.FieldDefinition{}}

	line := 3
	sourceDoc := &parser.ParsedDocument{
		FilePath: "notes/source.md",
		Objects: []*model.Object{
			{ID: "notes/source", Type: "note", Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1},
		},
		Refs: []*model.Reference{
			{
				SourceID:   "notes/source",
				SourceType: "object",
				TargetRaw:  "notes/other",
				FilePath:   "notes/source.md",
				Line:       &line,
			},
		},
	}
	otherDoc := &parser.ParsedDocument{
		FilePath: "notes/other.md",
		Objects: []*model.Object{
			{ID: "notes/other", Type: "note", Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1},
		},
	}

	// Build a stale state the way legacy indexes got into it: both files
	// indexed with auto-resolve off, leaving the source's ref pending even
	// though its target is indexed.
	db.SetAutoResolveRefs(false)
	if err := db.IndexDocument(sourceDoc, sch); err != nil {
		t.Fatalf("failed to index source: %v", err)
	}
	if err := db.IndexDocument(otherDoc, sch); err != nil {
		t.Fatalf("failed to index other: %v", err)
	}
	db.SetAutoResolveRefs(true)

	// Re-index the source unchanged: same object ID, no new candidates. The
	// file-scoped pass resolves the source's own pending ref, which is the
	// pre-existing per-file behavior.
	if err := db.IndexDocument(sourceDoc, sch); err != nil {
		t.Fatalf("failed to re-index source: %v", err)
	}

	// Now plant a pending ref in another file while auto-resolve is off, and
	// re-index source unchanged again: no new candidates means no vault-wide
	// pass, so the planted ref must stay pending.
	db.SetAutoResolveRefs(false)
	thirdDoc := &parser.ParsedDocument{
		FilePath: "notes/third.md",
		Objects: []*model.Object{
			{ID: "notes/third", Type: "note", Fields: map[string]fieldvalue.FieldValue{}, LineStart: 1},
		},
		Refs: []*model.Reference{
			{
				SourceID:   "notes/third",
				SourceType: "object",
				TargetRaw:  "notes/other",
				FilePath:   "notes/third.md",
				Line:       &line,
			},
		},
	}
	if err := db.IndexDocument(thirdDoc, sch); err != nil {
		t.Fatalf("failed to index third: %v", err)
	}
	db.SetAutoResolveRefs(true)

	if err := db.IndexDocument(sourceDoc, sch); err != nil {
		t.Fatalf("failed to re-index source again: %v", err)
	}

	var targetID *string
	err = db.db.QueryRow(`
		SELECT target_id FROM refs WHERE file_path = ? AND target_raw = ?
	`, "notes/third.md", "notes/other").Scan(&targetID)
	if err != nil {
		t.Fatalf("failed to query refs: %v", err)
	}
	if targetID != nil {
		t.Fatalf("expected pending ref in third.md to stay unresolved after no-candidate write, got %q", *targetID)
	}
}

func TestResolverStateAddsCandidates(t *testing.T) {
	t.Parallel()

	withObject := func(id string) *resolverFileState {
		state := newResolverFileState()
		state.objectIDs[id] = struct{}{}
		return state
	}
	withAlias := func(alias, id string) *resolverFileState {
		state := newResolverFileState()
		addResolverStateMatch(state.aliases, alias, id)
		return state
	}
	withNameField := func(name, id string) *resolverFileState {
		state := newResolverFileState()
		addResolverStateMatch(state.nameFields, name, id)
		return state
	}
	withAsset := func(id string) *resolverFileState {
		state := newResolverFileState()
		state.assetIDs[id] = struct{}{}
		return state
	}

	tests := []struct {
		name     string
		oldState *resolverFileState
		newState *resolverFileState
		want     bool
	}{
		{"nil new state", withObject("a"), nil, false},
		{"nil old state with new object", nil, withObject("a"), true},
		{"identical states", withObject("a"), withObject("a"), false},
		{"added object id", newResolverFileState(), withObject("a"), true},
		{"removed object id only", withObject("a"), newResolverFileState(), false},
		{"added alias", newResolverFileState(), withAlias("The Queen", "people/freya"), true},
		{"added name field", newResolverFileState(), withNameField("Freya", "people/freya"), true},
		{"added asset id", newResolverFileState(), withAsset("docs/spec.pdf"), true},
		{"both nil", nil, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolverStateAddsCandidates(tt.oldState, tt.newState); got != tt.want {
				t.Errorf("resolverStateAddsCandidates() = %v, want %v", got, tt.want)
			}
		})
	}
}
