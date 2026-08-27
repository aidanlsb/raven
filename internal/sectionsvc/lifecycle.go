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
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/schema"
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
	VaultPath      string
	VaultConfig    *config.VaultConfig
	Schema         *schema.Schema
	FileReference  string
	Title          string
	Level          int
	Placement      Placement
	Preview        bool
	ParseOptions   *parser.ParseOptions
	FailOnIndexErr bool
	Runtime        *vaultruntime.Runtime
}

type CreateResult struct {
	SectionID       string
	FileRelative    string
	Level           int
	Placement       string
	AnchorID        string
	WarningMessages []string
	IndexWarnings   []IndexWarning
}

type MoveRequest struct {
	VaultPath      string
	VaultConfig    *config.VaultConfig
	Schema         *schema.Schema
	Reference      string
	Placement      Placement
	Preview        bool
	ParseOptions   *parser.ParseOptions
	FailOnIndexErr bool
	Runtime        *vaultruntime.Runtime
}

type MoveResult struct {
	SectionID       string
	FileRelative    string
	Placement       string
	AnchorID        string
	WarningMessages []string
	IndexWarnings   []IndexWarning
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

type lifecycleContext struct {
	vaultPath    string
	vaultConfig  *config.VaultConfig
	schema       *schema.Schema
	parseOptions *parser.ParseOptions
	runtime      *vaultruntime.Runtime
}

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

// Create inserts a new, empty heading at a structural section boundary.
func Create(req CreateRequest) (*CreateResult, error) {
	rt, owned := vaultruntime.FromRequest(req.Runtime, req.VaultPath, req.VaultConfig, req.Schema, req.ParseOptions)
	if owned {
		defer rt.Close()
	}
	projectionLock, err := reindexsvc.LockProjection(rt, req.Preview)
	if err != nil {
		return nil, err
	}
	if projectionLock != nil {
		defer func() { _ = projectionLock.Close() }()
	}
	ctx, err := newLifecycleContext(rt, req.VaultPath, req.VaultConfig, req.Schema, req.ParseOptions)
	if err != nil {
		return nil, err
	}
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

	resolvedFile, err := ctx.resolveReference(strings.TrimSpace(req.FileReference))
	if err != nil {
		return nil, err
	}
	if resolvedFile.IsSection {
		return nil, svcerr.New(codes.ErrInvalidInput, "section create target must be a file, not a section").WithSuggestion("Pass the containing file before the title")
	}

	state, err := ctx.loadDocument(resolvedFile.FilePath, resolvedFile.FileObjectID)
	if err != nil {
		return nil, err
	}

	insertIndex := len(state.lines)
	var anchor *model.Section
	if placement.kind != placementEOF {
		anchor, err = ctx.resolveAnchor(state, placement.reference)
		if err != nil {
			return nil, err
		}
		if err := validatePlacementLevel(req.Level, anchor, placement.kind); err != nil {
			return nil, err
		}
		insertIndex = placementIndex(state.lines, anchor, placement.kind)
	}

	heading := strings.Repeat("#", req.Level) + " " + title
	updatedLines := insertTrackedLines(state.lines, insertIndex, []trackedLine{{text: heading}})
	updatedContent := joinTrackedLines(updatedLines, state.trailingNewline)
	updatedDoc, err := parser.ParseDocumentWithOptions(updatedContent, state.filePath, ctx.vaultPath, ctx.parseOptions)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrValidationFailed, "failed to parse created section", err).WithSuggestion("Check the heading title and level")
	}

	createdLine := insertIndex + 1
	created := sectionAtLine(updatedDoc.Sections, createdLine)
	if created == nil || created.Title != title || created.Level != req.Level {
		return nil, svcerr.New(codes.ErrValidationFailed, "new heading does not parse as the requested section").WithSuggestion("Use plain, single-line title text")
	}
	expectedSlug := parser.Slugify(title)
	if expectedSlug == "" {
		expectedSlug = "section"
	}
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

	warnings, indexWarnings, err := ctx.writeAndReindex(state.filePath, updatedContent, req.FailOnIndexErr)
	if err != nil {
		return nil, err
	}
	result.WarningMessages = warnings
	result.IndexWarnings = indexWarnings
	return result, nil
}

