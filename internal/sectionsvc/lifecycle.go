package sectionsvc

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// Placement selects one structural insertion point. At most one field may be
// set. An empty Placement means the end of the file.
type Placement struct {
	After  string
	Before string
	Under  string
}

type CreateRequest struct {
	FileReference  string
	Title          string
	Level          int
	Placement      Placement
	Preview        bool
	FailOnIndexErr bool
}

type CreateResult struct {
	SectionID       string
	FileRelative    string
	Level           int
	Placement       string
	AnchorID        string
	WarningMessages []string
	IndexWarnings   []reindexsvc.ProjectionWarning
}

type MoveRequest struct {
	Reference      string
	Placement      Placement
	Preview        bool
	FailOnIndexErr bool
}

type MoveResult struct {
	SectionID       string
	FileRelative    string
	Placement       string
	AnchorID        string
	WarningMessages []string
	IndexWarnings   []reindexsvc.ProjectionWarning
}

type placementKind string

const (
	placementEOF    placementKind = "eof"
	placementAfter  placementKind = "after"
	placementBefore placementKind = "before"
	placementUnder  placementKind = "under"
)

type parsedPlacement struct {
	kind      placementKind
	reference string
}

// Create inserts a new, empty heading at a structural section boundary.
func Create(rt *vaultruntime.Runtime, req CreateRequest) (*CreateResult, error) {
	unlock, err := lockSectionMutation(rt, req.Preview)
	if err != nil {
		return nil, err
	}
	defer unlock()

	title, err := validateCreateTitle(req.Title)
	if err != nil {
		return nil, err
	}
	if req.Level < 1 || req.Level > 6 {
		return nil, svcerr.New(codes.ErrInvalidInput, "section level must be between 1 and 6").WithSuggestion("Pass --level N with an integer from 1 through 6")
	}
	placement, err := parsePlacement(req.Placement)
	if err != nil {
		return nil, err
	}

	state, err := loadResolvedFile(rt, req.FileReference)
	if err != nil {
		return nil, err
	}

	insertIndex := len(state.lines)
	var anchor *model.Section
	if placement.kind != placementEOF {
		anchor, err = resolveAnchor(rt, state, placement.reference)
		if err != nil {
			return nil, err
		}
		if err := validatePlacementLevel(req.Level, anchor, placement.kind); err != nil {
			return nil, err
		}
		insertIndex = placementIndex(state.lines, anchor, placement.kind)
	}

	heading := strings.Repeat("#", req.Level) + " " + title
	inserted := headingInsertLines(state.lines, insertIndex, heading)
	updatedLines := insertTrackedLines(state.lines, insertIndex, inserted)
	updatedContent := joinTrackedLines(updatedLines, state.trailingNewline)
	updatedDoc, err := parseDocumentContent(rt, state.filePath, updatedContent, "failed to parse created section", "Check the heading title and level")
	if err != nil {
		return nil, err
	}

	createdLine := insertIndex + len(inserted)
	created := sectionAtLine(updatedDoc.Sections, createdLine)
	if created == nil || created.Title != title || created.Level != req.Level {
		return nil, svcerr.New(codes.ErrValidationFailed, "new heading does not parse as the requested section").WithSuggestion("Use plain, single-line title text")
	}
	expectedSlug := headingSlug(title)
	if created.Slug != expectedSlug {
		return nil, svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("creating would duplicate section slug '%s' in %s", expectedSlug, state.fileID)).WithSuggestion("Choose a heading title that is unique within the file")
	}
	if err := validateOriginalSectionSlugs(state, updatedDoc, updatedLines, createdLine); err != nil {
		return nil, err
	}
	if anchor != nil {
		if err := validateResultingPlacement(created, anchor, placement.kind); err != nil {
			return nil, err
		}
	}

	result := &CreateResult{
		SectionID:    created.ID,
		FileRelative: state.fileRelative,
		Level:        created.Level,
		Placement:    string(placement.kind),
	}
	if anchor != nil {
		result.AnchorID = anchor.ID
	}
	if req.Preview {
		return result, nil
	}

	warnings, indexWarnings, err := writeAndReindex(rt, state.filePath, updatedContent, req.FailOnIndexErr)
	if err != nil {
		return nil, err
	}
	result.WarningMessages = warnings
	result.IndexWarnings = indexWarnings
	return result, nil
}

