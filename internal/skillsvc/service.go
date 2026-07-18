package skillsvc

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/skills"
	"github.com/aidanlsb/raven/internal/svcerr"
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

func newError(code Code, message, suggestion string, details map[string]interface{}, err error) *svcerr.Error {
	return &svcerr.Error{Code: code, Message: message, Suggestion: suggestion, Details: details, Err: err}
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

type InstallRequest struct {
	Names   []string
	Scope   string
	Dest    string
	Confirm bool
}

// InstallSkillResult reports the sync plan for a single shipped skill.
type InstallSkillResult struct {
	Name string           `json:"name"`
	Plan *skills.SyncPlan `json:"plan"`
}

type InstallResult struct {
	Mode           string               `json:"mode"`
	Scope          string               `json:"scope"`
	Root           string               `json:"root"`
	NeedsConfirm   bool                 `json:"needs_confirm"`
	Requested      []string             `json:"requested"`
	Skills         []InstallSkillResult `json:"skills"`
	Installed      int                  `json:"installed"`
	Updated        int                  `json:"updated"`
	Skipped        int                  `json:"skipped"`
	ActionsApplied int                  `json:"actions_applied,omitempty"`
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

// Install installs shipped Raven skills in one shot. With no names it installs
// the full shipped catalog; names narrow it to specific shipped skills. It
// previews by default and applies writes only when req.Confirm is set. Missing
// skills are installed and existing Raven-managed skills are aligned with the
// shipped version, reusing the same receipt-based sync machinery as Sync.
func Install(req InstallRequest) (*InstallResult, error) {
	catalog, err := skills.LoadCatalog()
	if err != nil {
		return nil, newError(CodeInternal, "failed to load skill catalog", "", nil, err)
	}

	names, err := resolveInstallNames(catalog, req.Names)
	if err != nil {
		return nil, err
	}

	scope, err := skills.ParseScope(strings.TrimSpace(req.Scope))
	if err != nil {
		return nil, newError(CodeInvalidInput, err.Error(), "Use --scope user|project", nil, err)
	}
	root, err := skills.ResolveInstallRoot(scope, strings.TrimSpace(req.Dest), "")
	if err != nil {
		return nil, newError(CodeSkillPathUnresolved, err.Error(), "Use --dest to set an explicit install root", nil, err)
	}

	result := &InstallResult{
		Mode:      "preview",
		Scope:     string(scope),
		Root:      root,
		Requested: names,
	}

	for _, name := range names {
		plan, err := skills.PlanSync(catalog, name, scope, root)
		if err != nil {
			return nil, newError(CodeInternal, "failed to build install plan", "", nil, err)
		}
		result.Skills = append(result.Skills, InstallSkillResult{Name: name, Plan: plan})
		result.Installed += plan.Installed
		result.Updated += plan.Updated
		result.Skipped += plan.Skipped
		if plan.NeedsConfirm {
			result.NeedsConfirm = true
		}
	}

	if !req.Confirm {
		return result, nil
	}

	applied := 0
	for i := range result.Skills {
		n, err := skills.ApplySync(result.Skills[i].Plan)
		if err != nil {
			return nil, newError(CodeFileWriteError, "failed to apply install", "", nil, err)
		}
		applied += n
	}
	result.Mode = "applied"
	result.NeedsConfirm = false
	result.ActionsApplied = applied
	return result, nil
}

// resolveInstallNames returns the ordered, de-duplicated set of skills to
// install. An empty request installs the full shipped catalog; otherwise each
// requested name must exist in the catalog.
func resolveInstallNames(catalog map[string]*skills.Skill, requested []string) ([]string, error) {
	trimmed := make([]string, 0, len(requested))
	for _, raw := range requested {
		if name := strings.TrimSpace(raw); name != "" {
			trimmed = append(trimmed, name)
		}
	}

	if len(trimmed) == 0 {
		summaries := skills.SortedSummaries(catalog)
		names := make([]string, 0, len(summaries))
		for _, summary := range summaries {
			names = append(names, summary.Name)
		}
		return names, nil
	}

	seen := make(map[string]struct{}, len(trimmed))
	names := make([]string, 0, len(trimmed))
	for _, name := range trimmed {
		if _, ok := catalog[name]; !ok {
			available := skills.SortedSummaries(catalog)
			availableNames := make([]string, 0, len(available))
			for _, item := range available {
				availableNames = append(availableNames, item.Name)
			}
			return nil, newError(
				CodeSkillNotFound,
				fmt.Sprintf("skill '%s' not found", name),
				"Run 'rvn skill list' to see available skills",
				map[string]interface{}{"available": availableNames},
				nil,
			)
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names, nil
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
