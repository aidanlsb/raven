// Package assetsvc provides mutations for vault-local non-Markdown assets.
package assetsvc

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
)

type ImportMode string

const (
	ModeCopy ImportMode = "copy"
	ModeMove ImportMode = "move"
)

type ImportRequest struct {
	VaultPath   string
	VaultConfig *config.VaultConfig
	Source      string
	Destination string
	Move        bool
	Force       bool
	Preview     bool
}

type ImportResult struct {
	SourcePath     string
	DestinationAbs string
	DestinationRel string
	Mode           ImportMode
	Asset          *model.Asset
	ChangeSet      mutation.ChangeSet

	sourceInfo os.FileInfo
}

// Import validates and copies an external file into the configured asset root.
// Move mode deliberately leaves source removal to FinalizeMove so callers can
// first hand the destination to Raven's shared post-mutation index path.
func Import(req ImportRequest) (*ImportResult, error) {
	plan, err := planImport(req)
	if err != nil {
		return nil, err
	}
	if req.Preview {
		return plan, nil
	}

	if err := os.MkdirAll(filepath.Dir(plan.DestinationAbs), 0o755); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create asset destination directory", err).
			WithDetails(map[string]any{"path": plan.DestinationRel})
	}
	if err := paths.ValidateWithinVault(req.VaultPath, plan.DestinationAbs); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileOutsideVault, "asset destination resolves outside the vault", err).
			WithSuggestion("Choose a destination under the configured asset root").
			WithDetails(map[string]any{"path": plan.DestinationRel})
	}

	sourceInfo, err := copyFileAtomic(plan.SourcePath, plan.DestinationAbs, req.Force)
	if err != nil {
		return nil, err
	}
	plan.sourceInfo = sourceInfo

	destinationInfo, err := os.Stat(plan.DestinationAbs)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to inspect imported asset", err).
			WithDetails(map[string]any{"path": plan.DestinationRel})
	}
	plan.Asset = vault.BuildAsset(plan.DestinationRel, destinationInfo, req.VaultConfig)
	plan.ChangeSet.AddChanged(plan.DestinationRel)
	return plan, nil
}

// FinalizeMove removes the external source after the destination has been
// written and handed to the shared index coordinator.
func FinalizeMove(result *ImportResult) error {
	if result == nil || result.Mode != ModeMove {
		return nil
	}
	if result.sourceInfo == nil {
		return svcerr.New(codes.ErrFileWrite, "cannot remove asset source before import completes")
	}

	current, err := os.Stat(result.SourcePath)
	if err != nil {
		return svcerr.Wrap(codes.ErrFileWrite, "failed to verify asset source before removal", err).
			WithDetails(map[string]any{
				"source":      result.SourcePath,
				"destination": result.DestinationRel,
			})
	}
	if !os.SameFile(result.sourceInfo, current) ||
		result.sourceInfo.Size() != current.Size() ||
		!result.sourceInfo.ModTime().Equal(current.ModTime()) {
		return svcerr.New(codes.ErrFileWrite, "asset source changed during import; source was not removed").
			WithSuggestion("Verify the imported asset, then retry or remove the source manually").
			WithDetails(map[string]any{
				"source":      result.SourcePath,
				"destination": result.DestinationRel,
			})
	}
	if err := os.Remove(result.SourcePath); err != nil {
		return svcerr.Wrap(codes.ErrFileWrite, "asset was imported but the source could not be removed", err).
			WithSuggestion("The imported asset is intact; remove the source after verifying it").
			WithDetails(map[string]any{
				"source":      result.SourcePath,
				"destination": result.DestinationRel,
			})
	}
	return nil
}