// Move reorders or reparents one section and its complete subtree without
// changing any heading text, level, or slug.
func Move(rt *vaultruntime.Runtime, req MoveRequest) (*MoveResult, error) {
	unlock, err := lockSectionMutation(rt, req.Preview)
	if err != nil {
		return nil, err
	}
	defer unlock()

	placement, err := parsePlacement(req.Placement)
	if err != nil {
		return nil, err
	}

	state, source, err := loadResolvedSection(rt, req.Reference)
	if err != nil {
		return nil, err
	}

	sourceStart := source.LineStart - 1
	sourceEnd := sectionSubtreeEnd(source, len(state.lines))
	if sourceStart < 0 || sourceStart >= sourceEnd || sourceEnd > len(state.lines) {
		return nil, svcerr.New(codes.ErrInternal, "source section range is invalid").WithSuggestion("Run 'rvn reindex' and try again")
	}

	destinationIndex := len(state.lines)
	var anchor *model.Section
	if placement.kind != placementEOF {
		anchor, err = resolveAnchor(rt, state, placement.reference)
		if err != nil {
			return nil, err
		}
		if anchor.LineStart-1 >= sourceStart && anchor.LineStart-1 < sourceEnd {
			return nil, svcerr.New(codes.ErrInvalidInput, "cannot move a section relative to itself or its descendant").WithSuggestion("Choose an anchor outside the section's subtree")
		}
		if err := validatePlacementLevel(source.Level, anchor, placement.kind); err != nil {
			return nil, err
		}
		destinationIndex = placementIndex(state.lines, anchor, placement.kind)
	}

	movedLines := append([]trackedLine(nil), state.lines[sourceStart:sourceEnd]...)
	remaining := append([]trackedLine(nil), state.lines[:sourceStart]...)
	remaining = append(remaining, state.lines[sourceEnd:]...)
	if destinationIndex >= sourceEnd {
		destinationIndex -= sourceEnd - sourceStart
	} else if destinationIndex > sourceStart {
		return nil, svcerr.New(codes.ErrInvalidInput, "move destination is inside the source subtree").WithSuggestion("Choose an anchor outside the section's subtree")
	}
	updatedLines := insertTrackedLines(remaining, destinationIndex, movedLines)
	updatedContent := joinTrackedLines(updatedLines, state.trailingNewline)
	updatedDoc, err := parseDocumentContent(rt, state.filePath, updatedContent, "failed to parse moved section", "Choose a structurally compatible anchor")
	if err != nil {
		return nil, err
	}
	if err := validateOriginalSectionSlugs(state, updatedDoc, updatedLines, 0); err != nil {
		return nil, err
	}

	movedHeadingLine := trackedLineNumber(updatedLines, source.LineStart)
	moved := sectionAtLine(updatedDoc.Sections, movedHeadingLine)
	if moved == nil || moved.ID != source.ID || moved.Title != source.Title || moved.Level != source.Level {
		return nil, svcerr.New(codes.ErrValidationFailed, "moving the section would change its identity").WithSuggestion("Choose an anchor at a compatible heading depth")
	}
	if anchor != nil {
		if err := validateResultingPlacement(moved, anchor, placement.kind); err != nil {
			return nil, err
		}
	}

	result := &MoveResult{
		SectionID:    moved.ID,
		FileRelative: state.fileRelative,
		Placement:    string(placement.kind),
	}
	if anchor != nil {
		result.AnchorID = anchor.ID
	}
	if req.Preview {
		return result, nil
	}

	warnings, indexWarnings, err := writeAndReindex(rt, state.filePath, updatedContent, req.FailOnIndexErr)
	if err != nil {
		return nil, err
	}
	result.WarningMessages = warnings
	result.IndexWarnings = indexWarnings
	return result, nil
}

func validateCreateTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	switch {
	case title == "":
		return "", svcerr.New(codes.ErrInvalidInput, "section title is required").WithSuggestion(`Usage: rvn section create <file> "<title>" --level N`)
	case strings.ContainsAny(title, "\r\n"):
		return "", svcerr.New(codes.ErrInvalidInput, "section title must be a single line").WithSuggestion("Pass plain title text without line breaks")
	case strings.HasPrefix(title, "#"):
		return "", svcerr.New(codes.ErrInvalidInput, "section title must be plain text, not a Markdown heading").WithSuggestion(`Pass "Tasks" with --level 2 instead of "## Tasks"`)
	default:
		return title, nil
	}
}

func parsePlacement(raw Placement) (parsedPlacement, error) {
	values := []struct {
		kind  placementKind
		value string
	}{
		{kind: placementAfter, value: strings.TrimSpace(raw.After)},
		{kind: placementBefore, value: strings.TrimSpace(raw.Before)},
		{kind: placementUnder, value: strings.TrimSpace(raw.Under)},
	}
	result := parsedPlacement{kind: placementEOF}
	for _, candidate := range values {
		if candidate.value == "" {
			continue
		}
		if result.kind != placementEOF {
			return parsedPlacement{}, svcerr.New(codes.ErrInvalidInput, "--after, --before, and --under are mutually exclusive").WithSuggestion("Pass at most one structural anchor")
		}
		result = parsedPlacement{kind: candidate.kind, reference: candidate.value}
	}
	return result, nil
}

func validatePlacementLevel(level int, anchor *model.Section, kind placementKind) error {
	if anchor == nil {
		return svcerr.New(codes.ErrInternal, "anchor section is required")
	}
	requiredLevel := anchor.Level
	relation := "sibling"
	if kind == placementUnder {
		requiredLevel++
		relation = "direct child"
	}
	if requiredLevel > 6 {
		return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("section %s cannot have a child heading below level 6", anchor.ID)).WithSuggestion("Choose a shallower parent section")
	}
	if level != requiredLevel {
		return svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("heading level %d is not a legal %s level for %s; expected level %d", level, relation, anchor.ID, requiredLevel)).WithSuggestion("Choose an anchor at the same depth, or pass the required level without changing it implicitly").WithDetails(map[string]any{"anchor": anchor.ID, "actual_level": level, "required_level": requiredLevel})
	}
	return nil
}

func validateResultingPlacement(section, anchor *model.Section, kind placementKind) error {
	if section == nil || anchor == nil {
		return svcerr.New(codes.ErrInternal, "section placement validation failed")
	}
	if kind == placementUnder {
		if section.ParentScopeID() != anchor.ID {
			return svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("section would not be a direct child of %s", anchor.ID)).WithSuggestion("Choose a compatible --under anchor")
		}
		return nil
	}
	if section.ParentScopeID() != anchor.ParentScopeID() {
		return svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("section would not be a sibling of %s", anchor.ID)).WithSuggestion("Choose a compatible --before or --after anchor")
	}
	return nil
}

func placementIndex(lines []trackedLine, anchor *model.Section, kind placementKind) int {
	if kind == placementBefore {
		return anchor.LineStart - 1
	}
	return sectionSubtreeEnd(anchor, len(lines))
}

func headingInsertLines(lines []trackedLine, insertIndex int, heading string) []trackedLine {
	inserted := make([]trackedLine, 0, 2)
	if shouldInsertBlankLineBeforeHeading(lines, insertIndex) {
		inserted = append(inserted, trackedLine{text: ""})
	}
	return append(inserted, trackedLine{text: heading})
}

// shouldInsertBlankLineBeforeHeading reports whether the line immediately
// before insertIndex is non-empty. A blank previous line, or no previous
// line, is left unchanged.
func shouldInsertBlankLineBeforeHeading(lines []trackedLine, insertIndex int) bool {
	if insertIndex < 0 {
		insertIndex = 0
	}
	if insertIndex > len(lines) {
		insertIndex = len(lines)
	}
	if insertIndex == 0 {
		return false
	}
	return strings.TrimSpace(lines[insertIndex-1].text) != ""
}
