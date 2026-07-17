package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/skills"
	"github.com/aidanlsb/raven/internal/skillsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage Raven agent skills",
	Long:  "Sync and manage Raven-provided skills using the Agent Skills standard.",
}

var skillListCmd = newCanonicalLeafCommand("skill_list", canonicalLeafOptions{
	RenderHuman: renderSkillList,
})

var skillSyncCmd = newCanonicalLeafCommand("skill_sync", canonicalLeafOptions{
	RenderHuman: renderSkillSync,
})

var (
	skillInstallYes     bool
	skillInstallConfirm bool
	skillInstallScope   string
	skillInstallDest    string
)

var skillInstallCmd = newCanonicalLeafCommand("skill_install", canonicalLeafOptions{
	Args:            cobra.ArbitraryArgs,
	BuildArgs:       buildSkillInstallArgs,
	Invoke:          invokeSkillInstall,
	RenderHuman:     renderSkillInstall,
	SkipFlagBinding: true,
})

var skillRemoveCmd = newCanonicalLeafCommand("skill_remove", canonicalLeafOptions{
	RenderHuman: renderSkillRemove,
})

var skillDoctorCmd = newCanonicalLeafCommand("skill_doctor", canonicalLeafOptions{
	RenderHuman: renderSkillDoctor,
})

func init() {
	skillInstallCmd.Flags().BoolVar(&skillInstallYes, "yes", false, "Apply changes without prompting (required for --json/non-interactive runs)")
	skillInstallCmd.Flags().BoolVar(&skillInstallConfirm, "confirm", false, "Alias for --yes")
	skillInstallCmd.Flags().StringVar(&skillInstallScope, "scope", "user", "Install scope: user or project")
	skillInstallCmd.Flags().StringVar(&skillInstallDest, "dest", "", "Override install root path")

	skillCmd.AddCommand(skillListCmd)
	skillCmd.AddCommand(skillInstallCmd)
	skillCmd.AddCommand(skillSyncCmd)
	skillCmd.AddCommand(skillRemoveCmd)
	skillCmd.AddCommand(skillDoctorCmd)
	rootCmd.AddCommand(skillCmd)
}

func buildSkillInstallArgs(_ *cobra.Command, args []string) (map[string]interface{}, error) {
	built := map[string]interface{}{}
	if len(args) > 0 {
		built["names"] = stringsToAny(args)
	}
	if skillInstallScope != "" {
		built["scope"] = skillInstallScope
	}
	if skillInstallDest != "" {
		built["dest"] = skillInstallDest
	}
	return built, nil
}

func invokeSkillInstall(_ *cobra.Command, commandID, _ string, args map[string]interface{}) commandexec.Result {
	apply := skillInstallYes || skillInstallConfirm

	// Non-interactive or --json: never prompt. Apply only with --yes/--confirm;
	// otherwise return a preview that flags confirmation as required.
	if !shouldPromptForConfirm() {
		return executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			Args:      args,
			Confirm:   apply,
		})
	}

	// Interactive terminal with an explicit --yes/--confirm still applies without
	// prompting.
	if apply {
		return executeCanonicalRequest(commandexec.Request{
			CommandID: commandID,
			Args:      args,
			Confirm:   true,
		})
	}

	// Interactive terminal: preview, print the plan, and prompt before writing.
	preview := executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		Args:      args,
	})
	if !preview.OK {
		return preview
	}
	data := canonicalDataMap(preview)
	if !boolValue(data["needs_confirm"]) {
		return preview
	}

	printSkillInstallPlan(data)
	if !promptForConfirm("Install these skills?") {
		return commandexec.Success(map[string]interface{}{"mode": "cancelled"}, nil)
	}

	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		Args:      args,
		Confirm:   true,
	})
}

func renderSkillList(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	items := skillSummariesFromAny(data["skills"])

	fmt.Println(ui.SectionHeader("Agent Skills"))
	fmt.Printf("%s %s\n", ui.Hint("Scope:"), stringValue(data["scope"]))
	fmt.Printf("%s %s\n", ui.Hint("Root:"), ui.FilePath(stringValue(data["root"])))
	if len(items) == 0 {
		fmt.Println(ui.Star("No skills found."))
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

	fmt.Println(ui.Checkf("Synced skills at %s", ui.FilePath(plan.Root)))
	fmt.Println(ui.Hint(fmt.Sprintf("Applied %d file changes", intValue(data["actions_applied"]))))
	if len(plan.MissingAvailable) > 0 {
		fmt.Println(ui.Hint(fmt.Sprintf("%d shipped skills are available but not installed", len(plan.MissingAvailable))))
	}
	return nil
}

func renderSkillInstall(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	mode := stringValue(data["mode"])
	root := stringValue(data["root"])

	switch mode {
	case "cancelled":
		fmt.Println(ui.Star("Cancelled. No skills were installed."))
		return nil
	case "applied":
		fmt.Println(ui.Checkf("Installed skills at %s", ui.FilePath(root)))
		fmt.Println(ui.Hint(fmt.Sprintf("Applied %d file changes", intValue(data["actions_applied"]))))
		return nil
	}

	printSkillInstallPlan(data)
	if boolValue(data["needs_confirm"]) {
		fmt.Println(ui.Warning("Preview only — re-run with --yes to install."))
	} else {
		fmt.Println(ui.Hint("All requested skills are already installed and up to date."))
	}
	return nil
}

func printSkillInstallPlan(data map[string]interface{}) {
	entries := skillInstallResultsFromAny(data["skills"])
	fmt.Println(ui.SectionHeader("Skill install plan"))
	fmt.Printf("%s %s\n", ui.Hint("target:"), ui.FilePath(stringValue(data["root"])))
	for _, entry := range entries {
		status := skillInstallStatus(entry.Plan)
		fmt.Println(ui.Bullet(fmt.Sprintf("%s %s", ui.Bold.Render(entry.Name), ui.Hint("("+status+")"))))
	}
}

func skillInstallStatus(plan *skills.SyncPlan) string {
	if plan == nil {
		return "no changes"
	}
	switch {
	case plan.Installed > 0:
		return "install"
	case plan.Updated > 0:
		return "update"
	case plan.Skipped > 0:
		return "skipped (unmanaged)"
	default:
		return "up to date"
	}
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
		fmt.Println(ui.SectionHeader("Agent Skills"))
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

func skillInstallResultsFromAny(raw interface{}) []skillsvc.InstallSkillResult {
	entries, _ := raw.([]skillsvc.InstallSkillResult)
	return entries
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
