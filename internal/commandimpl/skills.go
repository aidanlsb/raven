package commandimpl

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/skills"
)

// HandleSkillList executes the canonical `skill list` command.
func HandleSkillList(_ context.Context, req commandexec.Request) commandexec.Result {
	catalog, err := skills.LoadCatalog()
	if err != nil {
		return commandexec.Failure("INTERNAL_ERROR", "failed to load skill catalog", nil, "")
	}

	targetRaw := strings.TrimSpace(stringArg(req.Args, "target"))
	installedOnly := boolArg(req.Args, "installed")

	if targetRaw == "" {
		if installedOnly {
			return commandexec.Failure("INVALID_INPUT", "--installed requires --target", nil, "Specify --target codex|claude|cursor")
		}
		return commandexec.Success(map[string]interface{}{
			"skills": skills.SortedSummaries(catalog),
		}, &commandexec.Meta{Count: len(catalog)})
	}

	target, err := skills.ParseTarget(targetRaw)
	if err != nil {
		return commandexec.Failure("SKILL_TARGET_UNSUPPORTED", err.Error(), nil, "Use --target codex|claude|cursor")
	}
	scope, err := skills.ParseScope(strings.TrimSpace(stringArg(req.Args, "scope")))
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "Use --scope user|project")
	}
	root, err := skills.ResolveInstallRoot(target, scope, strings.TrimSpace(stringArg(req.Args, "dest")), "")
	if err != nil {
		return commandexec.Failure("SKILL_PATH_UNRESOLVED", err.Error(), nil, "Use --dest to set an explicit install root")
	}

	items := skills.InstalledSummaries(catalog, root)
	if installedOnly {
		filtered := make([]skills.Summary, 0, len(items))
		for _, item := range items {
			if item.Installed {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	data := map[string]interface{}{
		"skills": items,
		"target": string(target),
		"scope":  string(scope),
		"root":   root,
	}
	return commandexec.Success(data, &commandexec.Meta{Count: len(items)})
}

// HandleSkillInstall executes the canonical `skill install` command.
func HandleSkillInstall(_ context.Context, req commandexec.Request) commandexec.Result {
	name := strings.TrimSpace(stringArg(req.Args, "name"))
	if name == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "specify skill name", nil, "Usage: rvn skill install <name>")
	}

	catalog, err := skills.LoadCatalog()
	if err != nil {
		return commandexec.Failure("INTERNAL_ERROR", "failed to load skill catalog", nil, "")
	}

	skillDef, ok := catalog[name]
	if !ok {
		available := skills.SortedSummaries(catalog)
		names := make([]string, 0, len(available))
		for _, item := range available {
			names = append(names, item.Name)
		}
		return commandexec.Failure(
			"SKILL_NOT_FOUND",
			fmt.Sprintf("skill '%s' not found", name),
			map[string]interface{}{"available": names},
			"Run 'rvn skill list' to see available skills",
		)
	}

	target, err := skills.ParseTarget(strings.TrimSpace(stringArg(req.Args, "target")))
	if err != nil {
		return commandexec.Failure("SKILL_TARGET_UNSUPPORTED", err.Error(), nil, "Use --target codex|claude|cursor")
	}
	scope, err := skills.ParseScope(strings.TrimSpace(stringArg(req.Args, "scope")))
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "Use --scope user|project")
	}
	root, err := skills.ResolveInstallRoot(target, scope, strings.TrimSpace(stringArg(req.Args, "dest")), "")
	if err != nil {
		return commandexec.Failure("SKILL_PATH_UNRESOLVED", err.Error(), nil, "Use --dest to set an explicit install root")
	}

	force := boolArg(req.Args, "force")
	plan, err := skills.PlanInstall(skillDef, target, scope, root, force)
	if err != nil {
		return commandexec.Failure("INTERNAL_ERROR", "failed to build install plan", nil, "")
	}
	if len(plan.Conflicts) > 0 {
		return commandexec.Failure(
			"SKILL_INSTALL_CONFLICT",
			"install has conflicts",
			map[string]interface{}{"conflicts": plan.Conflicts, "plan": plan},
			"Use --force to overwrite conflicting files",
		)
	}

	if !boolArg(req.Args, "confirm") && !req.Confirm {
		return commandexec.Success(map[string]interface{}{
			"mode":       "preview",
			"skill_name": name,
			"target":     string(target),
			"plan":       plan,
		}, nil)
	}

	receipt, applied, err := skills.ApplyInstall(plan)
	if err != nil {
		return commandexec.Failure("FILE_WRITE_ERROR", "failed to apply install", nil, "")
	}

	data := map[string]interface{}{
		"mode":       "applied",
		"skill_name": name,
		"target":     string(target),
		"plan":       plan,
	}
	if applied > 0 {
		data["actions_applied"] = applied
	}
	if receipt != nil {
		data["receipt"] = receipt
	}
	return commandexec.Success(data, nil)
}

