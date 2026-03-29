package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
)

func outputVaultPath(path string) error {
	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"path": path,
		}, nil)
		return nil
	}
	fmt.Println(path)
	return nil
}

var vaultPathCmd = newCanonicalLeafCommand("vault_path", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultPath,
})

func renderVaultPath(_ *cobra.Command, result commandexec.Result) error {
	return outputVaultPath(stringValue(canonicalDataMap(result)["path"]))
}