func planImport(req ImportRequest) (*ImportResult, error) {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "vault path is required")
	}
	if req.VaultConfig == nil {
		return nil, svcerr.New(codes.ErrConfigInvalid, "vault config is required").
			WithSuggestion("Fix raven.yaml and try again")
	}

	sourcePath, err := expandSourcePath(req.Source)
	if err != nil {
		return nil, err
	}
	sourceInfo, err := os.Stat(sourcePath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, svcerr.Wrap(codes.ErrFileNotFound, "asset source does not exist", err).
			WithDetails(map[string]any{"source": sourcePath})
	case err != nil:
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to inspect asset source", err).
			WithDetails(map[string]any{"source": sourcePath})
	case !sourceInfo.Mode().IsRegular():
		return nil, svcerr.New(codes.ErrInvalidInput, "asset source must be a regular file").
			WithDetails(map[string]any{"source": sourcePath})
	}
	if strings.EqualFold(filepath.Ext(sourcePath), paths.MDExtension) {
		return nil, svcerr.New(codes.ErrInvalidInput, "Markdown files cannot be imported as assets").
			WithSuggestion("Use Raven's content import or object write commands for Markdown files")
	}
	if err := paths.ValidateWithinVault(vaultPath, sourcePath); err == nil {
		return nil, svcerr.New(codes.ErrFileOutsideVault, "asset import source must be outside the vault").
			WithSuggestion("Use 'rvn move' for files already inside the vault").
			WithDetails(map[string]any{"source": sourcePath})
	} else if !errors.Is(err, paths.ErrPathOutsideVault) {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to validate asset source", err).
			WithDetails(map[string]any{"source": sourcePath})
	}

	destinationRel, destinationAbs, err := resolveDestination(
		vaultPath,
		req.VaultConfig,
		req.Destination,
		filepath.Base(sourcePath),
	)
	if err != nil {
		return nil, err
	}
	if err := mutationguard.ValidateContentMutationRelPath(req.VaultConfig, destinationRel); err != nil {
		return nil, err
	}

	destinationInfo, statErr := os.Stat(destinationAbs)
	switch {
	case statErr == nil && destinationInfo.IsDir():
		return nil, svcerr.New(codes.ErrInvalidInput, "asset destination resolves to a directory").
			WithSuggestion("Use a destination file path or a directory path ending with /").
			WithDetails(map[string]any{"path": destinationRel})
	case statErr == nil && !req.Force:
		return nil, svcerr.New(codes.ErrFileExists, "asset destination already exists").
			WithSuggestion("Choose a different destination or pass --force to overwrite it").
			WithDetails(map[string]any{"path": destinationRel})
	case statErr != nil && !errors.Is(statErr, os.ErrNotExist):
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to inspect asset destination", statErr).
			WithDetails(map[string]any{"path": destinationRel})
	}

	mode := ModeCopy
	if req.Move {
		mode = ModeMove
	}
	return &ImportResult{
		SourcePath:     sourcePath,
		DestinationAbs: destinationAbs,
		DestinationRel: destinationRel,
		Mode:           mode,
		Asset:          vault.BuildAsset(destinationRel, sourceInfo, req.VaultConfig),
		ChangeSet:      mutation.NewChangeSet(),
	}, nil
}

func expandSourcePath(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", svcerr.New(codes.ErrInvalidInput, "asset source is required")
	}
	if source == "~" || strings.HasPrefix(source, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", svcerr.Wrap(codes.ErrInvalidInput, "failed to expand asset source home directory", err)
		}
		source = filepath.Join(home, strings.TrimPrefix(source, "~/"))
	} else if strings.HasPrefix(source, "~") {
		return "", svcerr.New(codes.ErrInvalidInput, "asset source supports only '~' or '~/' home expansion").
			WithSuggestion("Use an absolute path or a path beginning with ~/")
	}
	if !filepath.IsAbs(source) {
		return "", svcerr.New(codes.ErrInvalidInput, "asset source must be an absolute or ~-relative path").
			WithSuggestion("Use a host filesystem path such as /tmp/file.pdf or ~/Downloads/file.pdf")
	}
	return filepath.Clean(source), nil
}

