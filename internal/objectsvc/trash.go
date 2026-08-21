package objectsvc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

const (
	TrashKindMarkdown = "markdown"
	TrashKindFile     = "file"
)

// TrashEntry describes one file in the configured trash directory. RestorePath
// is derived from the mirrored path used by DeleteFile.
type TrashEntry struct {
	Reference   string `json:"reference"`
	TrashPath   string `json:"trash_path"`
	RestorePath string `json:"restore_path"`
	Kind        string `json:"kind"`
}

type ListTrashRequest struct {
	VaultPath   string
	VaultConfig *config.VaultConfig
	Reference   string
	Kind        string
}

type ListTrashResult struct {
	Entries          []TrashEntry
	TrashDir         string
	DeletionBehavior string
}

// ListTrash returns files from the configured trash directory in stable path
// order. A missing trash directory is an empty list, not an error.
func ListTrash(req ListTrashRequest) (*ListTrashResult, error) {
	trashDir, trashRoot, err := resolveTrashRoot(req.VaultPath, req.VaultConfig)
	if err != nil {
		return nil, err
	}

	kind := strings.TrimSpace(req.Kind)
	switch kind {
	case "", TrashKindMarkdown, TrashKindFile:
	default:
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid trash kind: %q", kind)).
			WithSuggestion("Use 'markdown' or 'file'")
	}

	result := &ListTrashResult{
		Entries:          []TrashEntry{},
		TrashDir:         trashDir,
		DeletionBehavior: req.VaultConfig.GetDeletionConfig().Behavior,
	}
	info, statErr := os.Stat(trashRoot)
	if os.IsNotExist(statErr) {
		return result, nil
	}
	if statErr != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to read trash directory", statErr)
	}
	if !info.IsDir() {
		return nil, svcerr.New(codes.ErrConfigInvalid, fmt.Sprintf("configured trash path is not a directory: %s", trashDir)).
			WithSuggestion("Fix deletion.trash_dir in raven.yaml")
	}

	referenceFilter := strings.TrimSpace(req.Reference)
	walkErr := filepath.WalkDir(trashRoot, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		relTrashPath, relErr := filepath.Rel(trashRoot, filePath)
		if relErr != nil {
			return relErr
		}
		restorePath := paths.NormalizeVaultRelPath(relTrashPath)
		if !paths.IsValidVaultRelPath(restorePath) {
			return nil
		}

		entryKind := TrashKindFile
		reference := restorePath
		if paths.HasMDExtension(restorePath) {
			entryKind = TrashKindMarkdown
			reference = req.VaultConfig.FilePathToObjectID(restorePath)
		}
		if kind != "" && entryKind != kind {
			return nil
		}
		if referenceFilter != "" && reference != referenceFilter {
			return nil
		}

		result.Entries = append(result.Entries, TrashEntry{
			Reference:   reference,
			TrashPath:   paths.NormalizeVaultRelPath(filepath.Join(trashDir, relTrashPath)),
			RestorePath: restorePath,
			Kind:        entryKind,
		})
		return nil
	})
	if walkErr != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to list trash directory", walkErr)
	}

	sort.Slice(result.Entries, func(i, j int) bool {
		return result.Entries[i].TrashPath < result.Entries[j].TrashPath
	})
	return result, nil
}

type RestoreByReferenceRequest struct {
	VaultPath   string
	VaultConfig *config.VaultConfig
	Reference   string
	Runtime     *vaultruntime.Runtime
}

type RestoreByReferenceResult struct {
	Entry     TrashEntry
	ChangeSet mutation.ChangeSet
}

func PreviewRestoreByReference(req RestoreByReferenceRequest) (*RestoreByReferenceResult, error) {
	entry, err := prepareRestoreByReference(req)
	if err != nil {
		return nil, err
	}
	return &RestoreByReferenceResult{Entry: *entry}, nil
}

func RestoreByReference(req RestoreByReferenceRequest) (*RestoreByReferenceResult, error) {
	entry, err := prepareRestoreByReference(req)
	if err != nil {
		return nil, err
	}

	sourcePath := filepath.Join(req.VaultPath, filepath.FromSlash(entry.TrashPath))
	destinationPath := filepath.Join(req.VaultPath, filepath.FromSlash(entry.RestorePath))
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to create restore parent directory", err)
	}
	if err := paths.ValidateWithinVault(req.VaultPath, destinationPath); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileOutsideVault, "restore destination is outside the vault", err)
	}
	if err := os.Rename(sourcePath, destinationPath); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to restore trash entry", err)
	}

	changes := mutation.NewChangeSet()
	changes.AddMoved(entry.TrashPath, entry.RestorePath)
	return &RestoreByReferenceResult{
		Entry:     *entry,
		ChangeSet: changes,
	}, nil
}

