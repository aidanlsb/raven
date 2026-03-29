package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/maintsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var vaultStatsCmd = newCanonicalLeafCommand("vault_stats", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	HandleError: handleCanonicalMaintSvcFailure,
	RenderHuman: renderVaultStats,
})

func handleCanonicalMaintSvcFailure(result commandexec.Result) error {
	if result.Error == nil {
		return nil
	}
	return handleErrorWithDetails(mapMaintSvcCode(result.Error.Code), result.Error.Message, result.Error.Suggestion, result.Error.Details)
}

func renderVaultStats(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
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
