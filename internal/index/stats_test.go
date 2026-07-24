package index

import "testing"

func TestStatsForFile(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer db.Close()

	if _, err := db.db.Exec(`
		INSERT INTO objects (id, file_path, type, fields, line_start) VALUES
			('target', 'target.md', 'page', '{}', 1),
			('other', 'other.md', 'page', '{}', 1);
		INSERT INTO traits (id, file_path, parent_object_id, trait_type, content, line_number) VALUES
			('target:trait:1', 'target.md', 'target', 'todo', 'One', 2),
			('target:trait:2', 'target.md', 'target', 'todo', 'Two', 3),
			('other:trait:1', 'other.md', 'other', 'todo', 'Other', 2);
		INSERT INTO refs (source_id, target_raw, file_path, line_number) VALUES
			('target', 'other', 'target.md', 4),
			('target', 'missing', 'target.md', 5),
			('other', 'target', 'other.md', 4);
	`); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	tests := []struct {
		name     string
		filePath string
		want     IndexStats
	}{
		{
			name:     "indexed file",
			filePath: "target.md",
			want: IndexStats{
				ObjectCount: 1,
				TraitCount:  2,
				RefCount:    2,
			},
		},
		{
			name:     "unindexed file",
			filePath: "missing.md",
			want:     IndexStats{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.StatsForFile(tt.filePath)
			if err != nil {
				t.Fatalf("StatsForFile(%q): %v", tt.filePath, err)
			}
			if got != tt.want {
				t.Fatalf("StatsForFile(%q) = %#v, want %#v", tt.filePath, got, tt.want)
			}
		})
	}
}
