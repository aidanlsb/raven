package skillsvc

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/skills"
)

type Code = codes.ErrorCode

const (
	CodeInvalidInput        Code = codes.ErrInvalidInput
	CodeSkillNotFound       Code = codes.ErrSkillNotFound
	CodeSkillNotInstalled   Code = codes.ErrSkillNotInstalled
	CodeSkillPathUnresolved Code = codes.ErrSkillPathUnresolved
	CodeFileWriteError      Code = codes.ErrFileWrite
	CodeInternal            Code = codes.ErrInternal
)

type Error struct {
	Code       Code
	Message    string
	Suggestion string
	Details    map[string]interface{}
	Err        error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newError(code Code, message, suggestion string, details map[string]interface{}, err error) *Error {
	return &Error{Code: code, Message: message, Suggestion: suggestion, Details: details, Err: err}
}

func AsError(err error) (*Error, bool) {
	var svcErr *Error
	if errors.As(err, &svcErr) {
		return svcErr, true
	}
	return nil, false
}

type ListRequest struct {
	Scope         string
	Dest          string
	InstalledOnly bool
}

type ListResult struct {
	Scope  string           `json:"scope"`
	Root   string           `json:"root"`
	Skills []skills.Summary `json:"skills"`
}

type SyncRequest struct {
	Name    string
	Scope   string
	Dest    string
	Confirm bool
}

type SyncResult struct {
	Mode           string           `json:"mode"`
	SkillName      string           `json:"skill_name,omitempty"`
	Plan           *skills.SyncPlan `json:"plan,omitempty"`
	ActionsApplied int              `json:"actions_applied,omitempty"`
}

type RemoveRequest struct {
	Name    string
	Scope   string
	Dest    string
	Confirm bool
}

type RemoveResult struct {
	Mode      string             `json:"mode"`
	Removed   bool               `json:"removed,omitempty"`
	SkillName string             `json:"skill_name"`
	Plan      *skills.RemovePlan `json:"plan,omitempty"`
}

type DoctorRequest struct {
	Scope string
	Dest  string
}

type DoctorResult struct {
	Reports []skills.DoctorReport `json:"reports"`
}

func List(req ListRequest) (*ListResult, error) {
	catalog, err := skills.LoadCatalog()
	if err != nil {
		return nil, newError(CodeInternal, "failed to load skill catalog", "", nil, err)
	}

	scope, err := skills.ParseScope(strings.TrimSpace(req.Scope))
	if err != nil {
		return nil, newError(CodeInvalidInput, err.Error(), "Use --scope user|project", nil, err)
	}
	root, err := skills.ResolveInstallRoot(scope, strings.TrimSpace(req.Dest), "")
	if err != nil {
		return nil, newError(CodeSkillPathUnresolved, err.Error(), "Use --dest to set an explicit install root", nil, err)
	}

	items := skills.InstalledSummaries(catalog, root)
	if req.InstalledOnly {
		filtered := make([]skills.Summary, 0, len(items))
		for _, item := range items {
			if item.Installed {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	return &ListResult{
		Scope:  string(scope),
		Root:   root,
		Skills: items,
	}, nil
}

func Sync(req SyncRequest) (*SyncResult, error) {
	skillName := strings.TrimSpace(req.Name)
	catalog, err := skills.LoadCatalog()
	if err != nil {
		return nil, newError(CodeInternal, "failed to load skill catalog", "", nil, err)
	}

	if skillName != "" {
		if _, ok := catalog[skillName]; !ok {
			available := skills.SortedSummaries(catalog)
			names := make([]string, 0, len(available))
			for _, item := range available {
				names = append(names, item.Name)
			}
			return nil, newError(
				CodeSkillNotFound,
				fmt.Sprintf("skill '%s' not found", skillName),
				"Run 'rvn skill list' to see available skills",
				map[string]interface{}{"available": names},
				nil,
			)
		}
	}

	scope, err := skills.ParseScope(strings.TrimSpace(req.Scope))
	if err != nil {
		return nil, newError(CodeInvalidInput, err.Error(), "Use --scope user|project", nil, err)
	}
	root, err := skills.ResolveInstallRoot(scope, strings.TrimSpace(req.Dest), "")
	if err != nil {
		return nil, newError(CodeSkillPathUnresolved, err.Error(), "Use --dest to set an explicit install root", nil, err)
	}

	plan, err := skills.PlanSync(catalog, skillName, scope, root)
	if err != nil {
		return nil, newError(CodeInternal, "failed to build sync plan", "", nil, err)
	}

	if !req.Confirm {
		return &SyncResult{
			Mode:      "preview",
			SkillName: skillName,
			Plan:      plan,
		}, nil
	}

	applied, err := skills.ApplySync(plan)
	if err != nil {
		return nil, newError(CodeFileWriteError, "failed to apply sync", "", nil, err)
	}
	return &SyncResult{
		Mode:           "applied",
		SkillName:      skillName,
		Plan:           plan,
		ActionsApplied: applied,
	}, nil
}

func Remove(req RemoveRequest) (*RemoveResult, error) {
	skillName := strings.TrimSpace(req.Name)
	catalog, err := skills.LoadCatalog()
	if err != nil {
		return nil, newError(CodeInternal, "failed to load skill catalog", "", nil, err)
	}
	if _, ok := catalog[skillName]; !ok {
		return nil, newError(CodeSkillNotFound, fmt.Sprintf("skill '%s' not found", skillName), "Run 'rvn skill list' to see available skills", nil, nil)
	}

	scope, err := skills.ParseScope(strings.TrimSpace(req.Scope))
	if err != nil {
		return nil, newError(CodeInvalidInput, err.Error(), "Use --scope user|project", nil, err)
	}
	root, err := skills.ResolveInstallRoot(scope, strings.TrimSpace(req.Dest), "")
	if err != nil {
		return nil, newError(CodeSkillPathUnresolved, err.Error(), "Use --dest to set an explicit install root", nil, err)
	}

	plan, err := skills.PlanRemove(skillName, scope, root)
	if err != nil {
		return nil, newError(CodeInvalidInput, err.Error(), "", nil, err)
	}
	if !plan.Exists {
		return nil, newError(
			CodeSkillNotInstalled,
			fmt.Sprintf("skill '%s' is not installed", skillName),
			"Run 'rvn skill list --installed' to see installed skills",
			nil,
			nil,
		)
	}

	if !req.Confirm {
		return &RemoveResult{Mode: "preview", SkillName: skillName, Plan: plan}, nil
	}

	if err := skills.ApplyRemove(plan); err != nil {
		return nil, newError(CodeFileWriteError, "failed to apply removal", "", nil, err)
	}
	return &RemoveResult{Mode: "applied", Removed: true, SkillName: skillName, Plan: plan}, nil
}

func Doctor(req DoctorRequest) (*DoctorResult, error) {
	catalog, err := skills.LoadCatalog()
	if err != nil {
		return nil, newError(CodeInternal, "failed to load skill catalog", "", nil, err)
	}

	scope, err := skills.ParseScope(strings.TrimSpace(req.Scope))
	if err != nil {
		return nil, newError(CodeInvalidInput, err.Error(), "Use --scope user|project", nil, err)
	}

	root, err := skills.ResolveInstallRoot(scope, strings.TrimSpace(req.Dest), "")
	if err != nil {
		return nil, newError(CodeSkillPathUnresolved, err.Error(), "Use --dest to set an explicit install root", nil, err)
	}

	return &DoctorResult{Reports: []skills.DoctorReport{skills.Doctor(catalog, scope, root)}}, nil
}
