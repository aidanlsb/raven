package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/skills"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage Raven agent skills",
	Long:  "Install and manage Raven-provided skills for supported agent runtimes.",
}

var skillListCmd = newCanonicalLeafCommand("skill_list", canonicalLeafOptions{
	RenderHuman: renderSkillList,
})

var skillInstallCmd = newCanonicalLeafCommand("skill_install", canonicalLeafOptions{
	RenderHuman: renderSkillInstall,
})

var skillRemoveCmd = newCanonicalLeafCommand("skill_remove", canonicalLeafOptions{
	RenderHuman: renderSkillRemove,
})

var skillDoctorCmd = newCanonicalLeafCommand("skill_doctor", canonicalLeafOptions{
	RenderHuman: renderSkillDoctor,
})

func init() {
	skillCmd.AddCommand(skillListCmd)
	skillCmd.AddCommand(skillInstallCmd)
	skillCmd.AddCommand(skillRemoveCmd)
	skillCmd.AddCommand(skillDoctorCmd)
	rootCmd.AddCommand(skillCmd)
}

func renderSkillList(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	items := skillSummariesFromAny(data["skills"])
	target := strings.TrimSpace(stringValue(data["target"]))
	if target == "" {
		for _, item := range items {
			fmt.Printf("%-16s v%d  %s\n", item.Name, item.Version, item.Summary)
		}
		return nil
	}

	fmt.Printf("target=%s scope=%s root=%s\n", target, stringValue(data["scope"]), stringValue(data["root"]))
	for _, item := range items {
		status := "available"
		if item.Installed {
			status = "installed"
		}
		fmt.Printf("%-16s v%d  %-10s %s\n", item.Name, item.Version, status, item.Summary)
	}
	return nil
}

func renderSkillInstall(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	plan := skillInstallPlanFromAny(data["plan"])
	if stringValue(data["mode"]) == "preview" {
		fmt.Printf("Preview install: %s -> %s\n", stringValue(data["skill_name"]), plan.SkillPath)
		for _, action := range plan.Actions {
			fmt.Printf("  %-8s %s\n", action.Op, action.Path)
		}
		if len(plan.Actions) == 0 {
			fmt.Println("  no changes")
		}
		fmt.Println("Re-run with --confirm to apply.")
		return nil
	}

	fmt.Printf("Installed %s for %s at %s\n", stringValue(data["skill_name"]), stringValue(data["target"]), plan.SkillPath)
	fmt.Printf("Applied %d file changes\n", intValue(data["actions_applied"]))
	return nil
}

func renderSkillRemove(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	plan := skillRemovePlanFromAny(data["plan"])
	if stringValue(data["mode"]) == "preview" {
		fmt.Printf("Preview remove: %s\n", plan.SkillPath)
		for _, action := range plan.Actions {
			fmt.Printf("  %-8s %s\n", action.Op, action.Path)
		}
		fmt.Println("Re-run with --confirm to apply.")
		return nil
	}

	fmt.Printf("Removed %s from %s\n", stringValue(data["skill_name"]), plan.SkillPath)
	return nil
}

func renderSkillDoctor(_ *cobra.Command, result commandexec.Result) error {
	for _, report := range skillDoctorReportsFromAny(canonicalDataMap(result)["reports"]) {
		fmt.Printf("target=%s scope=%s root=%s\n", report.Target, report.Scope, report.Root)
		if len(report.Installed) == 0 {
			fmt.Println("  installed: none")
		} else {
			fmt.Println("  installed:")
			for _, item := range report.Installed {
				fmt.Printf("    - %s\n", item.Name)
			}
		}
		if len(report.Issues) > 0 {
			fmt.Println("  issues:")
			for _, issue := range report.Issues {
				fmt.Printf("    - %s\n", issue)
			}
		}
	}
	return nil
}

func skillSummariesFromAny(raw interface{}) []skills.Summary {
	items, _ := raw.([]skills.Summary)
	return items
}

func skillInstallPlanFromAny(raw interface{}) *skills.InstallPlan {
	plan, _ := raw.(*skills.InstallPlan)
	return plan
}

func skillRemovePlanFromAny(raw interface{}) *skills.RemovePlan {
	plan, _ := raw.(*skills.RemovePlan)
	return plan
}

func skillDoctorReportsFromAny(raw interface{}) []skills.DoctorReport {
	reports, _ := raw.([]skills.DoctorReport)
	return reports
}

func intValue(raw interface{}) int {
	switch value := raw.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}
