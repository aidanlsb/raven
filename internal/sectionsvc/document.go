package sectionsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type documentState struct {
	filePath        string
	fileRelative    string
	fileID          string
	trailingNewline bool
	lines           []trackedLine
	doc             *parser.ParsedDocument
	sectionsByID    map[string]*model.Section
}

type trackedLine struct {
	text         string
	originalLine int
}

type preservedSlugMatch struct {
	original *model.Section
	updated  *model.Section
}

type pendingWrite struct {
	path     string
	content  []byte
	perm     os.FileMode
	reportID string
	optional bool
}

func loadDocument(rt *vaultruntime.Runtime, filePath, fileID string) (*documentState, error) {
	if err := mutationguard.ValidateContentMutationFilePath(rt.VaultPath, rt.VaultCfg, filePath); err != nil {
		return nil, normalizeMutationError(err)
	}
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to read section file", err)
	}
	content := string(contentBytes)
	doc, err := parseDocumentContent(rt, filePath, content, "failed to parse section file", "Fix the file content and try again")
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(rt.VaultPath, filePath)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrInternal, "failed to resolve section file path", err)
	}
	lines, trailingNewline := splitTrackedLines(content)
	sectionsByID := make(map[string]*model.Section, len(doc.Sections))
	for _, section := range doc.Sections {
		if section != nil {
			sectionsByID[section.ID] = section
		}
	}
	return &documentState{
		filePath:        filePath,
		fileRelative:    paths.NormalizeVaultRelPath(relative),
		fileID:          fileID,
		trailingNewline: trailingNewline,
		lines:           lines,
		doc:             doc,
		sectionsByID:    sectionsByID,
	}, nil
}

func parseDocumentContent(rt *vaultruntime.Runtime, filePath, content, errMsg, suggestion string) (*parser.ParsedDocument, error) {
	doc, err := parser.ParseDocumentWithOptions(content, filePath, rt.VaultPath, rt.ParseOptions)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrValidationFailed, errMsg, err).WithSuggestion(suggestion)
	}
	return doc, nil
}

func headingSlug(title string) string {
	slug := parser.Slugify(title)
	if slug == "" {
		return "section"
	}
	return slug
}

func sectionSubtreeEnd(section *model.Section, lineCount int) int {
	if section != nil && section.SubtreeLineEnd != nil {
		return *section.SubtreeLineEnd
	}
	return lineCount
}

func splitTrackedLines(content string) ([]trackedLine, bool) {
	if content == "" {
		return nil, false
	}
	trailingNewline := strings.HasSuffix(content, "\n")
	if trailingNewline {
		content = strings.TrimSuffix(content, "\n")
	}
	rawLines := strings.Split(content, "\n")
	lines := make([]trackedLine, len(rawLines))
	for i, line := range rawLines {
		lines[i] = trackedLine{text: line, originalLine: i + 1}
	}
	return lines, trailingNewline
}

func insertTrackedLines(lines []trackedLine, index int, inserted []trackedLine) []trackedLine {
	if index < 0 {
		index = 0
	}
	if index > len(lines) {
		index = len(lines)
	}
	result := make([]trackedLine, 0, len(lines)+len(inserted))
	result = append(result, lines[:index]...)
	result = append(result, inserted...)
	result = append(result, lines[index:]...)
	return result
}

func joinTrackedLines(lines []trackedLine, trailingNewline bool) string {
	raw := make([]string, len(lines))
	for i := range lines {
		raw[i] = lines[i].text
	}
	content := strings.Join(raw, "\n")
	if trailingNewline {
		content += "\n"
	}
	return content
}

func sectionAtLine(sections []*model.Section, line int) *model.Section {
	for _, section := range sections {
		if section != nil && section.LineStart == line {
			return section
		}
	}
	return nil
}

func trackedLineNumber(lines []trackedLine, originalLine int) int {
	for i, line := range lines {
		if line.originalLine == originalLine {
			return i + 1
		}
	}
	return 0
}

