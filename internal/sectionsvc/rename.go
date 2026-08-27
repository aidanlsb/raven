// Package sectionsvc implements mutations on Markdown-derived sections.
package sectionsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/refresolve"
	"github.com/aidanlsb/raven/internal/refs"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type RenameRequest struct {
	VaultPath      string
	VaultConfig    *config.VaultConfig
	Schema         *schema.Schema
	Reference      string
	NewHeadingText string
	Preview        bool
	ParseOptions   *parser.ParseOptions
	FailOnIndexErr bool
	Runtime        *vaultruntime.Runtime
}

type RenameResult struct {
	SourceID        string
	SourceRelative  string
	DestinationID   string
	DestinationRel  string
	UpdatedRefs     []string
	WarningMessages []string
	IndexWarnings   []reindexsvc.ProjectionWarning
}

type fileRewrite struct {
	path           string
	content        []byte
	perm           os.FileMode
	reportSourceID string
	updatedContent []byte
}

// Rename renames a section heading in place and rewrites all inbound
// references from [[...#old-slug]] to [[...#new-slug]].
//
// NewHeadingText is plain heading text. The heading level is preserved and the
// new slug is derived using the same rules the parser applies to headings.
func Rename(req RenameRequest) (*RenameResult, error) {
	if err := vaultruntime.RequirePath(req.VaultPath); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if req.VaultConfig == nil {
		return nil, svcerr.New(codes.ErrValidationFailed, "vault config is required").WithSuggestion("Fix raven.yaml and try again")
	}
	rt, owned := vaultruntime.FromRequest(req.Runtime, req.VaultPath, req.VaultConfig, req.Schema, req.ParseOptions)
	if owned {
		defer rt.Close()
	}
	req.Runtime = rt
	projectionLock, err := reindexsvc.LockProjection(rt, req.Preview)
	if err != nil {
		return nil, err
	}
	if projectionLock != nil {
		defer func() { _ = projectionLock.Close() }()
	}

	reference := strings.TrimSpace(req.Reference)
	fileID, oldSlug, isSection := paths.ParseSectionID(reference)
	if !isSection || fileID == "" || oldSlug == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid section ID: %s", reference)).WithSuggestion("Use a section ID like project/website#tasks")
	}

	newTitle := strings.TrimSpace(req.NewHeadingText)
	if newTitle == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "new heading text is required").WithSuggestion(`Usage: rvn section rename <file#section> "<new heading text>"`)
	}
	if strings.HasPrefix(newTitle, "#") {
		return nil, svcerr.New(codes.ErrInvalidInput, "section destination must be the new heading text, not a markdown heading or fragment").WithSuggestion(`Pass plain heading text, e.g. rvn section rename project/website#tasks "Completed Tasks"; the heading level is preserved`)
	}
	if _, _, destinationIsSection := paths.ParseSectionID(newTitle); destinationIsSection {
		return nil, svcerr.New(codes.ErrInvalidInput, "section destination must be the new heading text, not a section ID").WithSuggestion(`Pass plain heading text, e.g. rvn section rename project/website#tasks "Completed Tasks"`)
	}

	resolved, err := resolveSectionReference(req, reference)
	if err != nil {
		return nil, err
	}
	oldSectionID := resolved.ObjectID
	fileID, oldSlug, isSection = paths.ParseSectionID(oldSectionID)
	if !isSection || oldSlug == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid section ID: %s", oldSectionID)).WithSuggestion("Use a section ID like project/website#tasks")
	}

	sourceFile := resolved.FilePath
	if err := mutationguard.ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, sourceFile); err != nil {
		return nil, normalizeMutationError(err)
	}
	sourceRelPath, err := filepath.Rel(req.VaultPath, sourceFile)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInternal, "failed to resolve source path", err)
	}
	sourceRelPath = paths.NormalizeVaultRelPath(sourceRelPath)

	contentBytes, err := os.ReadFile(sourceFile)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to read source file", err)
	}
	content := string(contentBytes)

	doc, err := parser.ParseDocumentWithOptions(content, sourceFile, req.VaultPath, req.ParseOptions)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrValidationFailed, "failed to parse source file", err).WithSuggestion("Fix the file content and try again")
	}
	var target *model.Section
	for _, section := range doc.Sections {
		if section != nil && section.ID == oldSectionID {
			target = section
			break
		}
	}
	if target == nil {
		return nil, svcerr.New(codes.ErrRefNotFound, fmt.Sprintf("section not found: %s", oldSectionID)).WithSuggestion("Run 'rvn reindex' if the index is stale")
	}

	lines := strings.Split(content, "\n")
	if target.LineStart < 1 || target.LineStart > len(lines) {
		return nil, svcerr.New(codes.ErrInternal, fmt.Sprintf("section heading line %d is out of range", target.LineStart)).WithSuggestion("Run 'rvn reindex' and try again")
	}
	lines[target.LineStart-1] = strings.Repeat("#", target.Level) + " " + newTitle
	updatedContent := strings.Join(lines, "\n")

	// Re-parse the updated content so the new slug is derived exactly as the
	// indexer would derive it (including duplicate suffixes).
	updatedDoc, err := parser.ParseDocumentWithOptions(updatedContent, sourceFile, req.VaultPath, req.ParseOptions)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrValidationFailed, "failed to parse renamed content", err).WithSuggestion("Check the new heading text and try again")
	}
	var renamed *model.Section
	for _, section := range updatedDoc.Sections {
		if section != nil && section.LineStart == target.LineStart {
			renamed = section
			break
		}
	}
	if renamed == nil {
		return nil, svcerr.New(codes.ErrValidationFailed, "renamed heading no longer parses as a section").WithSuggestion("Check the new heading text and try again")
	}

	expectedSlug := parser.Slugify(newTitle)
	if expectedSlug == "" {
		expectedSlug = "section"
	}
	if renamed.Slug != expectedSlug {
		return nil, svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("renaming would create a duplicate section slug: '%s' already exists in %s", expectedSlug, fileID)).WithSuggestion("Choose a heading that is unique within the file")
	}

	// Renaming must not shift any other section's slug (e.g. by introducing a
	// duplicate heading that pushes an existing section to a -2 suffix).
	originalSlugs := make(map[int]string, len(doc.Sections))
	for _, section := range doc.Sections {
		if section != nil {
			originalSlugs[section.LineStart] = section.Slug
		}
	}
	for _, section := range updatedDoc.Sections {
		if section == nil || section.LineStart == target.LineStart {
			continue
		}
		if before, seen := originalSlugs[section.LineStart]; seen && before != section.Slug {
			return nil, svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("renaming would create a duplicate section slug: '%s' would change section '%s#%s' to '%s#%s'", expectedSlug, fileID, before, fileID, section.Slug)).WithSuggestion("Choose a heading that is unique within the file")
		}
	}
	newSectionID := renamed.ID

	result := &RenameResult{
		SourceID:       oldSectionID,
		SourceRelative: sourceRelPath,
		DestinationID:  newSectionID,
		DestinationRel: sourceRelPath,
	}

	var db *index.Database
	if err := rt.OpenDB(); err != nil {
		if req.FailOnIndexErr || errors.Is(err, index.ErrIndexRebuildRequired) {
			return nil, svcerr.Wrap(codes.ErrValidationFailed, "failed to open index database for section rename", err).WithSuggestion("Run 'rvn reindex' to rebuild the database")
		}
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to open index database for section rename: %v", err))
	} else {
		db = rt.DB
	}

	rewritesByPath := make(map[string]*fileRewrite)
	var rewriteOrder []*fileRewrite
	updatedRefSeen := make(map[string]struct{})
	addUpdatedRef := func(ref string) {
		if strings.TrimSpace(ref) == "" {
			return
		}
		if _, seen := updatedRefSeen[ref]; seen {
			return
		}
		updatedRefSeen[ref] = struct{}{}
		result.UpdatedRefs = append(result.UpdatedRefs, ref)
	}

	if db != nil && renamed.Slug != oldSlug {
		backlinks, err := db.BacklinksWithRoots(oldSectionID, req.VaultConfig.GetObjectsRoot(), req.VaultConfig.GetPagesRoot())
		if err != nil {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to read backlinks for section rename: %v", err))
		}
		for _, bl := range backlinks {
			oldRaw := strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(bl.TargetRaw), "]]"), "[[")
			base, _, targetIsSection := paths.ParseSectionID(oldRaw)
			if !targetIsSection || base == "" {
				continue
			}
			newRaw := base + "#" + renamed.Slug

			line := 0
			if bl.Line != nil {
				line = *bl.Line
			}

			sourceFileID := bl.SourceID
			if idx := strings.Index(sourceFileID, "#"); idx >= 0 {
				sourceFileID = sourceFileID[:idx]
			}
			if idx := strings.Index(sourceFileID, ":trait:"); idx >= 0 {
				sourceFileID = sourceFileID[:idx]
			}

			if sourceFileID == fileID {
				updatedContent = rewriteSectionRefAtLine(updatedContent, line, oldRaw, newRaw)
				addUpdatedRef(fileID)
				continue
			}

			refFilePath, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, sourceFileID, req.VaultConfig)
			if err != nil {
				result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to update refs in %s: %v", sourceFileID, err))
				continue
			}
			if err := mutationguard.ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, refFilePath); err != nil {
				result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Skipped ref update in %s: %v", sourceFileID, err))
				continue
			}

			rewrite, exists := rewritesByPath[refFilePath]
			if !exists {
				rewrite, err = readFileRewrite(refFilePath, sourceFileID)
				if err != nil {
					result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to read %s for ref update: %v", sourceFileID, err))
					continue
				}
				rewritesByPath[refFilePath] = rewrite
				rewriteOrder = append(rewriteOrder, rewrite)
			}
			updated := rewriteSectionRefAtLine(string(rewrite.updatedContent), line, oldRaw, newRaw)
			if updated != string(rewrite.updatedContent) {
				rewrite.updatedContent = []byte(updated)
				addUpdatedRef(sourceFileID)
			}
		}
	}

	if req.Preview {
		return result, nil
	}

	perm := os.FileMode(0o644)
	if st, err := os.Stat(sourceFile); err == nil {
		perm = st.Mode()
	}
	if err := atomicfile.WriteFile(sourceFile, []byte(updatedContent), perm); err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to write renamed section", err)
	}
	writtenFiles := []string{sourceFile}
	for _, rewrite := range rewriteOrder {
		if string(rewrite.updatedContent) == string(rewrite.content) {
			continue
		}
		if err := atomicfile.WriteFile(rewrite.path, rewrite.updatedContent, rewrite.perm); err != nil {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to update refs in %s: %v", rewrite.reportSourceID, err))
			continue
		}
		writtenFiles = append(writtenFiles, rewrite.path)
	}

	if db != nil && req.Schema != nil {
		for _, path := range writtenFiles {
			result.IndexWarnings = append(result.IndexWarnings, reindexsvc.ProjectFileLocked(rt, path)...)
		}
	}

	return result, nil
}

