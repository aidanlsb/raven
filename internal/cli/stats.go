package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/maintsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show index statistics",
	Long: `Displays statistics about the vault index.

Examples:
  rvn stats
  rvn stats --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()

		stats, err := maintsvc.Stats(vaultPath)
		if err != nil {
			return mapMaintSvcError(err)
		}
		elapsed := time.Since(start).Milliseconds()

		if isJSONOutput() {
			outputSuccess(StatsResult{
				FileCount:   stats.FileCount,
				ObjectCount: stats.ObjectCount,
				TraitCount:  stats.TraitCount,
				RefCount:    stats.RefCount,
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		// Human-readable output
		fmt.Println(ui.SectionHeader("Vault Statistics"))
		fmt.Println(ui.Bullet(ui.Muted.Render("Files: ") + ui.Bold.Render(fmt.Sprintf("%d", stats.FileCount))))
		fmt.Println(ui.Bullet(ui.Muted.Render("Objects: ") + ui.Bold.Render(fmt.Sprintf("%d", stats.ObjectCount))))
		fmt.Println(ui.Bullet(ui.Muted.Render("Traits: ") + ui.Bold.Render(fmt.Sprintf("%d", stats.TraitCount))))
		fmt.Println(ui.Bullet(ui.Muted.Render("References: ") + ui.Bold.Render(fmt.Sprintf("%d", stats.RefCount))))

		return nil
	},
}

func mapMaintSvcError(err error) error {
	svcErr, ok := maintsvc.AsError(err)
	if !ok {
		return handleError(ErrInternal, err, "")
	}

	switch svcErr.Code {
	case maintsvc.CodeInvalidInput:
		return handleErrorMsg(ErrInvalidInput, svcErr.Message, svcErr.Suggestion)
	case maintsvc.CodeDatabaseError:
		return handleError(ErrDatabaseError, svcErr, svcErr.Suggestion)
	default:
		return handleError(ErrInternal, svcErr, svcErr.Suggestion)
	}
}

func init() {
	rootCmd.AddCommand(statsCmd)
}
