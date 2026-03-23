package versioninfo

import (
	"runtime/debug"

	"github.com/aidanlsb/raven/internal/maintsvc"
)

var ReadBuildInfo maintsvc.BuildInfoReader = debug.ReadBuildInfo

func Current() maintsvc.VersionInfo {
	info := maintsvc.CurrentVersionInfoWithReader(ReadBuildInfo)
	if info.ModulePath == "" {
		info.ModulePath = maintsvc.DefaultModulePath()
	}
	return info
}
