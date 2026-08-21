package sectionsvc

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type DeleteRequest struct {
	VaultPath      string
	VaultConfig    *config.VaultConfig
	Schema         *schema.Schema
	Reference      string
	Preview        bool
	ParseOptions   *parser.ParseOptions
	FailOnIndexErr bool
	Runtime        *vaultruntime.Runtime
}

type DeleteResult struct {
	SectionID       string
	FileRelative    string
	LineStart       int
	LineEnd         int
	RemovedContent  string
	DeletedSections []string
	Backlinks       []model.Reference
	WarningMessages []string
	IndexWarnings   []IndexWarning
}

// Delete removes one heading and its complete subtree. References from outside
// the deleted range are reported as backlinks and intentionally left unchanged:
// Raven cannot infer a safe replacement target for a deleted section.
func Delete(req DeleteRequest) (*DeleteResult, error) {
	rt, owned := vaultruntime.FromRequest(req.Runtime, req.VaultPath, req.VaultConfig, req.Schema, req.ParseOptions)
	if owned {
		defer rt.Close()
	}
	ctx, err := newLifecycleContext(rt, req.VaultPath, req.VaultConfig, req.Schema, req.ParseOptions)
	if err != nil {
		return nil, err
	}

	reference := strings.TrimSpace(req.Reference)
	fileID, slug, isSection := paths.ParseSectionID(reference)
	if !isSection || fileID == "" || slug == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("section reference required: %s", reference)).
			WithSuggestion("Use a section ID like project/website#tasks")
	}

	resolved, err := ctx.resolveSection(reference)
	if err != nil {
		return nil, err
	}
	state, err := ctx.loadDocument(resolved.FilePath, resolved.FileObjectID)
	if err != nil {
		return nil, err
	}
	target := state.sectionsByID[resolved.ObjectID]
	if target == nil {
		return nil, svcerr.New(codes.ErrRefNotFound, fmt.Sprintf("section not found: %s", resolved.ObjectID)).
			WithSuggestion("Run 'rvn reindex' if the index is stale")
	}

	start := target.LineStart - 1
	end := sectionSubtreeEnd(target, len(state.lines))
	if start < 0 || start >= end || end > len(state.lines) {
		return nil, svcerr.New(codes.ErrInternal, "section subtree range is invalid").
			WithSuggestion("Run 'rvn reindex' and try again")
	}

	deletedSections := sectionsInRange(state.doc.Sections, target.LineStart, end)
	remaining := append([]trackedLine(nil), state.lines[:start]...)
	remaining = append(remaining, state.lines[end:]...)
	updatedContent := joinTrackedLines(remaining, state.trailingNewline)
	updatedDoc, err := parser.ParseDocumentWithOptions(updatedContent, state.filePath, ctx.vaultPath, ctx.parseOptions)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrValidationFailed, "failed to parse content after section deletion", err).
			WithSuggestion("Fix the file content and try again")
	}
	if err := validateSurvivingSectionSlugs(state, updatedDoc, remaining, target.LineStart, end); err != nil {
		return nil, err
	}

	removedLines := state.lines[start:end]
	removedTrailingNewline := end == len(state.lines) && state.trailingNewline
	result := &DeleteResult{
		SectionID:       target.ID,
		FileRelative:    state.fileRelative,
		LineStart:       target.LineStart,
		LineEnd:         end,
		RemovedContent:  joinTrackedLines(removedLines, removedTrailingNewline),
		DeletedSections: deletedSections,
	}

	if err := rt.OpenDB(); err != nil {
		if req.FailOnIndexErr || errors.Is(err, index.ErrIndexRebuildRequired) {
			return nil, svcerr.Wrap(codes.ErrValidationFailed, "failed to open index database for section deletion", err).
				WithSuggestion("Run 'rvn reindex' to rebuild the database")
		}
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to open index database for section deletion: %v", err))
	} else {
		result.Backlinks, err = sectionDeletionBacklinks(
			rt.DB,
			deletedSections,
			state.fileRelative,
			target.LineStart,
			end,
			req.VaultConfig.GetObjectsRoot(),
			req.VaultConfig.GetPagesRoot(),
		)
		if err != nil {
			if req.FailOnIndexErr {
				return nil, svcerr.Wrap(codes.ErrDatabase, "failed to read backlinks for section deletion", err).
					WithSuggestion("Run 'rvn reindex' to rebuild the database")
			}
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Failed to read backlinks for section deletion: %v", err))
		}
	}

	if req.Preview {
		return result, nil
	}

	warnings, indexWarnings, err := ctx.writeAndReindex(state.filePath, updatedContent, req.FailOnIndexErr)
	if err != nil {
		return nil, err
	}
	result.WarningMessages = append(result.WarningMessages, warnings...)
	result.IndexWarnings = indexWarnings
	return result, nil
}

