package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

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
		catalog, err := skills.LoadCatalog()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}

		if strings.TrimSpace(skillListTarget) == "" {
			if skillListInstalledOnly {
				return handleErrorMsg(ErrInvalidInput, "--installed requires --target", "Specify --target codex|claude|cursor")
			}
			items := skills.SortedSummaries(catalog)
			if isJSONOutput() {
				outputSuccess(map[string]interface{}{
					"skills": items,
				}, &Meta{Count: len(items)})
				return nil
			}
			for _, item := range items {
				fmt.Printf("%-16s v%d  %s\n", item.Name, item.Version, item.Summary)
			}
			return nil
		}

		target, err := skills.ParseTarget(skillListTarget)
		if err != nil {
			return handleError(ErrSkillTargetUnsupported, err, "Use --target codex|claude|cursor")
		}
		scope, err := skills.ParseScope(skillListScope)
		if err != nil {
			return handleError(ErrInvalidInput, err, "Use --scope user|project")
		}
		root, err := skills.ResolveInstallRoot(target, scope, skillListDest, "")
		if err != nil {
			return handleError(ErrSkillPathUnresolved, err, "Use --dest to set an explicit install root")
		}

		items := skills.InstalledSummaries(catalog, root)
		if skillListInstalledOnly {
			filtered := make([]skills.Summary, 0, len(items))
			for _, item := range items {
				if item.Installed {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"target": string(target),
				"scope":  string(scope),
				"root":   root,
				"skills": items,
			}, &Meta{Count: len(items)})
			return nil
		}

		fmt.Printf("target=%s scope=%s root=%s\n", target, scope, root)
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
		skillName := strings.TrimSpace(args[0])
		catalog, err := skills.LoadCatalog()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		skill, ok := catalog[skillName]
		if !ok {
			available := skills.SortedSummaries(catalog)
			names := make([]string, 0, len(available))
			for _, item := range available {
				names = append(names, item.Name)
			}
			return handleErrorWithDetails(
				ErrSkillNotFound,
				fmt.Sprintf("skill '%s' not found", skillName),
				"Run 'rvn skill list' to see available skills",
				map[string]interface{}{"available": names},
			)
		}

		target, err := skills.ParseTarget(skillInstallTarget)
		if err != nil {
			return handleError(ErrSkillTargetUnsupported, err, "Use --target codex|claude|cursor")
		}
		scope, err := skills.ParseScope(skillInstallScope)
		if err != nil {
			return handleError(ErrInvalidInput, err, "Use --scope user|project")
		}
		root, err := skills.ResolveInstallRoot(target, scope, skillInstallDest, "")
		if err != nil {
			return handleError(ErrSkillPathUnresolved, err, "Use --dest to set an explicit install root")
		}

		plan, err := skills.PlanInstall(skill, target, scope, root, skillInstallForce)
		if err != nil {
			return handleError(ErrSkillRenderFailed, err, "")
		}

		if len(plan.Conflicts) > 0 {
			return handleErrorWithDetails(
				ErrSkillInstallConflict,
				"install has conflicts",
				"Use --force to overwrite conflicting files",
				map[string]interface{}{
					"conflicts": plan.Conflicts,
					"plan":      plan,
				},
			)
		}

		if !skillInstallConfirm {
			if isJSONOutput() {
				outputSuccess(map[string]interface{}{
					"mode": "preview",
					"plan": plan,
				}, nil)
				return nil
			}
			fmt.Printf("Preview install: %s -> %s\n", skillName, plan.SkillPath)
			for _, action := range plan.Actions {
				fmt.Printf("  %-8s %s\n", action.Op, action.Path)
			}
			if len(plan.Actions) == 0 {
				fmt.Println("  no changes")
			}
			fmt.Println("Re-run with --confirm to apply.")
			return nil
		}

		receipt, applied, err := skills.ApplyInstall(plan)
		if err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"mode":            "applied",
				"plan":            plan,
				"actions_applied": applied,
				"receipt":         receipt,
			}, nil)
			return nil
		}
		fmt.Printf("Installed %s for %s at %s\n", skillName, target, plan.SkillPath)
		fmt.Printf("Applied %d file changes\n", applied)
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
		skillName := strings.TrimSpace(args[0])
		catalog, err := skills.LoadCatalog()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		if _, ok := catalog[skillName]; !ok {
			return handleErrorMsg(ErrSkillNotFound, fmt.Sprintf("skill '%s' not found", skillName), "Run 'rvn skill list' to see available skills")
		}

		target, err := skills.ParseTarget(skillRemoveTarget)
		if err != nil {
			return handleError(ErrSkillTargetUnsupported, err, "Use --target codex|claude|cursor")
		}
		scope, err := skills.ParseScope(skillRemoveScope)
		if err != nil {
			return handleError(ErrInvalidInput, err, "Use --scope user|project")
		}
		root, err := skills.ResolveInstallRoot(target, scope, skillRemoveDest, "")
		if err != nil {
			return handleError(ErrSkillPathUnresolved, err, "Use --dest to set an explicit install root")
		}

		plan, err := skills.PlanRemove(skillName, target, scope, root)
		if err != nil {
			return handleError(ErrInvalidInput, err, "")
		}
		if !plan.Exists {
			return handleErrorMsg(ErrSkillNotInstalled, fmt.Sprintf("skill '%s' is not installed for target '%s'", skillName, target), "Run 'rvn skill list --target ... --installed' to see installed skills")
		}

		if !skillRemoveConfirm {
			if isJSONOutput() {
				outputSuccess(map[string]interface{}{
					"mode": "preview",
					"plan": plan,
				}, nil)
				return nil
			}
			fmt.Printf("Preview remove: %s\n", plan.SkillPath)
			for _, action := range plan.Actions {
				fmt.Printf("  %-8s %s\n", action.Op, action.Path)
			}
			fmt.Println("Re-run with --confirm to apply.")
			return nil
		}

		if err := skills.ApplyRemove(plan); err != nil {
			return handleError(ErrFileWriteError, err, "")
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"mode":    "applied",
				"removed": true,
				"plan":    plan,
			}, nil)
			return nil
		}
		fmt.Printf("Removed %s from %s\n", skillName, plan.SkillPath)
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
		catalog, err := skills.LoadCatalog()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		scope, err := skills.ParseScope(skillDoctorScope)
		if err != nil {
			return handleError(ErrInvalidInput, err, "Use --scope user|project")
		}

		reports := make([]skills.DoctorReport, 0)
		if strings.TrimSpace(skillDoctorTarget) == "" {
			if strings.TrimSpace(skillDoctorDest) != "" {
				return handleErrorMsg(ErrInvalidInput, "--dest requires --target", "Specify --target codex|claude|cursor when using --dest")
			}
			for _, target := range skills.AllTargets() {
				root, err := skills.ResolveInstallRoot(target, scope, "", "")
				if err != nil {
					reports = append(reports, skills.DoctorReport{
						Target: string(target),
						Scope:  string(scope),
						Issues: []string{fmt.Sprintf("failed to resolve root: %v", err)},
					})
					continue
				}
				reports = append(reports, skills.Doctor(catalog, target, scope, root))
			}
		} else {
			target, err := skills.ParseTarget(skillDoctorTarget)
			if err != nil {
				return handleError(ErrSkillTargetUnsupported, err, "Use --target codex|claude|cursor")
			}
			root, err := skills.ResolveInstallRoot(target, scope, skillDoctorDest, "")
			if err != nil {
				return handleError(ErrSkillPathUnresolved, err, "Use --dest to set an explicit install root")
			}
			reports = append(reports, skills.Doctor(catalog, target, scope, root))
		}

		sort.Slice(reports, func(i, j int) bool {
			return reports[i].Target < reports[j].Target
		})

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{"reports": reports}, &Meta{Count: len(reports)})
			return nil
		}

		for _, report := range reports {
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
