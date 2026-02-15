package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestCreateMissingPageUsesDirectoryRoots(t *testing.T) {
	vaultPath := t.TempDir()

	schemaYAML := strings.TrimSpace(`
version: 2
types:
  meeting:
    default_path: meeting/
`) + "\n"
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	s, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}

	tests := []struct {
		name            string
		targetPath      string
		wantRelPath     string
		unwantedRelPath string
	}{
		{
			name:            "explicit directory in target path",
			targetPath:      "meeting/team-sync",
			wantRelPath:     "objects/meeting/team-sync.md",
			unwantedRelPath: "meeting/team-sync.md",
		},
		{
			name:            "default_path resolution with object root",
			targetPath:      "retro",
			wantRelPath:     "objects/meeting/retro.md",
			unwantedRelPath: "meeting/retro.md",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := createMissingPage(vaultPath, s, tc.targetPath, "meeting", "objects/", ""); err != nil {
				t.Fatalf("createMissingPage failed: %v", err)
			}

			if _, err := os.Stat(filepath.Join(vaultPath, tc.wantRelPath)); err != nil {
				t.Fatalf("expected created file at %s: %v", tc.wantRelPath, err)
			}

			if _, err := os.Stat(filepath.Join(vaultPath, tc.unwantedRelPath)); err == nil {
				t.Fatalf("did not expect file at %s", tc.unwantedRelPath)
			}
		})
	}
}
