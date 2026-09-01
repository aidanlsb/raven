package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/ui"
)

var sectionCmd = buildRegistrySubtree(registrySubtreeSpec{
	Prefix:    []string{"section"},
	VaultPath: getVaultPath,
	Root: registryGroup{
		Short:      "Manage Markdown sections",
		Long:       "Manage Markdown-derived sections without editing headings or references by hand.",
		ParentOnly: true,
	},
	Renders: map[string]func(*cobra.Command, commandexec.Result) error{
		"section_create": renderSectionCreateResult,
		"section_delete": renderSectionDeleteResult,
		"section_move":   renderSectionMoveResult,
		"section_rename": renderSectionRenameResult,
	},
})

func init() {
	createCmd, _, err := sectionCmd.Find([]string{"create"})
	if err != nil {
		panic(err)
	}
	createCmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		if !cmd.Flags().Changed("level") {
			return handleErrorMsg(
				ErrInvalidInput,
				"--level is required",
				`Usage: rvn section create <file> "<title>" --level N`,
			)
		}
		return nil
	}
	rootCmd.AddCommand(sectionCmd)
}

func renderSectionCreateResult(_ *cobra.Command, result commandexec.Result) error {
	data, ok := result.Data.(commandpayload.SectionLifecycleResult)
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	if data.Preview {
		fmt.Println(ui.Star(fmt.Sprintf("Would create section %s", ui.FilePath(data.Section))))
		fmt.Println(ui.Hint("Dry run: re-run without --dry-run to apply"))
		return nil
	}

	renderSectionCreated(data.Section)
	return nil
}

func renderSectionMoveResult(_ *cobra.Command, result commandexec.Result) error {
	data, ok := result.Data.(commandpayload.SectionLifecycleResult)
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}
	description := data.Placement
	if data.Anchor != "" {
		description += " " + data.Anchor
	}

	if data.Preview {
		fmt.Println(ui.Star(fmt.Sprintf("Would move section %s %s", ui.FilePath(data.Section), description)))
		fmt.Println(ui.Hint("Dry run: re-run without --dry-run to apply"))
		return nil
	}

	fmt.Println(ui.Checkf("Moved section %s %s", ui.FilePath(data.Section), description))
	return nil
}

func renderSectionDeleteResult(_ *cobra.Command, result commandexec.Result) error {
	data, ok := result.Data.(commandpayload.SectionDeleteResult)
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}
	backlinkCount := len(data.Backlinks)

	if data.Preview {
		fmt.Println(ui.Star(fmt.Sprintf(
			"Would delete section %s from %s (lines %d-%d)",
			ui.FilePath(data.Section),
			ui.FilePath(data.File),
			data.LineStart,
			data.LineEnd,
		)))
		if data.RemovedContent != "" {
			fmt.Printf("\n%s\n", data.RemovedContent)
		}
		if backlinkCount > 0 {
			fmt.Printf("  %s\n", ui.Warning(fmt.Sprintf(
				"Would leave %d inbound reference(s) unchanged; repair or remove them explicitly",
				backlinkCount,
			)))
		}
		fmt.Println(ui.Hint("Preview only: re-run with --confirm to delete"))
		return nil
	}

	renderSectionDeleted(data.Section, data.File, data.LineStart, data.LineEnd)
	if backlinkCount > 0 {
		fmt.Printf("  %s\n", ui.Warning(fmt.Sprintf(
			"Left %d inbound reference(s) unchanged; repair or remove them explicitly",
			backlinkCount,
		)))
	}
	return nil
}

func renderSectionRenameResult(_ *cobra.Command, result commandexec.Result) error {
	data, ok := result.Data.(commandpayload.SectionRenameResult)
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	if data.Preview {
		fmt.Println(ui.Star(fmt.Sprintf("Would rename section %s → %s", ui.FilePath(data.Source), ui.FilePath(data.Destination))))
		if len(data.UpdatedRefs) > 0 {
			fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Would update %d references", len(data.UpdatedRefs))))
		}
		fmt.Println(ui.Hint("Dry run: re-run without --dry-run to apply"))
		return nil
	}

	fmt.Println(ui.Checkf("Renamed section %s → %s", ui.FilePath(data.Source), ui.FilePath(data.Destination)))
	if len(data.UpdatedRefs) > 0 {
		fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("Updated %d references", len(data.UpdatedRefs))))
	}
	return nil
}
