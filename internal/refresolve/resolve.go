// Package refresolve provides vault-aware reference resolution.
package refresolve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type ResolveResult struct {
	ObjectID       string
	FilePath       string
	IsSection      bool
	FileObjectID   string
	LineStart      int
	LineEnd        *int
	SubtreeLineEnd *int
	MatchSource    string
}

type AmbiguousRefError struct {
	Reference    string
	Matches      []string
	MatchSources map[string]string
}

func (e *AmbiguousRefError) Error() string {
	return fmt.Sprintf("reference '%s' is ambiguous, matches: %v", e.Reference, e.Matches)
}

// Unwrap exposes the canonical service error while retaining the typed wrapper
// that resolver callers use to distinguish ambiguity from a missing target.
func (e *AmbiguousRefError) Unwrap() error {
	if e == nil {
		return nil
	}
	details := map[string]any{
		"reference": e.Reference,
		"matches":   e.Matches,
	}
	if len(e.MatchSources) > 0 {
		details["match_sources"] = e.MatchSources
	}
	return svcerr.New(codes.ErrRefAmbiguous, e.Error()).
		WithSuggestion("Use a full object ID/path to disambiguate").
		WithDetails(details)
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

// Unwrap makes missing-reference errors available to the shared service-error
// adapter without erasing their typed resolver semantics.
func (e *RefNotFoundError) Unwrap() error {
	if e == nil {
		return nil
	}
	return svcerr.New(codes.ErrRefNotFound, e.Error())
}

func IsAmbiguousRef(err error) bool {
	var e *AmbiguousRefError
	return errors.As(err, &e)
}

func IsRefNotFound(err error) bool {
	var e *RefNotFoundError
	return errors.As(err, &e)
}

// Operation reuses the index resolver across a related group of lookups.
// It borrows its runtime and does not own or close the runtime's database.
type Operation struct {
	rt       *vaultruntime.Runtime
	db       *index.Database
	resolver *resolver.Resolver
}

func New(rt *vaultruntime.Runtime) (*Operation, error) {
	if rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	return &Operation{rt: rt, db: rt.DB}, nil
}

func (op *Operation) dailyDirectory() string {
	if op == nil || op.rt == nil || op.rt.VaultCfg == nil {
		return "daily"
	}
	dailyDir := op.rt.VaultCfg.GetDailyDirectory()
	if dailyDir == "" {
		return "daily"
	}
	return dailyDir
}

func (op *Operation) getDB() (*index.Database, error) {
	if op == nil || op.rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	if op.db != nil {
		return op.db, nil
	}

	if err := op.rt.OpenDB(); err != nil {
		return nil, fmt.Errorf("failed to open database: %w (run 'rvn reindex' to rebuild)", err)
	}
	op.db = op.rt.DB
	return op.db, nil
}

func (op *Operation) getResolver() (*resolver.Resolver, error) {
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

func (op *Operation) Resolve(reference string, allowMissing bool) (*ResolveResult, error) {
	if op == nil || op.rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}

	ref := strings.TrimSpace(reference)
	literalPathResult, err := tryLiteralPath(ref, op.rt.VaultPath, op.rt.VaultCfg)
	if err != nil {
		return nil, err
	}

	res, err := op.getResolver()
	if err != nil {
		// When the resolver cannot be built (e.g. the index is unavailable) but a
		// file exists at the literal path, fall back to it rather than failing.
		if literalPathResult != nil {
			return literalPathResult, nil
		}
		return nil, err
	}

	resolved := res.Resolve(ref)

	// A file sitting at the literal path participates as a resolution candidate
	// alongside the index. We never silently prefer the on-disk file over
	// indexed objects (or vice versa): if the index resolves the same reference
	// to a different object, or to multiple objects, the reference is ambiguous.
	if literalPathResult != nil {
		if ambiguousErr := literalPathResolverConflict(ref, literalPathResult, resolved); ambiguousErr != nil {
			return nil, ambiguousErr
		}
		// The literal file is the unique target: either the index has no other
		// match for this reference, or it agrees on the same object.
		return literalPathResult, nil
	}

	if resolved.Ambiguous {
		return nil, &AmbiguousRefError{
			Reference:    ref,
			Matches:      resolved.Matches,
			MatchSources: resolved.MatchSources,
		}
	}
	if resolved.TargetID == "" {
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

	if assetPath, ok, err := tryResolvedAssetPath(op.rt.VaultPath, result.FileObjectID); err != nil {
		return nil, err
	} else if ok {
		result.FilePath = assetPath
		return result, nil
	}

	filePath, err := vault.ResolveObjectToFileWithConfig(op.rt.VaultPath, result.FileObjectID, op.rt.VaultCfg)
	if err != nil {
		if allowMissing {
			// Daily notes have a bare-date object ID but live under the configured
			// daily directory; compute the on-disk path for the not-yet-created note.
			if dates.IsValidDate(result.FileObjectID) {
				result.FilePath = op.rt.VaultCfg.DailyNotePath(op.rt.VaultPath, result.FileObjectID)
				return result, nil
			}
			// Legacy compatibility: a daily-directory-prefixed object ID.
			if dailyDir := op.dailyDirectory(); strings.HasPrefix(result.FileObjectID, dailyDir+"/") {
				result.FilePath = filepath.Join(op.rt.VaultPath, result.FileObjectID+".md")
				return result, nil
			}
		}
		return nil, &RefNotFoundError{
			Reference: ref,
			Detail:    fmt.Sprintf("resolved to '%s' but file not found", resolved.TargetID),
		}
	}

	result.FilePath = filePath
	if err := op.addSectionMetadata(result); err != nil {
		return nil, err
	}
	return result, nil
}

func (op *Operation) addSectionMetadata(result *ResolveResult) error {
	if op == nil || result == nil || !result.IsSection {
		return nil
	}
	db, err := op.getDB()
	if err != nil {
		return err
	}
	section, err := db.GetSection(result.ObjectID)
	if err != nil {
		return err
	}
	if section == nil {
		return nil
	}
	result.LineStart = section.LineStart
	result.LineEnd = section.LineEnd
	result.SubtreeLineEnd = section.SubtreeLineEnd
	return nil
}

func tryResolvedAssetPath(vaultPath, targetID string) (string, bool, error) {
	if strings.TrimSpace(vaultPath) == "" || strings.TrimSpace(targetID) == "" {
		return "", false, nil
	}
	if filepath.Ext(targetID) == "" || strings.HasSuffix(strings.ToLower(targetID), ".md") {
		return "", false, nil
	}
	fullPath := filepath.Join(vaultPath, targetID)
	if err := paths.ValidateWithinVault(vaultPath, fullPath); err != nil {
		return "", false, err
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if info.IsDir() {
		return "", false, nil
	}
	return fullPath, true, nil
}

// literalPathResolverConflict reports ambiguity when a file found at the literal
// path collides with what the index resolves the same reference to. It returns
// nil when there is no conflict: the index found nothing for this reference, or
// it agrees on the same object as the literal file.
//
// This keeps bare-reference resolution honest — an on-disk page (e.g. a
// vault-root "freya.md") is never silently preferred over an indexed typed
// object (e.g. "people/freya"), and neither is preferred over the other.
func literalPathResolverConflict(reference string, literalPathResult *ResolveResult, resolved resolver.ResolveResult) error {
	if literalPathResult == nil {
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

func (op *Operation) ResolveDynamic(reference string, allowDynamicMissing bool) (*ResolveResult, error) {
	if op == nil || op.rt == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	if dynResult, handled, dynErr := resolveDynamicDateReference(reference, op.rt, allowDynamicMissing); handled {
		if dynErr != nil {
			return nil, dynErr
		}
		return dynResult, nil
	}

	result, err := op.Resolve(reference, allowDynamicMissing)
	if err == nil {
		return result, nil
	}
	if !IsRefNotFound(err) {
		return nil, err
	}
	return nil, err
}

func Resolve(reference string, rt *vaultruntime.Runtime, allowMissing bool) (*ResolveResult, error) {
	op, err := New(rt)
	if err != nil {
		return nil, err
	}
	return op.Resolve(reference, allowMissing)
}

func ResolveDynamic(reference string, rt *vaultruntime.Runtime, allowDynamicMissing bool) (*ResolveResult, error) {
	op, err := New(rt)
	if err != nil {
		return nil, err
	}
	return op.ResolveDynamic(reference, allowDynamicMissing)
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

func resolveDynamicDateReference(reference string, rt *vaultruntime.Runtime, allowMissing bool) (*ResolveResult, bool, error) {
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

	dateInput, ok, err := dates.ParseInput(baseRef, time.Now())
	if err != nil {
		return nil, true, err
	}
	if !ok || dateInput.Kind != dates.InputRelativeDate {
		return nil, false, nil
	}

	dateStr := dateInput.CalendarDate
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
