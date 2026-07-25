package versioninfo

import (
	stdbuildinfo "debug/buildinfo"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/aidanlsb/raven/internal/buildinfo"
)

const defaultModulePath = "github.com/aidanlsb/raven"

type VersionInfo struct {
	Version    string `json:"version"`
	ModulePath string `json:"module_path"`
	Commit     string `json:"commit,omitempty"`
	CommitTime string `json:"commit_time,omitempty"`
	Modified   bool   `json:"modified"`
	GoVersion  string `json:"go_version"`
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
}

type BuildInfoReader func() (*debug.BuildInfo, bool)

var ReadBuildInfo BuildInfoReader = debug.ReadBuildInfo

func Current() VersionInfo {
	info := CurrentVersionInfoWithReader(ReadBuildInfo)
	if info.ModulePath == "" {
		info.ModulePath = DefaultModulePath()
	}
	return info
}

func CurrentVersionInfo() VersionInfo {
	return CurrentVersionInfoWithReader(debug.ReadBuildInfo)
}

func CurrentVersionInfoWithReader(reader BuildInfoReader) VersionInfo {
	info := defaultVersionInfo()

	if reader == nil {
		applyLdflagsMetadata(&info)
		return info
	}

	buildInfo, ok := reader()
	if !ok || buildInfo == nil {
		applyLdflagsMetadata(&info)
		return info
	}

	info = versionInfoFromBuildInfo(buildInfo, info)
	applyLdflagsMetadata(&info)

	return info
}

func CurrentVersionInfoFromExecutable(executablePath string) VersionInfo {
	info := defaultVersionInfo()
	if strings.TrimSpace(executablePath) != "" {
		buildInfo, err := stdbuildinfo.ReadFile(executablePath)
		if err == nil && buildInfo != nil {
			info = versionInfoFromBuildInfo(buildInfo, info)
		}
	}
	applyLdflagsMetadata(&info)
	return info
}

func defaultVersionInfo() VersionInfo {
	return VersionInfo{
		Version:    "devel",
		ModulePath: defaultModulePath,
		GoVersion:  runtime.Version(),
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
	}
}

func versionInfoFromBuildInfo(buildInfo *debug.BuildInfo, info VersionInfo) VersionInfo {
	if buildInfo == nil {
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

func applyLdflagsMetadata(info *VersionInfo) {
	if info == nil {
		return
	}

	explicitReleaseMetadata := buildinfo.Version != "" || buildinfo.Commit != ""
	if buildinfo.Version != "" {
		info.Version = normalizeVersion(buildinfo.Version)
	}
	if buildinfo.Commit != "" {
		info.Commit = buildinfo.Commit
	}
	if buildinfo.Date != "" {
		info.CommitTime = buildinfo.Date
	}
	if explicitReleaseMetadata {
		// Release ldflags are the authority. Go's VCS stamping can report a
		// dirty checkout if release hooks touch files before compilation.
		info.Modified = false
	}
}

func DefaultModulePath() string {
	return defaultModulePath
}
