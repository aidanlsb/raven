package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