func mapPreservedSections(
	state *documentState,
	updatedDoc *parser.ParsedDocument,
	updatedLines []trackedLine,
	includeOriginal func(*model.Section) bool,
	skipUpdatedLine int,
) ([]preservedSlugMatch, error) {
	originalByLine := make(map[int]*model.Section, len(state.doc.Sections))
	for _, section := range state.doc.Sections {
		if section == nil || (includeOriginal != nil && !includeOriginal(section)) {
			continue
		}
		originalByLine[section.LineStart] = section
	}

	matches := make([]preservedSlugMatch, 0, len(originalByLine))
	seen := make(map[int]bool, len(originalByLine))
	for _, section := range updatedDoc.Sections {
		if section == nil || section.LineStart == skipUpdatedLine {
			continue
		}
		if section.LineStart < 1 || section.LineStart > len(updatedLines) {
			return nil, svcerr.New(codes.ErrInternal, "updated section line is out of range")
		}
		originalLine := updatedLines[section.LineStart-1].originalLine
		original := originalByLine[originalLine]
		if original == nil {
			continue
		}
		if seen[originalLine] {
			continue
		}
		seen[originalLine] = true
		matches = append(matches, preservedSlugMatch{original: original, updated: section})
	}
	return matches, nil
}

func validatePreservedSlugs(
	state *documentState,
	updatedDoc *parser.ParsedDocument,
	updatedLines []trackedLine,
	includeOriginal func(*model.Section) bool,
	skipUpdatedLine int,
	shiftErr func(original, updated *model.Section) error,
	missingErr error,
) error {
	matches, err := mapPreservedSections(state, updatedDoc, updatedLines, includeOriginal, skipUpdatedLine)
	if err != nil {
		return err
	}
	expected := 0
	for _, section := range state.doc.Sections {
		if section == nil || (includeOriginal != nil && !includeOriginal(section)) {
			continue
		}
		expected++
	}
	for _, match := range matches {
		if match.updated.Slug != match.original.Slug {
			return shiftErr(match.original, match.updated)
		}
	}
	if len(matches) != expected {
		return missingErr
	}
	return nil
}

func validateOriginalSectionSlugs(state *documentState, updatedDoc *parser.ParsedDocument, updatedLines []trackedLine, createdLine int) error {
	return validatePreservedSlugs(
		state,
		updatedDoc,
		updatedLines,
		nil,
		createdLine,
		func(original, updated *model.Section) error {
			return svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("section placement would shift slug '%s' to '%s'", original.Slug, updated.Slug)).
				WithSuggestion("Choose a unique heading title or a placement that preserves section identities").
				WithDetails(map[string]any{"section": original.ID, "new_slug": updated.Slug})
		},
		svcerr.New(codes.ErrValidationFailed, "section placement would change the existing outline").
			WithSuggestion("Choose a structurally compatible placement"),
	)
}

func openSectionIndex(rt *vaultruntime.Runtime, failOnIndexErr bool, operation string) (*index.Database, []string, error) {
	if err := rt.OpenDB(); err != nil {
		if failOnIndexErr || errors.Is(err, index.ErrIndexRebuildRequired) {
			return nil, nil, svcerr.Wrap(codes.ErrValidationFailed, fmt.Sprintf("failed to open index database for section %s", operation), err).WithSuggestion("Run 'rvn reindex' to rebuild the database")
		}
		return nil, []string{fmt.Sprintf("Failed to open index database for section %s: %v", operation, err)}, nil
	}
	return rt.DB, nil, nil
}

// writeDocument applies one durable section-file write and records it on a
// ChangeSet. Callers project the ChangeSet through commandimpl.applyChangeSet;
// this helper does not reindex.
func writeDocument(rt *vaultruntime.Runtime, filePath, content string) (mutation.ChangeSet, []string, error) {
	return writePendingFiles(rt, []pendingWrite{{
		path:    filePath,
		content: []byte(content),
	}})
}

// writePendingFiles applies planned durable writes and records each successful
// path on a ChangeSet. Optional inbound-ref writes warn instead of failing the
// mutation. Index projection is the caller's job via applyChangeSet.
func writePendingFiles(rt *vaultruntime.Runtime, writes []pendingWrite) (mutation.ChangeSet, []string, error) {
	changes := mutation.NewChangeSet()
	var warnings []string
	for i := range writes {
		write := writes[i]
		perm := write.perm
		if perm == 0 {
			perm = os.FileMode(0o644)
			if st, statErr := os.Stat(write.path); statErr == nil {
				perm = st.Mode()
			}
		}
		if err := atomicfile.WriteFile(write.path, write.content, perm); err != nil {
			if write.optional {
				reportID := write.reportID
				if reportID == "" {
					reportID = write.path
				}
				warnings = append(warnings, fmt.Sprintf("Failed to update refs in %s: %v", reportID, err))
				continue
			}
			return mutation.ChangeSet{}, nil, svcerr.Wrap(codes.ErrFileWrite, "failed to write section mutation", err)
		}
		if relPath, relErr := filepath.Rel(rt.VaultPath, write.path); relErr == nil {
			changes.AddChanged(relPath)
		}
	}
	return changes, warnings, nil
}
