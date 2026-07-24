package versioninfo

import (
	"runtime/debug"
	"testing"

	"github.com/aidanlsb/raven/internal/buildinfo"
)

func TestCurrentVersionInfoWithReader(t *testing.T) {
	t.Parallel()
	info := CurrentVersionInfoWithReader(func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			GoVersion: "go1.24.2",
			Main: debug.Module{
				Path:    "example.com/custom",
				Version: "v1.2.3",
			},
			Settings: []debug.BuildSetting{
				{Key: "GOOS", Value: "linux"},
				{Key: "GOARCH", Value: "arm64"},
				{Key: "vcs.revision", Value: "abc123"},
				{Key: "vcs.time", Value: "2026-01-01T00:00:00Z"},
				{Key: "vcs.modified", Value: "true"},
			},
		}, true
	})

	if info.ModulePath != "example.com/custom" {
		t.Fatalf("module path = %q, want %q", info.ModulePath, "example.com/custom")
	}
	if info.Version != "v1.2.3" {
		t.Fatalf("version = %q, want %q", info.Version, "v1.2.3")
	}
	if info.GOOS != "linux" || info.GOARCH != "arm64" {
		t.Fatalf("unexpected target platform: %s/%s", info.GOOS, info.GOARCH)
	}
	if info.Commit != "abc123" || info.CommitTime == "" || !info.Modified {
		t.Fatalf("unexpected vcs info: %#v", info)
	}
}

func TestCurrentVersionInfoPrefersInjectedReleaseMetadata(t *testing.T) {
	prevVersion := buildinfo.Version
	prevCommit := buildinfo.Commit
	prevDate := buildinfo.Date
	t.Cleanup(func() {
		buildinfo.Version = prevVersion
		buildinfo.Commit = prevCommit
		buildinfo.Date = prevDate
	})

	buildinfo.Version = "v1.2.4"
	buildinfo.Commit = "releasecommit"
	buildinfo.Date = "2026-05-18T01:04:20Z"

	info := CurrentVersionInfoWithReader(func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			GoVersion: "go1.25.0",
			Main: debug.Module{
				Path:    "example.com/custom",
				Version: "v1.2.4+dirty",
			},
			Settings: []debug.BuildSetting{
				{Key: "GOOS", Value: "darwin"},
				{Key: "GOARCH", Value: "arm64"},
				{Key: "vcs.revision", Value: "dirtycommit"},
				{Key: "vcs.time", Value: "2026-05-18T01:03:21Z"},
				{Key: "vcs.modified", Value: "true"},
			},
		}, true
	})

	if info.Version != "v1.2.4" {
		t.Fatalf("version = %q, want %q", info.Version, "v1.2.4")
	}
	if info.Commit != "releasecommit" {
		t.Fatalf("commit = %q, want %q", info.Commit, "releasecommit")
	}
	if info.CommitTime != "2026-05-18T01:04:20Z" {
		t.Fatalf("commit time = %q, want release date", info.CommitTime)
	}
	if info.Modified {
		t.Fatal("modified = true, want false for injected release metadata")
	}
}
