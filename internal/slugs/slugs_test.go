package slugs

import "testing"

func TestHeadingSlug(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{"Weekly Standup", "weekly-standup"},
		{"A:B", "a-b"},
		{"A__B", "a-b"},
		{"A - B", "a-b"},
		{"  Leading and trailing  ", "leading-and-trailing"},
		{"A:", "a"},
		{"!!!", ""},
		{"Привет мир", "привет-мир"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := HeadingSlug(tt.in); got != tt.want {
				t.Fatalf("HeadingSlug(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestComponentSlug(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{"Freya", "freya"},
		{"Sif", "sif"},
		{"My Awesome Project", "my-awesome-project"},
		{"UPPER CASE", "upper-case"},
		{"test.md", "test"},
		{"file-name", "file-name"},
		{"Special: Characters!", "special-characters"},
		{"internal/paths", "internal-paths"},
		{"config.VaultConfig duplicates internal/paths", "config-vaultconfig-duplicates-internal-paths"},
		// Fallback path (goslug cannot slugify): separators, including path
		// separators, are replaced so the result stays a single component.
		{"///", "---"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ComponentSlug(tt.in); got != tt.want {
				t.Fatalf("ComponentSlug(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPathSlug(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{"people/Freya", "people/freya"},
		{"people/Sif", "people/sif"},
		{"projects/My Project/docs", "projects/my-project/docs"},
		{"file.md", "file"},
		{"path/to/file.md", "path/to/file"},
		{"daily/2025-02-01#Team Sync", "daily/2025-02-01#team-sync"},
		{`game-notes\Competitions`, "game-notes/competitions"},
		{`daily\2025-02-01#Team Sync`, "daily/2025-02-01#team-sync"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := PathSlug(tt.in); got != tt.want {
				t.Fatalf("PathSlug(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
