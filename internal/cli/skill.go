package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/skills"
	"github.com/aidanlsb/raven/internal/ui"
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
		if len(items) == 0 {
			fmt.Println(ui.Star("No skills available."))
			return nil
		}
		for _, item := range items {
			fmt.Println(ui.Bullet(fmt.Sprintf("%s %s %s", ui.Bold.Render(item.Name), ui.Hint(fmt.Sprintf("v%d", item.Version)), item.Summary)))
		}
		return nil
	}

	fmt.Printf("%s %s\n", ui.SectionHeader("Target"), ui.Bold.Render(target))
	fmt.Printf("%s %s\n", ui.Hint("Scope:"), stringValue(data["scope"]))
	fmt.Printf("%s %s\n", ui.Hint("Root:"), ui.FilePath(stringValue(data["root"])))
	if len(items) == 0 {
		fmt.Println(ui.Star("No skills found for this target."))
		return nil
	}
	for _, item := range items {
		status := "available"
		if item.Installed {
			status = "installed"
		}
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s %s %s", ui.Bold.Render(item.Name), ui.Hint(fmt.Sprintf("v%d", item.Version)), ui.Hint("["+status+"]"), item.Summary)))
	}
	return nil
}

func renderSkillInstall(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	plan := skillInstallPlanFromAny(data["plan"])
	if stringValue(data["mode"]) == "preview" {
		fmt.Println(ui.SectionHeader(fmt.Sprintf("Preview install: %s", stringValue(data["skill_name"]))))
		fmt.Printf("%s %s\n", ui.Hint("target:"), ui.FilePath(plan.SkillPath))
		for _, action := range plan.Actions {
			fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render(action.Op), ui.FilePath(action.Path))))
		}
		if len(plan.Actions) == 0 {
			fmt.Println(ui.Bullet(ui.Hint("no changes")))
		}
		fmt.Println(ui.Hint("Re-run with --confirm to apply."))
		return nil
	}

	fmt.Println(ui.Checkf("Installed %s for %s at %s", stringValue(data["skill_name"]), stringValue(data["target"]), ui.FilePath(plan.SkillPath)))
	fmt.Println(ui.Hint(fmt.Sprintf("Applied %d file changes", intValue(data["actions_applied"]))))
	return nil
}

func renderSkillRemove(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	plan := skillRemovePlanFromAny(data["plan"])
	if stringValue(data["mode"]) == "preview" {
		fmt.Println(ui.SectionHeader("Preview remove"))
		fmt.Printf("%s %s\n", ui.Hint("target:"), ui.FilePath(plan.SkillPath))
		for _, action := range plan.Actions {
			fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render(action.Op), ui.FilePath(action.Path))))
		}
		fmt.Println(ui.Hint("Re-run with --confirm to apply."))
		return nil
	}

	fmt.Println(ui.Checkf("Removed %s from %s", stringValue(data["skill_name"]), ui.FilePath(plan.SkillPath)))
	return nil
}

func renderSkillDoctor(_ *cobra.Command, result commandexec.Result) error {
	for _, report := range skillDoctorReportsFromAny(canonicalDataMap(result)["reports"]) {
		fmt.Println(ui.SectionHeader(report.Target))
		fmt.Printf("%s %s\n", ui.Hint("scope:"), report.Scope)
		fmt.Printf("%s %s\n", ui.Hint("root:"), ui.FilePath(report.Root))
		if len(report.Installed) == 0 {
			fmt.Println(ui.Bullet(ui.Hint("installed: none")))
		} else {
			fmt.Println(ui.Bullet("installed:"))
			for _, item := range report.Installed {
				fmt.Println(ui.Indent(2, ui.Bullet(item.Name)))
			}
		}
		if len(report.Issues) > 0 {
			fmt.Println(ui.Bullet("issues:"))
			for _, issue := range report.Issues {
				fmt.Println(ui.Indent(2, ui.Bullet(ui.Warning(issue))))
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
