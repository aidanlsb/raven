package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/svcerr"
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

func TestInstallDefaultsToAgentSkillsRoot(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Chdir(cwd)

	result, err := Install(InstallRequest{Names: []string{"raven-core"}})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	wantRoot := filepath.Join(home, ".agents", "skills")
	if result.Root != wantRoot {
		t.Fatalf("Install() root = %q, want %q", result.Root, wantRoot)
	}
	if result.Scope != "user" {
		t.Fatalf("Install() scope = %q, want user", result.Scope)
	}
}

func TestInstallPreviewDoesNotWriteAnySkills(t *testing.T) {
	t.Parallel()

	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	root := t.TempDir()
	result, err := Install(InstallRequest{Scope: "project", Dest: root})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.Mode != "preview" {
		t.Fatalf("Install() mode = %q, want preview", result.Mode)
	}
	if !result.NeedsConfirm {
		t.Fatalf("Install() needs_confirm = false, want true when skills are missing")
	}
	// Default (no names) plans the full shipped catalog.
	if len(result.Skills) != len(catalog) {
		t.Fatalf("Install() planned %d skills, want %d (full catalog)", len(result.Skills), len(catalog))
	}
	if result.Installed != len(catalog) {
		t.Fatalf("Install() installed = %d, want %d", result.Installed, len(catalog))
	}
	if result.ActionsApplied != 0 {
		t.Fatalf("Install() applied = %d, want 0 in preview", result.ActionsApplied)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("Install() preview wrote %d entries to disk, want 0", len(entries))
	}
}

func TestInstallConfirmInstallsAllShippedSkills(t *testing.T) {
	t.Parallel()

	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	root := t.TempDir()
	result, err := Install(InstallRequest{Scope: "project", Dest: root, Confirm: true})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.Mode != "applied" {
		t.Fatalf("Install() mode = %q, want applied", result.Mode)
	}
	if result.NeedsConfirm {
		t.Fatalf("Install() needs_confirm = true, want false after apply")
	}
	if result.ActionsApplied == 0 {
		t.Fatalf("Install() applied = 0, want > 0")
	}
	for name := range catalog {
		if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); err != nil {
			t.Fatalf("Install() did not install %s: %v", name, err)
		}
	}

	// Re-running against an already-installed set should report no changes.
	rerun, err := Install(InstallRequest{Scope: "project", Dest: root})
	if err != nil {
		t.Fatalf("Install() rerun error = %v", err)
	}
	if rerun.NeedsConfirm {
		t.Fatalf("Install() rerun needs_confirm = true, want false when up to date")
	}
	if rerun.Installed != 0 || rerun.Updated != 0 {
		t.Fatalf("Install() rerun installed=%d updated=%d, want 0/0", rerun.Installed, rerun.Updated)
	}
}

