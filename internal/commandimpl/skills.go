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

	return commandexec.Success(*result, &commandexec.Meta{Count: len(result.Skills)})
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

	return commandexec.Success(*result, nil)
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

	return commandexec.Success(*result, nil)
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

	return commandexec.Success(*result, &commandexec.Meta{Count: len(result.Reports)})
}
