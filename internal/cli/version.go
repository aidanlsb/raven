package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/versioninfo"
)

const defaultModulePath = "github.com/aidanlsb/raven" // Kept for test compatibility.

type versionInfo = versioninfo.VersionInfo

var versionCmd = newCanonicalLeafCommand("version", canonicalLeafOptions{
	RenderHuman: renderVersion,
})

func currentVersionInfo() versionInfo {
	return versioninfo.Current()
}

func renderVersion(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[versioninfo.VersionInfo](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	fmt.Printf("rvn %s\n", data.Version)
	fmt.Printf("module: %s\n", data.ModulePath)
	if data.Commit != "" {
		fmt.Printf("commit: %s\n", data.Commit)
	}
	if data.CommitTime != "" {
		fmt.Printf("commit_time: %s\n", data.CommitTime)
	}
	fmt.Printf("go: %s\n", data.GoVersion)
	fmt.Printf("platform: %s/%s\n", data.GOOS, data.GOARCH)
	fmt.Printf("modified: %t\n", data.Modified)
	return nil
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
