package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatalog(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	if _, ok := catalog["raven-core"]; !ok {
		t.Fatalf("catalog missing raven-core")
	}
	if _, ok := catalog["raven-schema"]; !ok {
		t.Fatalf("catalog missing raven-schema")
	}
}

func TestResolveInstallRootOverride(t *testing.T) {
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