func TestInstallConfirmReconcilesFullManagedSet(t *testing.T) {
	t.Parallel()

	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	root := t.TempDir()
	if _, err := Install(InstallRequest{
		Names:   []string{"raven-core"},
		Scope:   "project",
		Dest:    root,
		Confirm: true,
	}); err != nil {
		t.Fatalf("Install() initial raven-core error = %v", err)
	}

	corePath := filepath.Join(root, "raven-core", "SKILL.md")
	if err := os.WriteFile(corePath, []byte("outdated managed content"), 0o644); err != nil {
		t.Fatalf("WriteFile() raven-core error = %v", err)
	}

	retiredPath := filepath.Join(root, "raven-retired")
	if err := os.MkdirAll(retiredPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() retired skill error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(retiredPath, "SKILL.md"), []byte("retired managed content"), 0o644); err != nil {
		t.Fatalf("WriteFile() retired SKILL.md error = %v", err)
	}
	userFile := filepath.Join(retiredPath, "notes.md")
	if err := os.WriteFile(userFile, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("WriteFile() retired notes.md error = %v", err)
	}
	if err := writeReceipt(filepath.Join(retiredPath, receiptFileName), &Receipt{
		Skill:   "raven-retired",
		Version: 1,
		Scope:   "project",
		Files:   []string{"SKILL.md"},
	}); err != nil {
		t.Fatalf("writeReceipt() error = %v", err)
	}

	result, err := Install(InstallRequest{Scope: "project", Dest: root, Confirm: true})
	if err != nil {
		t.Fatalf("Install() reconcile error = %v", err)
	}
	if result.Installed != len(catalog)-1 || result.Updated != 1 || result.Deleted != 1 {
		t.Fatalf(
			"Install() summary installed=%d updated=%d deleted=%d, want %d/1/1",
			result.Installed,
			result.Updated,
			result.Deleted,
			len(catalog)-1,
		)
	}

	renderedCore, err := RenderFiles(catalog["raven-core"])
	if err != nil {
		t.Fatalf("RenderFiles() error = %v", err)
	}
	gotCore, err := os.ReadFile(corePath)
	if err != nil {
		t.Fatalf("ReadFile() raven-core error = %v", err)
	}
	if string(gotCore) != string(renderedCore["SKILL.md"]) {
		t.Fatal("Install() did not replace existing Raven-managed skill content")
	}
	if _, err := os.Stat(filepath.Join(root, "raven-query", "SKILL.md")); err != nil {
		t.Fatalf("Install() did not install missing shipped skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(retiredPath, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("Install() left retired managed file in place, err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(retiredPath, receiptFileName)); !os.IsNotExist(err) {
		t.Fatalf("Install() left retired receipt in place, err = %v", err)
	}
	if _, err := os.Stat(userFile); err != nil {
		t.Fatalf("Install() removed non-Raven file from retired skill path: %v", err)
	}
}

func TestInstallNarrowsToRequestedSkills(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result, err := Install(InstallRequest{
		Names:   []string{"raven-core", "raven-query"},
		Scope:   "project",
		Dest:    root,
		Confirm: true,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(result.Skills) != 2 {
		t.Fatalf("Install() planned %d skills, want 2", len(result.Skills))
	}
	for _, name := range []string{"raven-core", "raven-query"} {
		if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); err != nil {
			t.Fatalf("Install() did not install %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "raven-onboarding")); !os.IsNotExist(err) {
		t.Fatalf("Install() installed an unrequested skill, err = %v", err)
	}
}

func TestInstallNamedDoesNotDeleteUnrelatedUnshippedSkill(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	retiredPath := filepath.Join(root, "raven-retired")
	if err := os.MkdirAll(retiredPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	managedPath := filepath.Join(retiredPath, "SKILL.md")
	if err := os.WriteFile(managedPath, []byte("retired managed content"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := writeReceipt(filepath.Join(retiredPath, receiptFileName), &Receipt{
		Skill:   "raven-retired",
		Version: 1,
		Scope:   "project",
		Files:   []string{"SKILL.md"},
	}); err != nil {
		t.Fatalf("writeReceipt() error = %v", err)
	}

	if _, err := Install(InstallRequest{
		Names:   []string{"raven-core"},
		Scope:   "project",
		Dest:    root,
		Confirm: true,
	}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if _, err := os.Stat(managedPath); err != nil {
		t.Fatalf("named Install() touched unrelated unshipped skill: %v", err)
	}
}

func TestInstallUnknownSkillReturnsNotFound(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	_, err := Install(InstallRequest{Names: []string{"nope"}, Scope: "project", Dest: root})
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("Install() error = %v, want structured service error", err)
	}
	if svcErr.Code != codes.ErrSkillNotFound {
		t.Fatalf("Install() code = %q, want %q", svcErr.Code, codes.ErrSkillNotFound)
	}
}

func TestInstallRealignsManagedSkillOnConfirm(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if _, err := Install(InstallRequest{Names: []string{"raven-core"}, Scope: "project", Dest: root, Confirm: true}); err != nil {
		t.Fatalf("Install() initial error = %v", err)
	}

	// Locally modify a managed skill; install should realign it.
	corePath := filepath.Join(root, "raven-core", "SKILL.md")
	if err := os.WriteFile(corePath, []byte("local edit"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	preview, err := Install(InstallRequest{Names: []string{"raven-core"}, Scope: "project", Dest: root})
	if err != nil {
		t.Fatalf("Install() preview error = %v", err)
	}
	if preview.Updated != 1 || !preview.NeedsConfirm {
		t.Fatalf("Install() preview updated=%d needs_confirm=%v, want 1/true", preview.Updated, preview.NeedsConfirm)
	}

	if _, err := Install(InstallRequest{Names: []string{"raven-core"}, Scope: "project", Dest: root, Confirm: true}); err != nil {
		t.Fatalf("Install() apply error = %v", err)
	}
	got, err := os.ReadFile(corePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) == "local edit" {
		t.Fatalf("Install() did not realign modified raven-core SKILL.md")
	}
}

func TestDoctorReturnsSingleResolvedRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result, err := RunDoctor(DoctorRequest{
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
