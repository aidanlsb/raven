package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/skillsvc"
)

// HandleSkillList executes the canonical `skill list` command.
func HandleSkillList(_ context.Context, req commandexec.Request) commandexec.Result {
	result, err := skillsvc.List(skillsvc.ListRequest{
		Target:        strings.TrimSpace(stringArg(req.Args, "target")),
		Scope:         strings.TrimSpace(stringArg(req.Args, "scope")),
		Dest:          strings.TrimSpace(stringArg(req.Args, "dest")),
		InstalledOnly: boolArg(req.Args, "installed"),
	})
	if err != nil {
		return mapSkillSvcFailure(err)
	}

	data := map[string]interface{}{
		"skills": result.Skills,
	}
	if strings.TrimSpace(result.Target) != "" {
		data["target"] = result.Target
		data["scope"] = result.Scope
		data["root"] = result.Root
	}
	return commandexec.Success(data, &commandexec.Meta{Count: len(result.Skills)})
}

// HandleSkillInstall executes the canonical `skill install` command.
func HandleSkillInstall(_ context.Context, req commandexec.Request) commandexec.Result {
	name := strings.TrimSpace(stringArg(req.Args, "name"))
	if name == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "specify skill name", nil, "Usage: rvn skill install <name>")
	}

	result, err := skillsvc.Install(skillsvc.InstallRequest{
		Name:    name,
		Target:  strings.TrimSpace(stringArg(req.Args, "target")),
		Scope:   strings.TrimSpace(stringArg(req.Args, "scope")),
		Dest:    strings.TrimSpace(stringArg(req.Args, "dest")),
		Force:   boolArg(req.Args, "force"),
		Confirm: boolArg(req.Args, "confirm"),
	})
	if err != nil {
		return mapSkillSvcFailure(err)
	}

	data := map[string]interface{}{
		"mode":       result.Mode,
		"skill_name": result.SkillName,
		"target":     result.Target,
		"plan":       result.Plan,
	}
	if result.ActionsApplied > 0 {
		data["actions_applied"] = result.ActionsApplied
	}
	if result.Receipt != nil {
		data["receipt"] = result.Receipt
	}
	return commandexec.Success(data, nil)
}

// HandleSkillRemove executes the canonical `skill remove` command.
func HandleSkillRemove(_ context.Context, req commandexec.Request) commandexec.Result {
	name := strings.TrimSpace(stringArg(req.Args, "name"))
	if name == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "specify skill name", nil, "Usage: rvn skill remove <name>")
	}

	result, err := skillsvc.Remove(skillsvc.RemoveRequest{
		Name:    name,
		Target:  strings.TrimSpace(stringArg(req.Args, "target")),
		Scope:   strings.TrimSpace(stringArg(req.Args, "scope")),
		Dest:    strings.TrimSpace(stringArg(req.Args, "dest")),
		Confirm: boolArg(req.Args, "confirm"),
	})
	if err != nil {
		return mapSkillSvcFailure(err)
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
	result, err := skillsvc.Doctor(skillsvc.DoctorRequest{
		Target: strings.TrimSpace(stringArg(req.Args, "target")),
		Scope:  strings.TrimSpace(stringArg(req.Args, "scope")),
		Dest:   strings.TrimSpace(stringArg(req.Args, "dest")),
	})
	if err != nil {
		return mapSkillSvcFailure(err)
	}

	return commandexec.Success(map[string]interface{}{
		"reports": result.Reports,
	}, &commandexec.Meta{Count: len(result.Reports)})
}

func mapSkillSvcFailure(err error) commandexec.Result {
	svcErr, ok := skillsvc.AsError(err)
	if !ok {
		return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
	}

	code := string(svcErr.Code)
	if svcErr.Code == skillsvc.CodeInternal {
		code = "INTERNAL_ERROR"
	}
	return commandexec.Failure(code, svcErr.Message, svcErr.Details, svcErr.Suggestion)
}
