package parser

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/linktarget"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/paths"
)

// ParsedDocument represents a fully parsed document.
type ParsedDocument struct {
	FilePath      string             // File path relative to vault
	RawContent    string             // Raw markdown content
	Body          string             // Content without frontmatter (for full-text search indexing)
	Objects       []*model.Object    // All objects in this document
	Sections      []*model.Section   // All sections in this document
	Traits        []*model.Trait     // All traits in this document
	Refs          []*model.Reference // All references in this document
	MarkdownLinks []MarkdownLink     // All direct Markdown links/images before target classification
	Links         []*model.Link      // Markdown links to non-Raven targets
}

// ParseOptions contains options for parsing documents.
type ParseOptions struct {
	// ObjectsRoot is the root directory for typed objects (e.g., "objects/").
	// If set, this prefix is stripped from file paths when computing object IDs.
	ObjectsRoot string

	// PagesRoot is the root directory for untyped pages (e.g., "pages/").
	// If set, this prefix is stripped from file paths when computing object IDs.
	PagesRoot string

	// DailyRoot is the daily notes directory (e.g., "daily/"). Daily notes have a
	// bare ISO date (YYYY-MM-DD) as their object ID; this prefix is stripped from
	// file paths under the daily directory when computing object IDs.
	DailyRoot string
}

// ParseDocument parses a markdown document.
func ParseDocument(content string, filePath string, vaultPath string) (*ParsedDocument, error) {
	return ParseDocumentWithOptions(content, filePath, vaultPath, nil)
}

// ParseDocumentWithOptions parses a markdown document with custom options.
func ParseDocumentWithOptions(content string, filePath string, vaultPath string, opts *ParseOptions) (*ParsedDocument, error) {
	relativePath := vaultRelativePath(filePath, vaultPath)
	fileID := filePathToID(relativePath, opts)

	var objects []*model.Object
	var sections []*model.Section
	var traits []*model.Trait
	var refs []*model.Reference
	var links []*model.Link

	// Parse frontmatter
	frontmatter, err := ParseFrontmatter(content)
	if err != nil {
		return nil, err
	}

	contentStartLine, bodyContent := frontmatterBody(content, frontmatter)

	// Create file-level object
	fileFields := copyFrontmatterFields(frontmatter)
	fileType := fileObjectType(frontmatter)

	objects = append(objects, &model.Object{
		ID:        fileID,
		Type:      fileType,
		Fields:    fileFields,
		FilePath:  relativePath,
		LineStart: 1,
	})

	// Extract references from frontmatter, if present.
	//
	// Historically, Raven only extracted refs from the markdown body, which meant
	// wikilinks inside YAML frontmatter were not indexed and therefore missing
	// from `rvn backlinks`. Frontmatter content starts on line 2 (the line after
	// the opening '---') and ends at frontmatter.EndLine-1.
	if frontmatter != nil && frontmatter.Raw != "" {
		refs = append(refs, frontmatterRefs(frontmatter, fileID, relativePath)...)
	}

	// Use goldmark AST to extract all content from the body.
	// This automatically skips code blocks (fenced, indented, inline).
	astContent, err := ExtractFromAST([]byte(bodyContent), contentStartLine)
	if err != nil {
		return nil, err
	}

	// Use markdown headings as built-in sections.
	headings := astContent.Headings

	// Track used IDs to ensure uniqueness
	usedIDs := make(map[string]int)

	// Parent stack for tracking section hierarchy
	type parentEntry struct {
		id    string
		level int
	}
	parentStack := []parentEntry{{id: fileID, level: 0}}

	// Process each heading
	for _, heading := range headings {
		// Pop parents that are at same or deeper level
		for len(parentStack) > 1 && parentStack[len(parentStack)-1].level >= heading.Level {
			parentStack = parentStack[:len(parentStack)-1]
		}
		currentParent := parentStack[len(parentStack)-1].id

		slug := sectionHeadingSlug(heading.Text, usedIDs)
		sectionID := fileID + "#" + slug
		var parentSectionID *string
		if currentParent != fileID {
			parent := currentParent
			parentSectionID = &parent
		}

		sections = append(sections, &model.Section{
			ID:              sectionID,
			FileObjectID:    fileID,
			FilePath:        relativePath,
			Slug:            slug,
			Title:           heading.Text,
			Level:           heading.Level,
			LineStart:       heading.Line,
			ParentSectionID: parentSectionID,
		})

		parentStack = append(parentStack, parentEntry{id: sectionID, level: heading.Level})
	}
	// Process traits from AST extraction - assign to the correct parent based on line number
	// Code blocks are already filtered out by the AST walker.
	for _, astTrait := range astContent.Traits {
		parentID := findScopeForLine(fileID, sections, astTrait.Line)

		traits = append(traits, &model.Trait{
			TraitType:     astTrait.TraitName,
			Value:         astTrait.Value,
			Content:       astTrait.Content,
			FilePath:      relativePath,
			ParentScopeID: parentID,
			Line:          astTrait.Line,
			PositionStart: astTrait.StartOffset,
			PositionEnd:   astTrait.EndOffset,
		})
	}

	// Process references from AST extraction
	// Code blocks are already filtered out by the AST walker.
	for _, astRef := range astContent.Refs {
		parentID := findScopeForLine(fileID, sections, astRef.Line)

		ref := model.NewInlineReference(
			parentID,
			astRef.TargetRaw,
			astRef.DisplayText,
			astRef.Line,
			astRef.Start,
			astRef.End,
		)
		ref.FilePath = relativePath
		refs = append(refs, ref)
	}

	// Process direct Markdown links and images as lightweight edges. Markdown
	// object/section targets stay out of this index.
	for _, astLink := range astContent.Links {
		if linktarget.IsRavenTarget(astLink.Target, relativePath, vaultPath) {
			continue
		}
		targetInfo := linktarget.AnalyzeAuthored(astLink.RawTarget, astLink.Target, relativePath, vaultPath)
		links = append(links, &model.Link{
			SourceID:      fileID,
			SourceType:    fileType,
			FilePath:      relativePath,
			Line:          astLink.Line,
			PositionStart: astLink.PositionStart,
			PositionEnd:   astLink.PositionEnd,
			RawTarget:     astLink.RawTarget,
			Display:       astLink.Display,
			IsImage:       astLink.IsImage,
			Scheme:        string(targetInfo.Scheme),
			Ext:           targetInfo.Ext,
			NormalizedKey: targetInfo.NormalizedKey,
		})
	}

	computeSectionLineEnds(sections)

	return &ParsedDocument{
		FilePath:      relativePath,
		RawContent:    content,
		Body:          bodyContent,
		Objects:       objects,
		Sections:      sections,
		Traits:        traits,
		Refs:          refs,
		MarkdownLinks: astContent.Links,
		Links:         links,
	}, nil
}

