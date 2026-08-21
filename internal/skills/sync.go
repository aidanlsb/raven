package skills

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SyncPlan struct {
	Skill        string   `json:"skill,omitempty"`
	Scope        string   `json:"scope"`
	Root         string   `json:"root"`
	NeedsConfirm bool     `json:"needs_confirm"`
	Actions      []Action `json:"actions"`
	Warnings     []string `json:"warnings,omitempty"`
	Installed    int      `json:"installed,omitempty"`
	Updated      int      `json:"updated,omitempty"`
	Deleted      int      `json:"deleted,omitempty"`
	Skipped      int      `json:"skipped,omitempty"`

	items []syncItem
}

type syncOp string

const (
	syncOpInstall syncOp = "install"
	syncOpUpdate  syncOp = "update"
	syncOpDelete  syncOp = "delete"
	syncOpSkip    syncOp = "skip"
)

type syncItem struct {
	op       syncOp
	skill    *Skill
	skillID  string
	path     string
	rendered map[string][]byte
	receipt  *Receipt
}

func PlanSync(catalog map[string]*Skill, skillName string, scope Scope, root string) (*SyncPlan, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("install root is empty")
	}

	skillName = strings.TrimSpace(skillName)
	plan := &SyncPlan{
		Skill: skillName,
		Scope: string(scope),
		Root:  root,
	}

	if skillName != "" {
		return planNamedSync(plan, catalog, skillName, root)
	}
	return planRootSync(plan, catalog, root)
}

func planNamedSync(plan *SyncPlan, catalog map[string]*Skill, skillName, root string) (*SyncPlan, error) {
	skill, ok := catalog[skillName]
	if !ok {
		return nil, fmt.Errorf("skill %q not found", skillName)
	}

	skillPath := filepath.Join(root, skillName)
	receipt, receiptErr := readReceipt(filepath.Join(skillPath, receiptFileName))
	if receiptErr != nil && !os.IsNotExist(receiptErr) {
		return nil, fmt.Errorf("read receipt for %s: %w", skillName, receiptErr)
	}
	if receipt == nil {
		if stat, err := os.Stat(skillPath); err == nil {
			if stat.IsDir() {
				addSyncSkip(plan, skillName, skillPath, "skill directory exists without Raven receipt")
				return plan, nil
			}
			addSyncSkip(plan, skillName, skillPath, "skill path exists but is not a directory")
			return plan, nil
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("inspect %s: %w", skillPath, err)
		}
		rendered, err := RenderFiles(skill)
		if err != nil {
			return nil, err
		}
		addSyncInstall(plan, skill, skillPath, rendered)
		return plan, nil
	}
	if receipt.Skill != skillName {
		addSyncSkip(plan, skillName, skillPath, fmt.Sprintf("receipt skill %q does not match directory name", receipt.Skill))
		return plan, nil
	}

	rendered, err := RenderFiles(skill)
	if err != nil {
		return nil, err
	}
	needsUpdate, err := syncManagedNeedsUpdate(skillPath, receipt, skill, plan.Scope, rendered)
	if err != nil {
		return nil, err
	}
	if needsUpdate {
		addSyncUpdate(plan, skill, skillPath, rendered, receipt)
	}
	return plan, nil
}

func planRootSync(plan *SyncPlan, catalog map[string]*Skill, root string) (*SyncPlan, error) {
	managed := make(map[string]struct{})
	entries, err := os.ReadDir(root)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read install root: %w", err)
		}
		entries = nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		skillPath := filepath.Join(root, dirName)
		receipt, err := readReceipt(filepath.Join(skillPath, receiptFileName))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read receipt for %s: %w", dirName, err)
		}
		managed[dirName] = struct{}{}
		if receipt.Skill != dirName {
			addSyncSkip(plan, dirName, skillPath, fmt.Sprintf("receipt skill %q does not match directory name", receipt.Skill))
			continue
		}

		skill, ok := catalog[dirName]
		if !ok {
			if err := addSyncDelete(plan, dirName, skillPath, receipt); err != nil {
				return nil, err
			}
			continue
		}
		rendered, err := RenderFiles(skill)
		if err != nil {
			return nil, err
		}
		needsUpdate, err := syncManagedNeedsUpdate(skillPath, receipt, skill, plan.Scope, rendered)
		if err != nil {
			return nil, err
		}
		if needsUpdate {
			addSyncUpdate(plan, skill, skillPath, rendered, receipt)
		}
	}

	summaries := SortedSummaries(catalog)
	for _, summary := range summaries {
		if _, ok := managed[summary.Name]; ok {
			continue
		}
		skillPath := filepath.Join(root, summary.Name)
		if stat, err := os.Stat(skillPath); err == nil {
			if stat.IsDir() {
				addSyncSkip(plan, summary.Name, skillPath, "skill directory exists without Raven receipt")
				continue
			}
			addSyncSkip(plan, summary.Name, skillPath, "skill path exists but is not a directory")
			continue
		} else if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("inspect %s: %w", skillPath, err)
		}
		skill := catalog[summary.Name]
		rendered, err := RenderFiles(skill)
		if err != nil {
			return nil, err
		}
		addSyncInstall(plan, skill, skillPath, rendered)
	}
	finalizeSyncPlan(plan)
	return plan, nil
}

