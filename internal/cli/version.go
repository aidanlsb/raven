package cli

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/buildinfo"
)

const defaultModulePath = "github.com/aidanlsb/raven"

type versionInfo struct {
	Version    string `json:"version"`
	ModulePath string `json:"module_path"`
	Commit     string `json:"commit,omitempty"`
	CommitTime string `json:"commit_time,omitempty"`
	Modified   bool   `json:"modified"`
	GoVersion  string `json:"go_version"`
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
}

var readBuildInfo = debug.ReadBuildInfo

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
	info := versionInfo{
		Version:    "devel",
		ModulePath: defaultModulePath,
		GoVersion:  runtime.Version(),
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
	}

	buildInfo, ok := readBuildInfo()
	if !ok || buildInfo == nil {
		applyLdflagsFallback(&info)
		return info
	}

	if buildInfo.Main.Path != "" {
		info.ModulePath = buildInfo.Main.Path
	}
	info.Version = normalizeVersion(buildInfo.Main.Version)

	if buildInfo.GoVersion != "" {
		info.GoVersion = buildInfo.GoVersion
	}

	if val := buildSetting(buildInfo, "GOOS"); val != "" {
		info.GOOS = val
	}
	if val := buildSetting(buildInfo, "GOARCH"); val != "" {
		info.GOARCH = val
	}

	info.Commit = buildSetting(buildInfo, "vcs.revision")
	info.CommitTime = buildSetting(buildInfo, "vcs.time")
	info.Modified = strings.EqualFold(buildSetting(buildInfo, "vcs.modified"), "true")
	applyLdflagsFallback(&info)

	return info
}

func normalizeVersion(version string) string {
	if version == "" || version == "(devel)" {
		return "devel"
	}
	return version
}

func buildSetting(info *debug.BuildInfo, key string) string {
	if info == nil {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == key {
			return setting.Value
		}
	}
	return ""
}

func applyLdflagsFallback(info *versionInfo) {
	if info == nil {
		return
	}

	if info.Version == "devel" && buildinfo.Version != "" {
		info.Version = normalizeVersion(buildinfo.Version)
	}
	if info.Commit == "" && buildinfo.Commit != "" {
		info.Commit = buildinfo.Commit
	}
	if info.CommitTime == "" && buildinfo.Date != "" {
		info.CommitTime = buildinfo.Date
	}
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