func resolveSectionReference(req RenameRequest, reference string) (*refresolve.ResolveResult, error) {
	resolved, err := refresolve.Resolve(reference, req.Runtime, false)
	if err != nil {
		var ambiguousErr *refresolve.AmbiguousRefError
		if errors.As(err, &ambiguousErr) {
			return nil, svcerr.Wrap(codes.ErrRefAmbiguous, ambiguousErr.Error(), err).WithSuggestion("Use a full section ID/path to disambiguate").WithDetails(map[string]any{"matches": ambiguousErr.Matches})
		}
		var notFoundErr *refresolve.RefNotFoundError
		if errors.As(err, &notFoundErr) {
			return nil, svcerr.Wrap(codes.ErrRefNotFound, notFoundErr.Error(), err).WithSuggestion("Check the section reference and run 'rvn reindex' if needed")
		}
		return nil, svcerr.Wrap(codes.ErrInternal, fmt.Sprintf("failed to resolve section reference: %v", err), err).WithSuggestion("Check the section reference and run 'rvn reindex' if needed")
	}
	if !resolved.IsSection {
		return nil, svcerr.New(codes.ErrInvalidInput, "source must be a section ID").WithSuggestion("Use a section ID like project/website#tasks")
	}
	return resolved, nil
}

func normalizeMutationError(err error) error {
	if _, ok := svcerr.AsError(err); ok {
		return err
	}
	return svcerr.Wrap(codes.ErrInternal, err.Error(), err)
}