func vaultRelativePath(filePath, vaultPath string) string {
	relativePath := filePath
	if vaultPath != "" {
		if rel, err := filepath.Rel(vaultPath, filePath); err == nil {
			relativePath = rel
		}
	}
	return filepath.ToSlash(relativePath)
}

func filePathToID(relativePath string, opts *ParseOptions) string {
	// File ID is derived from the vault-relative file path.
	// This is the canonical path->ID mapping, including directory roots.
	objectsRoot := ""
	pagesRoot := ""
	dailyRoot := ""
	if opts != nil {
		objectsRoot = opts.ObjectsRoot
		pagesRoot = opts.PagesRoot
		dailyRoot = opts.DailyRoot
	}
	return paths.FilePathToObjectID(relativePath, objectsRoot, pagesRoot, dailyRoot)
}

func frontmatterBody(content string, frontmatter *Frontmatter) (contentStartLine int, bodyContent string) {
	contentStartLine = 1
	bodyContent = content
	if frontmatter == nil {
		return contentStartLine, bodyContent
	}

	contentStartLine = frontmatter.EndLine + 1
	lines := strings.Split(content, "\n")
	if frontmatter.EndLine < len(lines) {
		bodyContent = strings.Join(lines[frontmatter.EndLine:], "\n")
	} else {
		bodyContent = ""
	}
	return contentStartLine, bodyContent
}

func copyFrontmatterFields(frontmatter *Frontmatter) map[string]fieldvalue.FieldValue {
	fileFields := make(map[string]fieldvalue.FieldValue)
	if frontmatter == nil {
		return fileFields
	}
	for k, v := range frontmatter.Fields {
		fileFields[k] = v
	}
	return fileFields
}

func fileObjectType(frontmatter *Frontmatter) string {
	fileType := "page"
	if frontmatter != nil && frontmatter.ObjectType != "" {
		fileType = frontmatter.ObjectType
	}
	return fileType
}

func frontmatterRefs(frontmatter *Frontmatter, fileID, filePath string) []*model.Reference {
	if frontmatter == nil || frontmatter.Raw == "" {
		return nil
	}
	fmRefs := ExtractRefs(frontmatter.Raw, 2)
	refs := make([]*model.Reference, 0, len(fmRefs))
	for _, refItem := range fmRefs {
		ref := model.NewInlineReference(
			fileID,
			refItem.TargetRaw,
			refItem.DisplayText,
			refItem.Line,
			refItem.Start,
			refItem.End,
		)
		ref.FilePath = filePath
		refs = append(refs, ref)
	}
	return refs
}

func sectionHeadingSlug(headingText string, usedIDs map[string]int) string {
	baseSlug := Slugify(headingText)
	if baseSlug == "" {
		baseSlug = "section"
	}
	return uniqueSlug(baseSlug, usedIDs)
}

func uniqueSlug(baseSlug string, usedIDs map[string]int) string {
	next := usedIDs[baseSlug] + 1
	for {
		slug := baseSlug
		if next > 1 {
			slug = baseSlug + "-" + strconv.Itoa(next)
		}
		if _, exists := usedIDs[slug]; !exists {
			usedIDs[baseSlug] = next
			if slug != baseSlug {
				usedIDs[slug] = 1
			}
			return slug
		}
		next++
	}
}

// findScopeForLine finds the nearest containing scope ID for a line.
func findScopeForLine(fileID string, sections []*model.Section, line int) string {
	idx := sort.Search(len(sections), func(i int) bool {
		return sections[i].LineStart > line
	})
	if idx > 0 {
		return sections[idx-1].ID
	}
	return fileID
}

// computeSectionLineEnds computes direct and subtree line ranges for each section.
func computeSectionLineEnds(sections []*model.Section) {
	if len(sections) == 0 {
		return
	}

	// Sort by line_start
	indices := make([]int, len(sections))
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(i, j int) bool {
		return sections[indices[i]].LineStart < sections[indices[j]].LineStart
	})

	for i := 0; i < len(indices); i++ {
		currentIdx := indices[i]
		if i+1 < len(indices) {
			nextLineEnd := sections[indices[i+1]].LineStart - 1
			sections[currentIdx].LineEnd = &nextLineEnd
		}

		current := sections[currentIdx]
		for j := i + 1; j < len(indices); j++ {
			next := sections[indices[j]]
			if next.Level <= current.Level {
				subtreeLineEnd := next.LineStart - 1
				current.SubtreeLineEnd = &subtreeLineEnd
				break
			}
		}
		// Nil end values extend to end of file.
	}
}

// (directory root stripping is handled by internal/paths)
