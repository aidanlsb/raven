package skillsvc

import (
	"path/filepath"
	"testing"
)

func TestListInstalledUsesResolvedRootWithoutTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result, err := List(ListRequest{
		Dest:          root,
		InstalledOnly: true,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if result.Scope != "user" {
		t.Fatalf("List() scope = %q, want user", result.Scope)
	}
	if result.Root != root {
		t.Fatalf("List() root = %q, want %q", result.Root, root)
	}
	if len(result.Skills) != 0 {
		t.Fatalf("List() installed skills = %d, want 0", len(result.Skills))
	}
}

func TestSyncDefaultsToAgentSkillsRoot(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(cwd)

	result, err := Sync(SyncRequest{Name: "raven-core"})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	wantRoot := filepath.Join(home, ".agents", "skills")
	if result.Plan.Root != wantRoot {
		t.Fatalf("Sync() root = %q, want %q", result.Plan.Root, wantRoot)
	}
	if result.Plan.Scope != "user" {
		t.Fatalf("Sync() scope = %q, want user", result.Plan.Scope)
	}
}

func TestDoctorReturnsSingleResolvedRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result, err := Doctor(DoctorRequest{
		Scope: "project",
		Dest:  root,
	})
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if len(result.Reports) != 1 {
		t.Fatalf("Doctor() reports = %d, want 1", len(result.Reports))
	}
	if result.Reports[0].Root != root {
		t.Fatalf("Doctor() root = %q, want %q", result.Reports[0].Root, root)
	}
	if result.Reports[0].Scope != "project" {
		t.Fatalf("Doctor() scope = %q, want project", result.Reports[0].Scope)
	}
}
