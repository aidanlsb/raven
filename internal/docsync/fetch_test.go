package docsync

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFetchInstallsDocsFromArchive(t *testing.T) {
	t.Parallel()

	archive := buildArchive(t, map[string]string{
		"raven-main/docs/index.yaml":                   "sections:\n  guide:\n    topics:\n      getting-started:\n        path: getting-started.md\n",
		"raven-main/docs/guide/getting-started.md":     "# Getting Started\n",
		"raven-main/docs/reference/query-language.md":  "# Query\n",
		"raven-main/internal/mcp/agent-guide/index.md": "ignored",
	})

	vaultPath := t.TempDir()
	result, err := Fetch(FetchOptions{
		VaultPath:     vaultPath,
		SourceBaseURL: "https://example.invalid/docs",
		Ref:           "main",
		CLIVersion:    "v0.0.1",
		Now: func() time.Time {
			return time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
		},
		HTTPClient: &http.Client{Transport: fakeTransport{
			StatusCode: http.StatusOK,
			Bodies: map[string][]byte{
				"/docs/main": archive,
			},
		}},
	})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if result.FileCount != 3 {
		t.Fatalf("FileCount = %d, want 3", result.FileCount)
	}
	if result.Manifest.Ref != "main" {
		t.Fatalf("manifest ref = %q, want main", result.Manifest.Ref)
	}

	if _, err := os.Stat(filepath.Join(vaultPath, StoreRelPath, DocsIndexFilename)); err != nil {
		t.Fatalf("expected docs index to exist: %v", err)
	}

	manifest, err := ReadManifest(vaultPath)
	if err != nil {
		t.Fatalf("ReadManifest() error = %v", err)
	}
	if manifest == nil || manifest.CLIVersion != "v0.0.1" {
		t.Fatalf("manifest cli_version = %v, want v0.0.1", manifest)
	}

	if _, err := OpenFS(vaultPath); err != nil {
		t.Fatalf("OpenFS() error = %v", err)
	}
}

func TestFetchReplacesExistingDocsCache(t *testing.T) {
	t.Parallel()

	archive := buildArchive(t, map[string]string{
		"raven-main/docs/index.yaml":     "sections:\n  guide:\n    topics:\n      start:\n        path: start.md\n",
		"raven-main/docs/guide/start.md": "# Start\n",
	})

	vaultPath := t.TempDir()
	oldPath := filepath.Join(vaultPath, StoreRelPath)
	if err := os.MkdirAll(oldPath, 0o755); err != nil {
		t.Fatalf("mkdir old docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldPath, "stale.md"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale docs: %v", err)
	}

	if _, err := Fetch(FetchOptions{
		VaultPath:     vaultPath,
		SourceBaseURL: "https://example.invalid/archive",
		Ref:           "main",
		HTTPClient: &http.Client{Transport: fakeTransport{
			StatusCode: http.StatusOK,
			Bodies: map[string][]byte{
				"/archive/main": archive,
			},
		}},
	}); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(vaultPath, StoreRelPath, "stale.md")); !os.IsNotExist(err) {
		t.Fatalf("expected stale docs to be removed, err=%v", err)
	}
}

func TestOpenFSMissingDocs(t *testing.T) {
	t.Parallel()

	_, err := OpenFS(t.TempDir())
	if !errors.Is(err, ErrDocsNotFetched) {
		t.Fatalf("OpenFS() error = %v, want ErrDocsNotFetched", err)
	}
}

func buildArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	buf := &bytes.Buffer{}
	gz := gzip.NewWriter(buf)
	tw := tar.NewWriter(gz)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%q): %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%q): %v", name, err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	return buf.Bytes()
}

type fakeTransport struct {
	StatusCode int
	Bodies     map[string][]byte
}

func (t fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, ok := t.Bodies[req.URL.Path]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Header:     make(http.Header),
		}, nil
	}
	status := t.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}
