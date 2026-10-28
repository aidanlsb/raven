package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	doctorStatusUpToDate  = "up_to_date"
	doctorStatusOutdated  = "outdated"
	doctorStatusModified  = "modified"
	doctorStatusUnmanaged = "unmanaged"
)

type DoctorReport struct {
	Scope     string    `json:"scope"`
	Root      string    `json:"root"`
	Exists    bool      `json:"exists"`
	Installed []Summary `json:"installed"`
	Issues    []string  `json:"issues,omitempty"`
}

func Doctor(catalog map[string]*Skill, scope Scope, root string) DoctorReport {
	report := DoctorReport{
		Scope: string(scope),
		Root:  root,
	}

	stat, err := os.Stat(root)
	if err == nil {
		report.Exists = stat.IsDir()
		if !stat.IsDir() {
			report.Issues = append(report.Issues, "install root exists but is not a directory")
			return report
		}
	} else if os.IsNotExist(err) {
		report.Exists = false
		report.Issues = append(report.Issues, "install root does not exist yet (it will be created on install)")
		return report
	} else {
		report.Issues = append(report.Issues, fmt.Sprintf("failed to inspect install root: %v", err))
		return report
	}

	summaries := SortedSummaries(catalog)
	catalogNames := make(map[string]struct{}, len(summaries))
	installed := make([]Summary, 0, len(summaries))
	for _, summary := range summaries {
		catalogNames[summary.Name] = struct{}{}
		inspected, present := inspectCatalogSkill(catalog[summary.Name], string(scope), root, summary)
		if !present {
			continue
		}
		installed = append(installed, inspected.summary)
		report.Issues = append(report.Issues, inspected.issues...)
	}

	extraIssues, extraErr := inspectUncataloguedSkillDirs(root, catalogNames)
	if extraErr != nil {
		report.Issues = append(report.Issues, extraErr.Error())
	}
	report.Issues = append(report.Issues, extraIssues...)

	sort.Slice(installed, func(i, j int) bool {
		return installed[i].Name < installed[j].Name
	})
	report.Installed = installed
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

type skillInspection struct {
	summary Summary
	issues  []string
}

func inspectCatalogSkill(skill *Skill, scope, root string, summary Summary) (skillInspection, bool) {
	skillPath := filepath.Join(root, summary.Name)
	stat, err := os.Stat(skillPath)
	if err != nil {
		if os.IsNotExist(err) {
			return skillInspection{}, false
		}
		return skillInspection{
			summary: summary,
			issues:  []string{fmt.Sprintf("%s: failed to inspect skill directory: %v", summary.Name, err)},
		}, true
	}
	if !stat.IsDir() {
		return skillInspection{}, false
	}

	summary.Installed = true
	summary.Path = skillPath

	receipt, receiptErr := readReceipt(filepath.Join(skillPath, receiptFileName))
	if receiptErr != nil {
		if os.IsNotExist(receiptErr) {
			summary.Status = doctorStatusUnmanaged
			return skillInspection{
				summary: summary,
				issues: []string{
					fmt.Sprintf("%s: skill directory exists without Raven receipt (run 'rvn skill install' to inspect)", summary.Name),
				},
			}, true
		}
		summary.Status = doctorStatusUnmanaged
		return skillInspection{
			summary: summary,
			issues: []string{
				fmt.Sprintf("%s: Raven receipt is unreadable or invalid (run 'rvn skill install' to inspect): %v", summary.Name, receiptErr),
			},
		}, true
	}
	if receipt.Skill != summary.Name {
		summary.Status = doctorStatusUnmanaged
		return skillInspection{
			summary: summary,
			issues: []string{
				fmt.Sprintf("%s: Raven receipt skill %q does not match directory name (run 'rvn skill install' to inspect)", summary.Name, receipt.Skill),
			},
		}, true
	}

	if skill == nil {
		summary.Status = doctorStatusOutdated
		return skillInspection{
			summary: summary,
			issues: []string{
				fmt.Sprintf("%s: installed skill is no longer in the shipped catalog (run 'rvn skill install --confirm' to remove)", summary.Name),
			},
		}, true
	}

	rendered, err := RenderFiles(skill)
	if err != nil {
		return skillInspection{
			summary: summary,
			issues:  []string{fmt.Sprintf("%s: failed to render shipped skill: %v", summary.Name, err)},
		}, true
	}

	modified := !filesMatchReceipt(skillPath, receipt)
	outdated := !receiptMatchesRendered(receipt, skill, scope, rendered)
	if !modified && !outdated {
		needsUpdate, syncErr := syncManagedNeedsUpdate(skillPath, receipt, skill, scope, rendered)
		if syncErr != nil {
			return skillInspection{
				summary: summary,
				issues:  []string{fmt.Sprintf("%s: failed to compare installed skill with shipped catalog: %v", summary.Name, syncErr)},
			}, true
		}
		outdated = needsUpdate
	}

	issues := make([]string, 0, 2)
	switch {
	case modified:
		summary.Status = doctorStatusModified
		issues = append(issues, fmt.Sprintf("%s: installed skill has local modifications or missing files (run 'rvn skill install --confirm' to restore)", summary.Name))
		if outdated {
			issues = append(issues, fmt.Sprintf("%s: installed skill is outdated compared to shipped catalog (run 'rvn skill install --confirm' to update)", summary.Name))
		}
	case outdated:
		summary.Status = doctorStatusOutdated
		issues = append(issues, fmt.Sprintf("%s: installed skill is outdated compared to shipped catalog (run 'rvn skill install --confirm' to update)", summary.Name))
	default:
		summary.Status = doctorStatusUpToDate
	}

	return skillInspection{summary: summary, issues: issues}, true
}

func inspectUncataloguedSkillDirs(root string, catalogNames map[string]struct{}) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read install root: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, inCatalog := catalogNames[name]; inCatalog {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	issues := make([]string, 0)
	for _, dirName := range names {
		skillPath := filepath.Join(root, dirName)
		receipt, receiptErr := readReceipt(filepath.Join(skillPath, receiptFileName))
		if receiptErr != nil {
			if os.IsNotExist(receiptErr) {
				continue
			}
			issues = append(issues, fmt.Sprintf("%s: Raven receipt is unreadable or invalid (run 'rvn skill install' to inspect): %v", dirName, receiptErr))
			continue
		}
		if receipt.Skill != dirName {
			issues = append(issues, fmt.Sprintf("%s: Raven receipt skill %q does not match directory name (run 'rvn skill install' to inspect)", dirName, receipt.Skill))
			continue
		}
		issues = append(issues, fmt.Sprintf("%s: installed skill is no longer in the shipped catalog (run 'rvn skill install --confirm' to remove)", dirName))
	}
	return issues, nil
}