func resolveDestination(vaultPath string, vaultCfg *config.VaultConfig, destination, sourceBase string) (string, string, error) {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return "", "", svcerr.New(codes.ErrInvalidInput, "asset destination is required")
	}
	if isAbsoluteOrRooted(destination) || strings.HasPrefix(destination, "~") {
		return "", "", svcerr.New(codes.ErrInvalidInput, "asset destination must be vault-relative").
			WithSuggestion("Use a path under the configured asset root, such as assets/pdfs/file.pdf")
	}

	directoryHint := strings.HasSuffix(destination, "/") || strings.HasSuffix(destination, "\\")
	candidateRel := paths.NormalizeVaultRelPath(destination)
	if !paths.IsValidVaultRelPath(candidateRel) {
		return "", "", svcerr.New(codes.ErrFileOutsideVault, "asset destination escapes the vault").
			WithSuggestion("Use a vault-relative path under the configured asset root")
	}
	candidateAbs := filepath.Join(vaultPath, filepath.FromSlash(candidateRel))
	if err := paths.ValidateWithinVault(vaultPath, candidateAbs); err != nil {
		return "", "", svcerr.Wrap(codes.ErrFileOutsideVault, "asset destination resolves outside the vault", err).
			WithSuggestion("Choose a destination under the configured asset root")
	}

	if info, err := os.Stat(candidateAbs); err == nil {
		if info.IsDir() {
			directoryHint = true
		} else if directoryHint {
			return "", "", svcerr.New(codes.ErrInvalidInput, "asset destination directory is an existing file").
				WithDetails(map[string]any{"path": candidateRel})
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", svcerr.Wrap(codes.ErrFileRead, "failed to inspect asset destination", err).
			WithDetails(map[string]any{"path": candidateRel})
	}

	if directoryHint {
		if strings.TrimSpace(sourceBase) == "" || sourceBase == "." {
			return "", "", svcerr.New(codes.ErrInvalidInput, "asset source has an invalid filename")
		}
		candidateRel = paths.NormalizeVaultRelPath(filepath.ToSlash(filepath.Join(candidateRel, sourceBase)))
		candidateAbs = filepath.Join(vaultPath, filepath.FromSlash(candidateRel))
	}

	base := filepath.Base(candidateRel)
	ext := filepath.Ext(base)
	if strings.TrimSpace(base) == "" || base == "." {
		return "", "", svcerr.New(codes.ErrInvalidInput, "asset destination has an empty filename")
	}
	if ext == "" {
		return "", "", svcerr.New(codes.ErrInvalidInput, "asset destination must include a file extension").
			WithSuggestion("Use a destination like assets/pdfs/file.pdf or a directory ending with /")
	}
	if strings.EqualFold(ext, paths.MDExtension) {
		return "", "", svcerr.New(codes.ErrInvalidInput, "Markdown files cannot be imported as assets").
			WithSuggestion("Choose a non-Markdown destination extension")
	}

	assetRoot := vaultCfg.GetAssetRoot()
	if !strings.HasPrefix(candidateRel, assetRoot) {
		return "", "", svcerr.New(codes.ErrFileOutsideVault, "asset destination is outside the configured asset root").
			WithSuggestion(fmt.Sprintf("Choose a destination under %s", assetRoot)).
			WithDetails(map[string]any{
				"path":       candidateRel,
				"asset_root": assetRoot,
			})
	}
	if err := paths.ValidateWithinVault(vaultPath, candidateAbs); err != nil {
		return "", "", svcerr.Wrap(codes.ErrFileOutsideVault, "asset destination resolves outside the vault", err).
			WithSuggestion("Choose a destination under the configured asset root").
			WithDetails(map[string]any{"path": candidateRel})
	}
	return candidateRel, candidateAbs, nil
}

func copyFileAtomic(sourcePath, destinationPath string, force bool) (os.FileInfo, error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to open asset source", err).
			WithDetails(map[string]any{"source": sourcePath})
	}
	defer source.Close()

	sourceInfo, err := source.Stat()
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to inspect asset source", err)
	}
	if !sourceInfo.Mode().IsRegular() {
		return nil, svcerr.New(codes.ErrInvalidInput, "asset source must be a regular file")
	}

	tmp, err := os.CreateTemp(filepath.Dir(destinationPath), "."+filepath.Base(destinationPath)+".tmp-*")
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create temporary asset file", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	_ = tmp.Chmod(sourceInfo.Mode().Perm())
	if _, err := io.Copy(tmp, source); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to copy asset data", err)
	}
	afterCopyInfo, err := source.Stat()
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to verify asset source", err)
	}
	if sourceInfo.Size() != afterCopyInfo.Size() || !sourceInfo.ModTime().Equal(afterCopyInfo.ModTime()) {
		return nil, svcerr.New(codes.ErrFileRead, "asset source changed during import").
			WithSuggestion("Wait for the source file to stop changing, then retry")
	}
	if err := tmp.Sync(); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to sync imported asset", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to close imported asset", err)
	}

	if !force {
		if err := os.Link(tmpPath, destinationPath); err != nil {
			if _, statErr := os.Lstat(destinationPath); statErr == nil {
				return nil, svcerr.New(codes.ErrFileExists, "asset destination already exists").
					WithSuggestion("Choose a different destination or pass --force to overwrite it")
			}
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to install imported asset", err)
		}
		if err := os.Remove(tmpPath); err != nil {
			_ = os.Remove(destinationPath)
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to finalize imported asset", err)
		}
		committed = true
		return sourceInfo, nil
	}
	if err := os.Rename(tmpPath, destinationPath); err != nil {
		if runtime.GOOS != "windows" {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to install imported asset", err)
		}
		if replaceErr := replaceFile(tmpPath, destinationPath); replaceErr != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to overwrite imported asset", errors.Join(err, replaceErr))
		}
	}
	committed = true
	return sourceInfo, nil
}

func replaceFile(tmpPath, destinationPath string) error {
	backupPath := tmpPath + ".backup"
	if err := os.Rename(destinationPath, backupPath); err != nil {
		return fmt.Errorf("back up destination: %w", err)
	}
	if err := os.Rename(tmpPath, destinationPath); err != nil {
		if restoreErr := os.Rename(backupPath, destinationPath); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("restore destination: %w", restoreErr))
		}
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func isAbsoluteOrRooted(value string) bool {
	if filepath.IsAbs(value) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "\\") {
		return true
	}
	return len(value) >= 2 &&
		((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) &&
		value[1] == ':'
}
