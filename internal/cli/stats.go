package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

var vaultStatsCmd = newCanonicalLeafCommand("vault_stats", canonicalLeafOptions{
	RenderHuman: renderVaultStats,
})

func renderVaultStats(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Println(ui.SectionHeader("Vault Statistics"))
	fmt.Println(ui.Bullet(ui.Muted.Render("Files: ") + ui.Bold.Render(fmt.Sprintf("%v", data["file_count"]))))
	fmt.Println(ui.Bullet(ui.Muted.Render("Objects: ") + ui.Bold.Render(fmt.Sprintf("%v", data["object_count"]))))
	fmt.Println(ui.Bullet(ui.Muted.Render("Traits: ") + ui.Bold.Render(fmt.Sprintf("%v", data["trait_count"]))))
	fmt.Println(ui.Bullet(ui.Muted.Render("References: ") + ui.Bold.Render(fmt.Sprintf("%v", data["ref_count"]))))
	return nil
}
