package readsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/vault"
)

type ResolveResult struct {
	ObjectID     string
	FilePath     string
	IsSection    bool
	FileObjectID string
	MatchSource  string
}

type ResolveOptions struct {
	VaultPath    string
	VaultConfig  *Runtime
	AllowMissing bool
}

type AmbiguousRefError struct {
	Reference    string
	Matches      []string
	MatchSources map[string]string
}

func (e *AmbiguousRefError) Error() string {
	return fmt.Sprintf("reference '%s' is ambiguous, matches: %v", e.Reference, e.Matches)
}

type RefNotFoundError struct {
	Reference string
	Detail    string
}

func (e *RefNotFoundError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("reference '%s' not found: %s", e.Reference, e.Detail)
	}
	return fmt.Sprintf("reference '%s' not found", e.Reference)
}

func IsAmbiguousRef(err error) bool {
	var e *AmbiguousRefError
	return errors.As(err, &e)
}

func IsRefNotFound(err error) bool {
	var e *RefNotFoundError
	return errors.As(err, &e)
}

func ResolveReference(reference string, rt *Runtime, allowMissing bool) (*ResolveResult, error) {
	if rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}

	if result := tryLiteralPath(reference, rt.VaultPath, rt.VaultCfg); result != nil {
		return result, nil
	}

	db := rt.DB
	closeDB := false
	if db == nil {
		var err error
		db, err = index.Open(rt.VaultPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open database: %w (run 'rvn reindex' to rebuild)", err)
		}
		closeDB = true
		db.SetDailyDirectory(rt.VaultCfg.GetDailyDirectory())
	}
	if closeDB {
		defer db.Close()
	}

	res, err := db.Resolver(index.ResolverOptions{
		DailyDirectory: rt.VaultCfg.GetDailyDirectory(),
		Schema:         rt.Schema,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: %w", err)
	}

	resolved := res.Resolve(reference)
	if resolved.Ambiguous {
		return nil, &AmbiguousRefError{
			Reference:    reference,
			Matches:      resolved.Matches,
			MatchSources: resolved.MatchSources,
		}
	}
	if resolved.TargetID == "" {
		return nil, &RefNotFoundError{Reference: reference}
	}

	matchSource := ""
	if resolved.MatchSources != nil {
		matchSource = resolved.MatchSources[resolved.TargetID]
	}
	result := &ResolveResult{
		ObjectID:    resolved.TargetID,
		MatchSource: matchSource,
	}

	if idx := strings.Index(resolved.TargetID, "#"); idx >= 0 {
		result.IsSection = true
		result.FileObjectID = resolved.TargetID[:idx]
	} else {
		result.FileObjectID = resolved.TargetID
	}

	filePath, err := vault.ResolveObjectToFileWithConfig(rt.VaultPath, result.FileObjectID, rt.VaultCfg)
	if err != nil {
		dailyDir := rt.VaultCfg.GetDailyDirectory()
		if dailyDir == "" {
			dailyDir = "daily"
		}
		if allowMissing && strings.HasPrefix(result.FileObjectID, dailyDir+"/") {
			result.FilePath = filepath.Join(rt.VaultPath, result.FileObjectID+".md")
			return result, nil
		}
		return nil, &RefNotFoundError{
			Reference: reference,
			Detail:    fmt.Sprintf("resolved to '%s' but file not found", resolved.TargetID),
		}
	}

	result.FilePath = filePath
	return result, nil
}

func ResolveReferenceWithDynamicDates(reference string, rt *Runtime, allowDynamicMissing bool) (*ResolveResult, error) {
	result, err := ResolveReference(reference, rt, rt != nil && rt.VaultCfg != nil)
	if err == nil {
		return result, nil
	}
	if !IsRefNotFound(err) {
		return nil, err
	}

	dynResult, handled, dynErr := resolveDynamicDateReference(reference, rt, allowDynamicMissing)
	if !handled {
		return nil, err
	}
	if dynErr != nil {
		return nil, dynErr
	}
	return dynResult, nil
}

func ResolveReferenceToFile(reference string, rt *Runtime, allowMissing bool) (string, error) {
	result, err := ResolveReference(reference, rt, allowMissing)
	if err != nil {
		return "", err
	}
	return result.FilePath, nil
}

func ResolveReferenceToObjectID(reference string, rt *Runtime, allowMissing bool) (string, error) {
	result, err := ResolveReference(reference, rt, allowMissing)
	if err != nil {
		return "", err
	}
	return result.ObjectID, nil
}

func tryLiteralPath(reference, vaultPath string, vaultCfg interface {
	FilePathToObjectID(string) string
}) *ResolveResult {
	candidates := []string{reference}
	if !strings.HasSuffix(reference, ".md") {
		candidates = append(candidates, reference+".md")
	}

	for _, candidate := range candidates {
		fullPath := filepath.Join(vaultPath, candidate)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			objectID := strings.TrimSuffix(candidate, ".md")
			if vaultCfg != nil {
				objectID = vaultCfg.FilePathToObjectID(objectID)
			}
			return &ResolveResult{
				ObjectID:     objectID,
				FilePath:     fullPath,
				IsSection:    false,
				FileObjectID: objectID,
				MatchSource:  "literal_path",
			}
		}
	}

	return nil
}

func resolveDynamicDateReference(reference string, rt *Runtime, allowMissing bool) (*ResolveResult, bool, error) {
	if rt == nil || rt.VaultCfg == nil {
		return nil, false, fmt.Errorf("runtime is required")
	}

	ref := strings.TrimSpace(reference)
	if ref == "" {
		return nil, false, nil
	}

	baseRef := ref
	fragment := ""
	if parts := strings.SplitN(ref, "#", 2); len(parts) == 2 {
		baseRef = parts[0]
		fragment = parts[1]
	}
	if baseRef == "" {
		return nil, false, nil
	}

	keyword := strings.ToLower(strings.TrimSpace(baseRef))
	relative, ok := dates.ResolveRelativeDateKeyword(keyword, time.Now(), time.Monday)
	if !ok || relative.Kind != dates.RelativeDateInstant {
		return nil, false, nil
	}

	dateStr := relative.Date.Format(dates.DateLayout)
	fileObjectID := rt.VaultCfg.DailyNoteID(dateStr)
	objectID := fileObjectID
	if fragment != "" {
		objectID = fileObjectID + "#" + fragment
	}
	filePath := rt.VaultCfg.DailyNotePath(rt.VaultPath, dateStr)

	if !allowMissing {
		if _, err := os.Stat(filePath); err != nil {
			if os.IsNotExist(err) {
				return nil, true, &RefNotFoundError{
					Reference: reference,
					Detail:    fmt.Sprintf("resolved to '%s' but file not found", objectID),
				}
			}
			return nil, true, err
		}
	}

	return &ResolveResult{
		ObjectID:     objectID,
		FilePath:     filePath,
		IsSection:    fragment != "",
		FileObjectID: fileObjectID,
		MatchSource:  "date",
	}, true, nil
}
