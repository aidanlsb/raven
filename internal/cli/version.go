package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/maintsvc"
	"github.com/aidanlsb/raven/internal/versioninfo"
)

const defaultModulePath = "github.com/aidanlsb/raven" // Kept for test compatibility.

type versionInfo = maintsvc.VersionInfo

var versionCmd = newCanonicalLeafCommand("version", canonicalLeafOptions{
	RenderHuman: renderVersion,
})

func currentVersionInfo() versionInfo {
	return versioninfo.Current()
}

func renderVersion(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	fmt.Printf("rvn %s\n", stringValue(data["version"]))
	fmt.Printf("module: %s\n", stringValue(data["module_path"]))
	if commit := stringValue(data["commit"]); commit != "" {
		fmt.Printf("commit: %s\n", commit)
	}
	if commitTime := stringValue(data["commit_time"]); commitTime != "" {
		fmt.Printf("commit_time: %s\n", commitTime)
	}
	fmt.Printf("go: %s\n", stringValue(data["go_version"]))
	fmt.Printf("platform: %s/%s\n", stringValue(data["goos"]), stringValue(data["goarch"]))
	fmt.Printf("modified: %t\n", boolValue(data["modified"]))
	return nil
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
