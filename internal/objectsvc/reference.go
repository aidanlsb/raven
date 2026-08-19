package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/refresolve"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func resolveReferenceForMutation(rt *vaultruntime.Runtime, reference string) (*refresolve.ResolveResult, error) {
	resolved, err := refresolve.Resolve(reference, rt, false)
	if err != nil {
		var ambiguousErr *refresolve.AmbiguousRefError
		if errors.As(err, &ambiguousErr) {
			return nil, svcerr.Wrap(codes.ErrRefAmbiguous, ambiguousErr.Error(), err).WithSuggestion("Use a full object ID/path to disambiguate").WithDetails(map[string]interface{}{"matches": ambiguousErr.Matches})
		}
		var notFoundErr *refresolve.RefNotFoundError
		if errors.As(err, &notFoundErr) {
			return nil, svcerr.Wrap(codes.ErrRefNotFound, notFoundErr.Error(), err).WithSuggestion("Check the object reference and run 'rvn reindex' if needed")
		}
		return nil, svcerr.Wrap(codes.ErrInternal, fmt.Sprintf("failed to resolve object reference: %v", err), err).WithSuggestion("Check the object reference and run 'rvn reindex' if needed")
	}

	if err := mutationguard.ValidateContentMutationFilePath(rt.VaultPath, rt.VaultCfg, resolved.FilePath); err != nil {
		return nil, err
	}

	return resolved, nil
}

// resolveLiteralNonMarkdownFileForMutation recognizes an explicit
// vault-relative file path for file operations such as move and delete. This is
// deliberately separate from Raven reference resolution: non-Markdown files
// are never object or wikilink identities.
func resolveLiteralNonMarkdownFileForMutation(rt *vaultruntime.Runtime, input string) (filePath, relPath string, ok bool, err error) {
	if rt == nil {
		return "", "", false, nil
	}
	input = strings.TrimSpace(input)
	if input == "" || filepath.IsAbs(input) {
		return "", "", false, nil
	}
	relPath = paths.NormalizeVaultRelPath(input)
	if !paths.IsValidVaultRelPath(relPath) || paths.HasMDExtension(relPath) {
		return "", "", false, nil
	}
	filePath = filepath.Join(rt.VaultPath, filepath.FromSlash(relPath))
	if err := paths.ValidateWithinVault(rt.VaultPath, filePath); err != nil {
		return "", "", false, err
	}
	info, statErr := os.Stat(filePath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return "", "", false, nil
		}
		return "", "", false, statErr
	}
	if info.IsDir() {
		return "", "", false, nil
	}
	if err := mutationguard.ValidateContentMutationFilePath(rt.VaultPath, rt.VaultCfg, filePath); err != nil {
		return "", "", false, err
	}
	return filePath, relPath, true, nil
}
