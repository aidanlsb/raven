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
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
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
}

type RenameResult struct {
	SourceID        string
	SourceRelative  string
	DestinationID   string
	DestinationRel  string
	UpdatedRefs     []string
	WarningMessages []string
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
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(codes.ErrInvalidInput, "vault path is required", "", nil, nil)
	}
	if req.VaultConfig == nil {
		return nil, newError(codes.ErrValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}

	reference := strings.TrimSpace(req.Reference)
	fileID, oldSlug, isSection := paths.ParseSectionID(reference)
	if !isSection || fileID == "" || oldSlug == "" {
		return nil, newError(
			codes.ErrInvalidInput,
			fmt.Sprintf("invalid section ID: %s", reference),
			"Use a section ID like project/website#tasks",
			nil,
			nil,
		)
	}

	newTitle := strings.TrimSpace(req.NewHeadingText)
	if newTitle == "" {
		return nil, newError(codes.ErrInvalidInput, "new heading text is required", `Usage: rvn section rename <file#section> "<new heading text>"`, nil, nil)
	}
	if strings.HasPrefix(newTitle, "#") {
		return nil, newError(
			codes.ErrInvalidInput,
			"section destination must be the new heading text, not a markdown heading or fragment",
			`Pass plain heading text, e.g. rvn section rename project/website#tasks "Completed Tasks"; the heading level is preserved`,
			nil,
			nil,
		)
	}
	if _, _, destinationIsSection := paths.ParseSectionID(newTitle); destinationIsSection {
		return nil, newError(
			codes.ErrInvalidInput,
			"section destination must be the new heading text, not a section ID",
			`Pass plain heading text, e.g. rvn section rename project/website#tasks "Completed Tasks"`,
			nil,
			nil,
		)
	}

	resolved, err := resolveSectionReference(req, reference)
	if err != nil {
		return nil, err
	}
	oldSectionID := resolved.ObjectID
	fileID, oldSlug, isSection = paths.ParseSectionID(oldSectionID)
	if !isSection || oldSlug == "" {
		return nil, newError(codes.ErrInvalidInput, fmt.Sprintf("invalid section ID: %s", oldSectionID), "Use a section ID like project/website#tasks", nil, nil)
	}

	sourceFile := resolved.FilePath
	if err := objectsvc.ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, sourceFile); err != nil {
		return nil, normalizeMutationError(err)
	}
	sourceRelPath, err := filepath.Rel(req.VaultPath, sourceFile)
	if err != nil {
		return nil, newError(codes.ErrInternal, "failed to resolve source path", "", nil, err)
	}
	sourceRelPath = paths.NormalizeVaultRelPath(sourceRelPath)

	contentBytes, err := os.ReadFile(sourceFile)
	if err != nil {
		return nil, newError(codes.ErrFileRead, "failed to read source file", "", nil, err)
	}
	content := string(contentBytes)

	doc, err := parser.ParseDocumentWithOptions(content, sourceFile, req.VaultPath, req.ParseOptions)
	if err != nil {
		return nil, newError(codes.ErrValidationFailed, "failed to parse source file", "Fix the file content and try again", nil, err)
	}
	var target *model.Section
	for _, section := range doc.Sections {
		if section != nil && section.ID == oldSectionID {
			target = section
			break
		}
	}
	if target == nil {
		return nil, newError(codes.ErrRefNotFound, fmt.Sprintf("section not found: %s", oldSectionID), "Run 'rvn reindex' if the index is stale", nil, nil)
	}

	lines := strings.Split(content, "\n")
	if target.LineStart < 1 || target.LineStart > len(lines) {
		return nil, newError(codes.ErrInternal, fmt.Sprintf("section heading line %d is out of range", target.LineStart), "Run 'rvn reindex' and try again", nil, nil)
	}
	lines[target.LineStart-1] = strings.Repeat("#", target.Level) + " " + newTitle
	updatedContent := strings.Join(lines, "\n")

	// Re-parse the updated content so the new slug is derived exactly as the
	// indexer would derive it (including duplicate suffixes).
	updatedDoc, err := parser.ParseDocumentWithOptions(updatedContent, sourceFile, req.VaultPath, req.ParseOptions)
	if err != nil {
		return nil, newError(codes.ErrValidationFailed, "failed to parse renamed content", "Check the new heading text and try again", nil, err)
	}
	var renamed *model.Section
	for _, section := range updatedDoc.Sections {
		if section != nil && section.LineStart == target.LineStart {
			renamed = section
			break
		}
	}
	if renamed == nil {
		return nil, newError(codes.ErrValidationFailed, "renamed heading no longer parses as a section", "Check the new heading text and try again", nil, nil)
	}

	expectedSlug := parser.Slugify(newTitle)
	if expectedSlug == "" {
		expectedSlug = "section"
	}
	if renamed.Slug != expectedSlug {
		return nil, newError(
			codes.ErrValidationFailed,
			fmt.Sprintf("renaming would create a duplicate section slug: '%s' already exists in %s", expectedSlug, fileID),
			"Choose a heading that is unique within the file",
			nil,
			nil,
		)
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
			return nil, newError(
				codes.ErrValidationFailed,
				fmt.Sprintf("renaming would create a duplicate section slug: '%s' would change section '%s#%s' to '%s#%s'", expectedSlug, fileID, before, fileID, section.Slug),
				"Choose a heading that is unique within the file",
				nil,
				nil,
			)
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
	db, err = index.Open(req.VaultPath)
	if err != nil {
		if req.FailOnIndexErr || errors.Is(err, index.ErrIndexRebuildRequired) {
			return nil, newError(codes.ErrValidationFailed, "failed to open index database for section rename", "Run 'rvn reindex' to rebuild the database", nil, err)
		}
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to open index database for section rename: %v", err))
	} else {
		defer db.Close()
		db.SetDailyDirectory(req.VaultConfig.GetDailyDirectory())
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
				updatedContent = replaceSectionRefAtLine(updatedContent, line, oldRaw, newRaw)
				addUpdatedRef(fileID)
				continue
			}

			refFilePath, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, sourceFileID, req.VaultConfig)
			if err != nil {
				result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to update refs in %s: %v", sourceFileID, err))
				continue
			}
			if err := objectsvc.ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, refFilePath); err != nil {
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
			updated := replaceSectionRefAtLine(string(rewrite.updatedContent), line, oldRaw, newRaw)
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
		return nil, newError(codes.ErrFileWrite, "failed to write renamed section", "", nil, err)
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
			if warning := reindexRenamedFile(db, req, path); warning != "" {
				result.WarningMessages = append(result.WarningMessages, warning)
			}
		}
	}

	return result, nil
}