func sectionsInRange(sections []*model.Section, start, end int) []string {
	within := make([]*model.Section, 0)
	for _, section := range sections {
		if section != nil && section.LineStart >= start && section.LineStart <= end {
			within = append(within, section)
		}
	}
	sort.Slice(within, func(i, j int) bool {
		return within[i].LineStart < within[j].LineStart
	})
	ids := make([]string, len(within))
	for i, section := range within {
		ids[i] = section.ID
	}
	return ids
}

func validateSurvivingSectionSlugs(
	state *documentState,
	updatedDoc *parser.ParsedDocument,
	updatedLines []trackedLine,
	removedStart,
	removedEnd int,
) error {
	originalByLine := make(map[int]*model.Section, len(state.doc.Sections))
	for _, section := range state.doc.Sections {
		if section == nil || (section.LineStart >= removedStart && section.LineStart <= removedEnd) {
			continue
		}
		originalByLine[section.LineStart] = section
	}

	seen := make(map[int]bool, len(originalByLine))
	for _, section := range updatedDoc.Sections {
		if section == nil || section.LineStart < 1 || section.LineStart > len(updatedLines) {
			return svcerr.New(codes.ErrInternal, "updated section line is out of range")
		}
		originalLine := updatedLines[section.LineStart-1].originalLine
		original := originalByLine[originalLine]
		if original == nil {
			continue
		}
		seen[originalLine] = true
		if section.Slug != original.Slug {
			return svcerr.New(
				codes.ErrValidationFailed,
				fmt.Sprintf("section deletion would shift slug '%s' to '%s'", original.Slug, section.Slug),
			).
				WithSuggestion("Rename duplicate headings before deleting this section").
				WithDetails(map[string]any{"section": original.ID, "new_slug": section.Slug})
		}
	}
	if len(seen) != len(originalByLine) {
		return svcerr.New(codes.ErrValidationFailed, "section deletion would change the surviving outline").
			WithSuggestion("Fix the heading structure and try again")
	}
	return nil
}

func sectionDeletionBacklinks(
	db *index.Database,
	deletedSections []string,
	sourceFile string,
	start,
	end int,
	objectRoot,
	pageRoot string,
) ([]model.Reference, error) {
	deletedSet := make(map[string]struct{}, len(deletedSections))
	for _, sectionID := range deletedSections {
		deletedSet[sectionID] = struct{}{}
	}

	var backlinks []model.Reference
	seen := make(map[string]struct{})
	for _, sectionID := range deletedSections {
		refsToSection, err := db.BacklinksWithRoots(sectionID, objectRoot, pageRoot)
		if err != nil {
			return nil, err
		}
		for _, backlink := range refsToSection {
			if _, deletedSource := deletedSet[backlink.SourceID]; deletedSource {
				continue
			}
			if backlink.FilePath == sourceFile && backlink.Line != nil && *backlink.Line >= start && *backlink.Line <= end {
				continue
			}
			key := fmt.Sprintf(
				"%s\x00%s\x00%d\x00%d\x00%d\x00%s",
				backlink.SourceID,
				backlink.FilePath,
				backlink.LineOrZero(),
				backlink.PositionStartOrZero(),
				backlink.PositionEndOrZero(),
				backlink.TargetRaw,
			)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			backlinks = append(backlinks, backlink)
		}
	}
	sort.Slice(backlinks, func(i, j int) bool {
		left, right := backlinks[i], backlinks[j]
		if left.FilePath != right.FilePath {
			return left.FilePath < right.FilePath
		}
		if left.LineOrZero() != right.LineOrZero() {
			return left.LineOrZero() < right.LineOrZero()
		}
		if left.PositionStartOrZero() != right.PositionStartOrZero() {
			return left.PositionStartOrZero() < right.PositionStartOrZero()
		}
		if left.TargetRaw != right.TargetRaw {
			return left.TargetRaw < right.TargetRaw
		}
		return left.SourceID < right.SourceID
	})
	return backlinks, nil
}
