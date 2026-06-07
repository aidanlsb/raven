package vault

import (
	"errors"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/paths"
)

// AssetWalkResult contains the result of processing an asset file.
type AssetWalkResult struct {
	Path         string
	RelativePath string
	Asset        *model.Asset
	FileMtime    int64
	Error        error
}

// AssetWalkOptions contains options for walking asset files.
type AssetWalkOptions struct {
	ExcludeMatcher *ravenignore.Matcher
}

// WalkAssetFilesWithOptions walks configured asset roots with custom options.
func WalkAssetFilesWithOptions(vaultPath string, vaultCfg *config.VaultConfig, opts *AssetWalkOptions, handler func(result AssetWalkResult) error) error {
	if vaultCfg == nil {
		vaultCfg = config.DefaultVaultConfig()
	}
	root := vaultCfg.GetAssetRoot()
	if root == "" {
		return nil
	}

	rootPath := filepath.Join(vaultPath, root)
	if info, err := os.Stat(rootPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return handler(AssetWalkResult{Path: rootPath, RelativePath: root, Error: err})
	} else if !info.IsDir() {
		return nil
	}

	return filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			relativePath, _ := filepath.Rel(vaultPath, path)
			return handler(AssetWalkResult{
				Path:         path,
				RelativePath: filepath.ToSlash(relativePath),
				Error:        err,
			})
		}

		relativePath, _ := filepath.Rel(vaultPath, path)
		rel := paths.NormalizeVaultRelPath(relativePath)

		if d.IsDir() {
			name := d.Name()
			if name == ".raven" || name == ".trash" || name == ".git" {
				return filepath.SkipDir
			}
			if rel != "." && opts != nil && opts.ExcludeMatcher.Match(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}

		if opts != nil && opts.ExcludeMatcher.Match(rel, false) {
			return nil
		}

		if strings.HasSuffix(strings.ToLower(path), paths.MDExtension) {
			return nil
		}
		if err := paths.ValidateWithinVault(vaultPath, path); err != nil {
			if errors.Is(err, paths.ErrPathOutsideVault) {
				return nil
			}
			relativePath, _ := filepath.Rel(vaultPath, path)
			return handler(AssetWalkResult{
				Path:         path,
				RelativePath: filepath.ToSlash(relativePath),
				Error:        err,
			})
		}

		info, err := d.Info()
		if err != nil {
			relativePath, _ := filepath.Rel(vaultPath, path)
			return handler(AssetWalkResult{
				Path:         path,
				RelativePath: filepath.ToSlash(relativePath),
				Error:        err,
			})
		}
		asset := BuildAsset(rel, info, vaultCfg)
		return handler(AssetWalkResult{
			Path:         path,
			RelativePath: rel,
			Asset:        asset,
			FileMtime:    info.ModTime().Unix(),
		})
	})
}

// BuildAsset constructs an asset model from filesystem metadata.
func BuildAsset(relPath string, info fs.FileInfo, vaultCfg *config.VaultConfig) *model.Asset {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(relPath)), ".")
	mediaType := ""
	if ext != "" {
		mediaType = mime.TypeByExtension("." + ext)
	}
	return &model.Asset{
		ID:        relPath,
		FilePath:  relPath,
		MediaType: mediaType,
		SizeBytes: info.Size(),
		FileMtime: info.ModTime().Unix(),
		Extension: ext,
		Filename:  filepath.Base(relPath),
	}
}
