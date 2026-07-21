package docsync

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RefreshOptions controls a lazy docs-cache refresh.
type RefreshOptions struct {
	ConfigPath    string
	CLIVersion    string
	SourceBaseURL string
	HTTPClient    *http.Client
	Now           func() time.Time
}

// RefreshResult reports whether a lazy refresh replaced the docs cache.
type RefreshResult struct {
	Refreshed bool
}

var docsRefreshState = struct {
	sync.Mutex
	attempted map[string]struct{}
}{
	attempted: make(map[string]struct{}),
}

// RefreshIfStale refreshes an existing docs cache once per cache/version pair
// when its recorded CLI version is older than the running release.
//
// Development builds and other versions that are not clean vX.Y.Z release
// tags are skipped because there is no safe source tag to fetch.
func RefreshIfStale(opts RefreshOptions) (RefreshResult, error) {
	if _, err := OpenFS(opts.ConfigPath); err != nil {
		return RefreshResult{}, err
	}

	currentVersion, currentParts, ok := parseReleaseTag(opts.CLIVersion)
	if !ok {
		return RefreshResult{}, nil
	}

	key := filepath.Clean(DocsPath(opts.ConfigPath)) + "\x00" + currentVersion
	docsRefreshState.Lock()
	defer docsRefreshState.Unlock()

	if _, attempted := docsRefreshState.attempted[key]; attempted {
		return RefreshResult{}, nil
	}
	docsRefreshState.attempted[key] = struct{}{}

	manifest, err := ReadManifest(opts.ConfigPath)
	if err != nil {
		return RefreshResult{}, err
	}

	needsRefresh := manifest == nil || strings.TrimSpace(manifest.CLIVersion) == ""
	if !needsRefresh {
		_, cachedParts, valid := parseReleaseTag(manifest.CLIVersion)
		if !valid {
			return RefreshResult{}, nil
		}
		needsRefresh = compareReleaseParts(cachedParts, currentParts) < 0
	}
	if !needsRefresh {
		return RefreshResult{}, nil
	}

	if _, err := Fetch(FetchOptions{
		ConfigPath:    opts.ConfigPath,
		SourceBaseURL: opts.SourceBaseURL,
		Ref:           currentVersion,
		CLIVersion:    currentVersion,
		HTTPClient:    opts.HTTPClient,
		Now:           opts.Now,
	}); err != nil {
		return RefreshResult{}, fmt.Errorf("refresh docs for CLI %s: %w", currentVersion, err)
	}

	return RefreshResult{Refreshed: true}, nil
}

func parseReleaseTag(raw string) (string, [3]string, bool) {
	version := strings.TrimSpace(raw)
	if !strings.HasPrefix(version, "v") {
		return "", [3]string{}, false
	}

	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) != 3 {
		return "", [3]string{}, false
	}

	var parsed [3]string
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return "", [3]string{}, false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return "", [3]string{}, false
			}
		}
		parsed[i] = part
	}
	return version, parsed, true
}

func compareReleaseParts(left, right [3]string) int {
	for i := range left {
		switch {
		case len(left[i]) < len(right[i]):
			return -1
		case len(left[i]) > len(right[i]):
			return 1
		case left[i] < right[i]:
			return -1
		case left[i] > right[i]:
			return 1
		}
	}
	return 0
}
