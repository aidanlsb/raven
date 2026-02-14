package cli

import (
	"encoding/json"
	"runtime"
	"runtime/debug"
	"testing"
)

func TestCurrentVersionInfoFromBuildInfo(t *testing.T) {
	prevRead := readBuildInfo
	t.Cleanup(func() {
		readBuildInfo = prevRead
	})

	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			GoVersion: "go1.23.4",
			Main: debug.Module{
				Path:    "github.com/aidanlsb/raven",
				Version: "v1.2.3",
			},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc123"},
				{Key: "vcs.time", Value: "2026-02-14T17:00:00Z"},
				{Key: "vcs.modified", Value: "true"},
				{Key: "GOOS", Value: "windows"},
				{Key: "GOARCH", Value: "amd64"},
			},
		}, true
	}

	info := currentVersionInfo()

	if info.Version != "v1.2.3" {
		t.Fatalf("Version = %q, want %q", info.Version, "v1.2.3")
	}
	if info.ModulePath != "github.com/aidanlsb/raven" {
		t.Fatalf("ModulePath = %q, want %q", info.ModulePath, "github.com/aidanlsb/raven")
	}
	if info.Commit != "abc123" {
		t.Fatalf("Commit = %q, want %q", info.Commit, "abc123")
	}
	if info.CommitTime != "2026-02-14T17:00:00Z" {
		t.Fatalf("CommitTime = %q, want %q", info.CommitTime, "2026-02-14T17:00:00Z")
	}
	if !info.Modified {
		t.Fatal("Modified = false, want true")
	}
	if info.GoVersion != "go1.23.4" {
		t.Fatalf("GoVersion = %q, want %q", info.GoVersion, "go1.23.4")
	}
	if info.GOOS != "windows" {
		t.Fatalf("GOOS = %q, want %q", info.GOOS, "windows")
	}
	if info.GOARCH != "amd64" {
		t.Fatalf("GOARCH = %q, want %q", info.GOARCH, "amd64")
	}
}

func TestCurrentVersionInfoFallbackWhenBuildInfoMissing(t *testing.T) {
	prevRead := readBuildInfo
	t.Cleanup(func() {
		readBuildInfo = prevRead
	})

	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return nil, false
	}

	info := currentVersionInfo()

	if info.Version != "devel" {
		t.Fatalf("Version = %q, want %q", info.Version, "devel")
	}
	if info.ModulePath != defaultModulePath {
		t.Fatalf("ModulePath = %q, want %q", info.ModulePath, defaultModulePath)
	}
	if info.GoVersion != runtime.Version() {
		t.Fatalf("GoVersion = %q, want runtime %q", info.GoVersion, runtime.Version())
	}
	if info.GOOS != runtime.GOOS {
		t.Fatalf("GOOS = %q, want runtime %q", info.GOOS, runtime.GOOS)
	}
	if info.GOARCH != runtime.GOARCH {
		t.Fatalf("GOARCH = %q, want runtime %q", info.GOARCH, runtime.GOARCH)
	}
}

func TestVersionCommandJSONOutput(t *testing.T) {
	prevRead := readBuildInfo
	prevJSON := jsonOutput
	t.Cleanup(func() {
		readBuildInfo = prevRead
		jsonOutput = prevJSON
	})

	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			GoVersion: "go1.23.4",
			Main: debug.Module{
				Path:    "github.com/aidanlsb/raven",
				Version: "(devel)",
			},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "deadbeef"},
				{Key: "vcs.time", Value: "2026-02-14T17:00:00Z"},
				{Key: "vcs.modified", Value: "false"},
				{Key: "GOOS", Value: "darwin"},
				{Key: "GOARCH", Value: "arm64"},
			},
		}, true
	}
	jsonOutput = true

	out := captureStdout(t, func() {
		if err := versionCmd.RunE(versionCmd, nil); err != nil {
			t.Fatalf("versionCmd.RunE: %v", err)
		}
	})

	var resp struct {
		OK   bool        `json:"ok"`
		Data versionInfo `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("expected JSON output, got parse error: %v; out=%s", err, out)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true; out=%s", out)
	}
	if resp.Data.Version != "devel" {
		t.Fatalf("Version = %q, want %q", resp.Data.Version, "devel")
	}
	if resp.Data.Commit != "deadbeef" {
		t.Fatalf("Commit = %q, want %q", resp.Data.Commit, "deadbeef")
	}
	if resp.Data.GOOS != "darwin" || resp.Data.GOARCH != "arm64" {
		t.Fatalf("platform = %s/%s, want darwin/arm64", resp.Data.GOOS, resp.Data.GOARCH)
	}
}
