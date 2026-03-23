package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/skills"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage Raven agent skills",
	Long:  "Install and manage Raven-provided skills for supported agent runtimes.",
}

var (
	skillListTarget        string
	skillListScope         string
	skillListDest          string
	skillListInstalledOnly bool
)

var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: commands.Registry["skill_list"].Description,
	Long:  commands.Registry["skill_list"].LongDesc,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := executeCanonicalCommand("skill_list", "", map[string]interface{}{
			"target":    skillListTarget,
			"scope":     skillListScope,
			"dest":      skillListDest,
			"installed": skillListInstalledOnly,
		})
		if !result.OK {
			return handleCanonicalSkillFailure(result)
		}

		if isJSONOutput() {
			outputJSON(result)
			return nil
		}

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
	},
}

var (
	skillInstallTarget  string
	skillInstallScope   string
	skillInstallDest    string
	skillInstallForce   bool
	skillInstallConfirm bool
)

var skillInstallCmd = &cobra.Command{
	Use:   "install <name>",
	Short: commands.Registry["skill_install"].Description,
	Long:  commands.Registry["skill_install"].LongDesc,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		result := executeCanonicalCommand("skill_install", "", map[string]interface{}{
			"name":    strings.TrimSpace(args[0]),
			"target":  skillInstallTarget,
			"scope":   skillInstallScope,
			"dest":    skillInstallDest,
			"force":   skillInstallForce,
			"confirm": skillInstallConfirm,
		})
		if !result.OK {
			return handleCanonicalSkillFailure(result)
		}

		if isJSONOutput() {
			outputJSON(result)
			return nil
		}

		data := canonicalDataMap(result)
		plan := skillInstallPlanFromAny(data["plan"])
		if stringValue(data["mode"]) == "preview" {
			fmt.Printf("Preview install: %s -> %s\n", strings.TrimSpace(args[0]), plan.SkillPath)
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
	},
}

var (
	skillRemoveTarget  string
	skillRemoveScope   string
	skillRemoveDest    string
	skillRemoveConfirm bool
)

var skillRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: commands.Registry["skill_remove"].Description,
	Long:  commands.Registry["skill_remove"].LongDesc,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		result := executeCanonicalCommand("skill_remove", "", map[string]interface{}{
			"name":    strings.TrimSpace(args[0]),
			"target":  skillRemoveTarget,
			"scope":   skillRemoveScope,
			"dest":    skillRemoveDest,
			"confirm": skillRemoveConfirm,
		})
		if !result.OK {
			return handleCanonicalSkillFailure(result)
		}

		if isJSONOutput() {
			outputJSON(result)
			return nil
		}

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
	},
}

var (
	skillDoctorTarget string
	skillDoctorScope  string
	skillDoctorDest   string
)

var skillDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: commands.Registry["skill_doctor"].Description,
	Long:  commands.Registry["skill_doctor"].LongDesc,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := executeCanonicalCommand("skill_doctor", "", map[string]interface{}{
			"target": skillDoctorTarget,
			"scope":  skillDoctorScope,
			"dest":   skillDoctorDest,
		})
		if !result.OK {
			return handleCanonicalSkillFailure(result)
		}

		if isJSONOutput() {
			outputJSON(result)
			return nil
		}

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
	},
}

func init() {
	skillListCmd.Flags().StringVar(&skillListTarget, "target", "", "Target runtime: codex, claude, or cursor")
	skillListCmd.Flags().StringVar(&skillListScope, "scope", string(skills.ScopeUser), "Install scope: user or project")
	skillListCmd.Flags().StringVar(&skillListDest, "dest", "", "Override install root path")
	skillListCmd.Flags().BoolVar(&skillListInstalledOnly, "installed", false, "Show installed skills only")

	skillInstallCmd.Flags().StringVar(&skillInstallTarget, "target", string(skills.TargetCodex), "Target runtime: codex, claude, or cursor")
	skillInstallCmd.Flags().StringVar(&skillInstallScope, "scope", string(skills.ScopeUser), "Install scope: user or project")
	skillInstallCmd.Flags().StringVar(&skillInstallDest, "dest", "", "Override install root path")
	skillInstallCmd.Flags().BoolVar(&skillInstallForce, "force", false, "Overwrite conflicting files")
	skillInstallCmd.Flags().BoolVar(&skillInstallConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")

	skillRemoveCmd.Flags().StringVar(&skillRemoveTarget, "target", string(skills.TargetCodex), "Target runtime: codex, claude, or cursor")
	skillRemoveCmd.Flags().StringVar(&skillRemoveScope, "scope", string(skills.ScopeUser), "Install scope: user or project")
	skillRemoveCmd.Flags().StringVar(&skillRemoveDest, "dest", "", "Override install root path")
	skillRemoveCmd.Flags().BoolVar(&skillRemoveConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")

	skillDoctorCmd.Flags().StringVar(&skillDoctorTarget, "target", "", "Target runtime: codex, claude, or cursor (omit to check all)")
	skillDoctorCmd.Flags().StringVar(&skillDoctorScope, "scope", string(skills.ScopeUser), "Install scope: user or project")
	skillDoctorCmd.Flags().StringVar(&skillDoctorDest, "dest", "", "Override install root path")

	skillCmd.AddCommand(skillListCmd)
	skillCmd.AddCommand(skillInstallCmd)
	skillCmd.AddCommand(skillRemoveCmd)
	skillCmd.AddCommand(skillDoctorCmd)
	rootCmd.AddCommand(skillCmd)
}

func handleCanonicalSkillFailure(result commandexec.Result) error {
	if isJSONOutput() {
		outputJSON(result)
		return nil
	}
	if result.Error != nil {
		return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
	}
	return handleErrorMsg(ErrInternal, "command execution failed", "")
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
