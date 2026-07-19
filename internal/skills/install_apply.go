package skills

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aidanlsb/raven/internal/atomicfile"
)

func ApplyInstall(plan *InstallPlan) (*Receipt, int, error) {
	if plan == nil {
		return nil, 0, fmt.Errorf("install plan is nil")
	}
	if len(plan.Conflicts) > 0 {
		return nil, 0, fmt.Errorf("plan has conflicts")
	}

	if err := os.MkdirAll(plan.SkillPath, 0o755); err != nil {
		return nil, 0, fmt.Errorf("create skill directory: %w", err)
	}

	applied := 0
	relPaths := sortedRenderedPaths(plan.rendered)
	for _, relPath := range relPaths {
		absPath := filepath.Join(plan.SkillPath, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return nil, applied, fmt.Errorf("create parent directory for %s: %w", absPath, err)
		}
		if err := atomicfile.WriteFile(absPath, plan.rendered[relPath], 0o644); err != nil {
			return nil, applied, fmt.Errorf("write %s: %w", absPath, err)
		}
		applied++
	}

	receipt := newReceipt(plan.spec.ID, plan.spec.Version, plan.Scope, plan.rendered)
	receiptPath := filepath.Join(plan.SkillPath, receiptFileName)
	if err := writeReceipt(receiptPath, receipt); err != nil {
		return nil, applied, err
	}
	applied++

	return receipt, applied, nil
}

func writeSkill(skill *Skill, scope, skillPath string, rendered map[string][]byte) (int, error) {
	if err := os.MkdirAll(skillPath, 0o755); err != nil {
		return 0, fmt.Errorf("create skill directory: %w", err)
	}

	applied := 0
	relPaths := sortedRenderedPaths(rendered)
	for _, relPath := range relPaths {
		absPath := filepath.Join(skillPath, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return applied, fmt.Errorf("create parent directory for %s: %w", absPath, err)
		}
		if err := atomicfile.WriteFile(absPath, rendered[relPath], 0o644); err != nil {
			return applied, fmt.Errorf("write %s: %w", absPath, err)
		}
		applied++
	}

	receipt := newReceipt(skill.Spec.ID, skill.Spec.Version, scope, rendered)
	receiptPath := filepath.Join(skillPath, receiptFileName)
	if err := writeReceipt(receiptPath, receipt); err != nil {
		return applied, err
	}
	applied++

	return applied, nil
}