// Move reorders or reparents one section and its complete subtree without
// changing any heading text, level, or slug.
func Move(req MoveRequest) (*MoveResult, error) {
	rt, owned := vaultruntime.FromRequest(req.Runtime, req.VaultPath, req.VaultConfig, req.Schema, req.ParseOptions)
	if owned {
		defer rt.Close()
	}
	projectionLock, err := reindexsvc.LockProjection(rt, req.Preview)
	if err != nil {
		return nil, err
	}
	if projectionLock != nil {
		defer func() { _ = projectionLock.Close() }()
	}
	ctx, err := newLifecycleContext(rt, req.VaultPath, req.VaultConfig, req.Schema, req.ParseOptions)
	if err != nil {
		return nil, err
	}
	placement, err := parsePlacement(req.Placement)
	if err != nil {
		return nil, err
	}

	resolvedSource, err := ctx.resolveSection(strings.TrimSpace(req.Reference))
	if err != nil {
		return nil, err
	}
	state, err := ctx.loadDocument(resolvedSource.FilePath, resolvedSource.FileObjectID)
	if err != nil {
		return nil, err
	}
	source := state.sectionsByID[resolvedSource.ObjectID]
	if source == nil {
		return nil, svcerr.New(codes.ErrRefNotFound, fmt.Sprintf("section not found: %s", resolvedSource.ObjectID)).WithSuggestion("Run 'rvn reindex' if the index is stale")
	}

	sourceStart := source.LineStart - 1
	sourceEnd := sectionSubtreeEnd(source, len(state.lines))
	if sourceStart < 0 || sourceStart >= sourceEnd || sourceEnd > len(state.lines) {
		return nil, svcerr.New(codes.ErrInternal, "source section range is invalid").WithSuggestion("Run 'rvn reindex' and try again")
	}

	destinationIndex := len(state.lines)
	var anchor *model.Section
	if placement.kind != placementEOF {
		anchor, err = ctx.resolveAnchor(state, placement.reference)
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
	updatedDoc, err := parser.ParseDocumentWithOptions(updatedContent, state.filePath, ctx.vaultPath, ctx.parseOptions)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrValidationFailed, "failed to parse moved section", err).WithSuggestion("Choose a structurally compatible anchor")
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

	warnings, indexWarnings, err := ctx.writeAndReindex(state.filePath, updatedContent, req.FailOnIndexErr)
	if err != nil {
		return nil, err
	}
	result.WarningMessages = warnings
	result.IndexWarnings = indexWarnings
	return result, nil
}

func newLifecycleContext(rt *vaultruntime.Runtime, vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, parseOptions *parser.ParseOptions) (*lifecycleContext, error) {
	if err := vaultruntime.RequirePath(vaultPath); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if vaultCfg == nil {
		return nil, svcerr.New(codes.ErrValidationFailed, "vault config is required").WithSuggestion("Fix raven.yaml and try again")
	}
	return &lifecycleContext{
		vaultPath:    vaultPath,
		vaultConfig:  vaultCfg,
		schema:       sch,
		parseOptions: parseOptions,
		runtime:      rt,
	}, nil
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

func (ctx *lifecycleContext) resolveReference(reference string) (*refresolve.ResolveResult, error) {
	if reference == "" {
		return nil, svcerr.New(codes.ErrInvalidInput, "file reference is required").WithSuggestion("Pass an existing Markdown file")
	}
	resolved, err := refresolve.Resolve(reference, ctx.runtime, false)
	if err == nil {
		return resolved, nil
	}
	var ambiguousErr *refresolve.AmbiguousRefError
	if errors.As(err, &ambiguousErr) {
		return nil, svcerr.Wrap(codes.ErrRefAmbiguous, ambiguousErr.Error(), err).WithSuggestion("Use a full object or section ID to disambiguate").WithDetails(map[string]any{"matches": ambiguousErr.Matches})
	}
	var notFoundErr *refresolve.RefNotFoundError
	if errors.As(err, &notFoundErr) {
		return nil, svcerr.Wrap(codes.ErrRefNotFound, notFoundErr.Error(), err).WithSuggestion("Check the reference and run 'rvn reindex' if needed")
	}
	return nil, svcerr.Wrap(codes.ErrInternal, fmt.Sprintf("failed to resolve reference: %v", err), err).WithSuggestion("Run 'rvn reindex' and try again")
}

func (ctx *lifecycleContext) resolveSection(reference string) (*refresolve.ResolveResult, error) {
	resolved, err := ctx.resolveReference(reference)
	if err != nil {
		return nil, err
	}
	if !resolved.IsSection {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("section reference required: %s", reference)).WithSuggestion("Use a section ID like project/website#tasks")
	}
	return resolved, nil
}

