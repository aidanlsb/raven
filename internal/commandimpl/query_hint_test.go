package commandimpl

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestBuildUnknownQuerySuggestion_IncludesReadOpenForResolvableRefs(t *testing.T) {
	// Use an in-memory index and insert a known object ID so the resolver can
	// resolve the short name into a read/open hint.
	db, err := index.OpenInMemory()
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	_, err = db.DB().Exec(`INSERT INTO objects (id, file_path, type, line_start, fields) VALUES (?, ?, ?, ?, '{}')`,
		"project/growth-experiments",
		"objects/project/growth-experiments.md",
		"project",
		1,
	)
	if err != nil {
		t.Fatalf("failed to insert object: %v", err)
	}

	s := buildUnknownQuerySuggestion(db, "growth-experiments", "daily", nil)
	if s == "" {
		t.Fatalf("expected suggestion")
	}
	if !strings.Contains(s, "rvn read") || !strings.Contains(s, "rvn open") {
		t.Fatalf("expected read/open hint, got: %q", s)
	}
}

func TestBuildUnknownQuerySuggestionListsEveryQueryRoot(t *testing.T) {
	suggestion := buildUnknownQuerySuggestion(nil, "issue .status==open", "daily", nil)
	for _, root := range []string{"type:", "trait:", "section", "asset"} {
		if !strings.Contains(suggestion, root) {
			t.Fatalf("suggestion %q does not mention query root %q", suggestion, root)
		}
	}
}

func TestBuildUnknownQuerySuggestionRecognizesSchemaType(t *testing.T) {
	sch := schema.New()
	sch.Types["issue"] = &schema.TypeDefinition{}

	suggestion := buildUnknownQuerySuggestion(nil, "issue", "daily", sch)
	if !strings.Contains(suggestion, "rvn query type:issue") {
		t.Fatalf("expected type query hint, got: %q", suggestion)
	}
}
