package lastresults

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/lastquery"
	"github.com/aidanlsb/raven/internal/model"
)

func TestWriteAndReadRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	results := []model.Result{
		model.Object{
			ID:        "people/freya",
			Type:      "person",
			Fields:    map[string]interface{}{"name": "Freya"},
			FilePath:  "people/freya.md",
			LineStart: 1,
		},
		model.Trait{
			ID:             "daily/2026-01-25.md:trait:0",
			TraitType:      "todo",
			Value:          strPtr("done"),
			Content:        "Fix bug",
			FilePath:       "daily/2026-01-25.md",
			Line:           42,
			ParentObjectID: "daily/2026-01-25",
		},
		model.Reference{
			SourceID:    "people/freya",
			SourceType:  "object",
			TargetRaw:   "projects/raven",
			FilePath:    "people/freya.md",
			Line:        intPtr(10),
			DisplayText: strPtr("Raven"),
		},
		model.SearchMatch{
			ObjectID: "people/freya",
			Title:    "Freya",
			FilePath: "people/freya.md",
			Snippet:  "Known for the Valkyries",
			Rank:     1.23,
		},
	}

	lr, err := NewFromResults(SourceSearch, "freya", "", results)
	if err != nil {
		t.Fatalf("NewFromResults failed: %v", err)
	}

	if err := Write(tmpDir, lr); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	readBack, err := Read(tmpDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if readBack.Source != lr.Source {
		t.Errorf("Source mismatch: got %q, want %q", readBack.Source, lr.Source)
	}
	if readBack.Query != lr.Query {
		t.Errorf("Query mismatch: got %q, want %q", readBack.Query, lr.Query)
	}
	if len(readBack.Results) != len(results) {
		t.Errorf("Results count mismatch: got %d, want %d", len(readBack.Results), len(results))
	}

	decoded, err := readBack.DecodeAll()
	if err != nil {
		t.Fatalf("DecodeAll failed: %v", err)
	}

	if _, ok := decoded[0].(model.Object); !ok {
		t.Fatalf("expected object result, got %T", decoded[0])
	}
	if _, ok := decoded[1].(model.Trait); !ok {
		t.Fatalf("expected trait result, got %T", decoded[1])
	}
	if _, ok := decoded[2].(model.Reference); !ok {
		t.Fatalf("expected reference result, got %T", decoded[2])
	}
	if _, ok := decoded[3].(model.SearchMatch); !ok {
		t.Fatalf("expected search result, got %T", decoded[3])
	}
}

func TestReadLegacyLastQuery(t *testing.T) {
	tmpDir := t.TempDir()
	ravenDir := filepath.Join(tmpDir, ".raven")
	if err := os.MkdirAll(ravenDir, 0755); err != nil {
		t.Fatalf("failed to create .raven dir: %v", err)
	}

	timestamp := time.Now().UTC().Truncate(time.Second)
	legacy := &lastquery.LastQuery{
		Query:     "trait:todo",
		Timestamp: timestamp,
		Type:      "trait",
		Results: []lastquery.ResultEntry{
			{
				Num:        1,
				ID:         "daily/2026-01-25.md:trait:0",
				Kind:       "trait",
				Content:    "Write tests",
				Location:   "daily/2026-01-25.md:42",
				FilePath:   "daily/2026-01-25.md",
				Line:       42,
				TraitType:  "todo",
				TraitValue: strPtr("done"),
			},
		},
	}

	if err := lastquery.Write(tmpDir, legacy); err != nil {
		t.Fatalf("legacy Write failed: %v", err)
	}

	lr, err := Read(tmpDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if lr.Source != SourceQuery {
		t.Errorf("Source mismatch: got %q, want %q", lr.Source, SourceQuery)
	}
	if lr.Query != legacy.Query {
		t.Errorf("Query mismatch: got %q, want %q", lr.Query, legacy.Query)
	}
	if !lr.Timestamp.Equal(legacy.Timestamp) {
		t.Errorf("Timestamp mismatch: got %v, want %v", lr.Timestamp, legacy.Timestamp)
	}

	traits, err := lr.DecodeTraits()
	if err != nil {
		t.Fatalf("DecodeTraits failed: %v", err)
	}
	if len(traits) != 1 {
		t.Fatalf("expected 1 trait, got %d", len(traits))
	}
	if traits[0].ID != legacy.Results[0].ID {
		t.Errorf("Trait ID mismatch: got %q, want %q", traits[0].ID, legacy.Results[0].ID)
	}
	if traits[0].TraitType != legacy.Results[0].TraitType {
		t.Errorf("Trait type mismatch: got %q, want %q", traits[0].TraitType, legacy.Results[0].TraitType)
	}
}

func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}
