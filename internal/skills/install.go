package skills

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/atomicfile"
)

const receiptFileName = ".rvn-skill-receipt.json"

type Action struct {
	Op      string `json:"op"`
	Path    string `json:"path"`
	RelPath string `json:"rel_path,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type InstallPlan struct {
	Skill        string   `json:"skill"`
	Target       string   `json:"target"`
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

type RemovePlan struct {
	Skill        string   `json:"skill"`
	Target       string   `json:"target"`
	Scope        string   `json:"scope"`
	Root         string   `json:"root"`
	SkillPath    string   `json:"skill_path"`
	Exists       bool     `json:"exists"`
	NeedsConfirm bool     `json:"needs_confirm"`
	Actions      []Action `json:"actions"`
}

type SyncPlan struct {
	Skill            string    `json:"skill,omitempty"`
	Target           string    `json:"target"`
	Scope            string    `json:"scope"`
	Root             string    `json:"root"`
	NeedsConfirm     bool      `json:"needs_confirm"`
	Actions          []Action  `json:"actions"`
	MissingAvailable []Summary `json:"missing_available,omitempty"`
	Warnings         []string  `json:"warnings,omitempty"`
	Installed        int       `json:"installed,omitempty"`
	Updated          int       `json:"updated,omitempty"`
	Deleted          int       `json:"deleted,omitempty"`
	Skipped          int       `json:"skipped,omitempty"`

	items []syncItem
}

type Receipt struct {
	Skill       string   `json:"skill"`
	Version     int      `json:"version"`
	Target      string   `json:"target"`
	Scope       string   `json:"scope"`
	Checksum    string   `json:"checksum"`
	Files       []string `json:"files"`
	InstalledAt string   `json:"installed_at"`
}

type DoctorReport struct {
	Target    string    `json:"target"`
	Scope     string    `json:"scope"`
	Root      string    `json:"root"`
	Exists    bool      `json:"exists"`
	Installed []Summary `json:"installed"`
	Issues    []string  `json:"issues,omitempty"`
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

func RenderFiles(skill *Skill, target Target) (map[string][]byte, error) {
	if skill == nil {
		return nil, fmt.Errorf("skill is nil")
	}

	files := map[string][]byte{
		"SKILL.md": []byte(buildSkillMarkdown(skill)),
	}

	refPaths := make([]string, 0, len(skill.References))
	for p := range skill.References {
		refPaths = append(refPaths, p)
	}
	sort.Strings(refPaths)
	for _, p := range refPaths {
		files[p] = []byte(skill.References[p])
	}

	if target == TargetCodex && strings.TrimSpace(skill.OpenAIMetadata) != "" {
		files["agents/openai.yaml"] = []byte(skill.OpenAIMetadata)
	}

	return files, nil
}

func PlanInstall(skill *Skill, target Target, scope Scope, root string, force bool) (*InstallPlan, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("install root is empty")
	}

	rendered, err := RenderFiles(skill, target)
	if err != nil {
		return nil, err
	}

	skillPath := filepath.Join(root, skill.Spec.ID)
	plan := &InstallPlan{
		Skill:     skill.Spec.ID,
		Target:    string(target),
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
	if receipt == nil || receipt.Checksum != receiptChecksum || receipt.Skill != skill.Spec.ID || receipt.Version != skill.Spec.Version || receipt.Target != string(target) || receipt.Scope != string(scope) {
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

func PlanSync(catalog map[string]*Skill, skillName string, target Target, scope Scope, root string) (*SyncPlan, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("install root is empty")
	}

	skillName = strings.TrimSpace(skillName)
	plan := &SyncPlan{
		Skill:  skillName,
		Target: string(target),
		Scope:  string(scope),
		Root:   root,
	}

	if skillName != "" {
		return planNamedSync(plan, catalog, skillName, target, root)
	}
	return planRootSync(plan, catalog, target, root)
}

func planNamedSync(plan *SyncPlan, catalog map[string]*Skill, skillName string, target Target, root string) (*SyncPlan, error) {
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
		rendered, err := RenderFiles(skill, target)
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

	rendered, err := RenderFiles(skill, target)
	if err != nil {
		return nil, err
	}
	needsUpdate, err := syncManagedNeedsUpdate(skillPath, receipt, skill, plan.Target, plan.Scope, rendered)
	if err != nil {
		return nil, err
	}
	if needsUpdate {
		addSyncUpdate(plan, skill, skillPath, rendered, receipt)
	}
	return plan, nil
}

func planRootSync(plan *SyncPlan, catalog map[string]*Skill, target Target, root string) (*SyncPlan, error) {
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
		if receipt.Skill != dirName {
			addSyncSkip(plan, dirName, skillPath, fmt.Sprintf("receipt skill %q does not match directory name", receipt.Skill))
			continue
		}
		managed[dirName] = struct{}{}

		skill, ok := catalog[dirName]
		if !ok {
			if err := addSyncDelete(plan, dirName, skillPath, receipt); err != nil {
				return nil, err
			}
			continue
		}
		rendered, err := RenderFiles(skill, target)
		if err != nil {
			return nil, err
		}
		needsUpdate, err := syncManagedNeedsUpdate(skillPath, receipt, skill, plan.Target, plan.Scope, rendered)
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
		plan.MissingAvailable = append(plan.MissingAvailable, summary)
	}
	finalizeSyncPlan(plan)
	return plan, nil
}

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

	receipt := &Receipt{
		Skill:       plan.spec.ID,
		Version:     plan.spec.Version,
		Target:      plan.Target,
		Scope:       plan.Scope,
		Checksum:    checksumForRendered(plan.rendered),
		Files:       relPaths,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}

	receiptBytes, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return nil, applied, fmt.Errorf("marshal receipt: %w", err)
	}
	receiptPath := filepath.Join(plan.SkillPath, receiptFileName)
	if err := atomicfile.WriteFile(receiptPath, receiptBytes, 0o644); err != nil {
		return nil, applied, fmt.Errorf("write receipt: %w", err)
	}
	applied++

	return receipt, applied, nil
}

func ApplySync(plan *SyncPlan) (int, error) {
	if plan == nil {
		return 0, fmt.Errorf("sync plan is nil")
	}

	applied := 0
	for _, item := range plan.items {
		switch item.op {
		case syncOpInstall, syncOpUpdate:
			written, err := writeSkill(item.skill, plan.Target, plan.Scope, item.path, item.rendered)
			if err != nil {
				return applied, err
			}
			applied += written
			if item.receipt != nil {
				removed, err := removeStaleReceiptFiles(item.path, item.receipt, item.rendered)
				if err != nil {
					return applied, err
				}
				applied += removed
			}
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

func PlanRemove(skillID string, target Target, scope Scope, root string) (*RemovePlan, error) {
	if strings.TrimSpace(skillID) == "" {
		return nil, fmt.Errorf("skill id is empty")
	}
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("install root is empty")
	}

	skillPath := filepath.Join(root, skillID)
	plan := &RemovePlan{
		Skill:     skillID,
		Target:    string(target),
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

func Doctor(catalog map[string]*Skill, target Target, scope Scope, root string) DoctorReport {
	report := DoctorReport{
		Target: string(target),
		Scope:  string(scope),
		Root:   root,
	}

	stat, err := os.Stat(root)
	if err == nil {
		report.Exists = stat.IsDir()
		if !stat.IsDir() {
			report.Issues = append(report.Issues, "install root exists but is not a directory")
		}
	} else if os.IsNotExist(err) {
		report.Exists = false
		report.Issues = append(report.Issues, "install root does not exist yet (it will be created on install)")
	} else {
		report.Issues = append(report.Issues, fmt.Sprintf("failed to inspect install root: %v", err))
	}

	summaries := SortedSummaries(catalog)
	for i := range summaries {
		skillPath := filepath.Join(root, summaries[i].Name)
		if stat, err := os.Stat(skillPath); err == nil && stat.IsDir() {
			summaries[i].Installed = true
			summaries[i].Path = skillPath
		}
	}

	report.Installed = make([]Summary, 0, len(summaries))
	for _, summary := range summaries {
		if summary.Installed {
			report.Installed = append(report.Installed, summary)
		}
	}
	sort.Slice(report.Installed, func(i, j int) bool {
		return report.Installed[i].Name < report.Installed[j].Name
	})

	return report
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

func InstalledSummaries(catalog map[string]*Skill, root string) []Summary {
	summaries := SortedSummaries(catalog)
	for i := range summaries {
		skillPath := filepath.Join(root, summaries[i].Name)
		if stat, err := os.Stat(skillPath); err == nil && stat.IsDir() {
			summaries[i].Installed = true
			summaries[i].Path = skillPath
		}
	}
	return summaries
}

func buildSkillMarkdown(skill *Skill) string {
	entry := strings.TrimSpace(skill.EntryMarkdown)
	if entry == "" {
		entry = "# " + skill.Spec.Title
	}

	return fmt.Sprintf("---\nname: %s\ndescription: %q\n---\n\n%s\n", skill.Spec.ID, skill.Spec.Summary, entry)
}

func sortedRenderedPaths(rendered map[string][]byte) []string {
	paths := make([]string, 0, len(rendered))
	for rel := range rendered {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	return paths
}

func checksumForRendered(rendered map[string][]byte) string {
	h := sha256.New()
	paths := sortedRenderedPaths(rendered)
	for _, rel := range paths {
		_, _ = h.Write([]byte(rel))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(rendered[rel])
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func receiptMatchesRendered(receipt *Receipt, skill *Skill, target, scope string, rendered map[string][]byte) bool {
	if receipt == nil || skill == nil {
		return false
	}
	return receipt.Checksum == checksumForRendered(rendered) &&
		receipt.Skill == skill.Spec.ID &&
		receipt.Version == skill.Spec.Version &&
		receipt.Target == target &&
		receipt.Scope == scope
}

func syncManagedNeedsUpdate(skillPath string, receipt *Receipt, skill *Skill, target, scope string, rendered map[string][]byte) (bool, error) {
	if !receiptMatchesRendered(receipt, skill, target, scope, rendered) {
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

func writeSkill(skill *Skill, target, scope, skillPath string, rendered map[string][]byte) (int, error) {
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

	receipt := &Receipt{
		Skill:       skill.Spec.ID,
		Version:     skill.Spec.Version,
		Target:      target,
		Scope:       scope,
		Checksum:    checksumForRendered(rendered),
		Files:       relPaths,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}

	receiptBytes, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return applied, fmt.Errorf("marshal receipt: %w", err)
	}
	receiptPath := filepath.Join(skillPath, receiptFileName)
	if err := atomicfile.WriteFile(receiptPath, receiptBytes, 0o644); err != nil {
		return applied, fmt.Errorf("write receipt: %w", err)
	}
	applied++

	return applied, nil
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
		return "", fmt.Errorf("receipt path escapes skill root: %s", relPath)
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

func readReceipt(path string) (*Receipt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Receipt
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
