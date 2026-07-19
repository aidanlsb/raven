package skills

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Action struct {
	Op      string `json:"op"`
	Path    string `json:"path"`
	RelPath string `json:"rel_path,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type InstallPlan struct {
	Skill        string   `json:"skill"`
	Scope        string   `json:"scope"`
	Root         string   `json:"root"`
	SkillPath    string   `json:"skill_path"`
	NeedsConfirm bool     `json:"needs_confirm"`
	Actions      []Action `json:"actions"`
	Conflicts    []string `json:"conflicts,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`

	rendered map[string][]byte
	spec     Spec
}

func PlanInstall(skill *Skill, scope Scope, root string, force bool) (*InstallPlan, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("install root is empty")
	}

	rendered, err := RenderFiles(skill)
	if err != nil {
		return nil, err
	}

	skillPath := filepath.Join(root, skill.Spec.ID)
	plan := &InstallPlan{
		Skill:     skill.Spec.ID,
		Scope:     string(scope),
		Root:      root,
		SkillPath: skillPath,
		rendered:  rendered,
		spec:      skill.Spec,
	}

	relPaths := sortedRenderedPaths(rendered)
	for _, relPath := range relPaths {
		content := rendered[relPath]
		absPath := filepath.Join(skillPath, filepath.FromSlash(relPath))

		existing, readErr := os.ReadFile(absPath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				plan.Actions = append(plan.Actions, Action{Op: "create", Path: absPath, RelPath: relPath})
				continue
			}
			return nil, fmt.Errorf("read %s: %w", absPath, readErr)
		}

		if bytes.Equal(existing, content) {
			continue
		}

		if force {
			plan.Actions = append(plan.Actions, Action{Op: "update", Path: absPath, RelPath: relPath})
			continue
		}

		plan.Actions = append(plan.Actions, Action{Op: "conflict", Path: absPath, RelPath: relPath, Reason: "file exists with different content"})
		plan.Conflicts = append(plan.Conflicts, absPath)
	}

	receiptPath := filepath.Join(skillPath, receiptFileName)
	receiptChecksum := checksumForRendered(rendered)
	receipt, _ := readReceipt(receiptPath)
	if receipt == nil || receipt.Checksum != receiptChecksum || receipt.Skill != skill.Spec.ID || receipt.Version != skill.Spec.Version || receipt.Scope != string(scope) {
		if _, err := os.Stat(receiptPath); err == nil {
			plan.Actions = append(plan.Actions, Action{Op: "update", Path: receiptPath, RelPath: receiptFileName})
		} else {
			plan.Actions = append(plan.Actions, Action{Op: "create", Path: receiptPath, RelPath: receiptFileName})
		}
	}

	plan.NeedsConfirm = len(plan.Actions) > 0 && len(plan.Conflicts) == 0
	if len(plan.Actions) == 0 {
		plan.Warnings = append(plan.Warnings, "skill is already up to date")
	}

	return plan, nil
}
