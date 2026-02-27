package buildinfo

// These values are injected by GoReleaser via ldflags for release binaries.
// They default to empty for local/dev builds.
var (
	Version = ""
	Commit  = ""
	Date    = ""
)
