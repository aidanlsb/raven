package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/maintsvc"
)

const defaultModulePath = "github.com/aidanlsb/raven" // Kept for test compatibility.

type versionInfo = maintsvc.VersionInfo

var readBuildInfo maintsvc.BuildInfoReader = debug.ReadBuildInfo

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show Raven version and build information",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		info := currentVersionInfo()

		if isJSONOutput() {
			outputSuccess(info, nil)
			return nil
		}

		fmt.Printf("rvn %s\n", info.Version)
		fmt.Printf("module: %s\n", info.ModulePath)
		if info.Commit != "" {
			fmt.Printf("commit: %s\n", info.Commit)
		}
		if info.CommitTime != "" {
			fmt.Printf("commit_time: %s\n", info.CommitTime)
		}
		fmt.Printf("go: %s\n", info.GoVersion)
		fmt.Printf("platform: %s/%s\n", info.GOOS, info.GOARCH)
		fmt.Printf("modified: %t\n", info.Modified)

		return nil
	},
}

func currentVersionInfo() versionInfo {
	info := maintsvc.CurrentVersionInfoWithReader(readBuildInfo)
	if info.ModulePath == "" {
		info.ModulePath = defaultModulePath
	}
	return info
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
