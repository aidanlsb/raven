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