// installSkillResults projects one full-catalog sync plan into the existing
// per-skill install response shape. The returned plans are display-only; the
// original plan remains the single plan passed to ApplySync.
func installSkillResults(plan *SyncPlan, shippedNames []string) []InstallSkillResult {
	if plan == nil {
		return nil
	}

	results := make([]InstallSkillResult, 0, len(shippedNames))
	byName := make(map[string]*SyncPlan, len(shippedNames))
	addResult := func(name string) *SyncPlan {
		skillPlan := &SyncPlan{
			Skill: name,
			Scope: plan.Scope,
			Root:  plan.Root,
		}
		results = append(results, InstallSkillResult{Name: name, Plan: skillPlan})
		byName[name] = skillPlan
		return skillPlan
	}
	for _, name := range shippedNames {
		addResult(name)
	}

	for _, item := range plan.items {
		skillPlan, ok := byName[item.skillID]
		if !ok {
			skillPlan = addResult(item.skillID)
		}
		for _, action := range plan.Actions {
			if syncActionBelongsToItem(action, item) {
				skillPlan.Actions = append(skillPlan.Actions, action)
			}
		}
		switch item.op {
		case syncOpInstall:
			skillPlan.Installed++
			skillPlan.NeedsConfirm = true
		case syncOpUpdate:
			skillPlan.Updated++
			skillPlan.NeedsConfirm = true
		case syncOpDelete:
			skillPlan.Deleted++
			skillPlan.NeedsConfirm = true
		case syncOpSkip:
			skillPlan.Skipped++
			for _, action := range skillPlan.Actions {
				if action.Op == "skip_conflict" && strings.TrimSpace(action.Reason) != "" {
					skillPlan.Warnings = append(skillPlan.Warnings, fmt.Sprintf("%s: %s", item.skillID, action.Reason))
				}
			}
		}
	}
	return results
}

func syncActionBelongsToItem(action Action, item syncItem) bool {
	actionPath := filepath.Clean(action.Path)
	itemPath := filepath.Clean(item.path)
	return actionPath == itemPath || strings.HasPrefix(actionPath, itemPath+string(filepath.Separator))
}

func ApplySync(plan *SyncPlan) (int, error) {
	if plan == nil {
		return 0, fmt.Errorf("sync plan is nil")
	}

	applied := 0
	for _, item := range plan.items {
		switch item.op {
		case syncOpInstall:
			written, err := writeSkill(item.skill, plan.Scope, item.path, item.rendered)
			if err != nil {
				return applied, err
			}
			applied += written
		case syncOpUpdate:
			written, err := writeSkillFiles(item.path, item.rendered)
			if err != nil {
				return applied, err
			}
			applied += written
			removed, err := removeStaleReceiptFiles(item.path, item.receipt, item.rendered)
			if err != nil {
				return applied, err
			}
			applied += removed
			receiptWritten, err := writeSkillReceipt(item.skill, plan.Scope, item.path, item.rendered)
			if err != nil {
				return applied, err
			}
			applied += receiptWritten
		case syncOpDelete:
			removed, err := removeReceiptManagedFiles(item.path, item.receipt)
			if err != nil {
				return applied, err
			}
			applied += removed
		case syncOpSkip:
			continue
		}
	}
	return applied, nil
}

func addSyncInstall(plan *SyncPlan, skill *Skill, skillPath string, rendered map[string][]byte) {
	plan.Actions = append(plan.Actions, Action{Op: "install", Path: skillPath, Reason: "install missing shipped skill"})
	plan.Installed++
	plan.items = append(plan.items, syncItem{op: syncOpInstall, skill: skill, skillID: skill.Spec.ID, path: skillPath, rendered: rendered})
	finalizeSyncPlan(plan)
}

func addSyncUpdate(plan *SyncPlan, skill *Skill, skillPath string, rendered map[string][]byte, receipt *Receipt) {
	plan.Actions = append(plan.Actions, Action{Op: "update", Path: skillPath, Reason: "align with shipped skill"})
	plan.Updated++
	plan.items = append(plan.items, syncItem{op: syncOpUpdate, skill: skill, skillID: skill.Spec.ID, path: skillPath, rendered: rendered, receipt: receipt})
	finalizeSyncPlan(plan)
}

func addSyncDelete(plan *SyncPlan, skillID, skillPath string, receipt *Receipt) error {
	for _, relPath := range receipt.Files {
		absPath, err := safeSkillPath(skillPath, relPath)
		if err != nil {
			return err
		}
		plan.Actions = append(plan.Actions, Action{Op: "delete_file", Path: absPath, RelPath: relPath, Reason: "shipped skill is no longer available"})
	}
	receiptPath, err := safeSkillPath(skillPath, receiptFileName)
	if err != nil {
		return err
	}
	plan.Actions = append(plan.Actions, Action{Op: "delete_receipt", Path: receiptPath, RelPath: receiptFileName, Reason: "shipped skill is no longer available"})
	plan.Actions = append(plan.Actions, Action{Op: "remove_empty_dir", Path: skillPath, Reason: "remove empty managed directories if safe"})
	plan.Deleted++
	plan.items = append(plan.items, syncItem{op: syncOpDelete, skillID: skillID, path: skillPath, receipt: receipt})
	finalizeSyncPlan(plan)
	return nil
}

