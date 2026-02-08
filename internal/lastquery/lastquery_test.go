package lastquery

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteAndRead(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	
	// Create .raven directory
	ravenDir := filepath.Join(tmpDir, ".raven")
	if err := os.MkdirAll(ravenDir, 0755); err != nil {
		t.Fatalf("failed to create .raven dir: %v", err)
	}

	// Create a LastQuery
	lq := &LastQuery{
		Query:     "trait:todo .value==todo",
		Timestamp: time.Now(),
		Type:      "trait",
		Results: []ResultEntry{
			{Num: 1, ID: "daily/2026-01-25.md:trait:0", Kind: "trait", Content: "Fix bug", Location: "daily/2026-01-25.md:42"},
			{Num: 2, ID: "daily/2026-01-25.md:trait:1", Kind: "trait", Content: "Write tests", Location: "daily/2026-01-25.md:43"},
			{Num: 3, ID: "projects/raven.md:trait:0", Kind: "trait", Content: "Update docs", Location: "projects/raven.md:15"},
		},
	}

	// Write
	err := Write(tmpDir, lq)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify file exists
	path := Path(tmpDir)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("last-query.json was not created")
	}

	// Read back
	lq2, err := Read(tmpDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Verify contents
	if lq2.Query != lq.Query {
		t.Errorf("Query mismatch: got %q, want %q", lq2.Query, lq.Query)
	}
	if lq2.Type != lq.Type {
		t.Errorf("Type mismatch: got %q, want %q", lq2.Type, lq.Type)
	}
	if len(lq2.Results) != len(lq.Results) {
		t.Errorf("Results count mismatch: got %d, want %d", len(lq2.Results), len(lq.Results))
	}
	for i, r := range lq2.Results {
		if r.ID != lq.Results[i].ID {
			t.Errorf("Result[%d].ID mismatch: got %q, want %q", i, r.ID, lq.Results[i].ID)
		}
		if r.Num != lq.Results[i].Num {
			t.Errorf("Result[%d].Num mismatch: got %d, want %d", i, r.Num, lq.Results[i].Num)
		}
	}
}

func TestReadNoFile(t *testing.T) {
	tmpDir := t.TempDir()
	
	_, err := Read(tmpDir)
	if !errors.Is(err, ErrNoLastQuery) {
		t.Errorf("Expected ErrNoLastQuery, got %v", err)
	}
}

func TestGetByNumbers(t *testing.T) {
	lq := &LastQuery{
		Query: "trait:todo",
		Type:  "trait",
		Results: []ResultEntry{
			{Num: 1, ID: "id1", Kind: "trait", Content: "First"},
			{Num: 2, ID: "id2", Kind: "trait", Content: "Second"},
			{Num: 3, ID: "id3", Kind: "trait", Content: "Third"},
			{Num: 4, ID: "id4", Kind: "trait", Content: "Fourth"},
			{Num: 5, ID: "id5", Kind: "trait", Content: "Fifth"},
		},
	}

	tests := []struct {
		name    string
		nums    []int
		wantIDs []string
		wantErr bool
	}{
		{
			name:    "single",
			nums:    []int{1},
			wantIDs: []string{"id1"},
		},
		{
			name:    "multiple",
			nums:    []int{1, 3, 5},
			wantIDs: []string{"id1", "id3", "id5"},
		},
		{
			name:    "range",
			nums:    []int{2, 3, 4},
			wantIDs: []string{"id2", "id3", "id4"},
		},
		{
			name:    "out of range high",
			nums:    []int{6},
			wantErr: true,
		},
		{
			name:    "out of range zero",
			nums:    []int{0},
			wantErr: true,
		},
		{
			name:    "mixed valid and invalid",
			nums:    []int{1, 10},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lq.GetByNumbers(tt.nums)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetByNumbers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.wantIDs) {
				t.Errorf("GetByNumbers() returned %d results, want %d", len(got), len(tt.wantIDs))
				return
			}
			for i, entry := range got {
				if entry.ID != tt.wantIDs[i] {
					t.Errorf("GetByNumbers()[%d].ID = %q, want %q", i, entry.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestGetAllIDs(t *testing.T) {
	lq := &LastQuery{
		Results: []ResultEntry{
			{Num: 1, ID: "id1"},
			{Num: 2, ID: "id2"},
			{Num: 3, ID: "id3"},
		},
	}

	ids := lq.GetAllIDs()
	if len(ids) != 3 {
		t.Errorf("GetAllIDs() returned %d IDs, want 3", len(ids))
	}
	
	expected := []string{"id1", "id2", "id3"}
	for i, id := range ids {
		if id != expected[i] {
			t.Errorf("GetAllIDs()[%d] = %q, want %q", i, id, expected[i])
		}
	}
}

func TestPath(t *testing.T) {
	path := Path("/vault")
	expected := "/vault/.raven/last-query.json"
	if path != expected {
		t.Errorf("Path() = %q, want %q", path, expected)
	}
}
