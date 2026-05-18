package vault

import (
	"errors"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
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

// WalkAssetFiles walks configured asset roots and calls handler for each asset.
func WalkAssetFiles(vaultPath string, vaultCfg *config.VaultConfig, handler func(result AssetWalkResult) error) error {
	if vaultCfg == nil {
		vaultCfg = config.DefaultVaultConfig()
	}
	assetCfg := vaultCfg.GetAssetsConfig()
	root := assetCfg.Root
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

		if d.IsDir() {
			name := d.Name()
			if name == ".raven" || name == ".trash" || name == ".git" {
				return filepath.SkipDir
			}
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
		relativePath, _ := filepath.Rel(vaultPath, path)
		rel := paths.NormalizeVaultRelPath(relativePath)
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
	kind := vaultCfg.AssetKindForPath(relPath, mediaType)
	defaultPath := ""
	nonCanonical := false
	if kind != "" {
		if kindCfg := vaultCfg.GetAssetsConfig().Kinds[kind]; kindCfg != nil {
			defaultPath = kindCfg.DefaultPath
			nonCanonical = assetNonCanonical(relPath, vaultCfg.GetAssetRoot(), defaultPath)
		}
	}
	return &model.Asset{
		ID:           relPath,
		FilePath:     relPath,
		Kind:         kind,
		MediaType:    mediaType,
		SizeBytes:    info.Size(),
		FileMtime:    info.ModTime().Unix(),
		Extension:    ext,
		Filename:     filepath.Base(relPath),
		DefaultPath:  defaultPath,
		NonCanonical: nonCanonical,
	}
}

func assetNonCanonical(relPath, root, defaultPath string) bool {
	if defaultPath == "" {
		return false
	}
	root = paths.NormalizeDirRoot(root)
	defaultPath = paths.NormalizeDirRoot(defaultPath)
	if root == "" || defaultPath == "" || !strings.HasPrefix(relPath, root) {
		return false
	}
	withinRoot := strings.TrimPrefix(relPath, root)
	return !strings.HasPrefix(withinRoot, defaultPath)
}
