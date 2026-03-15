package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/skills"
	"github.com/aidanlsb/raven/internal/skillsvc"
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
		result, err := skillsvc.List(skillsvc.ListRequest{
			Target:        skillListTarget,
			Scope:         skillListScope,
			Dest:          skillListDest,
			InstalledOnly: skillListInstalledOnly,
		})
		if err != nil {
			return mapSkillServiceError(err)
		}

		if isJSONOutput() {
			data := map[string]interface{}{
				"skills": result.Skills,
			}
			if strings.TrimSpace(result.Target) != "" {
				data["target"] = result.Target
				data["scope"] = result.Scope
				data["root"] = result.Root
			}
			outputSuccess(data, &Meta{Count: len(result.Skills)})
			return nil
		}

		if strings.TrimSpace(result.Target) == "" {
			for _, item := range result.Skills {
				fmt.Printf("%-16s v%d  %s\n", item.Name, item.Version, item.Summary)
			}
			return nil
		}

		fmt.Printf("target=%s scope=%s root=%s\n", result.Target, result.Scope, result.Root)
		for _, item := range result.Skills {
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
		result, err := skillsvc.Install(skillsvc.InstallRequest{
			Name:    strings.TrimSpace(args[0]),
			Target:  skillInstallTarget,
			Scope:   skillInstallScope,
			Dest:    skillInstallDest,
			Force:   skillInstallForce,
			Confirm: skillInstallConfirm,
		})
		if err != nil {
			return mapSkillServiceError(err)
		}

		if isJSONOutput() {
			data := map[string]interface{}{
				"mode": result.Mode,
				"plan": result.Plan,
			}
			if result.ActionsApplied > 0 {
				data["actions_applied"] = result.ActionsApplied
			}
			if result.Receipt != nil {
				data["receipt"] = result.Receipt
			}
			outputSuccess(data, nil)
			return nil
		}

		if result.Mode == "preview" {
			fmt.Printf("Preview install: %s -> %s\n", result.SkillName, result.Plan.SkillPath)
			for _, action := range result.Plan.Actions {
				fmt.Printf("  %-8s %s\n", action.Op, action.Path)
			}
			if len(result.Plan.Actions) == 0 {
				fmt.Println("  no changes")
			}
			fmt.Println("Re-run with --confirm to apply.")
			return nil
		}

		fmt.Printf("Installed %s for %s at %s\n", result.SkillName, result.Target, result.Plan.SkillPath)
		fmt.Printf("Applied %d file changes\n", result.ActionsApplied)
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
		result, err := skillsvc.Remove(skillsvc.RemoveRequest{
			Name:    strings.TrimSpace(args[0]),
			Target:  skillRemoveTarget,
			Scope:   skillRemoveScope,
			Dest:    skillRemoveDest,
			Confirm: skillRemoveConfirm,
		})
		if err != nil {
			return mapSkillServiceError(err)
		}

		if isJSONOutput() {
			data := map[string]interface{}{
				"mode": result.Mode,
				"plan": result.Plan,
			}
			if result.Removed {
				data["removed"] = true
			}
			outputSuccess(data, nil)
			return nil
		}

		if result.Mode == "preview" {
			fmt.Printf("Preview remove: %s\n", result.Plan.SkillPath)
			for _, action := range result.Plan.Actions {
				fmt.Printf("  %-8s %s\n", action.Op, action.Path)
			}
			fmt.Println("Re-run with --confirm to apply.")
			return nil
		}

		fmt.Printf("Removed %s from %s\n", result.SkillName, result.Plan.SkillPath)
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
		result, err := skillsvc.Doctor(skillsvc.DoctorRequest{
			Target: skillDoctorTarget,
			Scope:  skillDoctorScope,
			Dest:   skillDoctorDest,
		})
		if err != nil {
			return mapSkillServiceError(err)
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{"reports": result.Reports}, &Meta{Count: len(result.Reports)})
			return nil
		}

		for _, report := range result.Reports {
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

func mapSkillServiceError(err error) error {
	svcErr, ok := skillsvc.AsError(err)
	if !ok {
		return handleError(ErrInternal, err, "")
	}

	switch svcErr.Code {
	case skillsvc.CodeInvalidInput:
		return handleErrorMsg(ErrInvalidInput, svcErr.Message, svcErr.Suggestion)
	case skillsvc.CodeSkillNotFound:
		return handleErrorWithDetails(ErrSkillNotFound, svcErr.Message, svcErr.Suggestion, svcErr.Details)
	case skillsvc.CodeSkillNotInstalled:
		return handleErrorMsg(ErrSkillNotInstalled, svcErr.Message, svcErr.Suggestion)
	case skillsvc.CodeSkillTargetUnsupported:
		return handleErrorMsg(ErrSkillTargetUnsupported, svcErr.Message, svcErr.Suggestion)
	case skillsvc.CodeSkillInstallConflict:
		return handleErrorWithDetails(ErrSkillInstallConflict, svcErr.Message, svcErr.Suggestion, svcErr.Details)
	case skillsvc.CodeSkillPathUnresolved:
		return handleErrorMsg(ErrSkillPathUnresolved, svcErr.Message, svcErr.Suggestion)
	case skillsvc.CodeFileWriteError:
		return handleError(ErrFileWriteError, svcErr, svcErr.Suggestion)
	default:
		return handleError(ErrInternal, svcErr, svcErr.Suggestion)
	}
}