func (ctx *lifecycleContext) loadDocument(filePath, fileID string) (*documentState, error) {
	if err := mutationguard.ValidateContentMutationFilePath(ctx.vaultPath, ctx.vaultConfig, filePath); err != nil {
		return nil, normalizeMutationError(err)
	}
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileRead, "failed to read section file", err)
	}
	content := string(contentBytes)
	doc, err := parser.ParseDocumentWithOptions(content, filePath, ctx.vaultPath, ctx.parseOptions)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrValidationFailed, "failed to parse section file", err).WithSuggestion("Fix the file content and try again")
	}
	relative, err := filepath.Rel(ctx.vaultPath, filePath)
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

func (ctx *lifecycleContext) resolveAnchor(state *documentState, reference string) (*model.Section, error) {
	resolved, err := ctx.resolveSection(reference)
	if err != nil {
		return nil, err
	}
	if resolved.FileObjectID != state.fileID {
		return nil, svcerr.New(codes.ErrInvalidInput, fmt.Sprintf("anchor %s is not in %s", resolved.ObjectID, state.fileID)).WithSuggestion("Choose a section in the same file")
	}
	anchor := state.sectionsByID[resolved.ObjectID]
	if anchor == nil {
		return nil, svcerr.New(codes.ErrRefNotFound, fmt.Sprintf("anchor section not found: %s", resolved.ObjectID)).WithSuggestion("Run 'rvn reindex' if the index is stale")
	}
	return anchor, nil
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

func validateOriginalSectionSlugs(state *documentState, updatedDoc *parser.ParsedDocument, updatedLines []trackedLine, createdLine int) error {
	originalByLine := make(map[int]*model.Section, len(state.doc.Sections))
	for _, section := range state.doc.Sections {
		if section != nil {
			originalByLine[section.LineStart] = section
		}
	}
	seen := make(map[int]bool, len(originalByLine))
	for _, section := range updatedDoc.Sections {
		if section == nil || section.LineStart == createdLine {
			continue
		}
		if section.LineStart < 1 || section.LineStart > len(updatedLines) {
			return svcerr.New(codes.ErrInternal, "updated section line is out of range")
		}
		originalLine := updatedLines[section.LineStart-1].originalLine
		original := originalByLine[originalLine]
		if original == nil {
			continue
		}
		seen[originalLine] = true
		if section.Slug != original.Slug {
			return svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("section placement would shift slug '%s' to '%s'", original.Slug, section.Slug)).WithSuggestion("Choose a unique heading title or a placement that preserves section identities").WithDetails(map[string]any{"section": original.ID, "new_slug": section.Slug})
		}
	}
	if len(seen) != len(originalByLine) {
		return svcerr.New(codes.ErrValidationFailed, "section placement would change the existing outline").WithSuggestion("Choose a structurally compatible placement")
	}
	return nil
}

func (ctx *lifecycleContext) writeAndReindex(filePath, content string, failOnIndexErr bool) ([]string, []IndexWarning, error) {
	var warnings []string
	if err := ctx.runtime.OpenDB(); err != nil {
		if failOnIndexErr || errors.Is(err, index.ErrIndexRebuildRequired) {
			return nil, nil, svcerr.Wrap(codes.ErrValidationFailed, "failed to open index database for section mutation", err).WithSuggestion("Run 'rvn reindex' to rebuild the database")
		}
		warnings = append(warnings, fmt.Sprintf("Failed to open index database for section mutation: %v", err))
	}
	db := ctx.runtime.DB

	perm := os.FileMode(0o644)
	if st, statErr := os.Stat(filePath); statErr == nil {
		perm = st.Mode()
	}
	if err := atomicfile.WriteFile(filePath, []byte(content), perm); err != nil {
		return nil, nil, svcerr.Wrap(codes.ErrFileWrite, "failed to write section mutation", err)
	}
	var indexWarnings []IndexWarning
	if db != nil && ctx.schema != nil {
		req := RenameRequest{
			VaultPath:    ctx.vaultPath,
			VaultConfig:  ctx.vaultConfig,
			Schema:       ctx.schema,
			ParseOptions: ctx.parseOptions,
		}
		if warning := reindexRenamedFile(db, req, filePath); warning != nil {
			indexWarnings = append(indexWarnings, *warning)
		}
	}
	return warnings, indexWarnings, nil
}
