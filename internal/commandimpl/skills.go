package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/skills"
)

// HandleSkillList executes the canonical `skill list` command.
func HandleSkillList(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := skills.List(skills.ListRequest{
		Scope:         strings.TrimSpace(stringArg(req.Args, "scope")),
		Dest:          strings.TrimSpace(stringArg(req.Args, "dest")),
		InstalledOnly: boolArg(req.Args, "installed"),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	data := map[string]interface{}{
		"skills": result.Skills,
		"scope":  result.Scope,
		"root":   result.Root,
	}
	return commandexec.Success(data, &commandexec.Meta{Count: len(result.Skills)})
}

// HandleSkillInstall executes the canonical `skill install` command. With no
// names it reconciles the full Raven-managed set to the shipped catalog; names
// narrow installation and alignment to those shipped skills. Preview by
// default; applies on confirm.
func HandleSkillInstall(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := skills.Install(skills.InstallRequest{
		Names:   stringSliceArg(req.Args["names"]),
		Scope:   strings.TrimSpace(stringArg(req.Args, "scope")),
		Dest:    strings.TrimSpace(stringArg(req.Args, "dest")),
		Confirm: req.Confirm,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	data := map[string]interface{}{
		"mode":          result.Mode,
		"scope":         result.Scope,
		"root":          result.Root,
		"needs_confirm": result.NeedsConfirm,
		"requested":     result.Requested,
		"skills":        result.Skills,
		"installed":     result.Installed,
		"updated":       result.Updated,
		"deleted":       result.Deleted,
		"skipped":       result.Skipped,
	}
	if result.ActionsApplied > 0 {
		data["actions_applied"] = result.ActionsApplied
	}
	return commandexec.Success(data, nil)
}

// HandleSkillRemove executes the canonical `skill remove` command.
func HandleSkillRemove(_ context.Context, req commandexec.Request) commandexec.Result {
	name := strings.TrimSpace(stringArg(req.Args, "name"))
	if name == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "specify skill name", nil, "Usage: rvn skill remove <name>")
	}

	result, err := skills.Remove(skills.RemoveRequest{
		Name:    name,
		Scope:   strings.TrimSpace(stringArg(req.Args, "scope")),
		Dest:    strings.TrimSpace(stringArg(req.Args, "dest")),
		Confirm: req.Confirm,
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	data := map[string]interface{}{
		"mode":       result.Mode,
		"skill_name": result.SkillName,
		"plan":       result.Plan,
	}
	if result.Removed {
		data["removed"] = true
	}
	return commandexec.Success(data, nil)
}

// HandleSkillDoctor executes the canonical `skill doctor` command.
func HandleSkillDoctor(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := skills.RunDoctor(skills.DoctorRequest{
		Scope: strings.TrimSpace(stringArg(req.Args, "scope")),
		Dest:  strings.TrimSpace(stringArg(req.Args, "dest")),
	})
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	return commandexec.Success(map[string]interface{}{
		"reports": result.Reports,
	}, &commandexec.Meta{Count: len(result.Reports)})
}
