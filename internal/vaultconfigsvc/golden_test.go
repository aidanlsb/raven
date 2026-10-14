package vaultconfigsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// TestMutationOutputYAML locks the raven.yaml bytes written by each mutation
// path. Update with UPDATE_GOLDEN=1 after an intentional output change.
func TestMutationOutputYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		seed    string
		apply   func(*testing.T, *vaultruntime.Runtime)
		missing bool
	}{
		{
			name: "set_auto_reindex_false_creates",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := SetAutoReindex(rt, false)
					return err
				})
			},
		},
		{
			name: "set_auto_reindex_true_creates",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := SetAutoReindex(rt, true)
					return err
				})
			},
		},
		{
			name: "set_auto_reindex_noop_preserves_seed",
			seed: "auto_reindex: false\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := SetAutoReindex(rt, false)
					return err
				})
			},
		},
		{
			name: "unset_auto_reindex_clears",
			seed: "auto_reindex: false\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := UnsetAutoReindex(rt)
					return err
				})
			},
		},
		{
			name:    "unset_auto_reindex_missing_is_noop",
			missing: true,
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := UnsetAutoReindex(rt)
					return err
				})
			},
		},
		{
			name: "set_directories_creates",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := SetDirectories(rt, strPtr("./journal"), strPtr("objects"), nil, strPtr("templates/custom"))
					return err
				})
			},
		},
		{
			name: "set_directories_partial_keeps_others",
			seed: "directories:\n  daily: journal/\n  type: objects/\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := SetDirectories(rt, nil, nil, strPtr("pages"), nil)
					return err
				})
			},
		},
		{
			name: "set_directories_noop_preserves_seed",
			seed: "directories:\n  daily: journal/\n  type: objects/\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := SetDirectories(rt, strPtr("journal/"), nil, nil, nil)
					return err
				})
			},
		},
		{
			name: "set_directories_deprecated_alias_noop_preserves_seed",
			seed: "directories:\n  pages: notes/\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := SetDirectories(rt, nil, nil, strPtr("notes/"), nil)
					return err
				})
			},
		},
		{
			name: "unset_directories_all_removes_block",
			seed: "directories:\n  daily: journal/\n  type: objects/\n  template: templates/custom/\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := UnsetDirectories(rt, true, true, false, true)
					return err
				})
			},
		},
		{
			name: "unset_directories_one_field",
			seed: "directories:\n  daily: journal/\n  type: objects/\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := UnsetDirectories(rt, true, false, false, false)
					return err
				})
			},
		},
		{
			name: "set_capture_creates",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := SetCapture(rt, strPtr("inbox.md"), strPtr("## Captured"))
					return err
				})
			},
		},
		{
			name: "set_capture_heading_only",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := SetCapture(rt, nil, strPtr("## Inbox"))
					return err
				})
			},
		},
		{
			name: "set_capture_noop_preserves_seed",
			seed: "capture:\n  destination: inbox.md\n  heading: '## Captured'\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := SetCapture(rt, strPtr("inbox.md"), nil)
					return err
				})
			},
		},
		{
			name: "unset_capture_clears",
			seed: "capture:\n  destination: inbox.md\n  heading: '## Captured'\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := UnsetCapture(rt, true, true)
					return err
				})
			},
		},
		{
			name: "unset_capture_one_field",
			seed: "capture:\n  destination: inbox.md\n  heading: '## Captured'\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := UnsetCapture(rt, false, true)
					return err
				})
			},
		},
		{
			name: "set_deletion_creates",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := SetDeletion(rt, strPtr("trash"), strPtr("./archive//trash"))
					return err
				})
			},
		},
		{
			name: "set_deletion_noop_preserves_seed",
			seed: "deletion:\n  behavior: trash\n  trash_dir: archive/trash\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := SetDeletion(rt, strPtr("trash"), nil)
					return err
				})
			},
		},
		{
			name: "unset_deletion_clears",
			seed: "deletion:\n  behavior: permanent\n  trash_dir: archive/trash\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := UnsetDeletion(rt, true, true)
					return err
				})
			},
		},
		{
			name: "add_protected_prefix_creates",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := AddProtectedPrefix(rt, "./notes//team")
					return err
				})
			},
		},
		{
			name: "add_protected_prefix_appends_sorted",
			seed: "protected_prefixes:\n  - private/\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := AddProtectedPrefix(rt, "notes/team")
					return err
				})
			},
		},
		{
			name: "add_protected_prefix_duplicate_preserves_seed",
			seed: "protected_prefixes:\n  - private/\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := AddProtectedPrefix(rt, "private")
					return err
				})
			},
		},
		{
			name: "remove_protected_prefix_clears_key",
			seed: "protected_prefixes:\n  - private/\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := RemoveProtectedPrefix(rt, "private")
					return err
				})
			},
		},
		{
			name: "add_exclude_creates",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := AddExclude(rt, "  *.plan.md  ")
					return err
				})
			},
		},
		{
			name: "add_exclude_appends_preserves_order",
			seed: "exclude:\n  - .cursor/\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := AddExclude(rt, "*.plan.md")
					return err
				})
			},
		},
		{
			name: "add_exclude_duplicate_preserves_seed",
			seed: "exclude:\n  - '*.plan.md'\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := AddExclude(rt, " *.plan.md ")
					return err
				})
			},
		},
		{
			name: "remove_exclude_one",
			seed: "exclude:\n  - .cursor/\n  - '*.plan.md'\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := RemoveExclude(rt, " *.plan.md ")
					return err
				})
			},
		},
		{
			name: "set_deletion_rewrites_file_in_struct_order",
			seed: "capture:\n  destination: inbox.md\nauto_reindex: false\n",
			apply: func(t *testing.T, rt *vaultruntime.Runtime) {
				mustApply(t, func() error {
					_, err := SetDeletion(rt, strPtr("permanent"), nil)
					return err
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vaultPath := t.TempDir()
			if tt.seed != "" {
				if err := os.WriteFile(filepath.Join(vaultPath, "raven.yaml"), []byte(tt.seed), 0o644); err != nil {
					t.Fatalf("write seed raven.yaml: %v", err)
				}
			}
			tt.apply(t, configTestRuntime(t, vaultPath))
			assertConfigYAMLGolden(t, vaultPath, tt.name, tt.missing)
		})
	}
}

func mustApply(t *testing.T, fn func() error) {
	t.Helper()
	if err := fn(); err != nil {
		t.Fatal(err)
	}
}

func assertConfigYAMLGolden(t *testing.T, vaultPath, name string, wantMissing bool) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(vaultPath, "raven.yaml"))
	missing := os.IsNotExist(err)
	if err != nil && !missing {
		t.Fatalf("read raven.yaml: %v", err)
	}
	if wantMissing {
		if !missing {
			t.Fatalf("expected raven.yaml to be absent, got:\n%s", got)
		}
		return
	}

	goldenPath := filepath.Join("testdata", name+".yaml")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	if string(got) != string(want) {
		t.Fatalf("raven.yaml mismatch for %s\n got:\n%s\nwant:\n%s", name, got, want)
	}
}
