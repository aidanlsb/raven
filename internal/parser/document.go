package parser

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
)

// ParsedDocument represents a fully parsed document.
type ParsedDocument struct {
	FilePath   string          // File path relative to vault
	RawContent string          // Raw markdown content
	Body       string          // Content without frontmatter (for full-text search indexing)
	Objects    []*ParsedObject // All objects in this document
	Sections   []*ParsedSection
	Traits     []*ParsedTrait // All traits in this document
	Refs       []*ParsedRef   // All references in this document
}

// ParsedObject represents a parsed file-backed object.
type ParsedObject struct {
	ID           string                       // Unique file-backed object ID
	ObjectType   string                       // Type name
	Fields       map[string]schema.FieldValue // Fields/metadata
	Heading      *string                      // Reserved for legacy callers; file objects do not set this
	HeadingLevel *int                         // Reserved for legacy callers; file objects do not set this
	ParentID     *string                      // Reserved for legacy callers; file objects do not set this
	LineStart    int                          // Line where this object starts
	LineEnd      *int                         // Line where this object ends, when known
}

// ParsedSection represents a markdown heading-derived section.
type ParsedSection struct {
	ID              string  // Unique ID: file-id#slug
	FileObjectID    string  // Containing file-backed object ID
	Slug            string  // Fragment slug without file prefix
	Title           string  // Heading text
	Level           int     // Markdown heading level
	LineStart       int     // Line where this section starts
	LineEnd         *int    // Line where this section ends
	ParentSectionID *string // Parent section ID, nil for top-level sections
}

// ParsedTrait represents a parsed trait annotation.
type ParsedTrait struct {
	TraitType      string             // Trait type name (e.g., "due", "priority", "highlight")
	Value          *schema.FieldValue // Trait value (nil for boolean traits)
	Content        string             // The content the trait annotates
	ParentObjectID string             // Parent object ID
	Line           int                // Line number
}

// HasValue returns true if this trait has a value.
func (t *ParsedTrait) HasValue() bool {
	return traitHasValue(t.Value)
}

// ValueString returns the value as a string, or empty string if no value.
func (t *ParsedTrait) ValueString() string {
	return traitValueString(t.Value)
}

// ParsedRef represents a parsed reference.
type ParsedRef struct {
	SourceID    string  // Source object ID
	TargetRaw   string  // Raw target (as written)
	DisplayText *string // Display text
	Line        int     // Line number
	Start       int     // Start position
	End         int     // End position
}

// ParseOptions contains options for parsing documents.
type ParseOptions struct {
	// ObjectsRoot is the root directory for typed objects (e.g., "objects/").
	// If set, this prefix is stripped from file paths when computing object IDs.
	ObjectsRoot string

	// PagesRoot is the root directory for untyped pages (e.g., "pages/").
	// If set, this prefix is stripped from file paths when computing object IDs.
	PagesRoot string
}

// ParseDocument parses a markdown document.
func ParseDocument(content string, filePath string, vaultPath string) (*ParsedDocument, error) {
	return ParseDocumentWithOptions(content, filePath, vaultPath, nil)
}

// ParseDocumentWithOptions parses a markdown document with custom options.
func ParseDocumentWithOptions(content string, filePath string, vaultPath string, opts *ParseOptions) (*ParsedDocument, error) {
	relativePath := vaultRelativePath(filePath, vaultPath)
	fileID := filePathToID(relativePath, opts)

	var objects []*ParsedObject
	var sections []*ParsedSection
	var traits []*ParsedTrait
	var refs []*ParsedRef

	// Parse frontmatter
	frontmatter, err := ParseFrontmatter(content)
	if err != nil {
		return nil, err
	}

	contentStartLine, bodyContent := frontmatterBody(content, frontmatter)

	// Create file-level object
	fileFields := copyFrontmatterFields(frontmatter)
	fileType := fileObjectType(frontmatter)

	objects = append(objects, &ParsedObject{
		ID:         fileID,
		ObjectType: fileType,
		Fields:     fileFields,
		LineStart:  1,
	})

	// Extract references from frontmatter, if present.
	//
	// Historically, Raven only extracted refs from the markdown body, which meant
	// wikilinks inside YAML frontmatter were not indexed and therefore missing
	// from `rvn backlinks`. Frontmatter content starts on line 2 (the line after
	// the opening '---') and ends at frontmatter.EndLine-1.
	if frontmatter != nil && frontmatter.Raw != "" {
		refs = append(refs, frontmatterRefs(frontmatter, fileID)...)
	}

	// Use goldmark AST to extract all content from the body.
	// This automatically skips code blocks (fenced, indented, inline).
	astContent, err := ExtractFromAST([]byte(bodyContent), contentStartLine)
	if err != nil {
		return nil, err
	}

	// Use markdown headings as built-in sections. Legacy ::type(...) declarations
	// are treated as ordinary markdown text.
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

		sections = append(sections, &ParsedSection{
			ID:              sectionID,
			FileObjectID:    fileID,
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

		traits = append(traits, &ParsedTrait{
			TraitType:      astTrait.TraitName,
			Value:          astTrait.Value,
			Content:        astTrait.Content,
			ParentObjectID: parentID,
			Line:           astTrait.Line,
		})
	}

	// Process references from AST extraction
	// Code blocks are already filtered out by the AST walker.
	for _, astRef := range astContent.Refs {
		parentID := findScopeForLine(fileID, sections, astRef.Line)

		refs = append(refs, &ParsedRef{
			SourceID:    parentID,
			TargetRaw:   astRef.TargetRaw,
			DisplayText: astRef.DisplayText,
			Line:        astRef.Line,
			Start:       astRef.Start,
			End:         astRef.End,
		})
	}

	computeSectionLineEnds(sections)

	return &ParsedDocument{
		FilePath:   relativePath,
		RawContent: content,
		Body:       bodyContent,
		Objects:    objects,
		Sections:   sections,
		Traits:     traits,
		Refs:       refs,
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
	if opts != nil {
		objectsRoot = opts.ObjectsRoot
		pagesRoot = opts.PagesRoot
	}
	return paths.FilePathToObjectID(relativePath, objectsRoot, pagesRoot)
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

func copyFrontmatterFields(frontmatter *Frontmatter) map[string]schema.FieldValue {
	fileFields := make(map[string]schema.FieldValue)
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

func frontmatterRefs(frontmatter *Frontmatter, fileID string) []*ParsedRef {
	if frontmatter == nil || frontmatter.Raw == "" {
		return nil
	}
	fmRefs := ExtractRefs(frontmatter.Raw, 2)
	refs := make([]*ParsedRef, 0, len(fmRefs))
	for _, refItem := range fmRefs {
		refs = append(refs, &ParsedRef{
			SourceID:    fileID,
			TargetRaw:   refItem.TargetRaw,
			DisplayText: refItem.DisplayText,
			Line:        refItem.Line,
			Start:       refItem.Start,
			End:         refItem.End,
		})
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
func findScopeForLine(fileID string, sections []*ParsedSection, line int) string {
	idx := sort.Search(len(sections), func(i int) bool {
		return sections[i].LineStart > line
	})
	if idx > 0 {
		return sections[idx-1].ID
	}
	return fileID
}

// computeSectionLineEnds computes LineEnd for each section based on the next section's LineStart.
func computeSectionLineEnds(sections []*ParsedSection) {
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
		// Last object extends to end of file (nil)
	}
}

// (directory root stripping is handled by internal/paths)
