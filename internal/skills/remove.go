package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RemovePlan struct {
	Skill        string   `json:"skill"`
	Scope        string   `json:"scope"`
	Root         string   `json:"root"`
	SkillPath    string   `json:"skill_path"`
	Exists       bool     `json:"exists"`
	NeedsConfirm bool     `json:"needs_confirm"`
	Actions      []Action `json:"actions"`
}

func PlanRemove(skillID string, scope Scope, root string) (*RemovePlan, error) {
	if strings.TrimSpace(skillID) == "" {
		return nil, fmt.Errorf("skill id is empty")
	}
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("install root is empty")
	}

	skillPath := filepath.Join(root, skillID)
	plan := &RemovePlan{
		Skill:     skillID,
		Scope:     string(scope),
		Root:      root,
		SkillPath: skillPath,
	}

	if stat, err := os.Stat(skillPath); err == nil {
		plan.Exists = stat.IsDir()
		if !stat.IsDir() {
			return nil, fmt.Errorf("skill path exists but is not a directory: %s", skillPath)
		}
		plan.Actions = append(plan.Actions, Action{Op: "delete", Path: skillPath, Reason: "remove installed skill directory"})
		plan.NeedsConfirm = true
	} else if os.IsNotExist(err) {
		plan.Exists = false
	} else {
		return nil, fmt.Errorf("inspect %s: %w", skillPath, err)
	}

	return plan, nil
}

func ApplyRemove(plan *RemovePlan) error {
	if plan == nil {
		return fmt.Errorf("remove plan is nil")
	}
	if !plan.Exists {
		return fmt.Errorf("skill is not installed")
	}

	absRoot, err := filepath.Abs(plan.Root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	absSkill, err := filepath.Abs(plan.SkillPath)
	if err != nil {
		return fmt.Errorf("resolve skill path: %w", err)
	}

	prefix := absRoot + string(filepath.Separator)
	if absSkill == absRoot || !strings.HasPrefix(absSkill, prefix) {
		return fmt.Errorf("refusing to remove path outside install root")
	}

	if err := os.RemoveAll(absSkill); err != nil {
		return fmt.Errorf("remove skill directory: %w", err)
	}
	return nil
}