func readFileRewrite(path, reportSourceID string) (*fileRewrite, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	perm := os.FileMode(0)
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode()
	}
	return &fileRewrite{
		path:           path,
		content:        content,
		perm:           perm,
		reportSourceID: reportSourceID,
		updatedContent: append([]byte(nil), content...),
	}, nil
}

// rewriteSectionRefAtLine rewrites an exact section target, preserving the
// authored object base (including aliases) while replacing only its fragment.
func rewriteSectionRefAtLine(content string, line int, oldRaw, newRaw string) string {
	oldBase, oldFragment, oldIsSection := paths.ParseSectionID(strings.TrimSpace(oldRaw))
	if !oldIsSection || oldBase == "" || oldFragment == "" || oldRaw == newRaw {
		return content
	}

	decide := func(occ refs.Occurrence) (string, bool) {
		if occ.Base == oldBase && occ.HasFragment && occ.Fragment == oldFragment {
			return newRaw, true
		}
		return "", false
	}
	updated, _ := refs.RewriteContentAtLine(content, line, decide)
	return rewriteMarkdownSectionRefAtLine(updated, line, oldRaw, newRaw)
}

// rewriteMarkdownSectionRefAtLine preserves section rename's compatibility
// with direct Markdown links that share an indexed backlink line. Direct
// Markdown links are deliberately not Raven references, so refs.RewriteContent
// does not rewrite them. Limiting this fallback to the indexed line also keeps
// unindexed examples in fenced code blocks untouched.
func rewriteMarkdownSectionRefAtLine(content string, line int, oldRaw, newRaw string) string {
	if line <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	idx := line - 1
	if idx < 0 || idx >= len(lines) {
		return content
	}
	lines[idx] = strings.NewReplacer(
		"]("+oldRaw+")", "]("+newRaw+")",
		"](<"+oldRaw+">)", "](<"+newRaw+">)",
	).Replace(lines[idx])
	return strings.Join(lines, "\n")
}
