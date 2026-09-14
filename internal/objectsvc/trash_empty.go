package objectsvc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/svcerr"
)

var ageDayUnit = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]*)?)d`)

type EmptyTrashRequest struct {
	VaultPath   string
	VaultConfig *config.VaultConfig
	OlderThan   string
	Now         func() time.Time
}

type EmptyTrashResult struct {
	Entries   []TrashEntry
	TrashDir  string
	OlderThan string
}

// PreviewEmptyTrash lists trash entries that empty would permanently delete.
func PreviewEmptyTrash(req EmptyTrashRequest) (*EmptyTrashResult, error) {
	result, _, err := prepareEmptyTrash(req)
	return result, err
}

// EmptyTrash permanently deletes matching files from the configured trash
// directory. It never removes live vault objects.
func EmptyTrash(req EmptyTrashRequest) (*EmptyTrashResult, error) {
	result, trashRoot, err := prepareEmptyTrash(req)
	if err != nil {
		return nil, err
	}
	for _, entry := range result.Entries {
		if err := permanentlyRemoveTrashEntry(req.VaultPath, trashRoot, entry); err != nil {
			return nil, err
		}
	}
	if err := pruneEmptyTrashDirs(trashRoot); err != nil {
		return nil, err
	}
	return result, nil
}

func prepareEmptyTrash(req EmptyTrashRequest) (*EmptyTrashResult, string, error) {
	olderThan := strings.TrimSpace(req.OlderThan)
	var minAge time.Duration
	if olderThan != "" {
		var err error
		minAge, err = parseAgeDuration(olderThan)
		if err != nil {
			return nil, "", err
		}
	}

	listResult, err := ListTrash(ListTrashRequest{
		VaultPath:   req.VaultPath,
		VaultConfig: req.VaultConfig,
	})
	if err != nil {
		return nil, "", err
	}
	_, trashRoot, err := resolveTrashRoot(req.VaultPath, req.VaultConfig)
	if err != nil {
		return nil, "", err
	}

	entries := listResult.Entries
	if minAge > 0 {
		nowFn := req.Now
		if nowFn == nil {
			nowFn = time.Now
		}
		cutoff := nowFn().Add(-minAge)
		filtered := make([]TrashEntry, 0, len(entries))
		for _, entry := range entries {
			if !entry.modifiedAt.After(cutoff) {
				filtered = append(filtered, entry)
			}
		}
		entries = filtered
	}
	if entries == nil {
		entries = []TrashEntry{}
	}

	return &EmptyTrashResult{
		Entries:   entries,
		TrashDir:  listResult.TrashDir,
		OlderThan: olderThan,
	}, trashRoot, nil
}

func permanentlyRemoveTrashEntry(vaultPath, trashRoot string, entry TrashEntry) error {
	absPath := filepath.Join(vaultPath, filepath.FromSlash(entry.TrashPath))
	if err := paths.ValidateWithinVault(trashRoot, absPath); err != nil {
		return svcerr.Wrap(codes.ErrFileOutsideVault, "trash entry is outside the configured trash directory", err)
	}
	if err := os.Remove(absPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return svcerr.Wrap(codes.ErrFileWrite, "failed to permanently delete trash entry", err)
	}
	if entry.MetadataPath == "" {
		return nil
	}
	metaAbs := filepath.Join(vaultPath, filepath.FromSlash(entry.MetadataPath))
	if err := paths.ValidateWithinVault(trashRoot, metaAbs); err != nil {
		return svcerr.Wrap(codes.ErrFileOutsideVault, "trash metadata is outside the configured trash directory", err)
	}
	if err := os.Remove(metaAbs); err != nil && !errors.Is(err, os.ErrNotExist) {
		return svcerr.Wrap(codes.ErrFileWrite, "failed to remove trash metadata", err)
	}
	return nil
}

func pruneEmptyTrashDirs(trashRoot string) error {
	info, err := os.Lstat(trashRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return svcerr.Wrap(codes.ErrFileWrite, "failed to inspect trash directory", err)
	}
	if !info.IsDir() {
		return nil
	}

	var dirs []string
	walkErr := filepath.WalkDir(trashRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == trashRoot || !entry.IsDir() {
			return nil
		}
		dirs = append(dirs, path)
		return nil
	})
	if walkErr != nil {
		return svcerr.Wrap(codes.ErrFileWrite, "failed to inspect trash directories", walkErr)
	}

	sort.Slice(dirs, func(i, j int) bool {
		di := strings.Count(dirs[i], string(filepath.Separator))
		dj := strings.Count(dirs[j], string(filepath.Separator))
		if di != dj {
			return di > dj
		}
		return dirs[i] > dirs[j]
	})
	for _, dir := range dirs {
		if err := paths.ValidateWithinVault(trashRoot, dir); err != nil {
			return svcerr.Wrap(codes.ErrFileOutsideVault, "trash directory is outside the configured trash directory", err)
		}
		children, readErr := os.ReadDir(dir)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return svcerr.Wrap(codes.ErrFileWrite, "failed to inspect trash directory", readErr)
		}
		if len(children) > 0 {
			continue
		}
		if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
			return svcerr.Wrap(codes.ErrFileWrite, "failed to remove empty trash directory", err)
		}
	}
	return nil
}

func parseAgeDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, svcerr.New(codes.ErrInvalidInput, "older-than duration is empty").
			WithSuggestion("Use a duration such as 24h, 7d, or 30d")
	}

	normalized := ageDayUnit.ReplaceAllStringFunc(value, func(match string) string {
		number := match[:len(match)-1]
		days, err := strconv.ParseFloat(number, 64)
		if err != nil {
			return match
		}
		hours := days * 24
		if hours == float64(int64(hours)) {
			return strconv.FormatInt(int64(hours), 10) + "h"
		}
		return strconv.FormatFloat(hours, 'f', -1, 64) + "h"
	})
	duration, err := time.ParseDuration(normalized)
	if err != nil {
		return 0, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid older-than duration: %q", value)).
			WithSuggestion("Use a duration such as 24h, 7d, or 30d")
	}
	if duration <= 0 {
		return 0, svcerr.New(codes.ErrInvalidInput, "older-than duration must be greater than zero").
			WithSuggestion("Use a duration such as 24h, 7d, or 30d")
	}
	return duration, nil
}
