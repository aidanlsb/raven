package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var vaultStatsCmd = newCanonicalLeafCommand("vault_stats", canonicalLeafOptions{
	RenderHuman: renderVaultStats,
})

func renderVaultStats(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[readsvc.StatsPayload](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	fmt.Println(ui.SectionHeader("Vault Statistics"))
	fmt.Println(ui.Bullet(ui.Muted.Render("Files: ") + ui.Bold.Render(fmt.Sprintf("%d", data.FileCount))))
	fmt.Println(ui.Bullet(ui.Muted.Render("Objects: ") + ui.Bold.Render(fmt.Sprintf("%d", data.ObjectCount))))
	fmt.Println(ui.Bullet(ui.Muted.Render("Traits: ") + ui.Bold.Render(fmt.Sprintf("%d", data.TraitCount))))
	fmt.Println(ui.Bullet(ui.Muted.Render("References: ") + ui.Bold.Render(fmt.Sprintf("%d", data.RefCount))))
	return nil
}
