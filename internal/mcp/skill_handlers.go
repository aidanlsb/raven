package mcp

import (
	"strings"

	"github.com/aidanlsb/raven/internal/skillsvc"
)

func (s *Server) callDirectSkillList(args map[string]interface{}) (string, bool) {
	normalized := normalizeArgs(args)
	result, err := skillsvc.List(skillsvc.ListRequest{
		Target:        strings.TrimSpace(toString(normalized["target"])),
		Scope:         strings.TrimSpace(toString(normalized["scope"])),
		Dest:          strings.TrimSpace(toString(normalized["dest"])),
		InstalledOnly: boolValue(normalized["installed"]),
	})
	if err != nil {
		return mapDirectSkillSvcError(err)
	}

	data := map[string]interface{}{
		"skills": result.Skills,
	}
	if strings.TrimSpace(result.Target) != "" {
		data["target"] = result.Target
		data["scope"] = result.Scope
		data["root"] = result.Root
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectSkillInstall(args map[string]interface{}) (string, bool) {
	normalized := normalizeArgs(args)
	name := strings.TrimSpace(toString(normalized["name"]))
	if name == "" {
		return errorEnvelope("MISSING_ARGUMENT", "specify skill name", "Usage: rvn skill install <name>", nil), true
	}

	result, err := skillsvc.Install(skillsvc.InstallRequest{
		Name:    name,
		Target:  strings.TrimSpace(toString(normalized["target"])),
		Scope:   strings.TrimSpace(toString(normalized["scope"])),
		Dest:    strings.TrimSpace(toString(normalized["dest"])),
		Force:   boolValue(normalized["force"]),
		Confirm: boolValue(normalized["confirm"]),
	})
	if err != nil {
		return mapDirectSkillSvcError(err)
	}

	data := map[string]interface{}{
		"mode": result.Mode,
		"plan": result.Plan,
	}
	if result.ActionsApplied > 0 {
		data["actions_applied"] = result.ActionsApplied
	}
	if result.Receipt != nil {
		data["receipt"] = result.Receipt
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectSkillRemove(args map[string]interface{}) (string, bool) {
	normalized := normalizeArgs(args)
	name := strings.TrimSpace(toString(normalized["name"]))
	if name == "" {
		return errorEnvelope("MISSING_ARGUMENT", "specify skill name", "Usage: rvn skill remove <name>", nil), true
	}

	result, err := skillsvc.Remove(skillsvc.RemoveRequest{
		Name:    name,
		Target:  strings.TrimSpace(toString(normalized["target"])),
		Scope:   strings.TrimSpace(toString(normalized["scope"])),
		Dest:    strings.TrimSpace(toString(normalized["dest"])),
		Confirm: boolValue(normalized["confirm"]),
	})
	if err != nil {
		return mapDirectSkillSvcError(err)
	}

	data := map[string]interface{}{
		"mode": result.Mode,
		"plan": result.Plan,
	}
	if result.Removed {
		data["removed"] = true
	}
	return successEnvelope(data, nil), false
}

func (s *Server) callDirectSkillDoctor(args map[string]interface{}) (string, bool) {
	normalized := normalizeArgs(args)
	result, err := skillsvc.Doctor(skillsvc.DoctorRequest{
		Target: strings.TrimSpace(toString(normalized["target"])),
		Scope:  strings.TrimSpace(toString(normalized["scope"])),
		Dest:   strings.TrimSpace(toString(normalized["dest"])),
	})
	if err != nil {
		return mapDirectSkillSvcError(err)
	}

	return successEnvelope(map[string]interface{}{"reports": result.Reports}, nil), false
}

func mapDirectSkillSvcError(err error) (string, bool) {
	svcErr, ok := skillsvc.AsError(err)
	if !ok {
		return errorEnvelope("INTERNAL_ERROR", err.Error(), "", nil), true
	}

	code := string(svcErr.Code)
	if svcErr.Code == skillsvc.CodeInternal {
		code = "INTERNAL_ERROR"
	}
	return errorEnvelope(code, svcErr.Message, svcErr.Suggestion, svcErr.Details), true
}
