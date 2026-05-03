package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatalog(t *testing.T) {
	t.Parallel()
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	expected := []string{
		"raven-core",
		"raven-maintenance",
		"raven-query",
		"raven-schema",
		"raven-templates",
		"raven-vault-admin",
	}
	for _, skillID := range expected {
		if _, ok := catalog[skillID]; !ok {
			t.Fatalf("catalog missing %s", skillID)
		}
	}
}

func TestResolveInstallRootOverride(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()
	got, err := ResolveInstallRoot(TargetCodex, ScopeUser, "custom/skills", cwd)
	if err != nil {
		t.Fatalf("ResolveInstallRoot() error = %v", err)
	}
	want := filepath.Join(cwd, "custom", "skills")
	if got != want {
		t.Fatalf("ResolveInstallRoot() = %q, want %q", got, want)
	}
}

func TestInstallLifecycle(t *testing.T) {
	t.Parallel()
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	skill := catalog["raven-core"]
	root := t.TempDir()

	plan, err := PlanInstall(skill, TargetCodex, ScopeProject, root, false)
	if err != nil {
		t.Fatalf("PlanInstall() error = %v", err)
	}
	if len(plan.Conflicts) != 0 {
		t.Fatalf("PlanInstall() conflicts = %v, want none", plan.Conflicts)
	}
	if len(plan.Actions) == 0 {
		t.Fatalf("PlanInstall() returned no actions")
	}

	receipt, applied, err := ApplyInstall(plan)
	if err != nil {
		t.Fatalf("ApplyInstall() error = %v", err)
	}
	if applied == 0 {
		t.Fatalf("ApplyInstall() applied = 0, want > 0")
	}
	if receipt == nil || receipt.Skill != "raven-core" {
		t.Fatalf("ApplyInstall() receipt = %#v", receipt)
	}

	skillPath := filepath.Join(root, "raven-core")
	if _, err := os.Stat(filepath.Join(skillPath, "SKILL.md")); err != nil {
		t.Fatalf("expected SKILL.md after install: %v", err)
	}

	secondPlan, err := PlanInstall(skill, TargetCodex, ScopeProject, root, false)
	if err != nil {
		t.Fatalf("second PlanInstall() error = %v", err)
	}
	if len(secondPlan.Actions) != 0 {
		t.Fatalf("second PlanInstall() actions = %v, want none", secondPlan.Actions)
	}

	removePlan, err := PlanRemove("raven-core", TargetCodex, ScopeProject, root)
	if err != nil {
		t.Fatalf("PlanRemove() error = %v", err)
	}
	if !removePlan.Exists {
		t.Fatalf("PlanRemove() Exists = false, want true")
	}
	if err := ApplyRemove(removePlan); err != nil {
		t.Fatalf("ApplyRemove() error = %v", err)
	}
	if _, err := os.Stat(skillPath); !os.IsNotExist(err) {
		t.Fatalf("skill path still exists after remove")
	}
}