func addSyncSkip(plan *SyncPlan, skillID, skillPath, reason string) {
	plan.Actions = append(plan.Actions, Action{Op: "skip_conflict", Path: skillPath, Reason: reason})
	plan.Skipped++
	plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s: %s", skillID, reason))
	plan.items = append(plan.items, syncItem{op: syncOpSkip, skillID: skillID, path: skillPath})
	finalizeSyncPlan(plan)
}

func finalizeSyncPlan(plan *SyncPlan) {
	plan.NeedsConfirm = false
	for _, item := range plan.items {
		if item.op == syncOpInstall || item.op == syncOpUpdate || item.op == syncOpDelete {
			plan.NeedsConfirm = true
			return
		}
	}
}

func syncManagedNeedsUpdate(skillPath string, receipt *Receipt, skill *Skill, scope string, rendered map[string][]byte) (bool, error) {
	if !receiptMatchesRendered(receipt, skill, scope, rendered) {
		return true, nil
	}
	for relPath, content := range rendered {
		absPath, err := safeSkillPath(skillPath, relPath)
		if err != nil {
			return false, err
		}
		existing, err := os.ReadFile(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return true, nil
			}
			return false, fmt.Errorf("read %s: %w", absPath, err)
		}
		if !bytes.Equal(existing, content) {
			return true, nil
		}
	}

	current := make(map[string]struct{}, len(rendered))
	for relPath := range rendered {
		current[relPath] = struct{}{}
	}
	for _, relPath := range receipt.Files {
		if _, ok := current[relPath]; ok {
			continue
		}
		absPath, err := safeSkillPath(skillPath, relPath)
		if err != nil {
			return false, err
		}
		if _, err := os.Stat(absPath); err == nil {
			return true, nil
		} else if err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("inspect %s: %w", absPath, err)
		}
	}
	return false, nil
}

func removeStaleReceiptFiles(skillPath string, receipt *Receipt, rendered map[string][]byte) (int, error) {
	if receipt == nil {
		return 0, nil
	}
	current := make(map[string]struct{}, len(rendered))
	for relPath := range rendered {
		current[relPath] = struct{}{}
	}

	applied := 0
	for _, relPath := range receipt.Files {
		if _, ok := current[relPath]; ok {
			continue
		}
		removed, err := removeManagedFile(skillPath, relPath)
		if err != nil {
			return applied, err
		}
		applied += removed
	}
	if err := removeEmptyDirs(skillPath); err != nil {
		return applied, err
	}
	return applied, nil
}

func removeReceiptManagedFiles(skillPath string, receipt *Receipt) (int, error) {
	if receipt == nil {
		return 0, nil
	}

	applied := 0
	for _, relPath := range receipt.Files {
		removed, err := removeManagedFile(skillPath, relPath)
		if err != nil {
			return applied, err
		}
		applied += removed
	}
	removed, err := removeManagedFile(skillPath, receiptFileName)
	if err != nil {
		return applied, err
	}
	applied += removed
	if err := removeEmptyDirs(skillPath); err != nil {
		return applied, err
	}
	return applied, nil
}

func removeManagedFile(skillPath, relPath string) (int, error) {
	absPath, err := safeSkillPath(skillPath, relPath)
	if err != nil {
		return 0, err
	}
	if err := os.Remove(absPath); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("remove %s: %w", absPath, err)
	}
	return 1, nil
}

func safeSkillPath(skillPath, relPath string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(strings.TrimSpace(relPath)))
	if cleaned == "" || cleaned == "." || filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("skill path escapes skill root: %s", relPath)
	}

	rootInfo, err := os.Lstat(skillPath)
	if err != nil {
		return "", fmt.Errorf("inspect skill root %s: %w", skillPath, err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("skill root is a symlink: %s", skillPath)
	}

	current := skillPath
	for _, component := range strings.Split(cleaned, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				break
			}
			return "", fmt.Errorf("inspect skill path %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("skill path traverses symlink: %s", current)
		}
	}
	return filepath.Join(skillPath, cleaned), nil
}

func removeEmptyDirs(skillPath string) error {
	var dirs []string
	if err := filepath.WalkDir(skillPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("walk %s: %w", skillPath, err)
	}
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})
	for _, dir := range dirs {
		if err := os.Remove(dir); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			entries, readErr := os.ReadDir(dir)
			if readErr == nil && len(entries) > 0 {
				continue
			}
			return fmt.Errorf("remove empty directory %s: %w", dir, err)
		}
	}
	return nil
}
