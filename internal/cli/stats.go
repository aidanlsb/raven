package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/maintsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var vaultStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show index statistics",
	Long: `Displays statistics about the vault index.

Examples:
  rvn vault stats
  rvn vault stats --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runVaultStats()
	},
}

func runVaultStats() error {
	vaultPath := getVaultPath()
	result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
		CommandID: "vault_stats",
		VaultPath: vaultPath,
		Caller:    commandexec.CallerCLI,
	})
	if !result.OK {
		if isJSONOutput() {
			outputJSON(result)
			return nil
		}
		if result.Error != nil {
			return handleErrorWithDetails(mapMaintSvcCode(result.Error.Code), result.Error.Message, result.Error.Suggestion, result.Error.Details)
		}
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	if isJSONOutput() {
		outputJSON(result)
		return nil
	}

	data, _ := result.Data.(map[string]interface{})
	fmt.Println(ui.SectionHeader("Vault Statistics"))
	fmt.Println(ui.Bullet(ui.Muted.Render("Files: ") + ui.Bold.Render(fmt.Sprintf("%v", data["file_count"]))))
	fmt.Println(ui.Bullet(ui.Muted.Render("Objects: ") + ui.Bold.Render(fmt.Sprintf("%v", data["object_count"]))))
	fmt.Println(ui.Bullet(ui.Muted.Render("Traits: ") + ui.Bold.Render(fmt.Sprintf("%v", data["trait_count"]))))
	fmt.Println(ui.Bullet(ui.Muted.Render("References: ") + ui.Bold.Render(fmt.Sprintf("%v", data["ref_count"]))))

	return nil
}

func mapMaintSvcCode(code string) string {
	switch code {
	case string(maintsvc.CodeInvalidInput):
		return ErrInvalidInput
	case string(maintsvc.CodeDatabaseError):
		return ErrDatabaseError
	default:
		return ErrInternal
	}
}
