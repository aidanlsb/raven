package editsvc

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/refresolve"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type RunRequest struct {
	Reference string
	Edits     []EditSpec
	Preview   bool
}

type RunResult struct {
	Path      string
	Edits     []EditResult
	ChangeSet mutation.ChangeSet
}

func Run(rt *vaultruntime.Runtime, req RunRequest) (*RunResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault runtime is required", err)
	}
	reference := strings.TrimSpace(req.Reference)
	if reference == "" {
		return nil, svcerr.New(codes.ErrMissingArgument, "requires reference argument").
			WithSuggestion("Usage: rvn edit <reference> <old_str> <new_str> or --edits-json")
	}
	if len(req.Edits) == 0 {
		return nil, svcerr.New(codes.ErrInvalidInput, "no edits provided").
			WithSuggestion("Provide at least one edit")
	}

	literalPath := filepath.Join(rt.VaultPath, filepath.FromSlash(reference))
	if info, err := os.Stat(literalPath); err == nil && !info.IsDir() {
		if err := validateEditableContentPath(rt, literalPath); err != nil {
			return nil, err
		}
	}
	resolved, err := refresolve.Resolve(reference, rt, false)
	if err != nil {
		return nil, refresolve.NormalizeServiceError(err, reference)
	}
	if err := validateEditableContentPath(rt, resolved.FilePath); err != nil {
		return nil, err
	}
	content, err := os.ReadFile(resolved.FilePath)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, err.Error(), err)
	}
	relPath, _ := filepath.Rel(rt.VaultPath, resolved.FilePath)
	var scope *EditScope
	if resolved.IsSection && resolved.LineStart > 0 {
		scope = &EditScope{StartLine: resolved.LineStart}
		if resolved.SubtreeLineEnd != nil {
			scope.EndLine = *resolved.SubtreeLineEnd
		}
	}
	newContent, results, err := ApplyEditsInMemoryWithScope(string(content), relPath, req.Edits, scope)
	if err != nil {
		return nil, err
	}
	result := &RunResult{Path: relPath, Edits: results}
	if req.Preview {
		return result, nil
	}
	writeResult, err := WriteAppliedEdits(resolved.FilePath, relPath, newContent)
	if err != nil {
		return nil, err
	}
	result.ChangeSet = writeResult.ChangeSet
	return result, nil
}

func validateEditableContentPath(rt *vaultruntime.Runtime, filePath string) error {
	relPath, err := filepath.Rel(rt.VaultPath, filePath)
	if err != nil {
		return svcerr.New(codes.ErrValidationFailed, "edit only supports vault content files").
			WithSuggestion("Use edit for markdown content files inside the vault")
	}
	relPath = paths.NormalizeVaultRelPath(relPath)
	templateDir := ""
	var protectedPrefixes []string
	if rt.VaultCfg != nil {
		templateDir = rt.VaultCfg.GetTemplateDirectory()
		protectedPrefixes = rt.VaultCfg.ProtectedPrefixes
	}
	if paths.IsProtectedRelPath(relPath, protectedPrefixes) {
		suggestion := "Use the dedicated Raven command for this protected path"
		switch relPath {
		case "raven.yaml":
			suggestion = "Use 'rvn vault config ...' or 'rvn query saved ...' to mutate raven.yaml"
		case "schema.yaml":
			suggestion = "Use 'rvn schema ...' to mutate schema.yaml"
		}
		return svcerr.New(codes.ErrValidationFailed, "cannot edit protected or system-managed paths").
			WithDetails(map[string]interface{}{"path": relPath}).WithSuggestion(suggestion)
	}
	if rt.VaultCfg != nil {
		matcher, err := ravenignore.NewMatcher(rt.VaultCfg.GetExcludePatterns())
		if err != nil {
			return svcerr.Wrap(codes.ErrValidationFailed, "invalid exclude config", err).
				WithDetails(map[string]interface{}{"path": relPath}).
				WithSuggestion("Fix raven.yaml exclude patterns and try again")
		}
		if matcher.Match(relPath, false) {
			return svcerr.New(codes.ErrValidationFailed, "cannot edit excluded paths").
				WithDetails(map[string]interface{}{"path": relPath}).
				WithSuggestion("Choose a managed path, or update exclusions with 'rvn vault config exclude ...'")
		}
	}
	if templateDir != "" && strings.HasPrefix(relPath, templateDir) {
		return svcerr.New(codes.ErrValidationFailed, "edit only supports vault content files; template files are managed separately").
			WithDetails(map[string]interface{}{"path": relPath}).
			WithSuggestion("Use 'rvn template write' or 'rvn template delete' for template lifecycle changes")
	}
	if !paths.HasMDExtension(relPath) {
		return svcerr.New(codes.ErrValidationFailed, "edit only supports markdown content files").
			WithDetails(map[string]interface{}{"path": relPath}).
			WithSuggestion("Use dedicated Raven commands for vault config, schema, templates, and other non-content files")
	}
	return nil
}
