package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/maintsvc"
)

var untypedCmd = &cobra.Command{
	Use:   "untyped",
	Short: "List untyped pages",
	Long:  `Lists all files that are using the fallback 'page' type.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pages, err := maintsvc.Untyped(getVaultPath())
		if err != nil {
			return mapMaintSvcError(err)
		}

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"count": len(pages),
				"items": pages,
			}, &Meta{Count: len(pages)})
			return nil
		}

		if len(pages) == 0 {
			fmt.Println("All files have explicit types! ✓")
			return nil
		}

		fmt.Println("Untyped pages (using 'page' fallback):")
		fmt.Println()
		for _, page := range pages {
			fmt.Printf("  %s\n", page)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(untypedCmd)
}
