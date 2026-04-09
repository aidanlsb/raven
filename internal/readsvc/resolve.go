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
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/resolver"
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

type resolveOperation struct {
	rt       *Runtime
	db       *index.Database
	closeDB  bool
	resolver *resolver.Resolver
}

func newResolveOperation(rt *Runtime) (*resolveOperation, error) {
	if rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	return &resolveOperation{rt: rt, db: rt.DB}, nil
}

func (op *resolveOperation) Close() error {
	if op == nil || !op.closeDB || op.db == nil {
		return nil
	}
	return op.db.Close()
}

func (op *resolveOperation) dailyDirectory() string {
	if op == nil || op.rt == nil || op.rt.VaultCfg == nil {
		return "daily"
	}
	dailyDir := op.rt.VaultCfg.GetDailyDirectory()
	if dailyDir == "" {
		return "daily"
	}
	return dailyDir
}

func (op *resolveOperation) getDB() (*index.Database, error) {
	if op == nil || op.rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	if op.db != nil {
		return op.db, nil
	}

	db, err := index.Open(op.rt.VaultPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w (run 'rvn reindex' to rebuild)", err)
	}
	db.SetDailyDirectory(op.dailyDirectory())
	op.db = db
	op.closeDB = true
	return op.db, nil
}

func (op *resolveOperation) getResolver() (*resolver.Resolver, error) {
	if op == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	if op.resolver != nil {
		return op.resolver, nil
	}

	db, err := op.getDB()
	if err != nil {
		return nil, err
	}

	res, err := db.Resolver(index.ResolverOptions{
		DailyDirectory: op.dailyDirectory(),
		Schema:         op.rt.Schema,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create resolver: %w", err)
	}
	op.resolver = res
	return op.resolver, nil
}

func (op *resolveOperation) resolveReference(reference string, allowMissing bool) (*ResolveResult, error) {
	if op == nil || op.rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}

	ref := strings.TrimSpace(reference)
	literalPathResult, err := tryLiteralPath(ref, op.rt.VaultPath, op.rt.VaultCfg)
	if err != nil {
		return nil, err
	}
	if literalPathResult != nil && !dates.IsValidDate(ref) {
		return literalPathResult, nil
	}

	res, err := op.getResolver()
	if err != nil {
		return nil, err
	}

	resolved := res.Resolve(ref)
	if literalPathResult != nil {
		if ambiguousErr := isoDateLiteralPathAmbiguity(ref, literalPathResult, resolved); ambiguousErr != nil {
			return nil, ambiguousErr
		}
	}
	if resolved.Ambiguous {
		return nil, &AmbiguousRefError{
			Reference:    ref,
			Matches:      resolved.Matches,
			MatchSources: resolved.MatchSources,
		}
	}
	if resolved.TargetID == "" {
		if literalPathResult != nil {
			return literalPathResult, nil
		}
		return nil, &RefNotFoundError{Reference: ref}
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

	filePath, err := vault.ResolveObjectToFileWithConfig(op.rt.VaultPath, result.FileObjectID, op.rt.VaultCfg)
	if err != nil {
		dailyDir := op.dailyDirectory()
		if allowMissing && strings.HasPrefix(result.FileObjectID, dailyDir+"/") {
			result.FilePath = filepath.Join(op.rt.VaultPath, result.FileObjectID+".md")
			return result, nil
		}
		return nil, &RefNotFoundError{
			Reference: ref,
			Detail:    fmt.Sprintf("resolved to '%s' but file not found", resolved.TargetID),
		}
	}

	result.FilePath = filePath
	return result, nil
}

func isoDateLiteralPathAmbiguity(reference string, literalPathResult *ResolveResult, resolved resolver.ResolveResult) error {
	if literalPathResult == nil || !dates.IsValidDate(reference) {
		return nil
	}

	if resolved.Ambiguous {
		matches := append([]string{}, resolved.Matches...)
		matchSources := copyMatchSources(resolved.MatchSources)
		if !containsMatch(matches, literalPathResult.ObjectID) {
			matches = append(matches, literalPathResult.ObjectID)
		}
		matchSources[literalPathResult.ObjectID] = literalPathResult.MatchSource
		return &AmbiguousRefError{
			Reference:    reference,
			Matches:      matches,
			MatchSources: matchSources,
		}
	}

	if resolved.TargetID != "" && resolved.TargetID != literalPathResult.ObjectID {
		matchSources := copyMatchSources(resolved.MatchSources)
		matchSources[literalPathResult.ObjectID] = literalPathResult.MatchSource
		if _, ok := matchSources[resolved.TargetID]; !ok {
			matchSources[resolved.TargetID] = "date"
		}
		return &AmbiguousRefError{
			Reference:    reference,
			Matches:      []string{literalPathResult.ObjectID, resolved.TargetID},
			MatchSources: matchSources,
		}
	}

	return nil
}

func copyMatchSources(sources map[string]string) map[string]string {
	if len(sources) == 0 {
		return make(map[string]string)
	}
	copied := make(map[string]string, len(sources))
	for id, source := range sources {
		copied[id] = source
	}
	return copied
}

func containsMatch(matches []string, want string) bool {
	for _, match := range matches {
		if match == want {
			return true
		}
	}
	return false
}

func (op *resolveOperation) resolveReferenceWithDynamicDates(reference string, allowDynamicMissing bool) (*ResolveResult, error) {
	if dynResult, handled, dynErr := resolveDynamicDateReference(reference, op.rt, allowDynamicMissing); handled {
		if dynErr != nil {
			return nil, dynErr
		}
		return dynResult, nil
	}

	result, err := op.resolveReference(reference, allowDynamicMissing)
	if err == nil {
		return result, nil
	}
	if !IsRefNotFound(err) {
		return nil, err
	}
	return nil, err
}

func ResolveReference(reference string, rt *Runtime, allowMissing bool) (*ResolveResult, error) {
	op, err := newResolveOperation(rt)
	if err != nil {
		return nil, err
	}
	defer op.Close()
	return op.resolveReference(reference, allowMissing)
}

func ResolveReferenceWithDynamicDates(reference string, rt *Runtime, allowDynamicMissing bool) (*ResolveResult, error) {
	op, err := newResolveOperation(rt)
	if err != nil {
		return nil, err
	}
	defer op.Close()
	return op.resolveReferenceWithDynamicDates(reference, allowDynamicMissing)
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
}) (*ResolveResult, error) {
	candidates := []string{reference}
	if !strings.HasSuffix(reference, ".md") {
		candidates = append(candidates, reference+".md")
	}

	for _, candidate := range candidates {
		fullPath := filepath.Join(vaultPath, candidate)
		if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
			continue
		}
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
			}, nil
		}
	}

	return nil, nil
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
