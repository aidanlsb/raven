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
	Long:  "Sync and manage Raven-provided skills for supported agent runtimes.",
}

var skillListCmd = newCanonicalLeafCommand("skill_list", canonicalLeafOptions{
	RenderHuman: renderSkillList,
})

var skillSyncCmd = newCanonicalLeafCommand("skill_sync", canonicalLeafOptions{
	RenderHuman: renderSkillSync,
})

var skillRemoveCmd = newCanonicalLeafCommand("skill_remove", canonicalLeafOptions{
	RenderHuman: renderSkillRemove,
})

var skillDoctorCmd = newCanonicalLeafCommand("skill_doctor", canonicalLeafOptions{
	RenderHuman: renderSkillDoctor,
})

func init() {
	skillCmd.AddCommand(skillListCmd)
	skillCmd.AddCommand(skillSyncCmd)
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

func renderSkillSync(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	plan := skillSyncPlanFromAny(data["plan"])
	if plan == nil {
		return nil
	}
	if stringValue(data["mode"]) == "preview" {
		title := "Preview sync"
		if skillName := strings.TrimSpace(stringValue(data["skill_name"])); skillName != "" {
			title = fmt.Sprintf("Preview sync: %s", skillName)
		}
		fmt.Println(ui.SectionHeader(title))
		fmt.Printf("%s %s\n", ui.Hint("target:"), ui.FilePath(plan.Root))
		for _, action := range plan.Actions {
			line := fmt.Sprintf("%s %s", ui.Bold.Render(action.Op), ui.FilePath(action.Path))
			if strings.TrimSpace(action.Reason) != "" {
				line += " " + ui.Hint("("+action.Reason+")")
			}
			fmt.Println(ui.Bullet(line))
		}
		if len(plan.Actions) == 0 {
			fmt.Println(ui.Bullet(ui.Hint("no changes")))
		}
		if len(plan.MissingAvailable) > 0 {
			fmt.Println(ui.Bullet("available but not installed:"))
			for _, item := range plan.MissingAvailable {
				fmt.Println(ui.Indent(2, ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render(item.Name), ui.Hint(fmt.Sprintf("v%d", item.Version))))))
			}
		}
		if plan.NeedsConfirm {
			fmt.Println(ui.Hint("Re-run with --confirm to apply."))
		}
		return nil
	}

	fmt.Println(ui.Checkf("Synced skills for %s at %s", stringValue(data["target"]), ui.FilePath(plan.Root)))
	fmt.Println(ui.Hint(fmt.Sprintf("Applied %d file changes", intValue(data["actions_applied"]))))
	if len(plan.MissingAvailable) > 0 {
		fmt.Println(ui.Hint(fmt.Sprintf("%d shipped skills are available but not installed", len(plan.MissingAvailable))))
	}
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

func skillSyncPlanFromAny(raw interface{}) *skills.SyncPlan {
	plan, _ := raw.(*skills.SyncPlan)
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
