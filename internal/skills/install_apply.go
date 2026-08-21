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
		absPath, err := safeSkillPath(plan.SkillPath, relPath)
		if err != nil {
			return nil, applied, err
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return nil, applied, fmt.Errorf("create parent directory for %s: %w", absPath, err)
		}
		if err := atomicfile.WriteFile(absPath, plan.rendered[relPath], 0o644); err != nil {
			return nil, applied, fmt.Errorf("write %s: %w", absPath, err)
		}
		applied++
	}

	receipt := newReceipt(plan.spec.ID, plan.spec.Version, plan.Scope, plan.rendered)
	receiptPath, err := safeSkillPath(plan.SkillPath, receiptFileName)
	if err != nil {
		return nil, applied, err
	}
	if err := writeReceipt(receiptPath, receipt); err != nil {
		return nil, applied, err
	}
	applied++

	return receipt, applied, nil
}

func writeSkill(skill *Skill, scope, skillPath string, rendered map[string][]byte) (int, error) {
	written, err := writeSkillFiles(skillPath, rendered)
	if err != nil {
		return written, err
	}
	receiptWritten, err := writeSkillReceipt(skill, scope, skillPath, rendered)
	return written + receiptWritten, err
}

func writeSkillFiles(skillPath string, rendered map[string][]byte) (int, error) {
	if err := os.MkdirAll(skillPath, 0o755); err != nil {
		return 0, fmt.Errorf("create skill directory: %w", err)
	}

	applied := 0
	relPaths := sortedRenderedPaths(rendered)
	for _, relPath := range relPaths {
		absPath, err := safeSkillPath(skillPath, relPath)
		if err != nil {
			return applied, err
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return applied, fmt.Errorf("create parent directory for %s: %w", absPath, err)
		}
		if err := atomicfile.WriteFile(absPath, rendered[relPath], 0o644); err != nil {
			return applied, fmt.Errorf("write %s: %w", absPath, err)
		}
		applied++
	}
	return applied, nil
}

func writeSkillReceipt(skill *Skill, scope, skillPath string, rendered map[string][]byte) (int, error) {
	receipt := newReceipt(skill.Spec.ID, skill.Spec.Version, scope, rendered)
	receiptPath, err := safeSkillPath(skillPath, receiptFileName)
	if err != nil {
		return 0, err
	}
	if err := writeReceipt(receiptPath, receipt); err != nil {
		return 0, err
	}
	return 1, nil
}