func prepareRestoreByReference(req RestoreByReferenceRequest) (*TrashEntry, error) {
	if err := vaultruntime.RequirePath(req.VaultPath); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if req.VaultConfig == nil {
		return nil, svcerr.New(codes.ErrValidationFailed, "vault config is required").
			WithSuggestion("Fix raven.yaml and try again")
	}
	reference := strings.TrimSpace(req.Reference)
	if reference == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "trash reference or path is required").
			WithSuggestion("Usage: rvn restore <trash-reference-or-path>")
	}

	listResult, err := ListTrash(ListTrashRequest{
		VaultPath:   req.VaultPath,
		VaultConfig: req.VaultConfig,
	})
	if err != nil {
		return nil, err
	}

	normalizedReference, err := normalizeRestoreInput(req.VaultPath, reference)
	if err != nil {
		return nil, err
	}
	trashDirPrefix := paths.NormalizeDirRoot(listResult.TrashDir)
	matches := make([]TrashEntry, 0, 1)
	for _, entry := range listResult.Entries {
		relativeTrashPath := strings.TrimPrefix(entry.TrashPath, trashDirPrefix)
		if normalizedReference == entry.Reference ||
			normalizedReference == entry.TrashPath ||
			normalizedReference == relativeTrashPath ||
			normalizedReference == entry.RestorePath {
			matches = append(matches, entry)
		}
	}

	if len(matches) == 0 {
		return nil, svcerr.New(codes.ErrFileNotFound, fmt.Sprintf("trash entry not found: %s", reference)).
			WithSuggestion("Run 'rvn trash list --json' to inspect available entries").
			WithDetails(map[string]any{
				"reference": reference,
				"trash_dir": listResult.TrashDir,
			})
	}
	if len(matches) > 1 {
		candidates := make([]string, 0, len(matches))
		for _, match := range matches {
			candidates = append(candidates, match.TrashPath)
		}
		return nil, svcerr.New(codes.ErrRefAmbiguous, fmt.Sprintf("trash reference is ambiguous: %s", reference)).
			WithSuggestion("Retry with an exact trash_path from 'rvn trash list --json'").
			WithDetails(map[string]any{
				"reference": reference,
				"matches":   candidates,
			})
	}

	entry := matches[0]
	sourcePath := filepath.Join(req.VaultPath, filepath.FromSlash(entry.TrashPath))
	if err := paths.ValidateWithinVault(req.VaultPath, sourcePath); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileOutsideVault, "trash entry is outside the vault", err)
	}
	if _, err := os.Lstat(sourcePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, svcerr.New(codes.ErrFileNotFound, fmt.Sprintf("trash entry not found: %s", reference)).
				WithSuggestion("Run 'rvn trash list --json' to inspect available entries")
		}
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to inspect trash entry", err)
	}

	if err := mutationguard.ValidateContentMutationRelPath(req.VaultConfig, entry.RestorePath); err != nil {
		return nil, err
	}
	destinationPath := filepath.Join(req.VaultPath, filepath.FromSlash(entry.RestorePath))
	if err := paths.ValidateWithinVault(req.VaultPath, destinationPath); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileOutsideVault, "restore destination is outside the vault", err)
	}
	if _, err := os.Lstat(destinationPath); err == nil {
		return nil, svcerr.New(codes.ErrFileExists, fmt.Sprintf("restore destination already exists: %s", entry.RestorePath)).
			WithSuggestion("Move or delete the existing file, then retry the restore").
			WithDetails(map[string]any{
				"reference":    entry.Reference,
				"trash_path":   entry.TrashPath,
				"restore_path": entry.RestorePath,
			})
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to inspect restore destination", err)
	}

	return &entry, nil
}

func resolveTrashRoot(vaultPath string, vaultCfg *config.VaultConfig) (string, string, error) {
	if err := vaultruntime.RequirePath(vaultPath); err != nil {
		return "", "", svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if vaultCfg == nil {
		return "", "", svcerr.New(codes.ErrValidationFailed, "vault config is required").
			WithSuggestion("Fix raven.yaml and try again")
	}

	trashDir := paths.NormalizeVaultRelPath(vaultCfg.GetDeletionConfig().TrashDir)
	if !paths.IsValidVaultRelPath(trashDir) {
		return "", "", svcerr.New(codes.ErrConfigInvalid, fmt.Sprintf("invalid deletion.trash_dir: %q", trashDir)).
			WithSuggestion("Use a vault-relative path such as '.trash' or 'archive/trash'")
	}
	trashRoot := filepath.Join(vaultPath, filepath.FromSlash(trashDir))
	if err := paths.ValidateWithinVault(vaultPath, trashRoot); err != nil {
		return "", "", svcerr.Wrap(codes.ErrConfigInvalid, "deletion.trash_dir is outside the vault", err).
			WithSuggestion("Use a vault-relative path such as '.trash' or 'archive/trash'")
	}
	return trashDir, trashRoot, nil
}

func normalizeRestoreInput(vaultPath, input string) (string, error) {
	if filepath.IsAbs(input) {
		relPath, err := filepath.Rel(vaultPath, input)
		if err != nil {
			return "", svcerr.Wrap(codes.ErrInvalidInput, "failed to resolve trash path", err)
		}
		if relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
			return "", svcerr.New(codes.ErrFileOutsideVault, "trash path is outside the vault")
		}
		input = relPath
	}
	return paths.NormalizeVaultRelPath(input), nil
}
