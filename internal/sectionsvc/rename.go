package sectionsvc

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type RenameRequest struct {
	Reference      string
	NewHeadingText string
	Preview        bool
	FailOnIndexErr bool
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

// Rename renames a section heading in place and rewrites all inbound
// references from [[...#old-slug]] to [[...#new-slug]].
//
// NewHeadingText is plain heading text. The heading level is preserved and the
// new slug is derived using the same rules the parser applies to headings.
func Rename(rt *vaultruntime.Runtime, req RenameRequest) (*RenameResult, error) {
	unlock, err := lockSectionMutation(rt, req.Preview)
	if err != nil {
		return nil, err
	}
	defer unlock()

	reference := strings.TrimSpace(req.Reference)
	fileID, oldSlug, isSection := paths.ParseSectionID(reference)
	if !isSection || fileID == "" || oldSlug == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid section ID: %s", reference)).WithSuggestion("Use a section ID like project/website#tasks")
	}

	newTitle, err := validateRenameTitle(req.NewHeadingText)
	if err != nil {
		return nil, err
	}

	state, target, err := loadResolvedSection(rt, reference)
	if err != nil {
		return nil, err
	}
	oldSectionID := target.ID
	fileID, oldSlug, isSection = paths.ParseSectionID(oldSectionID)
	if !isSection || oldSlug == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("invalid section ID: %s", oldSectionID)).WithSuggestion("Use a section ID like project/website#tasks")
	}

	updatedContent, renamed, err := rewriteHeading(rt, state, target, newTitle)
	if err != nil {
		return nil, err
	}

	result := &RenameResult{
		SourceID:       oldSectionID,
		SourceRelative: state.fileRelative,
		DestinationID:  renamed.ID,
		DestinationRel: state.fileRelative,
	}

	db, warnings, err := openSectionIndex(rt, req.FailOnIndexErr, "rename")
	if err != nil {
		return nil, err
	}
	result.WarningMessages = append(result.WarningMessages, warnings...)

	inbound := planInboundSectionRewrites(rt, db, fileID, oldSectionID, oldSlug, renamed.Slug, updatedContent)
	result.UpdatedRefs = append(result.UpdatedRefs, inbound.updatedRefs...)
	result.WarningMessages = append(result.WarningMessages, inbound.warnings...)

	if req.Preview {
		return result, nil
	}

	writes := []pendingWrite{{
		path:    state.filePath,
		content: []byte(inbound.sourceContent),
	}}
	for _, rewrite := range inbound.files {
		if string(rewrite.updatedContent) == string(rewrite.content) {
			continue
		}
		writes = append(writes, pendingWrite{
			path:     rewrite.path,
			content:  rewrite.updatedContent,
			perm:     rewrite.perm,
			reportID: rewrite.reportSourceID,
			optional: true,
		})
	}

	writeWarnings, indexWarnings, err := writeAndReindexFiles(rt, writes, req.FailOnIndexErr)
	if err != nil {
		return nil, err
	}
	result.WarningMessages = append(result.WarningMessages, writeWarnings...)
	result.IndexWarnings = indexWarnings
	return result, nil
}

func validateRenameTitle(raw string) (string, error) {
	newTitle := strings.TrimSpace(raw)
	if newTitle == "" {
		return "", svcerr.New(codes.ErrInvalidInput, "new heading text is required").WithSuggestion(`Usage: rvn section rename <file#section> "<new heading text>"`)
	}
	if strings.HasPrefix(newTitle, "#") {
		return "", svcerr.New(codes.ErrInvalidInput, "section destination must be the new heading text, not a markdown heading or fragment").WithSuggestion(`Pass plain heading text, e.g. rvn section rename project/website#tasks "Completed Tasks"; the heading level is preserved`)
	}
	if _, _, destinationIsSection := paths.ParseSectionID(newTitle); destinationIsSection {
		return "", svcerr.New(codes.ErrInvalidInput, "section destination must be the new heading text, not a section ID").WithSuggestion(`Pass plain heading text, e.g. rvn section rename project/website#tasks "Completed Tasks"`)
	}
	return newTitle, nil
}

func rewriteHeading(rt *vaultruntime.Runtime, state *documentState, target *model.Section, newTitle string) (string, *model.Section, error) {
	if target.LineStart < 1 || target.LineStart > len(state.lines) {
		return "", nil, svcerr.New(codes.ErrInternal, fmt.Sprintf("section heading line %d is out of range", target.LineStart)).WithSuggestion("Run 'rvn reindex' and try again")
	}

	updatedLines := append([]trackedLine(nil), state.lines...)
	updatedLines[target.LineStart-1].text = strings.Repeat("#", target.Level) + " " + newTitle
	updatedContent := joinTrackedLines(updatedLines, state.trailingNewline)

	updatedDoc, err := parseDocumentContent(rt, state.filePath, updatedContent, "failed to parse renamed content", "Check the new heading text and try again")
	if err != nil {
		return "", nil, err
	}
	renamed := sectionAtLine(updatedDoc.Sections, target.LineStart)
	if renamed == nil {
		return "", nil, svcerr.New(codes.ErrValidationFailed, "renamed heading no longer parses as a section").WithSuggestion("Check the new heading text and try again")
	}

	expectedSlug := headingSlug(newTitle)
	if renamed.Slug != expectedSlug {
		return "", nil, svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("renaming would create a duplicate section slug: '%s' already exists in %s", expectedSlug, state.fileID)).WithSuggestion("Choose a heading that is unique within the file")
	}

	include := func(section *model.Section) bool {
		return section.LineStart != target.LineStart
	}
	if err := validatePreservedSlugs(
		state,
		updatedDoc,
		updatedLines,
		include,
		target.LineStart,
		func(original, updated *model.Section) error {
			return svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("renaming would create a duplicate section slug: '%s' would change section '%s#%s' to '%s#%s'", expectedSlug, state.fileID, original.Slug, state.fileID, updated.Slug)).WithSuggestion("Choose a heading that is unique within the file")
		},
		svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("renaming would create a duplicate section slug: '%s' already exists in %s", expectedSlug, state.fileID)).WithSuggestion("Choose a heading that is unique within the file"),
	); err != nil {
		return "", nil, err
	}

	return updatedContent, renamed, nil
}