func TestPlanInstallConflictAndForce(t *testing.T) {
	t.Parallel()
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	skill := catalog["raven-core"]
	root := t.TempDir()
	skillPath := filepath.Join(root, "raven-core")
	if err := os.MkdirAll(skillPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillPath, "SKILL.md"), []byte("different"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	plan, err := PlanInstall(skill, TargetCodex, ScopeProject, root, false)
	if err != nil {
		t.Fatalf("PlanInstall() error = %v", err)
	}
	if len(plan.Conflicts) == 0 {
		t.Fatalf("PlanInstall() expected conflicts, got none")
	}

	forcePlan, err := PlanInstall(skill, TargetCodex, ScopeProject, root, true)
	if err != nil {
		t.Fatalf("PlanInstall(force) error = %v", err)
	}
	if len(forcePlan.Conflicts) != 0 {
		t.Fatalf("PlanInstall(force) conflicts = %v, want none", forcePlan.Conflicts)
	}
}

func TestPlanSyncNamedInstallsMissingSkill(t *testing.T) {
	t.Parallel()
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	root := t.TempDir()

	plan, err := PlanSync(catalog, "raven-core", TargetCodex, ScopeProject, root)
	if err != nil {
		t.Fatalf("PlanSync() error = %v", err)
	}
	if plan.Installed != 1 || len(plan.Actions) != 1 || plan.Actions[0].Op != "install" {
		t.Fatalf("PlanSync() install summary = installed %d actions %#v, want one install", plan.Installed, plan.Actions)
	}
	if _, err := ApplySync(plan); err != nil {
		t.Fatalf("ApplySync() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "raven-core", "SKILL.md")); err != nil {
		t.Fatalf("expected SKILL.md after sync: %v", err)
	}
}

func TestPlanSyncNamedUpdatesManagedSkill(t *testing.T) {
	t.Parallel()
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	skill := catalog["raven-core"]
	root := t.TempDir()

	installPlan, err := PlanInstall(skill, TargetCodex, ScopeProject, root, false)
	if err != nil {
		t.Fatalf("PlanInstall() error = %v", err)
	}
	if _, _, err := ApplyInstall(installPlan); err != nil {
		t.Fatalf("ApplyInstall() error = %v", err)
	}
	skillPath := filepath.Join(root, "raven-core")
	if err := os.WriteFile(filepath.Join(skillPath, "SKILL.md"), []byte("local edit"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	syncPlan, err := PlanSync(catalog, "raven-core", TargetCodex, ScopeProject, root)
	if err != nil {
		t.Fatalf("PlanSync() error = %v", err)
	}
	if syncPlan.Updated != 1 || len(syncPlan.Actions) != 1 || syncPlan.Actions[0].Op != "update" {
		t.Fatalf("PlanSync() update summary = updated %d actions %#v, want one update", syncPlan.Updated, syncPlan.Actions)
	}
	if _, err := ApplySync(syncPlan); err != nil {
		t.Fatalf("ApplySync() error = %v", err)
	}

	rendered, err := RenderFiles(skill, TargetCodex)
	if err != nil {
		t.Fatalf("RenderFiles() error = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(skillPath, "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(rendered["SKILL.md"]) {
		t.Fatalf("SKILL.md was not restored to shipped content")
	}
}

func TestPlanSyncWithoutNameSurfacesMissingSkillsOnly(t *testing.T) {
	t.Parallel()
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	root := t.TempDir()

	plan, err := PlanSync(catalog, "", TargetCodex, ScopeProject, root)
	if err != nil {
		t.Fatalf("PlanSync() error = %v", err)
	}
	if len(plan.Actions) != 0 {
		t.Fatalf("PlanSync() actions = %#v, want none", plan.Actions)
	}
	if len(plan.MissingAvailable) != len(catalog) {
		t.Fatalf("PlanSync() missing = %d, want %d", len(plan.MissingAvailable), len(catalog))
	}
	if _, err := os.Stat(filepath.Join(root, "raven-core")); !os.IsNotExist(err) {
		t.Fatalf("no-name sync created raven-core, err = %v", err)
	}
}

func TestPlanSyncRemovesOnlyReceiptListedFilesForUnshippedSkill(t *testing.T) {
	t.Parallel()
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	skill := catalog["raven-core"]
	root := t.TempDir()

	installPlan, err := PlanInstall(skill, TargetCodex, ScopeProject, root, false)
	if err != nil {
		t.Fatalf("PlanInstall() error = %v", err)
	}
	if _, _, err := ApplyInstall(installPlan); err != nil {
		t.Fatalf("ApplyInstall() error = %v", err)
	}
	skillPath := filepath.Join(root, "raven-core")
	extraPath := filepath.Join(skillPath, "notes.md")
	if err := os.WriteFile(extraPath, []byte("user note"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	withoutCore := make(map[string]*Skill, len(catalog)-1)
	for id, item := range catalog {
		if id != "raven-core" {
			withoutCore[id] = item
		}
	}
	syncPlan, err := PlanSync(withoutCore, "", TargetCodex, ScopeProject, root)
	if err != nil {
		t.Fatalf("PlanSync() error = %v", err)
	}
	if syncPlan.Deleted != 1 {
		t.Fatalf("PlanSync() deleted = %d, want 1", syncPlan.Deleted)
	}
	if _, err := ApplySync(syncPlan); err != nil {
		t.Fatalf("ApplySync() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillPath, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("managed SKILL.md still exists, err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillPath, receiptFileName)); !os.IsNotExist(err) {
		t.Fatalf("receipt still exists, err = %v", err)
	}
	if _, err := os.Stat(extraPath); err != nil {
		t.Fatalf("user-created file was not preserved: %v", err)
	}
}

func TestPlanSyncSkipsUnreceiptedSkillDirectory(t *testing.T) {
	t.Parallel()
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	root := t.TempDir()
	skillPath := filepath.Join(root, "raven-core")
	if err := os.MkdirAll(skillPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	customPath := filepath.Join(skillPath, "SKILL.md")
	if err := os.WriteFile(customPath, []byte("handcrafted"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	plan, err := PlanSync(catalog, "raven-core", TargetCodex, ScopeProject, root)
	if err != nil {
		t.Fatalf("PlanSync() error = %v", err)
	}
	if plan.Skipped != 1 || len(plan.Actions) != 1 || plan.Actions[0].Op != "skip_conflict" {
		t.Fatalf("PlanSync() skip summary = skipped %d actions %#v, want one skip_conflict", plan.Skipped, plan.Actions)
	}
	if _, err := ApplySync(plan); err != nil {
		t.Fatalf("ApplySync() error = %v", err)
	}
	got, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "handcrafted" {
		t.Fatalf("custom skill was overwritten: %q", got)
	}
}