// HandleSkillRemove executes the canonical `skill remove` command.
func HandleSkillRemove(_ context.Context, req commandexec.Request) commandexec.Result {
	name := strings.TrimSpace(stringArg(req.Args, "name"))
	if name == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "specify skill name", nil, "Usage: rvn skill remove <name>")
	}

	catalog, err := skills.LoadCatalog()
	if err != nil {
		return commandexec.Failure("INTERNAL_ERROR", "failed to load skill catalog", nil, "")
	}
	if _, ok := catalog[name]; !ok {
		return commandexec.Failure("SKILL_NOT_FOUND", fmt.Sprintf("skill '%s' not found", name), nil, "Run 'rvn skill list' to see available skills")
	}

	target, err := skills.ParseTarget(strings.TrimSpace(stringArg(req.Args, "target")))
	if err != nil {
		return commandexec.Failure("SKILL_TARGET_UNSUPPORTED", err.Error(), nil, "Use --target codex|claude|cursor")
	}
	scope, err := skills.ParseScope(strings.TrimSpace(stringArg(req.Args, "scope")))
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "Use --scope user|project")
	}
	root, err := skills.ResolveInstallRoot(target, scope, strings.TrimSpace(stringArg(req.Args, "dest")), "")
	if err != nil {
		return commandexec.Failure("SKILL_PATH_UNRESOLVED", err.Error(), nil, "Use --dest to set an explicit install root")
	}

	plan, err := skills.PlanRemove(name, target, scope, root)
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "")
	}
	if !plan.Exists {
		return commandexec.Failure(
			"SKILL_NOT_INSTALLED",
			fmt.Sprintf("skill '%s' is not installed for target '%s'", name, target),
			nil,
			"Run 'rvn skill list --target ... --installed' to see installed skills",
		)
	}

	if !boolArg(req.Args, "confirm") && !req.Confirm {
		return commandexec.Success(map[string]interface{}{
			"mode":       "preview",
			"skill_name": name,
			"plan":       plan,
		}, nil)
	}

	if err := skills.ApplyRemove(plan); err != nil {
		return commandexec.Failure("FILE_WRITE_ERROR", "failed to apply removal", nil, "")
	}

	data := map[string]interface{}{
		"mode":       "applied",
		"removed":    true,
		"skill_name": name,
		"plan":       plan,
	}
	return commandexec.Success(data, nil)
}

// HandleSkillDoctor executes the canonical `skill doctor` command.
func HandleSkillDoctor(_ context.Context, req commandexec.Request) commandexec.Result {
	catalog, err := skills.LoadCatalog()
	if err != nil {
		return commandexec.Failure("INTERNAL_ERROR", "failed to load skill catalog", nil, "")
	}

	scope, err := skills.ParseScope(strings.TrimSpace(stringArg(req.Args, "scope")))
	if err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "Use --scope user|project")
	}

	reports := make([]skills.DoctorReport, 0)
	targetRaw := strings.TrimSpace(stringArg(req.Args, "target"))

	if targetRaw == "" {
		destRaw := strings.TrimSpace(stringArg(req.Args, "dest"))
		if destRaw != "" {
			return commandexec.Failure("INVALID_INPUT", "--dest requires --target", nil, "Specify --target codex|claude|cursor when using --dest")
		}
		for _, target := range skills.AllTargets() {
			root, err := skills.ResolveInstallRoot(target, scope, "", "")
			if err != nil {
				reports = append(reports, skills.DoctorReport{
					Target: string(target),
					Scope:  string(scope),
					Issues: []string{fmt.Sprintf("failed to resolve root: %v", err)},
				})
				continue
			}
			reports = append(reports, skills.Doctor(catalog, target, scope, root))
		}
	} else {
		target, err := skills.ParseTarget(targetRaw)
		if err != nil {
			return commandexec.Failure("SKILL_TARGET_UNSUPPORTED", err.Error(), nil, "Use --target codex|claude|cursor")
		}
		root, err := skills.ResolveInstallRoot(target, scope, strings.TrimSpace(stringArg(req.Args, "dest")), "")
		if err != nil {
			return commandexec.Failure("SKILL_PATH_UNRESOLVED", err.Error(), nil, "Use --dest to set an explicit install root")
		}
		reports = append(reports, skills.Doctor(catalog, target, scope, root))
	}

	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Target < reports[j].Target
	})
	return commandexec.Success(map[string]interface{}{
		"reports": reports,
	}, &commandexec.Meta{Count: len(reports)})
}
