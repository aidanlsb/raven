package maintsvc

import (
	stdbuildinfo "debug/buildinfo"
	"errors"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/aidanlsb/raven/internal/buildinfo"
	"github.com/aidanlsb/raven/internal/index"
)

type Code string

const (
	CodeInvalidInput  Code = "INVALID_INPUT"
	CodeDatabaseError Code = "DATABASE_ERROR"
)

type Error struct {
	Code       Code
	Message    string
	Suggestion string
	Err        error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newError(code Code, message, suggestion string, err error) *Error {
	return &Error{Code: code, Message: message, Suggestion: suggestion, Err: err}
}

func AsError(err error) (*Error, bool) {
	var svcErr *Error
	if errors.As(err, &svcErr) {
		return svcErr, true
	}
	return nil, false
}

type StatsResult struct {
	FileCount   int `json:"file_count"`
	ObjectCount int `json:"object_count"`
	TraitCount  int `json:"trait_count"`
	RefCount    int `json:"ref_count"`
}

func Stats(vaultPath string) (*StatsResult, error) {
	if strings.TrimSpace(vaultPath) == "" {
		return nil, newError(CodeInvalidInput, "vault path is required", "", nil)
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		return nil, newError(CodeDatabaseError, "failed to open database", "Run 'rvn reindex' to rebuild the database", err)
	}
	defer db.Close()

	stats, err := db.Stats()
	if err != nil {
		return nil, newError(CodeDatabaseError, "failed to query stats", "", err)
	}

	return &StatsResult{
		FileCount:   stats.FileCount,
		ObjectCount: stats.ObjectCount,
		TraitCount:  stats.TraitCount,
		RefCount:    stats.RefCount,
	}, nil
}

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

func CurrentVersionInfo() VersionInfo {
	return CurrentVersionInfoWithReader(debug.ReadBuildInfo)
}

func CurrentVersionInfoWithReader(reader BuildInfoReader) VersionInfo {
	info := defaultVersionInfo()

	if reader == nil {
		applyLdflagsFallback(&info)
		return info
	}

	buildInfo, ok := reader()
	if !ok || buildInfo == nil {
		applyLdflagsFallback(&info)
		return info
	}

	info = versionInfoFromBuildInfo(buildInfo, info)
	applyLdflagsFallback(&info)

	return info
}

func CurrentVersionInfoFromExecutable(executablePath string) VersionInfo {
	info := defaultVersionInfo()
	if strings.TrimSpace(executablePath) == "" {
		return info
	}

	buildInfo, err := stdbuildinfo.ReadFile(executablePath)
	if err != nil || buildInfo == nil {
		return info
	}
	return versionInfoFromBuildInfo(buildInfo, info)
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

func applyLdflagsFallback(info *VersionInfo) {
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

func DefaultModulePath() string {
	return defaultModulePath
}
