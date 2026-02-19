package docsync

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	StoreRelPath        = ".raven/docs"
	ManifestFilename    = "manifest.json"
	DocsIndexFilename   = "index.yaml"
	DefaultSourceBase   = "https://codeload.github.com/aidanlsb/raven/tar.gz"
	DefaultSourceRef    = "main"
	manifestSchemaV1    = 1
	fetchRequestTimeout = 60 * time.Second
)

var ErrDocsNotFetched = errors.New("docs are not fetched for this vault")

// Manifest records the source and timing of the last docs sync.
type Manifest struct {
	SchemaVersion int    `json:"schema_version"`
	SourceBaseURL string `json:"source_base_url"`
	Ref           string `json:"ref"`
	ArchiveURL    string `json:"archive_url"`
	FetchedAt     string `json:"fetched_at"`
	CLIVersion    string `json:"cli_version,omitempty"`
}

// FetchOptions controls docs sync behavior.
type FetchOptions struct {
	VaultPath     string
	SourceBaseURL string
	Ref           string
	CLIVersion    string
	HTTPClient    *http.Client
	Now           func() time.Time
}

// FetchResult summarizes a docs sync.
type FetchResult struct {
	DocsPath  string
	FileCount int
	ByteCount int64
	Manifest  Manifest
}

// DocsPath returns the absolute docs cache path for a vault.
func DocsPath(vaultPath string) string {
	return filepath.Join(vaultPath, filepath.FromSlash(StoreRelPath))
}

// OpenFS opens the vault-local docs cache for read operations.
func OpenFS(vaultPath string) (fs.FS, error) {
	docsPath := DocsPath(vaultPath)
	indexPath := filepath.Join(docsPath, DocsIndexFilename)
	if _, err := os.Stat(indexPath); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrDocsNotFetched
		}
		return nil, fmt.Errorf("check docs index: %w", err)
	}
	return os.DirFS(docsPath), nil
}

// ReadManifest returns the docs manifest if present.
func ReadManifest(vaultPath string) (*Manifest, error) {
	manifestPath := filepath.Join(DocsPath(vaultPath), ManifestFilename)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read docs manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse docs manifest: %w", err)
	}
	return &manifest, nil
}

// Fetch downloads and installs docs into .raven/docs for the given vault.
func Fetch(opts FetchOptions) (*FetchResult, error) {
	vaultPath := strings.TrimSpace(opts.VaultPath)
	if vaultPath == "" {
		return nil, fmt.Errorf("vault path is required")
	}

	ref := strings.TrimSpace(opts.Ref)
	if ref == "" {
		ref = DefaultSourceRef
	}

	sourceBase := strings.TrimRight(strings.TrimSpace(opts.SourceBaseURL), "/")
	if sourceBase == "" {
		sourceBase = DefaultSourceBase
	}
	archiveURL := sourceBase + "/" + ref

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: fetchRequestTimeout}
	}

	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	req, err := http.NewRequest(http.MethodGet, archiveURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build docs request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download docs archive: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download docs archive: status %d", resp.StatusCode)
	}

	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read docs archive: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	ravenDir := filepath.Join(vaultPath, ".raven")
	if err := os.MkdirAll(ravenDir, 0o755); err != nil {
		return nil, fmt.Errorf("create .raven directory: %w", err)
	}

	stagingDir := filepath.Join(ravenDir, fmt.Sprintf(".docs-staging-%d", nowFn().UnixNano()))
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return nil, fmt.Errorf("create docs staging directory: %w", err)
	}
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	fileCount := 0
	var byteCount int64
	indexFound := false

	for {
		hdr, err := tarReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("read docs archive entries: %w", err)
		}

		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		relPath, ok := docsRelativePathFromArchive(hdr.Name)
		if !ok {
			continue
		}

		destPath := filepath.Join(stagingDir, filepath.FromSlash(relPath))
		if err := ensureWithin(stagingDir, destPath); err != nil {
			return nil, fmt.Errorf("invalid docs archive path %q: %w", relPath, err)
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return nil, fmt.Errorf("create docs file directory: %w", err)
		}

		f, err := os.Create(destPath)
		if err != nil {
			return nil, fmt.Errorf("create docs file %q: %w", relPath, err)
		}

		written, copyErr := io.Copy(f, tarReader)
		closeErr := f.Close()
		if copyErr != nil {
			return nil, fmt.Errorf("write docs file %q: %w", relPath, copyErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close docs file %q: %w", relPath, closeErr)
		}

		fileCount++
		byteCount += written
		if relPath == DocsIndexFilename {
			indexFound = true
		}
	}

	if !indexFound {
		return nil, fmt.Errorf("downloaded docs archive does not contain docs/%s", DocsIndexFilename)
	}

	manifest := Manifest{
		SchemaVersion: manifestSchemaV1,
		SourceBaseURL: sourceBase,
		Ref:           ref,
		ArchiveURL:    archiveURL,
		FetchedAt:     nowFn().UTC().Format(time.RFC3339),
		CLIVersion:    strings.TrimSpace(opts.CLIVersion),
	}

	manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("serialize docs manifest: %w", err)
	}
	manifestRaw = append(manifestRaw, '\n')
	if err := os.WriteFile(filepath.Join(stagingDir, ManifestFilename), manifestRaw, 0o644); err != nil {
		return nil, fmt.Errorf("write docs manifest: %w", err)
	}

	targetDir := DocsPath(vaultPath)
	backupDir := targetDir + ".backup"
	_ = os.RemoveAll(backupDir)

	if _, err := os.Stat(targetDir); err == nil {
		if err := os.Rename(targetDir, backupDir); err != nil {
			return nil, fmt.Errorf("replace docs cache: %w", err)
		}
	}

	if err := os.Rename(stagingDir, targetDir); err != nil {
		if _, backupErr := os.Stat(backupDir); backupErr == nil {
			_ = os.Rename(backupDir, targetDir)
		}
		return nil, fmt.Errorf("activate docs cache: %w", err)
	}

	_ = os.RemoveAll(backupDir)
	cleanupStaging = false

	return &FetchResult{
		DocsPath:  targetDir,
		FileCount: fileCount,
		ByteCount: byteCount,
		Manifest:  manifest,
	}, nil
}

func docsRelativePathFromArchive(name string) (string, bool) {
	clean := path.Clean(strings.TrimPrefix(strings.TrimSpace(name), "./"))
	if clean == "." || strings.HasPrefix(clean, "../") {
		return "", false
	}

	parts := strings.Split(clean, "/")
	if len(parts) < 3 {
		return "", false
	}
	if parts[1] != "docs" {
		return "", false
	}

	rel := path.Clean(strings.Join(parts[2:], "/"))
	if rel == "." || rel == "" || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

func ensureWithin(basePath, candidatePath string) error {
	baseAbs, err := filepath.Abs(basePath)
	if err != nil {
		return err
	}
	candidateAbs, err := filepath.Abs(candidatePath)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(baseAbs, candidateAbs)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes base directory")
	}
	return nil
}