func resolveSectionReference(req RenameRequest, reference string) (*readsvc.ResolveResult, error) {
	rt := &readsvc.Runtime{
		VaultPath: req.VaultPath,
		VaultCfg:  req.VaultConfig,
		Schema:    req.Schema,
	}
	resolved, err := readsvc.ResolveReference(reference, rt, false)
	if err != nil {
		var ambiguousErr *readsvc.AmbiguousRefError
		if errors.As(err, &ambiguousErr) {
			return nil, newError(
				codes.ErrRefAmbiguous,
				ambiguousErr.Error(),
				"Use a full section ID/path to disambiguate",
				map[string]any{"matches": ambiguousErr.Matches},
				err,
			)
		}
		var notFoundErr *readsvc.RefNotFoundError
		if errors.As(err, &notFoundErr) {
			return nil, newError(
				codes.ErrRefNotFound,
				notFoundErr.Error(),
				"Check the section reference and run 'rvn reindex' if needed",
				nil,
				err,
			)
		}
		return nil, newError(
			codes.ErrInternal,
			fmt.Sprintf("failed to resolve section reference: %v", err),
			"Check the section reference and run 'rvn reindex' if needed",
			nil,
			err,
		)
	}
	if !resolved.IsSection {
		return nil, newError(codes.ErrInvalidInput, "source must be a section ID", "Use a section ID like project/website#tasks", nil, nil)
	}
	return resolved, nil
}

func normalizeMutationError(err error) error {
	if _, ok := svcerr.AsError(err); ok {
		return err
	}
	return newError(codes.ErrInternal, err.Error(), "", nil, err)
}

func newError(code codes.ErrorCode, message, suggestion string, details map[string]any, err error) *svcerr.Error {
	return &svcerr.Error{
		Code:       code,
		Message:    message,
		Suggestion: suggestion,
		Details:    details,
		Err:        err,
	}
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

// replaceSectionRefAtLine replaces wikilink and markdown-link occurrences of
// oldRaw with newRaw, preferring the given 1-based line and falling back to the
// whole content when the line does not contain the reference.
func replaceSectionRefAtLine(content string, line int, oldRaw, newRaw string) string {
	if oldRaw == "" || oldRaw == newRaw {
		return content
	}
	if line > 0 {
		lines := strings.Split(content, "\n")
		idx := line - 1
		if idx >= 0 && idx < len(lines) {
			updated := replaceSectionRefVariants(lines[idx], oldRaw, newRaw)
			if updated != lines[idx] {
				lines[idx] = updated
				return strings.Join(lines, "\n")
			}
		}
	}
	return replaceSectionRefVariants(content, oldRaw, newRaw)
}

func replaceSectionRefVariants(content, oldRaw, newRaw string) string {
	replacer := strings.NewReplacer(
		"[["+oldRaw+"]]", "[["+newRaw+"]]",
		"[["+oldRaw+"|", "[["+newRaw+"|",
		"]("+oldRaw+")", "]("+newRaw+")",
		"](<"+oldRaw+">)", "](<"+newRaw+">)",
	)
	return replacer.Replace(content)
}

func reindexRenamedFile(db *index.Database, req RenameRequest, filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Sprintf("Failed to reindex %s: %v", filePath, err)
	}
	doc, err := parser.ParseDocumentWithOptions(string(content), filePath, req.VaultPath, req.ParseOptions)
	if err != nil || doc == nil {
		return fmt.Sprintf("Failed to reindex %s: %v", filePath, err)
	}
	var mtime int64
	if st, err := os.Stat(filePath); err == nil {
		mtime = st.ModTime().Unix()
	}
	if err := db.IndexDocumentWithMtime(doc, req.Schema, mtime); err != nil {
		return fmt.Sprintf("Failed to reindex %s: %v", filePath, err)
	}
	return ""
}
