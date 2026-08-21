package skills

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/svcerr"
)

type ListRequest struct {
	Scope         string
	Dest          string
	InstalledOnly bool
}

type ListResult struct {
	Scope  string    `json:"scope"`
	Root   string    `json:"root"`
	Skills []Summary `json:"skills"`
}

type InstallRequest struct {
	Names   []string
	Scope   string
	Dest    string
	Confirm bool
}

// InstallSkillResult reports the plan for one shipped or receipt-managed skill.
type InstallSkillResult struct {
	Name string    `json:"name"`
	Plan *SyncPlan `json:"plan"`
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
	Deleted        int                  `json:"deleted"`
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
	Mode      string      `json:"mode"`
	Removed   bool        `json:"removed,omitempty"`
	SkillName string      `json:"skill_name"`
	Plan      *RemovePlan `json:"plan,omitempty"`
}

type DoctorRequest struct {
	Scope string
	Dest  string
}

type DoctorResult struct {
	Reports []DoctorReport `json:"reports"`
}

func List(req ListRequest) (*ListResult, error) {
	catalog, err := LoadCatalog()
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInternal, "failed to load skill catalog", err)
	}

	scope, err := ParseScope(strings.TrimSpace(req.Scope))
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err).WithSuggestion("Use --scope user|project")
	}
	root, err := ResolveInstallRoot(scope, strings.TrimSpace(req.Dest), "")
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrSkillPathUnresolved, err.Error(), err).WithSuggestion("Use --dest to set an explicit install root")
	}

	items := InstalledSummaries(catalog, root)
	if req.InstalledOnly {
		filtered := make([]Summary, 0, len(items))
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

// Install reconciles shipped Raven skills in one shot. With no names it aligns
// the complete Raven-managed set with the shipped catalog, including installing
// missing skills and removing receipt-managed skills that are no longer
// shipped. Names narrow changes to those specific shipped skills. It previews
// by default and applies writes only when req.Confirm is set.
func Install(req InstallRequest) (*InstallResult, error) {
	catalog, err := LoadCatalog()
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInternal, "failed to load skill catalog", err)
	}

	fullCatalog := installRequestIsFullCatalog(req.Names)
	names, err := resolveInstallNames(catalog, req.Names)
	if err != nil {
		return nil, err
	}

	scope, err := ParseScope(strings.TrimSpace(req.Scope))
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err).WithSuggestion("Use --scope user|project")
	}
	root, err := ResolveInstallRoot(scope, strings.TrimSpace(req.Dest), "")
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrSkillPathUnresolved, err.Error(), err).WithSuggestion("Use --dest to set an explicit install root")
	}

	result := &InstallResult{
		Mode:      "preview",
		Scope:     string(scope),
		Root:      root,
		Requested: names,
	}

	var applyPlans []*SyncPlan
	if fullCatalog {
		plan, err := PlanSync(catalog, "", scope, root)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrInternal, "failed to build install plan", err)
		}
		result.Skills = installSkillResults(plan, names)
		result.Installed = plan.Installed
		result.Updated = plan.Updated
		result.Deleted = plan.Deleted
		result.Skipped = plan.Skipped
		result.NeedsConfirm = plan.NeedsConfirm
		applyPlans = append(applyPlans, plan)
	} else {
		for _, name := range names {
			plan, err := PlanSync(catalog, name, scope, root)
			if err != nil {
				return nil, svcerr.Wrap(codes.ErrInternal, "failed to build install plan", err)
			}
			result.Skills = append(result.Skills, InstallSkillResult{Name: name, Plan: plan})
			result.Installed += plan.Installed
			result.Updated += plan.Updated
			result.Skipped += plan.Skipped
			if plan.NeedsConfirm {
				result.NeedsConfirm = true
			}
			applyPlans = append(applyPlans, plan)
		}
	}

	if !req.Confirm {
		return result, nil
	}

	applied := 0
	for _, plan := range applyPlans {
		n, err := ApplySync(plan)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to apply install", err)
		}
		applied += n
	}
	result.Mode = "applied"
	result.NeedsConfirm = false
	result.ActionsApplied = applied
	return result, nil
}

func installRequestIsFullCatalog(requested []string) bool {
	for _, raw := range requested {
		if strings.TrimSpace(raw) != "" {
			return false
		}
	}
	return true
}

// resolveInstallNames returns the ordered, de-duplicated set of skills to
// install. An empty request installs the full shipped catalog; otherwise each
// requested name must exist in the catalog.
func resolveInstallNames(catalog map[string]*Skill, requested []string) ([]string, error) {
	trimmed := make([]string, 0, len(requested))
	for _, raw := range requested {
		if name := strings.TrimSpace(raw); name != "" {
			trimmed = append(trimmed, name)
		}
	}

	if len(trimmed) == 0 {
		summaries := SortedSummaries(catalog)
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
			available := SortedSummaries(catalog)
			availableNames := make([]string, 0, len(available))
			for _, item := range available {
				availableNames = append(availableNames, item.Name)
			}
			return nil, svcerr.New(codes.ErrSkillNotFound, fmt.Sprintf("skill '%s' not found", name)).WithSuggestion("Run 'rvn skill list' to see available skills").WithDetails(map[string]interface{}{"available": availableNames})
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
	catalog, err := LoadCatalog()
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInternal, "failed to load skill catalog", err)
	}
	if _, ok := catalog[skillName]; !ok {
		return nil, svcerr.New(codes.ErrSkillNotFound, fmt.Sprintf("skill '%s' not found", skillName)).WithSuggestion("Run 'rvn skill list' to see available skills")
	}

	scope, err := ParseScope(strings.TrimSpace(req.Scope))
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err).WithSuggestion("Use --scope user|project")
	}
	root, err := ResolveInstallRoot(scope, strings.TrimSpace(req.Dest), "")
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrSkillPathUnresolved, err.Error(), err).WithSuggestion("Use --dest to set an explicit install root")
	}

	plan, err := PlanRemove(skillName, scope, root)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err)
	}
	if !plan.Exists {
		return nil, svcerr.New(codes.ErrSkillNotInstalled, fmt.Sprintf("skill '%s' is not installed", skillName)).WithSuggestion("Run 'rvn skill list --installed' to see installed skills")
	}

	if !req.Confirm {
		return &RemoveResult{Mode: "preview", SkillName: skillName, Plan: plan}, nil
	}

	if err := ApplyRemove(plan); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to apply removal", err)
	}
	return &RemoveResult{Mode: "applied", Removed: true, SkillName: skillName, Plan: plan}, nil
}

func RunDoctor(req DoctorRequest) (*DoctorResult, error) {
	catalog, err := LoadCatalog()
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInternal, "failed to load skill catalog", err)
	}

	scope, err := ParseScope(strings.TrimSpace(req.Scope))
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err).WithSuggestion("Use --scope user|project")
	}

	root, err := ResolveInstallRoot(scope, strings.TrimSpace(req.Dest), "")
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrSkillPathUnresolved, err.Error(), err).WithSuggestion("Use --dest to set an explicit install root")
	}

	return &DoctorResult{Reports: []DoctorReport{Doctor(catalog, scope, root)}}, nil
}
